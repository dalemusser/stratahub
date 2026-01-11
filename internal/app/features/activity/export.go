// internal/app/features/activity/export.go
package activity

// Terminology: User Identifiers
//   - UserID / userID / user_id: The MongoDB ObjectID (_id) that uniquely identifies a user record
//   - LoginID / loginID / login_id: The human-readable string users type to log in

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"time"

	uierrors "github.com/dalemusser/stratahub/internal/app/features/errors"
	"github.com/dalemusser/stratahub/internal/app/system/authz"
	"github.com/dalemusser/stratahub/internal/app/system/timeouts"
	"github.com/dalemusser/stratahub/internal/app/system/viewdata"
	"github.com/dalemusser/stratahub/internal/app/system/workspace"
	"github.com/dalemusser/waffle/pantry/templates"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo/options"
	"go.uber.org/zap"
)

// ServeExport renders the export UI with filters and aggregated stats.
func (h *Handler) ServeExport(w http.ResponseWriter, r *http.Request) {
	role, userName, _, ok := authz.UserCtx(r)
	if !ok {
		uierrors.RenderUnauthorized(w, r, "/login")
		return
	}

	ctx, cancel := timeouts.WithTimeout(r.Context(), timeouts.Long(), h.Log, "activity export page")
	defer cancel()

	// Parse date range from query params (default to last 30 days)
	endDate := time.Now()
	startDate := endDate.AddDate(0, 0, -30)

	if s := r.URL.Query().Get("start"); s != "" {
		if t, err := time.Parse("2006-01-02", s); err == nil {
			startDate = t
		}
	}
	if e := r.URL.Query().Get("end"); e != "" {
		if t, err := time.Parse("2006-01-02", e); err == nil {
			endDate = t.Add(24*time.Hour - time.Second) // end of day
		}
	}

	selectedOrg := r.URL.Query().Get("org")
	selectedGroup := r.URL.Query().Get("group")

	// Fetch organizations and groups based on role
	orgs, groups := h.fetchOrgsAndGroups(ctx, r, role, selectedOrg, selectedGroup)

	// Build scope filter for queries
	scopeFilter := h.buildScopeFilter(r, role, selectedOrg, selectedGroup)

	// Compute aggregated stats
	stats := h.computeAggregateStats(ctx, scopeFilter, startDate, endDate)

	_ = userName // used for logging

	totalDurationMins := int(stats.TotalDurationSecs / 60)

	data := exportData{
		BaseVM:        viewdata.NewBaseVM(r, h.DB, "Data Export", "/activity/export"),
		SelectedOrg:   selectedOrg,
		SelectedGroup: selectedGroup,
		Orgs:          orgs,
		Groups:        groups,
		StartDate:     startDate.Format("2006-01-02"),
		EndDate:       endDate.Format("2006-01-02"),

		TotalSessions:    stats.TotalSessions,
		TotalUsers:       stats.TotalUsers,
		TotalDurationStr: formatMinutes(totalDurationMins),
		AvgSessionMins:   h.safeDiv(int(stats.TotalDurationSecs/60), stats.TotalSessions),
		PeakHour:         h.findPeakHour(stats.SessionsByHour),
		MostActiveDay:    h.findMostActiveDay(stats.SessionsByDay),
	}

	templates.Render(w, r, "activity_export", data)
}

