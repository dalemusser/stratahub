// internal/app/features/mhsdashboard/dashboard.go
package mhsdashboard

import (
	"context"
	"fmt"
	"net/http"
	"sort"
	"time"

	uierrors "github.com/dalemusser/stratahub/internal/app/features/errors"
	settingsstore "github.com/dalemusser/stratahub/internal/app/store/settings"
	"github.com/dalemusser/stratahub/internal/app/system/authz"
	"github.com/dalemusser/stratahub/internal/app/system/timeouts"
	"github.com/dalemusser/stratahub/internal/app/system/viewdata"
	"github.com/dalemusser/stratahub/internal/app/system/workspace"
	"github.com/dalemusser/stratahub/internal/domain/models"
	"github.com/dalemusser/waffle/pantry/templates"
	"github.com/gorilla/csrf"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.uber.org/zap"
)

// staleDeviceThreshold is how old a device's last_seen can be before it's considered stale.
const staleDeviceThreshold = 7 * 24 * time.Hour

// ServeDashboard renders the main MHS dashboard 3 page.
func (h *Handler) ServeDashboard(w http.ResponseWriter, r *http.Request) {
	role, _, userID, ok := authz.UserCtx(r)
	if !ok {
		uierrors.RenderUnauthorized(w, r, "/login")
		return
	}

	// Only leaders (and admins/coordinators for oversight) can access
	if role != "leader" && role != "admin" && role != "coordinator" && role != "superadmin" {
		uierrors.RenderForbidden(w, r, "You do not have access to this dashboard.", "/dashboard")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), timeouts.Short())
	defer cancel()

	// Load progress configuration
	cfg, err := LoadProgressConfig()
	if err != nil {
		h.ErrLog.LogServerError(w, r, "failed to load progress config", err, "Configuration error.", "/dashboard")
		return
	}

	// Load site settings for AI summaries toggle
	wsID := workspace.IDFromRequest(r)
	siteSettings, err := settingsstore.New(h.DB).Get(ctx, wsID)
	if err != nil {
		h.Log.Warn("failed to load site settings for dashboard", zap.Error(err))
	}
	enableSummaries := siteSettings.EnableClaudeSummaries

	// Get groups for this user
	groups, err := h.getGroupsForUser(ctx, r, userID, role)
	if err != nil {
		h.ErrLog.LogServerError(w, r, "failed to load groups", err, "A database error occurred.", "/dashboard")
		return
	}

	// Build headers for the grid
	unitHeaders, pointHeaders := h.buildHeaders(cfg)

	if len(groups) == 0 {
		// No groups - show empty state
		base := viewdata.LoadBase(r, h.DB)
		data := DashboardData{
			BaseVM:                base,
			Groups:                nil,
			UnitHeaders:           unitHeaders,
			PointHeaders:          pointHeaders,
			Members:               nil,
			EnableClaudeSummaries: enableSummaries,
		}
		templates.Render(w, r, "mhsdashboard_view", data)
		return
	}

	// Get selected group from query param, default to first group
	selectedGroup := r.URL.Query().Get("group")
	if selectedGroup == "" {
		selectedGroup = groups[0].ID.Hex()
	}

	// Get sort parameters from query, default to name ascending
	sortBy := r.URL.Query().Get("sort")
	if sortBy == "" {
		sortBy = "name"
	}
	sortDir := r.URL.Query().Get("dir")
	if sortDir != "desc" {
		sortDir = "asc"
	}

	isAdmin := role == "admin" || role == "coordinator" || role == "superadmin"

	// Build group options for dropdown (leader view)
	groupOptions := make([]GroupOption, len(groups))
	var selectedGroupName string
	var selectedGroupOrgID primitive.ObjectID
	for i, g := range groups {
		isSelected := g.ID.Hex() == selectedGroup
		groupOptions[i] = GroupOption{
			ID:       g.ID.Hex(),
			Name:     g.Name,
			Selected: isSelected,
		}
		if isSelected {
			selectedGroupName = g.Name
			selectedGroupOrgID = g.OrganizationID
		}
	}

	// Get members for selected group
	selectedOID, err := primitive.ObjectIDFromHex(selectedGroup)
	if err != nil {
		selectedOID = groups[0].ID
		selectedGroup = selectedOID.Hex()
		selectedGroupName = groups[0].Name
		selectedGroupOrgID = groups[0].OrganizationID
		groupOptions[0].Selected = true
	}

	// Load member counts for styled group dropdown (all roles)
	memberCounts, err := h.loadActiveMemberCounts(ctx, r, groups)
	if err != nil {
		h.ErrLog.LogServerError(w, r, "failed to load member counts", err, "A database error occurred.", "/dashboard")
		return
	}

	groupsEx := make([]GroupOptionEx, len(groups))
	for i, g := range groups {
		groupsEx[i] = GroupOptionEx{
			ID:          g.ID.Hex(),
			Name:        g.Name,
			OrgID:       g.OrganizationID.Hex(),
			MemberCount: memberCounts[g.ID],
			Selected:    g.ID.Hex() == selectedGroup,
		}
	}

	// Build admin view data (org dropdown)
	var orgs []OrgOption
	var selectedOrg string
	if isAdmin {
		orgs, err = h.loadOrgsWithGroupCounts(ctx, groups)
		if err != nil {
			h.ErrLog.LogServerError(w, r, "failed to load organizations", err, "A database error occurred.", "/dashboard")
			return
		}

		selectedOrg = selectedGroupOrgID.Hex()
		for i := range orgs {
			orgs[i].Selected = orgs[i].ID == selectedOrg
		}
	}

	members, err := h.getMembersForGroup(ctx, r, selectedOID, sortDir)
	if err != nil {
		h.ErrLog.LogServerError(w, r, "failed to load members", err, "A database error occurred.", "/dashboard")
		return
	}

	// Get organization timezone and format the time
	lastUpdated, tzAbbr := h.formatTimeInOrgTimezone(ctx, selectedGroupOrgID)

	// Load device status for all members
	deviceMap := h.loadDeviceMap(ctx, r, members)

	// Build progress rows with real grade data
	memberRows := h.buildProgressRows(ctx, r, members, cfg, deviceMap)

	base := viewdata.LoadBase(r, h.DB)
	data := DashboardData{
		BaseVM:                base,
		Groups:                groupOptions,
		SelectedGroup:         selectedGroup,
		GroupName:             selectedGroupName,
		MemberCount:           len(members),
		LastUpdated:           lastUpdated,
		TimezoneAbbr:          tzAbbr,
		IsAdmin:               isAdmin,
		Orgs:                  orgs,
		SelectedOrg:           selectedOrg,
		GroupsEx:               groupsEx,
		UnitHeaders:           unitHeaders,
		PointHeaders:          pointHeaders,
		Members:               memberRows,
		SortBy:                sortBy,
		SortDir:               sortDir,
		EnableClaudeSummaries: enableSummaries,
	}

	templates.Render(w, r, "mhsdashboard_view", data)
}

