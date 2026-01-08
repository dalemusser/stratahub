// internal/app/features/activity/summary.go
package activity

import (
	"context"
	"fmt"
	"net/http"
	"sort"
	"time"

	"github.com/dalemusser/stratahub/internal/app/system/authz"
	"github.com/dalemusser/stratahub/internal/app/system/timeouts"
	"github.com/dalemusser/stratahub/internal/app/system/viewdata"
	"github.com/dalemusser/waffle/pantry/query"
	"github.com/dalemusser/waffle/pantry/templates"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

// ServeSummary renders the weekly summary view.
// GET /activity/summary
func (h *Handler) ServeSummary(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), timeouts.Long())
	defer cancel()

	selectedGroup := query.Get(r, "group")
	weekParam := query.Get(r, "week") // Format: 2025-01-13

	// Calculate week range
	now := time.Now().UTC()
	weekStart := getWeekStart(now)
	if weekParam != "" {
		if parsed, err := time.Parse("2006-01-02", weekParam); err == nil {
			weekStart = getWeekStart(parsed)
		}
	}
	weekEnd := weekStart.AddDate(0, 0, 7)

	// Get groups based on user role (reuse from dashboard)
	role, _, userID, ok := authz.UserCtx(r)
	if !ok {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}

	db := h.DB

	// Get groups based on role
	var groupIDs []primitive.ObjectID
	var groupMap = make(map[primitive.ObjectID]string)

	switch role {
	case "superadmin", "admin":
		groups, err := h.fetchAllGroups(ctx, db)
		if err != nil {
			h.ErrLog.LogServerError(w, r, "failed to fetch groups", err, "A database error occurred.", "/activity")
			return
		}
		for _, g := range groups {
			groupIDs = append(groupIDs, g.ID)
			groupMap[g.ID] = g.Name
		}

	case "coordinator":
		orgIDs := authz.UserOrgIDs(r)
		groups, err := h.fetchGroupsByOrgs(ctx, db, orgIDs)
		if err != nil {
			h.ErrLog.LogServerError(w, r, "failed to fetch groups", err, "A database error occurred.", "/activity")
			return
		}
		for _, g := range groups {
			groupIDs = append(groupIDs, g.ID)
			groupMap[g.ID] = g.Name
		}

	case "leader":
		groups, err := h.fetchLeaderGroups(ctx, db, userID)
		if err != nil {
			h.ErrLog.LogServerError(w, r, "failed to fetch groups", err, "A database error occurred.", "/activity")
			return
		}
		for _, g := range groups {
			groupIDs = append(groupIDs, g.ID)
			groupMap[g.ID] = g.Name
		}
	}

	// Build group options for dropdown
	var groups []groupOption
	groups = append(groups, groupOption{ID: "", Name: "All Groups", Selected: selectedGroup == ""})
	for id, name := range groupMap {
		groups = append(groups, groupOption{
			ID:       id.Hex(),
			Name:     name,
			Selected: selectedGroup == id.Hex(),
		})
	}
	sort.Slice(groups[1:], func(i, j int) bool {
		return groups[i+1].Name < groups[j+1].Name
	})

	// Filter to selected group if specified
	var filterGroupIDs []primitive.ObjectID
	if selectedGroup != "" {
		if oid, err := primitive.ObjectIDFromHex(selectedGroup); err == nil {
			if _, ok := groupMap[oid]; ok {
				filterGroupIDs = []primitive.ObjectID{oid}
			}
		}
	}
	if len(filterGroupIDs) == 0 {
		filterGroupIDs = groupIDs
	}

	// Get summary data for members
	members, err := h.fetchWeeklySummary(ctx, filterGroupIDs, groupMap, weekStart, weekEnd)
	if err != nil {
		h.ErrLog.LogServerError(w, r, "failed to fetch summary", err, "A database error occurred.", "/activity")
		return
	}

	data := summaryData{
		BaseVM:        viewdata.NewBaseVM(r, h.DB, "Weekly Summary", "/activity/summary"),
		SelectedGroup: selectedGroup,
		Groups:        groups,
		WeekStart:     weekStart.Format("Jan 2"),
		WeekEnd:       weekEnd.AddDate(0, 0, -1).Format("Jan 2, 2006"),
		Members:       members,
	}

	templates.Render(w, r, "activity_summary", data)
}

