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
	activitystore "github.com/dalemusser/stratahub/internal/app/store/activity"
	sessionsstore "github.com/dalemusser/stratahub/internal/app/store/sessions"
	"github.com/dalemusser/stratahub/internal/app/system/authz"
	"github.com/dalemusser/stratahub/internal/app/system/timeouts"
	"github.com/dalemusser/stratahub/internal/app/system/timezones"
	"github.com/dalemusser/stratahub/internal/app/system/viewdata"
	"github.com/dalemusser/waffle/pantry/templates"
	"github.com/go-chi/chi/v5"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// ServeMemberDetail renders the detailed activity view for a specific user.
// GET /activity/member/{memberID}
func (h *Handler) ServeMemberDetail(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), timeouts.Medium())
	defer cancel()

	memberIDStr := chi.URLParam(r, "memberID")
	memberID, err := primitive.ObjectIDFromHex(memberIDStr)
	if err != nil {
		uierrors.RenderBadRequest(w, r, "Invalid user ID.", "/activity")
		return
	}

	db := h.DB

	// Check authorization - can the current user view this user's activity?
	canAccess, err := h.canViewUserActivity(ctx, r, memberID)
	if err != nil {
		h.ErrLog.LogServerError(w, r, "failed to check user access", err, "A database error occurred.", "/activity")
		return
	}
	if !canAccess {
		uierrors.RenderForbidden(w, r, "You don't have permission to view this user's activity.", "/activity")
		return
	}

	// Get timezone parameter (will be set client-side via JavaScript)
	tzParam := strings.TrimSpace(r.URL.Query().Get("tz"))

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

	// Get organization name
	var orgName string
	if member.OrgID != nil {
		var org struct {
			Name string `bson:"name"`
		}
		if err := db.Collection("organizations").FindOne(ctx, bson.M{"_id": member.OrgID}).Decode(&org); err == nil {
			orgName = org.Name
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

	// Build session blocks with events (timestamps will be formatted client-side)
	sessionBlocks := h.buildSessionBlocks(sessions, events)

	// Get timezone groups for selector
	tzGroups, _ := timezones.Groups()

	data := memberDetailData{
		BaseVM:           viewdata.NewBaseVM(r, h.DB, "Activity History", "/activity"),
		MemberID:         memberIDStr,
		MemberName:       member.Name,
		LoginID:          member.LoginID,
		Email:            member.Email,
		GroupNames:       groupNames,
		OrgName:          orgName,
		Timezone:         tzParam,
		TimezoneGroups:   tzGroups,
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
		http.Error(w, "Invalid user ID", http.StatusBadRequest)
		return
	}

	// Check authorization
	canAccess, err := h.canViewUserActivity(ctx, r, memberID)
	if err != nil || !canAccess {
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
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

	// Build session blocks (timestamps will be formatted client-side)
	sessionBlocks := h.buildSessionBlocks(sessions, events)

	data := memberDetailData{
		MemberID:         memberIDStr,
		TotalSessions:    totalSessions,
		TotalTimeStr:     formatDuration(int64(totalMins) * 60),
		AvgSessionMins:   avgSessionMins,
		ResourceLaunches: resourceLaunches,
		Sessions:         sessionBlocks,
	}

	templates.RenderSnippet(w, "activity_member_detail_content", data)
}

// getMemberGroupNames returns a comma-separated list of group names for a user.
// Returns groups where the user is a member or leader.
func (h *Handler) getMemberGroupNames(ctx context.Context, memberID primitive.ObjectID) (string, error) {
	pipeline := []bson.M{
		{"$match": bson.M{"user_id": memberID}},
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
	LastActivity time.Time          `bson:"last_activity"`
	CreatedBy    string             `bson:"created_by"`
	EndReason    string             `bson:"end_reason"`
	DurationSecs int64              `bson:"duration_secs"`
}

// getMemberSessions gets sessions for a member since the given time.
// It also closes any stale sessions (open but inactive for more than 10 minutes).
func (h *Handler) getMemberSessions(ctx context.Context, memberID primitive.ObjectID, since time.Time) ([]sessionRecord, error) {
	// First, close any stale sessions for this user
	// A session is stale if it has no logout_at and last_activity is > 10 minutes ago
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
		"user_id":       userID,
		"logout_at":     nil,
		"last_activity": bson.M{"$lt": threshold},
	})
	if err != nil {
		return
	}
	defer cur.Close(ctx)

	for cur.Next(ctx) {
		var s struct {
			ID           primitive.ObjectID `bson:"_id"`
			LoginAt      time.Time          `bson:"login_at"`
			LastActivity time.Time          `bson:"last_activity"`
		}
		if err := cur.Decode(&s); err != nil {
			continue
		}

		// Close the session - use last_activity as the logout time
		duration := int64(s.LastActivity.Sub(s.LoginAt).Seconds())
		_, _ = h.DB.Collection("sessions").UpdateOne(ctx,
			bson.M{"_id": s.ID},
			bson.M{"$set": bson.M{
				"logout_at":     s.LastActivity,
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
// Timestamps are provided in ISO format for client-side timezone formatting.
func (h *Handler) buildSessionBlocks(sessions []sessionRecord, events []activitystore.Event) []sessionBlock {
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
		// Format times in UTC as fallback (client-side JS will format in selected timezone)
		date := s.LoginAt.UTC().Format("Jan 2, 2006")
		loginTime := s.LoginAt.UTC().Format("3:04 PM")
		loginTimeISO := s.LoginAt.UTC().Format(time.RFC3339)

		logoutTime := ""
		logoutTimeISO := ""
		if s.LogoutAt != nil {
			logoutTime = s.LogoutAt.UTC().Format("3:04 PM")
			logoutTimeISO = s.LogoutAt.UTC().Format(time.RFC3339)
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
			TimeLabel:   s.LoginAt.UTC().Format("3:04 PM"),
			TimeISO:     s.LoginAt.UTC().Format(time.RFC3339),
			EventType:   loginEventType,
			Description: loginDesc,
		})

		for _, e := range eventsBySession[s.ID] {
			// Skip events that occurred before this session started (orphan events from other sessions)
			if e.Timestamp.Before(s.LoginAt) {
				continue
			}

			ae := activityEvent{
				Time:      e.Timestamp,
				TimeLabel: e.Timestamp.UTC().Format("3:04 PM"),
				TimeISO:   e.Timestamp.UTC().Format(time.RFC3339),
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
				TimeLabel:   s.LogoutAt.UTC().Format("3:04 PM"),
				TimeISO:     s.LogoutAt.UTC().Format(time.RFC3339),
				EventType:   "logout",
				Description: logoutDesc,
			})
		}

		// Sort events by time (oldest first), then reverse so newest are first
		slices.SortFunc(activityEvents, func(a, b activityEvent) int {
			return a.Time.Compare(b.Time)
		})
		slices.Reverse(activityEvents)

		// For active sessions, add "Last activity" at the very top (after reversal)
		if s.LogoutAt == nil {
			// Use the session's last_activity time (updated by every heartbeat)
			lastActivityTime := s.LastActivity
			if lastActivityTime.IsZero() {
				lastActivityTime = s.LoginAt
			}
			// Prepend status event showing last activity
			idleEvent := activityEvent{
				Time:        lastActivityTime,
				TimeLabel:   lastActivityTime.UTC().Format("3:04 PM"),
				TimeISO:     lastActivityTime.UTC().Format(time.RFC3339),
				EventType:   "idle",
				Description: "Last activity",
			}
			activityEvents = append([]activityEvent{idleEvent}, activityEvents...)
		}

		blocks = append(blocks, sessionBlock{
			Date:          date,
			LoginTime:     loginTime,
			LoginTimeISO:  loginTimeISO,
			LogoutTime:    logoutTime,
			LogoutTimeISO: logoutTimeISO,
			Duration:      duration,
			EndReason:     endReason,
			Events:        activityEvents,
		})
	}

	return blocks
}

// canViewUserActivity checks if the current user can view activity for the target user.
// Authorization rules:
//   - Admin/SuperAdmin: can view activity for any user
//   - Coordinator: can view activity for members and leaders in their assigned organizations
//   - Leader: can view activity for members in groups they lead
func (h *Handler) canViewUserActivity(ctx context.Context, r *http.Request, targetUserID primitive.ObjectID) (bool, error) {
	viewerRole, _, viewerID, ok := authz.UserCtx(r)
	if !ok {
		return false, nil
	}

	// Users can always view their own activity
	if viewerID == targetUserID {
		return true, nil
	}

	// Admin/SuperAdmin can view anyone
	if viewerRole == "superadmin" || viewerRole == "admin" {
		return true, nil
	}

	// Get the target user's info
	var targetUser struct {
		Role  string              `bson:"role"`
		OrgID *primitive.ObjectID `bson:"organization_id"`
	}
	err := h.DB.Collection("users").FindOne(ctx, bson.M{"_id": targetUserID}).Decode(&targetUser)
	if err != nil {
		return false, err
	}

	switch viewerRole {
	case "coordinator":
		// Coordinators can view members and leaders in their assigned orgs
		if targetUser.Role != "member" && targetUser.Role != "leader" {
			return false, nil
		}
		if targetUser.OrgID == nil {
			return false, nil
		}
		return authz.CanAccessOrg(r, *targetUser.OrgID), nil

	case "leader":
		// Leaders can only view members in groups they lead
		if targetUser.Role != "member" {
			return false, nil
		}
		// Check if target user is in any group the viewer leads
		leaderGroups, err := h.getLeaderGroupIDs(ctx, viewerID)
		if err != nil {
			return false, err
		}
		if len(leaderGroups) == 0 {
			return false, nil
		}
		// Check if target user is a member in any of those groups
		count, err := h.DB.Collection("group_memberships").CountDocuments(ctx, bson.M{
			"user_id":  targetUserID,
			"group_id": bson.M{"$in": leaderGroups},
			"role":     "member",
		})
		if err != nil {
			return false, err
		}
		return count > 0, nil

	default:
		return false, nil
	}
}

// getLeaderGroupIDs returns the group IDs where the user is a leader.
func (h *Handler) getLeaderGroupIDs(ctx context.Context, userID primitive.ObjectID) ([]primitive.ObjectID, error) {
	cur, err := h.DB.Collection("group_memberships").Find(ctx, bson.M{
		"user_id": userID,
		"role":    "leader",
	})
	if err != nil {
		return nil, err
	}
	defer cur.Close(ctx)

	var groupIDs []primitive.ObjectID
	for cur.Next(ctx) {
		var doc struct {
			GroupID primitive.ObjectID `bson:"group_id"`
		}
		if err := cur.Decode(&doc); err != nil {
			return nil, err
		}
		groupIDs = append(groupIDs, doc.GroupID)
	}
	return groupIDs, nil
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