// ServeSessionsCSV exports sessions as CSV.
func (h *Handler) ServeSessionsCSV(w http.ResponseWriter, r *http.Request) {
	role, userName, _, ok := authz.UserCtx(r)
	if !ok {
		uierrors.RenderUnauthorized(w, r, "/login")
		return
	}

	ctx, cancel := timeouts.WithTimeout(r.Context(), timeouts.Long(), h.Log, "sessions CSV export")
	defer cancel()

	startDate, endDate := h.parseDateRange(r)
	selectedOrg := r.URL.Query().Get("org")
	selectedGroup := r.URL.Query().Get("group")
	scopeFilter := h.buildScopeFilter(r, role, selectedOrg, selectedGroup)

	rows, err := h.fetchSessionExportRows(ctx, scopeFilter, startDate, endDate)
	if err != nil {
		h.ErrLog.LogServerError(w, r, "fetch sessions for export failed", err, "A database error occurred.", "/activity/export")
		return
	}

	filename := fmt.Sprintf("sessions_%s_%s.csv", startDate.Format("20060102"), endDate.Format("20060102"))
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, url.PathEscape(filename)))

	// UTF-8 BOM for Excel
	if _, err := w.Write([]byte{0xEF, 0xBB, 0xBF}); err != nil {
		h.Log.Error("CSV write failed (BOM)", zap.Error(err))
		return
	}

	cw := csv.NewWriter(w)
	cw.UseCRLF = true
	defer cw.Flush()

	// Header
	if err := cw.Write([]string{"user_id", "user_name", "email", "organization", "group", "login_at", "logout_at", "end_reason", "duration_secs", "ip"}); err != nil {
		h.Log.Error("CSV write failed (header)", zap.Error(err))
		return
	}

	// Rows
	for _, row := range rows {
		if err := cw.Write([]string{
			row.UserID,
			sanitizeCSVField(row.UserName),
			row.Email,
			sanitizeCSVField(row.Organization),
			sanitizeCSVField(row.Group),
			row.LoginAt.Format(time.RFC3339),
			row.LogoutAt,
			row.EndReason,
			fmt.Sprintf("%d", row.DurationSecs),
			row.IP,
		}); err != nil {
			h.Log.Error("CSV write failed (row)", zap.Error(err))
			return
		}
	}

	h.Log.Info("sessions CSV exported", zap.String("user", userName), zap.Int("rows", len(rows)))
}

// ServeSessionsJSON exports sessions as JSON.
func (h *Handler) ServeSessionsJSON(w http.ResponseWriter, r *http.Request) {
	role, userName, _, ok := authz.UserCtx(r)
	if !ok {
		uierrors.RenderUnauthorized(w, r, "/login")
		return
	}

	ctx, cancel := timeouts.WithTimeout(r.Context(), timeouts.Long(), h.Log, "sessions JSON export")
	defer cancel()

	startDate, endDate := h.parseDateRange(r)
	selectedOrg := r.URL.Query().Get("org")
	selectedGroup := r.URL.Query().Get("group")
	scopeFilter := h.buildScopeFilter(r, role, selectedOrg, selectedGroup)

	rows, err := h.fetchSessionExportRows(ctx, scopeFilter, startDate, endDate)
	if err != nil {
		h.ErrLog.LogServerError(w, r, "fetch sessions for export failed", err, "A database error occurred.", "/activity/export")
		return
	}

	filename := fmt.Sprintf("sessions_%s_%s.json", startDate.Format("20060102"), endDate.Format("20060102"))
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, url.PathEscape(filename)))

	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	if err := enc.Encode(rows); err != nil {
		h.Log.Error("JSON encode failed", zap.Error(err))
	}

	h.Log.Info("sessions JSON exported", zap.String("user", userName), zap.Int("rows", len(rows)))
}

// ServeEventsCSV exports activity events as CSV.
func (h *Handler) ServeEventsCSV(w http.ResponseWriter, r *http.Request) {
	role, userName, _, ok := authz.UserCtx(r)
	if !ok {
		uierrors.RenderUnauthorized(w, r, "/login")
		return
	}

	ctx, cancel := timeouts.WithTimeout(r.Context(), timeouts.Long(), h.Log, "events CSV export")
	defer cancel()

	startDate, endDate := h.parseDateRange(r)
	selectedOrg := r.URL.Query().Get("org")
	selectedGroup := r.URL.Query().Get("group")
	scopeFilter := h.buildScopeFilter(r, role, selectedOrg, selectedGroup)

	rows, err := h.fetchEventExportRows(ctx, scopeFilter, startDate, endDate)
	if err != nil {
		h.ErrLog.LogServerError(w, r, "fetch events for export failed", err, "A database error occurred.", "/activity/export")
		return
	}

	filename := fmt.Sprintf("activity_events_%s_%s.csv", startDate.Format("20060102"), endDate.Format("20060102"))
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, url.PathEscape(filename)))

	// UTF-8 BOM for Excel
	if _, err := w.Write([]byte{0xEF, 0xBB, 0xBF}); err != nil {
		h.Log.Error("CSV write failed (BOM)", zap.Error(err))
		return
	}

	cw := csv.NewWriter(w)
	cw.UseCRLF = true
	defer cw.Flush()

	// Header
	if err := cw.Write([]string{"user_id", "user_name", "session_id", "timestamp", "event_type", "resource_name", "details"}); err != nil {
		h.Log.Error("CSV write failed (header)", zap.Error(err))
		return
	}

	// Rows
	for _, row := range rows {
		detailsJSON := ""
		if len(row.Details) > 0 {
			if b, err := json.Marshal(row.Details); err == nil {
				detailsJSON = string(b)
			}
		}
		if err := cw.Write([]string{
			row.UserID,
			sanitizeCSVField(row.UserName),
			row.SessionID,
			row.Timestamp.Format(time.RFC3339),
			row.EventType,
			sanitizeCSVField(row.ResourceName),
			detailsJSON,
		}); err != nil {
			h.Log.Error("CSV write failed (row)", zap.Error(err))
			return
		}
	}

	h.Log.Info("events CSV exported", zap.String("user", userName), zap.Int("rows", len(rows)))
}

