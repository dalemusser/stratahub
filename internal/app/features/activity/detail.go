// internal/app/features/activity/detail.go
package activity

// Terminology: User Identifiers
//   - UserID / userID / user_id: The MongoDB ObjectID (_id) that uniquely identifies a user record
//   - LoginID / loginID / login_id: The human-readable string users type to log in

import (
	"context"
	"fmt"
	"net/http"
	"slices"
	"strings"
	"time"

	uierrors "github.com/dalemusser/stratahub/internal/app/features/errors"
	"github.com/dalemusser/stratahub/internal/app/policy/memberpolicy"
	activitystore "github.com/dalemusser/stratahub/internal/app/store/activity"
	sessionsstore "github.com/dalemusser/stratahub/internal/app/store/sessions"
	"github.com/dalemusser/stratahub/internal/app/system/timeouts"
	"github.com/dalemusser/stratahub/internal/app/system/viewdata"
	"github.com/dalemusser/waffle/pantry/templates"
	"github.com/go-chi/chi/v5"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// ServeMemberDetail renders the detailed activity view for a specific member.
// GET /activity/member/{memberID}
func (h *Handler) ServeMemberDetail(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), timeouts.Medium())
	defer cancel()

	memberIDStr := chi.URLParam(r, "memberID")
	memberID, err := primitive.ObjectIDFromHex(memberIDStr)
	if err != nil {
		uierrors.RenderBadRequest(w, r, "Invalid member ID.", "/activity")
		return
	}

	db := h.DB

	// Check authorization - can the current user view this member?
	memberInfo, canAccess, err := memberpolicy.CheckMemberAccess(ctx, db, r, memberID)
	if err != nil {
		h.ErrLog.LogServerError(w, r, "failed to check member access", err, "A database error occurred.", "/activity")
		return
	}
	if memberInfo == nil {
		uierrors.RenderNotFound(w, r, "Member not found.", "/activity")
		return
	}
	if !canAccess {
		uierrors.RenderForbidden(w, r, "You don't have permission to view this member's activity.", "/activity")
		return
	}

	// Get member details including organization
	var member struct {
		ID      primitive.ObjectID  `bson:"_id"`
		Name    string              `bson:"full_name"`
		LoginID string              `bson:"login_id"`
		Email   string              `bson:"email"`
		OrgID   *primitive.ObjectID `bson:"organization_id"`
	}
	if err := db.Collection("users").FindOne(ctx, bson.M{"_id": memberID}).Decode(&member); err != nil {
		h.ErrLog.LogServerError(w, r, "failed to fetch member", err, "A database error occurred.", "/activity")
		return
	}

	// Get organization timezone
	var orgName, tzName string
	var loc *time.Location = time.UTC // Default to UTC
	if member.OrgID != nil {
		var org struct {
			Name     string `bson:"name"`
			TimeZone string `bson:"time_zone"`
		}
		if err := db.Collection("organizations").FindOne(ctx, bson.M{"_id": member.OrgID}).Decode(&org); err == nil {
			orgName = org.Name
			tzName = org.TimeZone
			if org.TimeZone != "" {
				if parsedLoc, err := time.LoadLocation(org.TimeZone); err == nil {
					loc = parsedLoc
				}
			}
		}
	}

	// Get member's groups
	groupNames, err := h.getMemberGroupNames(ctx, memberID)
	if err != nil {
		h.ErrLog.LogServerError(w, r, "failed to fetch groups", err, "A database error occurred.", "/activity")
		return
	}

	// Get session history (last 30 days)
	thirtyDaysAgo := time.Now().UTC().AddDate(0, 0, -30)
	sessions, err := h.getMemberSessions(ctx, memberID, thirtyDaysAgo)
	if err != nil {
		h.ErrLog.LogServerError(w, r, "failed to fetch sessions", err, "A database error occurred.", "/activity")
		return
	}

	// Get activity events for these sessions
	var sessionIDs []primitive.ObjectID
	for _, s := range sessions {
		sessionIDs = append(sessionIDs, s.ID)
	}

	events, err := h.getEventsForSessions(ctx, sessionIDs, memberID, thirtyDaysAgo)
	if err != nil {
		h.ErrLog.LogServerError(w, r, "failed to fetch events", err, "A database error occurred.", "/activity")
		return
	}

	// Calculate stats
	totalSessions := len(sessions)
	var totalMins, resourceLaunches int
	for _, s := range sessions {
		if s.DurationSecs > 0 {
			totalMins += int(s.DurationSecs / 60)
		} else if s.LogoutAt == nil {
			// Active session - calculate duration from login to now
			totalMins += int(time.Since(s.LoginAt).Minutes())
		}
	}
	for _, e := range events {
		if e.EventType == activitystore.EventResourceLaunch {
			resourceLaunches++
		}
	}

	avgSessionMins := 0
	if totalSessions > 0 {
		avgSessionMins = totalMins / totalSessions
	}

	// Build session blocks with events (converted to organization timezone)
	sessionBlocks := h.buildSessionBlocks(sessions, events, loc)

	// Get timezone abbreviation for display
	// Always show timezone - default to UTC if org has no timezone set
	if tzName == "" {
		tzName = "UTC"
	}
	tzLabel := time.Now().In(loc).Format("MST")

	data := memberDetailData{
		BaseVM:           viewdata.NewBaseVM(r, h.DB, "Member Activity", "/activity"),
		MemberID:         memberIDStr,
		MemberName:       member.Name,
		LoginID:          member.LoginID,
		Email:            member.Email,
		GroupNames:       groupNames,
		OrgName:          orgName,
		TimezoneName:     tzName,
		TimezoneLabel:    tzLabel,
		TotalSessions:    totalSessions,
		TotalTimeStr:     formatDuration(int64(totalMins) * 60), // Convert mins to secs for formatDuration
		AvgSessionMins:   avgSessionMins,
		ResourceLaunches: resourceLaunches,
		Sessions:         sessionBlocks,
	}

	templates.Render(w, r, "activity_member_detail", data)
}