// getWeekStart returns the Monday of the week containing the given time.
func getWeekStart(t time.Time) time.Time {
	t = time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC)
	weekday := int(t.Weekday())
	if weekday == 0 {
		weekday = 7 // Sunday = 7
	}
	return t.AddDate(0, 0, -(weekday - 1))
}

// fetchWeeklySummary gets session and activity summaries for members.
func (h *Handler) fetchWeeklySummary(ctx context.Context, groupIDs []primitive.ObjectID, groupMap map[primitive.ObjectID]string, weekStart, weekEnd time.Time) ([]summaryRow, error) {
	if len(groupIDs) == 0 {
		return nil, nil
	}

	db := h.DB

	// Get members in these groups
	pipeline := []bson.M{
		{"$match": bson.M{"group_id": bson.M{"$in": groupIDs}, "role": "member"}},
		{"$lookup": bson.M{
			"from":         "users",
			"localField":   "user_id",
			"foreignField": "_id",
			"as":           "user",
		}},
		{"$unwind": "$user"},
		{"$match": bson.M{"user.status": "active"}},
		{"$project": bson.M{
			"user_id":  "$user._id",
			"name":     "$user.full_name",
			"email":    "$user.email",
			"group_id": 1,
		}},
	}

	cur, err := db.Collection("group_memberships").Aggregate(ctx, pipeline)
	if err != nil {
		return nil, err
	}
	defer cur.Close(ctx)

	// Collect unique members
	type memberInfo struct {
		UserID   primitive.ObjectID
		Name     string
		Email    string
		GroupIDs []primitive.ObjectID
	}
	memberMap := make(map[primitive.ObjectID]*memberInfo)

	for cur.Next(ctx) {
		var doc struct {
			UserID  primitive.ObjectID `bson:"user_id"`
			Name    string             `bson:"name"`
			Email   string             `bson:"email"`
			GroupID primitive.ObjectID `bson:"group_id"`
		}
		if err := cur.Decode(&doc); err != nil {
			return nil, err
		}

		if m, ok := memberMap[doc.UserID]; ok {
			m.GroupIDs = append(m.GroupIDs, doc.GroupID)
		} else {
			memberMap[doc.UserID] = &memberInfo{
				UserID:   doc.UserID,
				Name:     doc.Name,
				Email:    doc.Email,
				GroupIDs: []primitive.ObjectID{doc.GroupID},
			}
		}
	}

	if len(memberMap) == 0 {
		return nil, nil
	}

	var userIDs []primitive.ObjectID
	for uid := range memberMap {
		userIDs = append(userIDs, uid)
	}

	// Get session stats for the week
	sessionStats, err := h.getWeeklySessionStats(ctx, userIDs, weekStart, weekEnd)
	if err != nil {
		return nil, err
	}

	// Get resource time for the week
	resourceTime, err := h.getWeeklyResourceTime(ctx, userIDs, weekStart, weekEnd)
	if err != nil {
		return nil, err
	}

	// Build summary rows
	var members []summaryRow
	for _, m := range memberMap {
		groupName := ""
		if len(m.GroupIDs) > 0 {
			groupName = groupMap[m.GroupIDs[0]]
			if len(m.GroupIDs) > 1 {
				groupName += " (+)"
			}
		}

		stats := sessionStats[m.UserID]
		resTime := resourceTime[m.UserID]

		members = append(members, summaryRow{
			ID:              m.UserID.Hex(),
			Name:            m.Name,
			Email:           m.Email,
			GroupName:       groupName,
			SessionCount:    stats.SessionCount,
			TotalTimeStr:    formatMins(stats.TotalMins),
			ResourceTimeStr: formatMins(resTime),
			OutsideClass:    stats.OutsideClass,
		})
	}

	// Sort by name
	sort.Slice(members, func(i, j int) bool {
		return members[i].Name < members[j].Name
	})

	return members, nil
}

type weeklyStats struct {
	SessionCount int
	TotalMins    int
	OutsideClass int
}