// ServeEventsJSON exports activity events as JSON.
func (h *Handler) ServeEventsJSON(w http.ResponseWriter, r *http.Request) {
	role, userName, _, ok := authz.UserCtx(r)
	if !ok {
		uierrors.RenderUnauthorized(w, r, "/login")
		return
	}

	ctx, cancel := timeouts.WithTimeout(r.Context(), timeouts.Long(), h.Log, "events JSON export")
	defer cancel()

	startDate, endDate := h.parseDateRange(r)
	selectedOrg := r.URL.Query().Get("org")
	selectedGroup := r.URL.Query().Get("group")
	scopeFilter := h.buildScopeFilter(r, role, selectedOrg, selectedGroup)

	rows, err := h.fetchEventExportRows(ctx, scopeFilter, startDate, endDate)
	if err != nil {
		h.ErrLog.LogServerError(w, r, "fetch events for export failed", err, "A database error occurred.", "/activity/export")
		return
	}

	filename := fmt.Sprintf("activity_events_%s_%s.json", startDate.Format("20060102"), endDate.Format("20060102"))
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, url.PathEscape(filename)))

	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	if err := enc.Encode(rows); err != nil {
		h.Log.Error("JSON encode failed", zap.Error(err))
	}

	h.Log.Info("events JSON exported", zap.String("user", userName), zap.Int("rows", len(rows)))
}

// fetchOrgsAndGroups returns organizations and groups based on user role.
func (h *Handler) fetchOrgsAndGroups(ctx context.Context, r *http.Request, role, selectedOrg, selectedGroup string) ([]orgOption, []groupOption) {
	var orgs []orgOption
	var groups []groupOption

	// Fetch organizations with workspace scoping
	orgFilter := bson.M{}
	workspace.Filter(r, orgFilter)
	if role == "coordinator" {
		// Coordinator sees their assigned orgs
		orgIDs := authz.UserOrgIDs(r)
		if len(orgIDs) > 0 {
			orgFilter["_id"] = bson.M{"$in": orgIDs}
		}
	}
	// Admin sees all orgs

	orgCur, err := h.DB.Collection("organizations").Find(ctx, orgFilter, options.Find().
		SetSort(bson.D{{Key: "name", Value: 1}}).
		SetProjection(bson.M{"name": 1}))
	if err != nil {
		h.Log.Warn("fetch orgs failed", zap.Error(err))
	} else {
		defer orgCur.Close(ctx)
		for orgCur.Next(ctx) {
			var o struct {
				ID   primitive.ObjectID `bson:"_id"`
				Name string             `bson:"name"`
			}
			if err := orgCur.Decode(&o); err != nil {
				continue
			}
			orgs = append(orgs, orgOption{
				ID:       o.ID.Hex(),
				Name:     o.Name,
				Selected: o.ID.Hex() == selectedOrg,
			})
		}
	}

	// Fetch groups with workspace scoping
	groupFilter := bson.M{}
	workspace.Filter(r, groupFilter)
	if selectedOrg != "" {
		if oid, err := primitive.ObjectIDFromHex(selectedOrg); err == nil {
			groupFilter["organization_id"] = oid
		}
	} else if role == "coordinator" {
		orgIDs := authz.UserOrgIDs(r)
		if len(orgIDs) > 0 {
			groupFilter["organization_id"] = bson.M{"$in": orgIDs}
		}
	}

	groupCur, err := h.DB.Collection("groups").Find(ctx, groupFilter, options.Find().
		SetSort(bson.D{{Key: "name", Value: 1}}).
		SetProjection(bson.M{"name": 1}))
	if err != nil {
		h.Log.Warn("fetch groups failed", zap.Error(err))
	} else {
		defer groupCur.Close(ctx)
		for groupCur.Next(ctx) {
			var g struct {
				ID   primitive.ObjectID `bson:"_id"`
				Name string             `bson:"name"`
			}
			if err := groupCur.Decode(&g); err != nil {
				continue
			}
			groups = append(groups, groupOption{
				ID:       g.ID.Hex(),
				Name:     g.Name,
				Selected: g.ID.Hex() == selectedGroup,
			})
		}
	}

	return orgs, groups
}