// ServeGrid renders just the grid content for HTMX refresh.
func (h *Handler) ServeGrid(w http.ResponseWriter, r *http.Request) {
	role, _, userID, ok := authz.UserCtx(r)
	if !ok {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}

	if role != "leader" && role != "admin" && role != "coordinator" && role != "superadmin" {
		w.WriteHeader(http.StatusForbidden)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), timeouts.Short())
	defer cancel()

	cfg, err := LoadProgressConfig()
	if err != nil {
		h.Log.Error("failed to load progress config", zap.Error(err))
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	// Load site settings for AI summaries toggle
	gridWsID := workspace.IDFromRequest(r)
	gridSettings, settingsErr := settingsstore.New(h.DB).Get(ctx, gridWsID)
	if settingsErr != nil {
		h.Log.Warn("failed to load site settings for grid", zap.Error(settingsErr))
	}
	gridEnableSummaries := gridSettings.EnableClaudeSummaries

	// Get groups for this user
	groups, err := h.getGroupsForUser(ctx, r, userID, role)
	if err != nil {
		h.Log.Error("failed to load groups", zap.Error(err))
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	// Build headers for the grid
	unitHeaders, pointHeaders := h.buildHeaders(cfg)

	gridIsAdmin := role == "admin" || role == "coordinator" || role == "superadmin"

	if len(groups) == 0 {
		data := GridData{
			UnitHeaders:           unitHeaders,
			PointHeaders:          pointHeaders,
			Members:               nil,
			CSRFToken:             csrf.Token(r),
			EnableClaudeSummaries: gridEnableSummaries,
			IsAdmin:               gridIsAdmin,
		}
		templates.Render(w, r, "mhsdashboard_grid", data)
		return
	}

	selectedGroup := r.URL.Query().Get("group")
	if selectedGroup == "" {
		selectedGroup = groups[0].ID.Hex()
	}

	// Get sort parameters from query, default to name ascending
	sortBy := r.URL.Query().Get("sort")
	if sortBy == "" {
		sortBy = "name"
	}
	sortDir := r.URL.Query().Get("dir")
	if sortDir != "desc" {
		sortDir = "asc"
	}

	selectedOID, err := primitive.ObjectIDFromHex(selectedGroup)
	if err != nil {
		selectedOID = groups[0].ID
		selectedGroup = selectedOID.Hex()
	}

	// Find group name and organization ID
	var groupName string
	var groupOrgID primitive.ObjectID
	for _, g := range groups {
		if g.ID == selectedOID {
			groupName = g.Name
			groupOrgID = g.OrganizationID
			break
		}
	}

	members, err := h.getMembersForGroup(ctx, r, selectedOID, sortDir)
	if err != nil {
		h.Log.Error("failed to load members", zap.Error(err))
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	// Get organization timezone and format the time
	lastUpdated, _ := h.formatTimeInOrgTimezone(ctx, groupOrgID)

	// Load device status for all members
	deviceMap := h.loadDeviceMap(ctx, r, members)

	memberRows := h.buildProgressRows(ctx, r, members, cfg, deviceMap)

	data := GridData{
		SelectedGroup:         selectedGroup,
		GroupName:             groupName,
		MemberCount:           len(members),
		LastUpdated:           lastUpdated,
		UnitHeaders:           unitHeaders,
		PointHeaders:          pointHeaders,
		Members:               memberRows,
		CSRFToken:             csrf.Token(r),
		SortBy:                sortBy,
		SortDir:               sortDir,
		EnableClaudeSummaries: gridEnableSummaries,
		IsAdmin:               gridIsAdmin,
	}

	templates.Render(w, r, "mhsdashboard_grid", data)
}

// HandleSetProgress sets a student's progress to a specific unit.
func (h *Handler) HandleSetProgress(w http.ResponseWriter, r *http.Request) {
	role, _, userID, ok := authz.UserCtx(r)
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	if role != "leader" && role != "admin" && role != "coordinator" && role != "superadmin" {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}

	targetUserHex := r.FormValue("user_id")
	targetUnit := r.FormValue("unit")
	group := r.FormValue("group")

	if targetUserHex == "" || targetUnit == "" || group == "" {
		http.Error(w, "missing required fields", http.StatusBadRequest)
		return
	}

	targetUserID, err := primitive.ObjectIDFromHex(targetUserHex)
	if err != nil {
		http.Error(w, "invalid user_id", http.StatusBadRequest)
		return
	}

	groupOID, err := primitive.ObjectIDFromHex(group)
	if err != nil {
		http.Error(w, "invalid group", http.StatusBadRequest)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), timeouts.Short())
	defer cancel()

	// Verify the requester has access to this group
	groups, err := h.getGroupsForUser(ctx, r, userID, role)
	if err != nil {
		h.Log.Error("failed to verify group access", zap.Error(err))
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	hasAccess := false
	for _, g := range groups {
		if g.ID == groupOID {
			hasAccess = true
			break
		}
	}
	if !hasAccess {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}

	// Verify the target user is a member of the group
	wsID := workspace.IDFromRequest(r)
	membershipFilter := bson.M{
		"group_id": groupOID,
		"user_id":  targetUserID,
		"role":     "member",
	}
	if wsID != primitive.NilObjectID {
		membershipFilter["workspace_id"] = wsID
	}

	count, err := h.DB.Collection("group_memberships").CountDocuments(ctx, membershipFilter)
	if err != nil {
		h.Log.Error("failed to verify membership", zap.Error(err))
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	if count == 0 {
		http.Error(w, "user is not a member of this group", http.StatusBadRequest)
		return
	}

	// Set progress
	if err := h.ProgressStore.SetToUnit(ctx, wsID, targetUserID, targetUnit); err != nil {
		h.Log.Error("failed to set progress", zap.Error(err), zap.String("targetUnit", targetUnit))
		http.Error(w, "failed to set progress", http.StatusInternalServerError)
		return
	}

	h.Log.Info("progress set",
		zap.String("targetUser", targetUserHex),
		zap.String("targetUnit", targetUnit),
		zap.String("group", group),
		zap.String("setBy", userID.Hex()),
	)

	// Trigger grid refresh via HTMX
	w.Header().Set("HX-Trigger", "refreshGrid")
	w.WriteHeader(http.StatusOK)
}

// getGroupsForUser returns the groups accessible to the given user based on their role.
func (h *Handler) getGroupsForUser(ctx context.Context, r *http.Request, userID primitive.ObjectID, role string) ([]models.Group, error) {
	wsID := workspace.IDFromRequest(r)

	if role == "admin" || role == "superadmin" || role == "coordinator" {
		// Admins/coordinators can see all groups in the workspace
		filter := bson.M{"status": "active"}
		if wsID != primitive.NilObjectID {
			filter["workspace_id"] = wsID
		}

		cur, err := h.DB.Collection("groups").Find(ctx, filter)
		if err != nil {
			return nil, err
		}
		defer cur.Close(ctx)

		var groups []models.Group
		if err := cur.All(ctx, &groups); err != nil {
			return nil, err
		}

		// Sort by name
		sort.Slice(groups, func(i, j int) bool {
			return groups[i].Name < groups[j].Name
		})

		return groups, nil
	}

	// Leaders see only their assigned groups
	membershipFilter := bson.M{
		"user_id": userID,
		"role":    "leader",
	}
	if wsID != primitive.NilObjectID {
		membershipFilter["workspace_id"] = wsID
	}

	cur, err := h.DB.Collection("group_memberships").Find(ctx, membershipFilter)
	if err != nil {
		return nil, err
	}
	defer cur.Close(ctx)

	var groupIDs []primitive.ObjectID
	for cur.Next(ctx) {
		var membership struct {
			GroupID primitive.ObjectID `bson:"group_id"`
		}
		if err := cur.Decode(&membership); err == nil {
			groupIDs = append(groupIDs, membership.GroupID)
		}
	}

	if len(groupIDs) == 0 {
		return nil, nil
	}

	// Fetch the actual groups
	groupCur, err := h.DB.Collection("groups").Find(ctx, bson.M{
		"_id":    bson.M{"$in": groupIDs},
		"status": "active",
	})
	if err != nil {
		return nil, err
	}
	defer groupCur.Close(ctx)

	var groups []models.Group
	if err := groupCur.All(ctx, &groups); err != nil {
		return nil, err
	}

	// Sort by name
	sort.Slice(groups, func(i, j int) bool {
		return groups[i].Name < groups[j].Name
	})

	return groups, nil
}

// getMembersForGroup returns all members of the given group, sorted by name.
// sortDir should be "asc" for A-Z or "desc" for Z-A.
func (h *Handler) getMembersForGroup(ctx context.Context, r *http.Request, groupID primitive.ObjectID, sortDir string) ([]models.User, error) {
	wsID := workspace.IDFromRequest(r)

	// Get member user IDs from memberships
	membershipFilter := bson.M{
		"group_id": groupID,
		"role":     "member",
	}
	if wsID != primitive.NilObjectID {
		membershipFilter["workspace_id"] = wsID
	}

	cur, err := h.DB.Collection("group_memberships").Find(ctx, membershipFilter)
	if err != nil {
		return nil, err
	}
	defer cur.Close(ctx)

	var userIDs []primitive.ObjectID
	for cur.Next(ctx) {
		var membership struct {
			UserID primitive.ObjectID `bson:"user_id"`
		}
		if err := cur.Decode(&membership); err == nil {
			userIDs = append(userIDs, membership.UserID)
		}
	}

	if len(userIDs) == 0 {
		return nil, nil
	}

	// Fetch user details
	userCur, err := h.DB.Collection("users").Find(ctx, bson.M{
		"_id":    bson.M{"$in": userIDs},
		"status": "active",
	})
	if err != nil {
		return nil, err
	}
	defer userCur.Close(ctx)

	var users []models.User
	if err := userCur.All(ctx, &users); err != nil {
		return nil, err
	}

	// Sort by name based on direction (case-insensitive using FullNameCI)
	if sortDir == "desc" {
		sort.Slice(users, func(i, j int) bool {
			return users[i].FullNameCI > users[j].FullNameCI
		})
	} else {
		sort.Slice(users, func(i, j int) bool {
			return users[i].FullNameCI < users[j].FullNameCI
		})
	}

	return users, nil
}

// buildHeaders builds the pre-computed header data for the grid.
func (h *Handler) buildHeaders(cfg *ProgressConfig) ([]UnitHeader, []PointHeader) {
	unitHeaders := make([]UnitHeader, len(cfg.Units))
	var pointHeaders []PointHeader

	for i, unit := range cfg.Units {
		unitHeaders[i] = UnitHeader{
			ID:             unit.ID,
			Title:          unit.Title,
			Width:          len(unit.ProgressPoints) * 28,
			AnalyticsWidth: len(unit.ProgressPoints) * 64,
			PointCount:     len(unit.ProgressPoints),
		}

		for j, point := range unit.ProgressPoints {
			pointHeaders = append(pointHeaders, PointHeader{
				ID:          point.ID,
				ShortName:   point.ShortName,
				Description: point.Description,
				IsUnitStart: j == 0,
			})
		}
	}

	return unitHeaders, pointHeaders
}

// ProgressGradeDoc represents a document from the progress_point_grades collection.
type ProgressGradeDoc struct {
	Game        string                         `bson:"game"`
	PlayerID    string                         `bson:"playerId"`
	Grades      map[string][]ProgressGradeItem `bson:"grades"`
	CurrentUnit string                         `bson:"currentUnit,omitempty"`
	LastUpdated time.Time                      `bson:"lastUpdated"`
}

// ProgressGradeItem represents a single grade within the grades array.
type ProgressGradeItem struct {
	Attempt            int            `bson:"attempt"`                      // 1-based attempt number
	Status             string         `bson:"status"`                       // "active", "passed", or "flagged"
	ComputedAt         time.Time      `bson:"computedAt"`
	RuleID             string         `bson:"ruleId"`
	ReasonCode         string         `bson:"reasonCode,omitempty"`         // Only for flagged grades
	Metrics            map[string]any `bson:"metrics,omitempty"`            // Grade metrics (mistakeCount, etc.)
	StartTime          *time.Time     `bson:"startTime,omitempty"`          // Activity start
	EndTime            *time.Time     `bson:"endTime,omitempty"`            // Activity end
	DurationSecs       *float64       `bson:"durationSecs,omitempty"`       // Wall-clock completion time in seconds
	ActiveDurationSecs *float64       `bson:"activeDurationSecs,omitempty"` // Active time excluding gaps in seconds
}

// formatDuration formats seconds into a human-readable duration string.
// Returns "m:ss" for durations under an hour, "h:mm:ss" for longer.
func formatDuration(secs float64) string {
	total := int(secs)
	if total < 0 {
		return ""
	}
	h := total / 3600
	m := (total % 3600) / 60
	s := total % 60
	if h > 0 {
		return fmt.Sprintf("%d:%02d:%02d", h, m, s)
	}
	return fmt.Sprintf("%d:%02d", m, s)
}

// reasonCodeToMessage maps reason codes to human-readable messages.
var reasonCodeToMessage = map[string]string{
	"NO_TRIGGER":                "Student has not yet completed the trigger event for this activity.",
	"TOO_MANY_TARGETS":         "Student used more targets than allowed for efficient problem-solving.",
	"TOO_MANY_TESTS":           "Student ran more tests than expected, may need guidance on efficiency.",
	"TOO_MANY_NEGATIVES":       "Student received too many negative responses during the activity.",
	"MISSING_SUCCESS_NODE":     "Student did not reach the expected success outcome.",
	"SCORE_BELOW_THRESHOLD":    "Student's score was below the expected threshold.",
	"HINT_OR_TOO_MANY_GUESSES": "Student needed hints or made too many incorrect attempts.",
	"PUZZLE_TOO_SLOW":          "Student took longer than expected to complete the puzzle.",
	"WRONG_ARG_SELECTED":       "Student needed multiple attempts to construct a correct scientific argument.",
	"BAD_FEEDBACK":             "Student received repeated corrective feedback during the activity.",
	"HIT_YELLOW_NODE":          "Student made an incorrect selection when evaluating evidence.",
}

// loadProgressGrades fetches progress grades from the mhsgrader database for the given player IDs.
func (h *Handler) loadProgressGrades(ctx context.Context, playerIDs []string) (map[string]*ProgressGradeDoc, error) {
	if h.GradesDB == nil {
		h.Log.Warn("mhsgrader database not configured, returning empty grades")
		return make(map[string]*ProgressGradeDoc), nil
	}

	h.Log.Debug("loading progress grades",
		zap.Int("playerCount", len(playerIDs)),
		zap.Strings("playerIDs", playerIDs),
	)

	filter := bson.M{
		"game":     "mhs",
		"playerId": bson.M{"$in": playerIDs},
	}

	cur, err := h.GradesDB.Collection("progress_point_grades").Find(ctx, filter)
	if err != nil {
		return nil, err
	}
	defer cur.Close(ctx)

	grades := make(map[string]*ProgressGradeDoc)
	for cur.Next(ctx) {
		var doc ProgressGradeDoc
		if err := cur.Decode(&doc); err != nil {
			h.Log.Warn("failed to decode progress grade document", zap.Error(err))
			continue
		}
		grades[doc.PlayerID] = &doc
	}

	h.Log.Debug("loaded progress grades",
		zap.Int("foundCount", len(grades)),
	)

	return grades, nil
}

// loadDeviceMap fetches device status for all members and returns a map keyed by user ID hex.
func (h *Handler) loadDeviceMap(ctx context.Context, r *http.Request, members []models.User) map[string][]DeviceInfo {
	deviceMap := make(map[string][]DeviceInfo)
	if len(members) == 0 {
		return deviceMap
	}

	wsID := workspace.IDFromRequest(r)
	userIDs := make([]primitive.ObjectID, len(members))
	for i, m := range members {
		userIDs[i] = m.ID
	}

	statuses, err := h.DeviceStatusStore.ListByUserIDs(ctx, wsID, userIDs)
	if err != nil {
		h.Log.Error("failed to load device statuses", zap.Error(err))
		return deviceMap
	}

	now := time.Now()
	for _, s := range statuses {
		uid := s.UserID.Hex()
		var pct int
		if s.StorageQuota > 0 {
			pct = int(s.StorageUsage * 100 / s.StorageQuota)
		}
		unitStatus := s.UnitStatus
		if unitStatus == nil {
			unitStatus = make(map[string]string)
		}
		deviceMap[uid] = append(deviceMap[uid], DeviceInfo{
			DeviceType:    s.DeviceType,
			DeviceDetails: s.DeviceDetails,
			PWAInstalled:  s.PWAInstalled,
			UnitStatus:    unitStatus,
			StorageUsage:  s.StorageUsage,
			StorageQuota:  s.StorageQuota,
			StoragePct:    pct,
			StorageUsed:   formatBytes(s.StorageUsage),
			StorageTotal:  formatBytes(s.StorageQuota),
			LastSeen:      s.LastSeen,
			IsStale:       now.Sub(s.LastSeen) > staleDeviceThreshold,
		})
	}

	return deviceMap
}

// buildProgressRows builds the progress rows for the given members using real grade data.
func (h *Handler) buildProgressRows(ctx context.Context, r *http.Request, members []models.User, cfg *ProgressConfig, deviceMap map[string][]DeviceInfo) []MemberRow {
	totalPoints := cfg.TotalProgressPoints()
	result := make([]MemberRow, len(members))

	// Collect player IDs (login IDs) for batch lookup
	// The playerId in mhsgrader matches the login_id in stratahub
	playerIDs := make([]string, 0, len(members))
	for _, member := range members {
		if member.LoginID != nil && *member.LoginID != "" {
			playerIDs = append(playerIDs, *member.LoginID)
		}
	}

	// Load grades from mhsgrader database
	grades, err := h.loadProgressGrades(ctx, playerIDs)
	if err != nil {
		h.Log.Error("failed to load progress grades", zap.Error(err))
		// Continue with empty grades rather than failing
		grades = make(map[string]*ProgressGradeDoc)
	}

	// Load MHS user progress (current unit from Mission HydroSci)
	wsID := workspace.IDFromRequest(r)
	userIDs := make([]primitive.ObjectID, len(members))
	for idx, m := range members {
		userIDs[idx] = m.ID
	}
	mhsProgress, err := h.ProgressStore.ListByUserIDs(ctx, wsID, userIDs)
	if err != nil {
		h.Log.Error("failed to load MHS user progress", zap.Error(err))
		mhsProgress = make(map[string]models.MHSUserProgress)
	}

	for i, member := range members {
		cells := make([]CellData, totalPoints)
		var gradeDoc *ProgressGradeDoc
		if member.LoginID != nil {
			gradeDoc = grades[*member.LoginID]
		}

		// Read current unit from grader (if available)
		var currentUnit string
		if gradeDoc != nil {
			currentUnit = gradeDoc.CurrentUnit
		}

		// Read current unit from MHS progress store
		var mhsCurrentUnit string
		if p, ok := mhsProgress[member.ID.Hex()]; ok {
			mhsCurrentUnit = p.CurrentUnit
		}

		pointIdx := 0
		for _, unit := range cfg.Units {
			for j, point := range unit.ProgressPoints {
				var value int
				var cellClass, borderClass, reviewReason string

				// Look up the latest grade for this progress point
				var gradeItem *ProgressGradeItem
				var attemptCount int
				if gradeDoc != nil {
					if items, ok := gradeDoc.Grades[point.ID]; ok && len(items) > 0 {
						latest := items[len(items)-1]
						gradeItem = &latest
						attemptCount = len(items)
					}
				}

				if gradeItem == nil {
					// No grade — pending (not started)
					value = 0
					cellClass = "mhs-cell-empty"
					borderClass = "border-gray-200 dark:border-gray-600"
				} else if gradeItem.Status == "passed" {
					value = 2
					cellClass = "mhs-cell-success"
					borderClass = "border-green-300"
				} else if gradeItem.Status == "active" {
					value = 3
					cellClass = "mhs-cell-active"
					borderClass = "border-blue-300"
				} else if gradeItem.Status == "flagged" {
					value = 1
					cellClass = "mhs-cell-warning"
					borderClass = "border-yellow-300"
					// Get human-readable message from reason code
					if msg, ok := reasonCodeToMessage[gradeItem.ReasonCode]; ok {
						reviewReason = msg
					} else if gradeItem.ReasonCode != "" {
						reviewReason = "Needs improvement: " + gradeItem.ReasonCode
					} else {
						reviewReason = "This progress point needs review."
					}
				}

				// Format completion durations if available
				var durationDisplay, activeDurationDisplay string
				if gradeItem != nil && gradeItem.DurationSecs != nil {
					durationDisplay = formatDuration(*gradeItem.DurationSecs)
				}
				if gradeItem != nil && gradeItem.ActiveDurationSecs != nil {
					activeDurationDisplay = formatDuration(*gradeItem.ActiveDurationSecs)
				}

				// Extract mistake count from grade metrics (-1 means no data)
				mistakeCount := -1
				if gradeItem != nil && gradeItem.Metrics != nil {
					if mc, ok := gradeItem.Metrics["mistakeCount"]; ok {
						switch v := mc.(type) {
						case int64:
							mistakeCount = int(v)
						case int32:
							mistakeCount = int(v)
						case float64:
							mistakeCount = int(v)
						}
					}
				}

				cells[pointIdx] = CellData{
					Value:                 value,
					IsUnitStart:           j == 0,
					IsInCurrentUnit:       currentUnit != "" && currentUnit == unit.ID,
					IsInMHSUnit:           mhsCurrentUnit != "" && mhsCurrentUnit == unit.ID,
					CellClass:             cellClass,
					BorderClass:           borderClass,
					PointID:               point.ID,
					PointTitle:            point.ShortName,
					StudentName:           member.FullName,
					ReviewReason:          reviewReason,
					DurationDisplay:       durationDisplay,
					ActiveDurationDisplay: activeDurationDisplay,
					MistakeCount:          mistakeCount,
					AttemptCount:          attemptCount,
				}
				pointIdx++
			}
		}

		// Mark skipped points:
		// - An "active" cell with passed cells after it (started but never completed)
		// - A "passed" cell with no duration data (end event fired but no meaningful timing)
		hasPassedAfter := false
		for j := len(cells) - 1; j >= 0; j-- {
			if cells[j].Value == 2 { // passed
				if cells[j].DurationDisplay == "" && cells[j].ActiveDurationDisplay == "" {
					cells[j].Skipped = true
				}
				hasPassedAfter = true
			} else if cells[j].Value == 3 && hasPassedAfter { // active with passed after
				cells[j].Skipped = true
			}
		}

		// Compute unit-level progress status
		unitProgress := make(map[string]string, len(cfg.Units))
		foundCurrent := false
		for _, unit := range cfg.Units {
			allPassed := true
			for _, point := range unit.ProgressPoints {
				var isPassed bool
				if gradeDoc != nil {
					if items, ok := gradeDoc.Grades[point.ID]; ok && len(items) > 0 {
						if items[len(items)-1].Status == "passed" {
							isPassed = true
						}
					}
				}
				if !isPassed {
					allPassed = false
					break
				}
			}
			if allPassed {
				unitProgress[unit.ID] = "completed"
			} else if !foundCurrent {
				unitProgress[unit.ID] = "current"
				foundCurrent = true
			} else {
				unitProgress[unit.ID] = "future"
			}
		}

		result[i] = MemberRow{
			ID:           member.ID.Hex(),
			Name:         member.FullName,
			IsEven:       i%2 == 0,
			Cells:        cells,
			Devices:      deviceMap[member.ID.Hex()],
			UnitProgress: unitProgress,
			CurrentUnit:  currentUnit,
		}
	}

	return result
}

// loadOrgsWithGroupCounts builds the organization dropdown options from the
// already-loaded groups slice, fetching org names from the database.
func (h *Handler) loadOrgsWithGroupCounts(ctx context.Context, groups []models.Group) ([]OrgOption, error) {
	// Count groups per org and collect unique org IDs
	orgGroupCount := make(map[primitive.ObjectID]int)
	var orgIDs []primitive.ObjectID
	seen := make(map[primitive.ObjectID]bool)
	for _, g := range groups {
		orgGroupCount[g.OrganizationID]++
		if !seen[g.OrganizationID] {
			seen[g.OrganizationID] = true
			orgIDs = append(orgIDs, g.OrganizationID)
		}
	}

	// Fetch org names
	cur, err := h.DB.Collection("organizations").Find(ctx, bson.M{
		"_id": bson.M{"$in": orgIDs},
	})
	if err != nil {
		return nil, err
	}
	defer cur.Close(ctx)

	orgNames := make(map[primitive.ObjectID]string)
	for cur.Next(ctx) {
		var org struct {
			ID   primitive.ObjectID `bson:"_id"`
			Name string             `bson:"name"`
		}
		if err := cur.Decode(&org); err == nil {
			orgNames[org.ID] = org.Name
		}
	}

	// Build sorted options
	orgs := make([]OrgOption, 0, len(orgIDs))
	for _, id := range orgIDs {
		name := orgNames[id]
		if name == "" {
			name = "Unknown Organization"
		}
		orgs = append(orgs, OrgOption{
			ID:         id.Hex(),
			Name:       name,
			GroupCount: orgGroupCount[id],
		})
	}
	sort.Slice(orgs, func(i, j int) bool {
		return orgs[i].Name < orgs[j].Name
	})

	return orgs, nil
}

// loadActiveMemberCounts returns a map of group ID to active member count.
// It joins group_memberships with users to count only active members,
// matching the grid display.
func (h *Handler) loadActiveMemberCounts(ctx context.Context, r *http.Request, groups []models.Group) (map[primitive.ObjectID]int, error) {
	wsID := workspace.IDFromRequest(r)

	groupIDs := make([]primitive.ObjectID, len(groups))
	for i, g := range groups {
		groupIDs[i] = g.ID
	}

	pipeline := mongo.Pipeline{
		{{Key: "$match", Value: bson.M{
			"group_id":     bson.M{"$in": groupIDs},
			"role":         "member",
			"workspace_id": wsID,
		}}},
		{{Key: "$lookup", Value: bson.M{
			"from":         "users",
			"localField":   "user_id",
			"foreignField": "_id",
			"as":           "user",
		}}},
		{{Key: "$unwind", Value: "$user"}},
		{{Key: "$match", Value: bson.M{
			"user.status": "active",
		}}},
		{{Key: "$group", Value: bson.M{
			"_id":   "$group_id",
			"count": bson.M{"$sum": 1},
		}}},
	}

	cur, err := h.DB.Collection("group_memberships").Aggregate(ctx, pipeline)
	if err != nil {
		return nil, err
	}
	defer cur.Close(ctx)

	counts := make(map[primitive.ObjectID]int)
	for cur.Next(ctx) {
		var result struct {
			ID    primitive.ObjectID `bson:"_id"`
			Count int                `bson:"count"`
		}
		if err := cur.Decode(&result); err == nil {
			counts[result.ID] = result.Count
		}
	}

	return counts, nil
}

// formatTimeInOrgTimezone formats the current time in the organization's timezone.
// Returns the formatted time string and the timezone abbreviation.
// Falls back to UTC if the organization or timezone is not found.
func (h *Handler) formatTimeInOrgTimezone(ctx context.Context, orgID primitive.ObjectID) (string, string) {
	now := time.Now()

	if orgID == primitive.NilObjectID {
		return now.UTC().Format("Jan 2, 2006 3:04 PM"), "UTC"
	}

	// Get organization timezone
	var org struct {
		TimeZone string `bson:"time_zone"`
	}
	err := h.DB.Collection("organizations").FindOne(ctx, bson.M{"_id": orgID}).Decode(&org)
	if err != nil || org.TimeZone == "" {
		return now.UTC().Format("Jan 2, 2006 3:04 PM"), "UTC"
	}

	// Load the timezone location
	loc, err := time.LoadLocation(org.TimeZone)
	if err != nil {
		return now.UTC().Format("Jan 2, 2006 3:04 PM"), "UTC"
	}

	localTime := now.In(loc)
	tzAbbr := localTime.Format("MST") // Gets the timezone abbreviation
	return localTime.Format("Jan 2, 2006 3:04 PM"), tzAbbr
}

// formatBytes converts bytes to a human-readable string (MB or GB).
func formatBytes(b int64) string {
	const (
		mb = 1024 * 1024
		gb = 1024 * 1024 * 1024
	)
	switch {
	case b >= gb:
		return fmt.Sprintf("%.1f GB", float64(b)/float64(gb))
	case b >= mb:
		return fmt.Sprintf("%.0f MB", float64(b)/float64(mb))
	default:
		return fmt.Sprintf("%d B", b)
	}
}