// ServeMemberDetailContent renders just the refreshable content portion (HTMX partial).
// GET /activity/member/{memberID}/content
func (h *Handler) ServeMemberDetailContent(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), timeouts.Medium())
	defer cancel()

	memberIDStr := chi.URLParam(r, "memberID")
	memberID, err := primitive.ObjectIDFromHex(memberIDStr)
	if err != nil {
		http.Error(w, "Invalid member ID", http.StatusBadRequest)
		return
	}

	db := h.DB

	// Check authorization
	_, canAccess, err := memberpolicy.CheckMemberAccess(ctx, db, r, memberID)
	if err != nil || !canAccess {
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}

	// Get organization timezone
	var member struct {
		OrgID *primitive.ObjectID `bson:"organization_id"`
	}
	if err := db.Collection("users").FindOne(ctx, bson.M{"_id": memberID}).Decode(&member); err != nil {
		http.Error(w, "Member not found", http.StatusNotFound)
		return
	}

	var tzName string
	var loc *time.Location = time.UTC
	if member.OrgID != nil {
		var org struct {
			TimeZone string `bson:"time_zone"`
		}
		if err := db.Collection("organizations").FindOne(ctx, bson.M{"_id": member.OrgID}).Decode(&org); err == nil {
			tzName = org.TimeZone
			if org.TimeZone != "" {
				if parsedLoc, err := time.LoadLocation(org.TimeZone); err == nil {
					loc = parsedLoc
				}
			}
		}
	}

	// Get session history (last 30 days)
	thirtyDaysAgo := time.Now().UTC().AddDate(0, 0, -30)
	sessions, err := h.getMemberSessions(ctx, memberID, thirtyDaysAgo)
	if err != nil {
		http.Error(w, "Database error", http.StatusInternalServerError)
		return
	}

	// Get activity events for these sessions
	var sessionIDs []primitive.ObjectID
	for _, s := range sessions {
		sessionIDs = append(sessionIDs, s.ID)
	}

	events, err := h.getEventsForSessions(ctx, sessionIDs, memberID, thirtyDaysAgo)
	if err != nil {
		http.Error(w, "Database error", http.StatusInternalServerError)
		return
	}

	// Calculate stats
	totalSessions := len(sessions)
	var totalMins, resourceLaunches int
	for _, s := range sessions {
		if s.DurationSecs > 0 {
			totalMins += int(s.DurationSecs / 60)
		} else if s.LogoutAt == nil {
			// Active session - calculate duration from login to now
			totalMins += int(time.Since(s.LoginAt).Minutes())
		}
	}
	for _, e := range events {
		if e.EventType == activitystore.EventResourceLaunch {
			resourceLaunches++
		}
	}

	avgSessionMins := 0
	if totalSessions > 0 {
		avgSessionMins = totalMins / totalSessions
	}

	// Build session blocks
	sessionBlocks := h.buildSessionBlocks(sessions, events, loc)

	// Get timezone label
	if tzName == "" {
		tzName = "UTC"
	}
	tzLabel := time.Now().In(loc).Format("MST")

	data := memberDetailData{
		MemberID:         memberIDStr,
		TimezoneName:     tzName,
		TimezoneLabel:    tzLabel,
		TotalSessions:    totalSessions,
		TotalTimeStr:     formatDuration(int64(totalMins) * 60),
		AvgSessionMins:   avgSessionMins,
		ResourceLaunches: resourceLaunches,
		Sessions:         sessionBlocks,
	}

	templates.RenderSnippet(w, "activity_member_detail_content", data)
}