// buildScopeFilter creates a MongoDB filter based on user role and selections.
func (h *Handler) buildScopeFilter(r *http.Request, role, selectedOrg, selectedGroup string) bson.M {
	filter := bson.M{}

	if selectedOrg != "" {
		if oid, err := primitive.ObjectIDFromHex(selectedOrg); err == nil {
			filter["organization_id"] = oid
		}
	} else if role == "coordinator" {
		orgIDs := authz.UserOrgIDs(r)
		if len(orgIDs) > 0 {
			filter["organization_id"] = bson.M{"$in": orgIDs}
		}
	}
	// Admin with no org selected = all orgs

	// Group filter requires joining via user memberships (handled in queries)
	// We'll store the group ID for use in queries
	if selectedGroup != "" {
		// This will be used by queries to filter by group membership
		filter["_group_filter"] = selectedGroup
	}

	return filter
}

// parseDateRange extracts start and end dates from query params.
func (h *Handler) parseDateRange(r *http.Request) (time.Time, time.Time) {
	endDate := time.Now()
	startDate := endDate.AddDate(0, 0, -30)

	if s := r.URL.Query().Get("start"); s != "" {
		if t, err := time.Parse("2006-01-02", s); err == nil {
			startDate = t
		}
	}
	if e := r.URL.Query().Get("end"); e != "" {
		if t, err := time.Parse("2006-01-02", e); err == nil {
			endDate = t.Add(24*time.Hour - time.Second)
		}
	}

	return startDate, endDate
}

// fetchSessionExportRows fetches sessions for export.
func (h *Handler) fetchSessionExportRows(ctx context.Context, scopeFilter bson.M, startDate, endDate time.Time) ([]sessionExportRow, error) {
	// Get user IDs if group filter is specified
	var userIDFilter interface{}
	if groupHex, ok := scopeFilter["_group_filter"].(string); ok {
		if gid, err := primitive.ObjectIDFromHex(groupHex); err == nil {
			userIDs, err := h.getUserIDsInGroup(ctx, gid)
			if err != nil {
				return nil, err
			}
			if len(userIDs) == 0 {
				return []sessionExportRow{}, nil
			}
			userIDFilter = bson.M{"$in": userIDs}
		}
	}

	// Build session filter
	sessFilter := bson.M{
		"login_at": bson.M{"$gte": startDate, "$lte": endDate},
	}
	if orgID, ok := scopeFilter["organization_id"]; ok {
		sessFilter["organization_id"] = orgID
	}
	if userIDFilter != nil {
		sessFilter["user_id"] = userIDFilter
	}

	cur, err := h.DB.Collection("sessions").Find(ctx, sessFilter, options.Find().
		SetSort(bson.D{{Key: "login_at", Value: -1}}))
	if err != nil {
		return nil, err
	}
	defer cur.Close(ctx)

	// Collect user IDs and org IDs for batch lookup
	type sessDoc struct {
		ID         primitive.ObjectID  `bson:"_id"`
		UserID     primitive.ObjectID  `bson:"user_id"`
		OrgID      primitive.ObjectID  `bson:"organization_id"`
		LoginAt    time.Time           `bson:"login_at"`
		LogoutAt   *time.Time          `bson:"logout_at"`
		EndReason  string              `bson:"end_reason"`
		Duration   int64               `bson:"duration_secs"`
		IP         string              `bson:"ip"`
	}

	var sessions []sessDoc
	userIDSet := make(map[primitive.ObjectID]struct{})
	orgIDSet := make(map[primitive.ObjectID]struct{})

	for cur.Next(ctx) {
		var s sessDoc
		if err := cur.Decode(&s); err != nil {
			continue
		}
		sessions = append(sessions, s)
		userIDSet[s.UserID] = struct{}{}
		orgIDSet[s.OrgID] = struct{}{}
	}

	// Batch fetch user info
	userInfo := h.fetchUserInfo(ctx, userIDSet)
	orgNames := h.fetchOrgNames(ctx, orgIDSet)
	userGroups := h.fetchUserGroups(ctx, userIDSet)

	// Build export rows
	var rows []sessionExportRow
	for _, s := range sessions {
		ui := userInfo[s.UserID]
		logoutStr := ""
		if s.LogoutAt != nil {
			logoutStr = s.LogoutAt.Format(time.RFC3339)
		}
		rows = append(rows, sessionExportRow{
			UserID:       s.UserID.Hex(),
			UserName:     ui.FullName,
			Email:        ui.Email,
			Organization: orgNames[s.OrgID],
			Group:        userGroups[s.UserID],
			LoginAt:      s.LoginAt,
			LogoutAt:     logoutStr,
			EndReason:    s.EndReason,
			DurationSecs: s.Duration,
			IP:           s.IP,
		})
	}

	return rows, nil
}