// getWeeklySessionStats calculates session statistics for users in a week.
func (h *Handler) getWeeklySessionStats(ctx context.Context, userIDs []primitive.ObjectID, weekStart, weekEnd time.Time) (map[primitive.ObjectID]weeklyStats, error) {
	if len(userIDs) == 0 {
		return nil, nil
	}

	// Define "class hours" (8 AM - 4 PM weekdays) for outside class detection
	// This is a simplified approach

	pipeline := []bson.M{
		{"$match": bson.M{
			"user_id": bson.M{"$in": userIDs},
			"login_at": bson.M{
				"$gte": weekStart,
				"$lt":  weekEnd,
			},
		}},
		{"$project": bson.M{
			"user_id": 1,
			"duration_mins": bson.M{
				"$cond": bson.M{
					"if": bson.M{"$ne": bson.A{"$duration_secs", nil}},
					"then": bson.M{"$divide": bson.A{"$duration_secs", 60}},
					"else": bson.M{"$divide": bson.A{
						bson.M{"$subtract": bson.A{time.Now().UTC(), "$login_at"}},
						60000,
					}},
				},
			},
			"hour": bson.M{"$hour": "$login_at"},
			"dow":  bson.M{"$dayOfWeek": "$login_at"}, // 1=Sun, 7=Sat
		}},
		{"$group": bson.M{
			"_id":           "$user_id",
			"session_count": bson.M{"$sum": 1},
			"total_mins":    bson.M{"$sum": "$duration_mins"},
			"outside_class": bson.M{"$sum": bson.M{
				"$cond": bson.M{
					"if": bson.M{"$or": bson.A{
						bson.M{"$in": bson.A{"$dow", bson.A{1, 7}}}, // Weekend
						bson.M{"$lt": bson.A{"$hour", 8}},           // Before 8 AM
						bson.M{"$gte": bson.A{"$hour", 16}},         // After 4 PM
					}},
					"then": 1,
					"else": 0,
				},
			}},
		}},
	}

	cur, err := h.DB.Collection("sessions").Aggregate(ctx, pipeline)
	if err != nil {
		return nil, err
	}
	defer cur.Close(ctx)

	result := make(map[primitive.ObjectID]weeklyStats)
	for cur.Next(ctx) {
		var doc struct {
			ID           primitive.ObjectID `bson:"_id"`
			SessionCount int                `bson:"session_count"`
			TotalMins    float64            `bson:"total_mins"`
			OutsideClass int                `bson:"outside_class"`
		}
		if err := cur.Decode(&doc); err != nil {
			return nil, err
		}
		result[doc.ID] = weeklyStats{
			SessionCount: doc.SessionCount,
			TotalMins:    int(doc.TotalMins),
			OutsideClass: doc.OutsideClass,
		}
	}

	return result, nil
}

// getWeeklyResourceTime calculates time spent in resources for users in a week.
func (h *Handler) getWeeklyResourceTime(ctx context.Context, userIDs []primitive.ObjectID, weekStart, weekEnd time.Time) (map[primitive.ObjectID]int, error) {
	if len(userIDs) == 0 || h.Activity == nil {
		return nil, nil
	}

	// Sum time_away_secs from resource_return events
	pipeline := []bson.M{
		{"$match": bson.M{
			"user_id":    bson.M{"$in": userIDs},
			"event_type": "resource_return",
			"timestamp": bson.M{
				"$gte": weekStart,
				"$lt":  weekEnd,
			},
		}},
		{"$group": bson.M{
			"_id":        "$user_id",
			"total_secs": bson.M{"$sum": "$details.time_away_secs"},
		}},
	}

	cur, err := h.DB.Collection("activity_events").Aggregate(ctx, pipeline)
	if err != nil {
		return nil, err
	}
	defer cur.Close(ctx)

	result := make(map[primitive.ObjectID]int)
	for cur.Next(ctx) {
		var doc struct {
			ID        primitive.ObjectID `bson:"_id"`
			TotalSecs int64              `bson:"total_secs"`
		}
		if err := cur.Decode(&doc); err != nil {
			return nil, err
		}
		result[doc.ID] = int(doc.TotalSecs / 60)
	}

	return result, nil
}

// formatMins formats minutes as "Xh Ym" or "X min".
func formatMins(mins int) string {
	if mins >= 60 {
		return fmt.Sprintf("%dh %dm", mins/60, mins%60)
	}
	return fmt.Sprintf("%d min", mins)
}