// getMemberGroupNames returns a comma-separated list of group names for a member.
func (h *Handler) getMemberGroupNames(ctx context.Context, memberID primitive.ObjectID) (string, error) {
	pipeline := []bson.M{
		{"$match": bson.M{"user_id": memberID, "role": "member"}},
		{"$lookup": bson.M{
			"from":         "groups",
			"localField":   "group_id",
			"foreignField": "_id",
			"as":           "group",
		}},
		{"$unwind": "$group"},
		{"$project": bson.M{"name": "$group.name"}},
	}

	cur, err := h.DB.Collection("group_memberships").Aggregate(ctx, pipeline)
	if err != nil {
		return "", err
	}
	defer cur.Close(ctx)

	var names []string
	for cur.Next(ctx) {
		var doc struct {
			Name string `bson:"name"`
		}
		if err := cur.Decode(&doc); err != nil {
			return "", err
		}
		names = append(names, doc.Name)
	}

	return strings.Join(names, ", "), nil
}

// sessionRecord is a minimal session for the detail view.
type sessionRecord struct {
	ID           primitive.ObjectID `bson:"_id"`
	LoginAt      time.Time          `bson:"login_at"`
	LogoutAt     *time.Time         `bson:"logout_at"`
	LastActiveAt time.Time          `bson:"last_active_at"`
	CreatedBy    string             `bson:"created_by"`
	EndReason    string             `bson:"end_reason"`
	DurationSecs int64              `bson:"duration_secs"`
}

// getMemberSessions gets sessions for a member since the given time.
// It also closes any stale sessions (open but inactive for more than 10 minutes).
func (h *Handler) getMemberSessions(ctx context.Context, memberID primitive.ObjectID, since time.Time) ([]sessionRecord, error) {
	// First, close any stale sessions for this user
	// A session is stale if it has no logout_at and last_active_at is > 10 minutes ago
	staleThreshold := time.Now().UTC().Add(-10 * time.Minute)
	h.closeStaleSessionsForUser(ctx, memberID, staleThreshold)

	opts := options.Find().
		SetSort(bson.D{{Key: "login_at", Value: -1}}).
		SetLimit(100) // Limit to last 100 sessions

	cur, err := h.DB.Collection("sessions").Find(ctx, bson.M{
		"user_id":  memberID,
		"login_at": bson.M{"$gte": since},
	}, opts)
	if err != nil {
		return nil, err
	}
	defer cur.Close(ctx)

	var sessions []sessionRecord
	for cur.Next(ctx) {
		var s sessionRecord
		if err := cur.Decode(&s); err != nil {
			return nil, err
		}
		sessions = append(sessions, s)
	}

	return sessions, nil
}