// fetchEventExportRows fetches activity events for export.
func (h *Handler) fetchEventExportRows(ctx context.Context, scopeFilter bson.M, startDate, endDate time.Time) ([]activityExportRow, error) {
	// Get user IDs if group filter is specified
	var userIDFilter interface{}
	if groupHex, ok := scopeFilter["_group_filter"].(string); ok {
		if gid, err := primitive.ObjectIDFromHex(groupHex); err == nil {
			userIDs, err := h.getUserIDsInGroup(ctx, gid)
			if err != nil {
				return nil, err
			}
			if len(userIDs) == 0 {
				return []activityExportRow{}, nil
			}
			userIDFilter = bson.M{"$in": userIDs}
		}
	}

	// Build event filter
	eventFilter := bson.M{
		"timestamp": bson.M{"$gte": startDate, "$lte": endDate},
	}
	if orgID, ok := scopeFilter["organization_id"]; ok {
		eventFilter["organization_id"] = orgID
	}
	if userIDFilter != nil {
		eventFilter["user_id"] = userIDFilter
	}

	cur, err := h.DB.Collection("activity_events").Find(ctx, eventFilter, options.Find().
		SetSort(bson.D{{Key: "timestamp", Value: -1}}))
	if err != nil {
		return nil, err
	}
	defer cur.Close(ctx)

	// Collect user IDs for batch lookup
	type eventDoc struct {
		ID           primitive.ObjectID      `bson:"_id"`
		UserID       primitive.ObjectID      `bson:"user_id"`
		SessionID    primitive.ObjectID      `bson:"session_id"`
		Timestamp    time.Time               `bson:"timestamp"`
		EventType    string                  `bson:"event_type"`
		ResourceName string                  `bson:"resource_name"`
		Details      map[string]interface{}  `bson:"details"`
	}

	var events []eventDoc
	userIDSet := make(map[primitive.ObjectID]struct{})

	for cur.Next(ctx) {
		var e eventDoc
		if err := cur.Decode(&e); err != nil {
			continue
		}
		events = append(events, e)
		userIDSet[e.UserID] = struct{}{}
	}

	// Batch fetch user info
	userInfo := h.fetchUserInfo(ctx, userIDSet)

	// Build export rows
	var rows []activityExportRow
	for _, e := range events {
		ui := userInfo[e.UserID]
		rows = append(rows, activityExportRow{
			UserID:       e.UserID.Hex(),
			UserName:     ui.FullName,
			SessionID:    e.SessionID.Hex(),
			Timestamp:    e.Timestamp,
			EventType:    e.EventType,
			ResourceName: e.ResourceName,
			Details:      e.Details,
		})
	}

	return rows, nil
}

// computeAggregateStats computes aggregate statistics for the date range.
func (h *Handler) computeAggregateStats(ctx context.Context, scopeFilter bson.M, startDate, endDate time.Time) aggregateStats {
	stats := aggregateStats{
		SessionsByHour: make(map[int]int),
		SessionsByDay:  make(map[string]int),
	}

	// Get user IDs if group filter is specified
	var userIDFilter interface{}
	if groupHex, ok := scopeFilter["_group_filter"].(string); ok {
		if gid, err := primitive.ObjectIDFromHex(groupHex); err == nil {
			userIDs, err := h.getUserIDsInGroup(ctx, gid)
			if err != nil || len(userIDs) == 0 {
				return stats
			}
			userIDFilter = bson.M{"$in": userIDs}
		}
	}

	// Build session filter
	sessFilter := bson.M{
		"login_at": bson.M{"$gte": startDate, "$lte": endDate},
	}
	if orgID, ok := scopeFilter["organization_id"]; ok {
		sessFilter["organization_id"] = orgID
	}
	if userIDFilter != nil {
		sessFilter["user_id"] = userIDFilter
	}

	// Fetch sessions for stats
	cur, err := h.DB.Collection("sessions").Find(ctx, sessFilter)
	if err != nil {
		h.Log.Warn("fetch sessions for stats failed", zap.Error(err))
		return stats
	}
	defer cur.Close(ctx)

	userSet := make(map[primitive.ObjectID]struct{})
	for cur.Next(ctx) {
		var s struct {
			UserID       primitive.ObjectID `bson:"user_id"`
			LoginAt      time.Time          `bson:"login_at"`
			DurationSecs int64              `bson:"duration_secs"`
		}
		if err := cur.Decode(&s); err != nil {
			continue
		}

		stats.TotalSessions++
		stats.TotalDurationSecs += s.DurationSecs
		userSet[s.UserID] = struct{}{}

		// Track by hour and day
		hour := s.LoginAt.Hour()
		stats.SessionsByHour[hour]++

		day := s.LoginAt.Weekday().String()
		stats.SessionsByDay[day]++
	}

	stats.TotalUsers = len(userSet)

	return stats
}

// getUserIDsInGroup returns user IDs that are members of the given group.
func (h *Handler) getUserIDsInGroup(ctx context.Context, groupID primitive.ObjectID) ([]primitive.ObjectID, error) {
	cur, err := h.DB.Collection("group_memberships").Find(ctx, bson.M{
		"group_id": groupID,
		"role":     "member",
	}, options.Find().SetProjection(bson.M{"user_id": 1}))
	if err != nil {
		return nil, err
	}
	defer cur.Close(ctx)

	var userIDs []primitive.ObjectID
	for cur.Next(ctx) {
		var m struct {
			UserID primitive.ObjectID `bson:"user_id"`
		}
		if err := cur.Decode(&m); err != nil {
			continue
		}
		userIDs = append(userIDs, m.UserID)
	}
	return userIDs, nil
}

type userInfoCache struct {
	FullName string
	Email    string
}

// fetchUserInfo batch fetches user names and emails.
func (h *Handler) fetchUserInfo(ctx context.Context, userIDs map[primitive.ObjectID]struct{}) map[primitive.ObjectID]userInfoCache {
	result := make(map[primitive.ObjectID]userInfoCache)
	if len(userIDs) == 0 {
		return result
	}

	ids := make([]primitive.ObjectID, 0, len(userIDs))
	for id := range userIDs {
		ids = append(ids, id)
	}

	cur, err := h.DB.Collection("users").Find(ctx, bson.M{"_id": bson.M{"$in": ids}}, options.Find().
		SetProjection(bson.M{"full_name": 1, "email": 1}))
	if err != nil {
		h.Log.Warn("fetch user info failed", zap.Error(err))
		return result
	}
	defer cur.Close(ctx)

	for cur.Next(ctx) {
		var u struct {
			ID       primitive.ObjectID `bson:"_id"`
			FullName string             `bson:"full_name"`
			Email    *string            `bson:"email"`
		}
		if err := cur.Decode(&u); err != nil {
			continue
		}
		email := ""
		if u.Email != nil {
			email = *u.Email
		}
		result[u.ID] = userInfoCache{FullName: u.FullName, Email: email}
	}

	return result
}

// fetchOrgNames batch fetches organization names.
func (h *Handler) fetchOrgNames(ctx context.Context, orgIDs map[primitive.ObjectID]struct{}) map[primitive.ObjectID]string {
	result := make(map[primitive.ObjectID]string)
	if len(orgIDs) == 0 {
		return result
	}

	ids := make([]primitive.ObjectID, 0, len(orgIDs))
	for id := range orgIDs {
		ids = append(ids, id)
	}

	cur, err := h.DB.Collection("organizations").Find(ctx, bson.M{"_id": bson.M{"$in": ids}}, options.Find().
		SetProjection(bson.M{"name": 1}))
	if err != nil {
		h.Log.Warn("fetch org names failed", zap.Error(err))
		return result
	}
	defer cur.Close(ctx)

	for cur.Next(ctx) {
		var o struct {
			ID   primitive.ObjectID `bson:"_id"`
			Name string             `bson:"name"`
		}
		if err := cur.Decode(&o); err != nil {
			continue
		}
		result[o.ID] = o.Name
	}

	return result
}