// closeStaleSessionsForUser closes sessions that are open but inactive.
func (h *Handler) closeStaleSessionsForUser(ctx context.Context, userID primitive.ObjectID, threshold time.Time) {
	cur, err := h.DB.Collection("sessions").Find(ctx, bson.M{
		"user_id":        userID,
		"logout_at":      nil,
		"last_active_at": bson.M{"$lt": threshold},
	})
	if err != nil {
		return
	}
	defer cur.Close(ctx)

	for cur.Next(ctx) {
		var s struct {
			ID           primitive.ObjectID `bson:"_id"`
			LoginAt      time.Time          `bson:"login_at"`
			LastActiveAt time.Time          `bson:"last_active_at"`
		}
		if err := cur.Decode(&s); err != nil {
			continue
		}

		// Close the session - use last_active_at as the logout time
		duration := int64(s.LastActiveAt.Sub(s.LoginAt).Seconds())
		_, _ = h.DB.Collection("sessions").UpdateOne(ctx,
			bson.M{"_id": s.ID},
			bson.M{"$set": bson.M{
				"logout_at":     s.LastActiveAt,
				"end_reason":    "inactive",
				"duration_secs": duration,
			}},
		)
	}
}

// getEventsForSessions gets activity events for the given sessions.
// Queries by both session_id and user_id to handle cases where session linking may be incomplete.
func (h *Handler) getEventsForSessions(ctx context.Context, sessionIDs []primitive.ObjectID, memberID primitive.ObjectID, since time.Time) ([]activitystore.Event, error) {
	if h.Activity == nil {
		return nil, nil
	}

	opts := options.Find().SetSort(bson.D{{Key: "timestamp", Value: 1}})

	// Query by user_id and time range (more reliable than session_id matching)
	// This ensures we capture all events even if session_id linking is inconsistent
	filter := bson.M{
		"user_id":   memberID,
		"timestamp": bson.M{"$gte": since},
	}

	cur, err := h.DB.Collection("activity_events").Find(ctx, filter, opts)
	if err != nil {
		return nil, err
	}
	defer cur.Close(ctx)

	var events []activitystore.Event
	if err := cur.All(ctx, &events); err != nil {
		return nil, err
	}

	return events, nil
}