// fetchUserGroups batch fetches primary group name for each user.
func (h *Handler) fetchUserGroups(ctx context.Context, userIDs map[primitive.ObjectID]struct{}) map[primitive.ObjectID]string {
	result := make(map[primitive.ObjectID]string)
	if len(userIDs) == 0 {
		return result
	}

	ids := make([]primitive.ObjectID, 0, len(userIDs))
	for id := range userIDs {
		ids = append(ids, id)
	}

	// Get memberships
	cur, err := h.DB.Collection("group_memberships").Find(ctx, bson.M{
		"user_id": bson.M{"$in": ids},
		"role":    "member",
	}, options.Find().SetProjection(bson.M{"user_id": 1, "group_id": 1}))
	if err != nil {
		h.Log.Warn("fetch user groups failed", zap.Error(err))
		return result
	}
	defer cur.Close(ctx)

	userGroups := make(map[primitive.ObjectID][]primitive.ObjectID)
	groupIDSet := make(map[primitive.ObjectID]struct{})

	for cur.Next(ctx) {
		var m struct {
			UserID  primitive.ObjectID `bson:"user_id"`
			GroupID primitive.ObjectID `bson:"group_id"`
		}
		if err := cur.Decode(&m); err != nil {
			continue
		}
		userGroups[m.UserID] = append(userGroups[m.UserID], m.GroupID)
		groupIDSet[m.GroupID] = struct{}{}
	}

	// Fetch group names
	groupNames := make(map[primitive.ObjectID]string)
	if len(groupIDSet) > 0 {
		gids := make([]primitive.ObjectID, 0, len(groupIDSet))
		for id := range groupIDSet {
			gids = append(gids, id)
		}

		gcur, err := h.DB.Collection("groups").Find(ctx, bson.M{"_id": bson.M{"$in": gids}}, options.Find().
			SetProjection(bson.M{"name": 1}))
		if err == nil {
			defer gcur.Close(ctx)
			for gcur.Next(ctx) {
				var g struct {
					ID   primitive.ObjectID `bson:"_id"`
					Name string             `bson:"name"`
				}
				if err := gcur.Decode(&g); err != nil {
					continue
				}
				groupNames[g.ID] = g.Name
			}
		}
	}

	// Build user -> group name mapping (use first group if multiple)
	for userID, gids := range userGroups {
		if len(gids) > 0 {
			result[userID] = groupNames[gids[0]]
		}
	}

	return result
}

// safeDiv performs integer division, returning 0 if divisor is 0.
func (h *Handler) safeDiv(a, b int) int {
	if b == 0 {
		return 0
	}
	return a / b
}

// findPeakHour finds the hour with most sessions.
func (h *Handler) findPeakHour(hourCounts map[int]int) string {
	if len(hourCounts) == 0 {
		return "N/A"
	}
	maxHour := 0
	maxCount := 0
	for hour, count := range hourCounts {
		if count > maxCount {
			maxCount = count
			maxHour = hour
		}
	}
	return fmt.Sprintf("%02d:00", maxHour)
}

// findMostActiveDay finds the weekday with most sessions.
func (h *Handler) findMostActiveDay(dayCounts map[string]int) string {
	if len(dayCounts) == 0 {
		return "N/A"
	}
	maxDay := ""
	maxCount := 0
	for day, count := range dayCounts {
		if count > maxCount {
			maxCount = count
			maxDay = day
		}
	}
	return maxDay
}

// sanitizeCSVField prevents CSV formula injection.
func sanitizeCSVField(s string) string {
	if len(s) == 0 {
		return s
	}
	switch s[0] {
	case '=', '+', '-', '@':
		return "'" + s
	}
	return s
}

// formatMinutes formats a duration in minutes as "Xh Ym" or "X min".
func formatMinutes(mins int) string {
	if mins >= 60 {
		h := mins / 60
		m := mins % 60
		return fmt.Sprintf("%dh %dm", h, m)
	}
	return fmt.Sprintf("%d min", mins)
}