// buildSessionBlocks organizes sessions and events into display blocks.
// Events are matched to sessions by timestamp range (login_at to logout_at) or by session_id.
func (h *Handler) buildSessionBlocks(sessions []sessionRecord, events []activitystore.Event, loc *time.Location) []sessionBlock {
	// First, try to group events by session_id (direct match)
	eventsBySession := make(map[primitive.ObjectID][]activitystore.Event)
	unmatchedEvents := make([]activitystore.Event, 0)

	// Create a set of valid session IDs for quick lookup
	sessionIDSet := make(map[primitive.ObjectID]bool)
	for _, s := range sessions {
		sessionIDSet[s.ID] = true
	}

	for _, e := range events {
		if sessionIDSet[e.SessionID] {
			// Direct session_id match
			eventsBySession[e.SessionID] = append(eventsBySession[e.SessionID], e)
		} else {
			// No direct match - try to match by timestamp
			unmatchedEvents = append(unmatchedEvents, e)
		}
	}

	// For unmatched events, try to associate them with sessions by timestamp range
	for _, e := range unmatchedEvents {
		matched := false
		for _, s := range sessions {
			// Check if event timestamp falls within session's time range
			sessionEnd := time.Now().UTC()
			if s.LogoutAt != nil {
				sessionEnd = *s.LogoutAt
			}
			if (e.Timestamp.Equal(s.LoginAt) || e.Timestamp.After(s.LoginAt)) &&
				(e.Timestamp.Equal(sessionEnd) || e.Timestamp.Before(sessionEnd)) {
				eventsBySession[s.ID] = append(eventsBySession[s.ID], e)
				matched = true
				break
			}
		}
		// If still no match and there's an active session, add to the first active session
		if !matched {
			for _, s := range sessions {
				if s.LogoutAt == nil { // Active session
					eventsBySession[s.ID] = append(eventsBySession[s.ID], e)
					break
				}
			}
		}
	}

	var blocks []sessionBlock
	for _, s := range sessions {
		// Convert times to organization timezone
		loginLocal := s.LoginAt.In(loc)
		date := loginLocal.Format("Jan 2, 2006")
		loginTime := loginLocal.Format("3:04 PM")

		logoutTime := ""
		if s.LogoutAt != nil {
			logoutTime = s.LogoutAt.In(loc).Format("3:04 PM")
		}

		duration := formatDuration(s.DurationSecs)
		if s.DurationSecs == 0 && s.LogoutAt == nil {
			// Active session
			duration = formatDuration(int64(time.Since(s.LoginAt).Seconds()))
			logoutTime = "(active)"
		}

		endReason := s.EndReason
		if endReason == "" && s.LogoutAt == nil {
			endReason = "active"
		}

		// Build events for this session
		var activityEvents []activityEvent

		// Add login/resumed event at the start
		loginDesc := "Logged in"
		loginEventType := "login"
		if s.CreatedBy == sessionsstore.CreatedByHeartbeat {
			loginDesc = "Session resumed"
			loginEventType = "resumed"
		}
		activityEvents = append(activityEvents, activityEvent{
			Time:        s.LoginAt,
			TimeLabel:   loginLocal.Format("3:04 PM"),
			EventType:   loginEventType,
			Description: loginDesc,
		})

		for _, e := range eventsBySession[s.ID] {
			ae := activityEvent{
				Time:      e.Timestamp,
				TimeLabel: e.Timestamp.In(loc).Format("3:04 PM"),
				EventType: e.EventType,
			}

			switch e.EventType {
			case activitystore.EventResourceLaunch:
				ae.Description = fmt.Sprintf("Launched \"%s\"", e.ResourceName)
			case activitystore.EventResourceView:
				ae.Description = fmt.Sprintf("Viewed \"%s\"", e.ResourceName)
			case activitystore.EventPageView:
				ae.Description = fmt.Sprintf("Viewed %s", e.PagePath)
			default:
				ae.Description = e.EventType
			}

			activityEvents = append(activityEvents, ae)
		}

		// Add logout event if session ended
		if s.LogoutAt != nil {
			logoutDesc := "Logged out"
			if s.EndReason == "inactive" {
				logoutDesc = "Session timed out"
			}
			activityEvents = append(activityEvents, activityEvent{
				Time:        *s.LogoutAt,
				TimeLabel:   s.LogoutAt.In(loc).Format("3:04 PM"),
				EventType:   "logout",
				Description: logoutDesc,
			})
		}

		// For active sessions, add idle status if no recent activity events
		if s.LogoutAt == nil {
			// Find the latest activity event (excluding login/resumed)
			var lastEventTime time.Time
			for _, e := range eventsBySession[s.ID] {
				if e.Timestamp.After(lastEventTime) {
					lastEventTime = e.Timestamp
				}
			}
			// If no events, use login time as the idle start
			if lastEventTime.IsZero() {
				lastEventTime = s.LoginAt
			}
			// Add idle event showing when they became idle
			activityEvents = append(activityEvents, activityEvent{
				Time:        lastEventTime,
				TimeLabel:   lastEventTime.In(loc).Format("3:04 PM"),
				EventType:   "idle",
				Description: fmt.Sprintf("Idle since %s", lastEventTime.In(loc).Format("3:04 PM")),
			})
		}

		// Reverse events so newest are first (consistent with session ordering)
		slices.Reverse(activityEvents)

		blocks = append(blocks, sessionBlock{
			Date:       date,
			LoginTime:  loginTime,
			LogoutTime: logoutTime,
			Duration:   duration,
			EndReason:  endReason,
			Events:     activityEvents,
		})
	}

	return blocks
}

// formatDuration formats seconds as a human-readable duration.
func formatDuration(secs int64) string {
	if secs < 60 {
		return fmt.Sprintf("%d sec", secs)
	}
	mins := secs / 60
	if mins < 60 {
		return fmt.Sprintf("%d min", mins)
	}
	hours := mins / 60
	remainingMins := mins % 60
	if remainingMins == 0 {
		return fmt.Sprintf("%dh", hours)
	}
	return fmt.Sprintf("%dh %dm", hours, remainingMins)
}
