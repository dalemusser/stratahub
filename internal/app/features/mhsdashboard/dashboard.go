// internal/app/features/mhsdashboard/dashboard.go
package mhsdashboard

import (
	"context"
	"net/http"
	"sort"
	"time"

	uierrors "github.com/dalemusser/stratahub/internal/app/features/errors"
	"github.com/dalemusser/stratahub/internal/app/system/authz"
	"github.com/dalemusser/stratahub/internal/app/system/timeouts"
	"github.com/dalemusser/stratahub/internal/app/system/viewdata"
	"github.com/dalemusser/stratahub/internal/app/system/workspace"
	"github.com/dalemusser/stratahub/internal/domain/models"
	"github.com/dalemusser/waffle/pantry/templates"
	"github.com/gorilla/csrf"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.uber.org/zap"
)

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
			BaseVM:       base,
			Groups:       nil,
			UnitHeaders:  unitHeaders,
			PointHeaders: pointHeaders,
			Members:      nil,
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

	// Build group options for dropdown
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

	members, err := h.getMembersForGroup(ctx, r, selectedOID, sortDir)
	if err != nil {
		h.ErrLog.LogServerError(w, r, "failed to load members", err, "A database error occurred.", "/dashboard")
		return
	}

	// Get organization timezone and format the time
	lastUpdated, tzAbbr := h.formatTimeInOrgTimezone(ctx, selectedGroupOrgID)

	// Build progress rows with real grade data
	memberRows := h.buildProgressRows(ctx, members, cfg)

	base := viewdata.LoadBase(r, h.DB)
	data := DashboardData{
		BaseVM:        base,
		Groups:        groupOptions,
		SelectedGroup: selectedGroup,
		GroupName:     selectedGroupName,
		MemberCount:   len(members),
		LastUpdated:   lastUpdated,
		TimezoneAbbr:  tzAbbr,
		UnitHeaders:   unitHeaders,
		PointHeaders:  pointHeaders,
		Members:       memberRows,
		SortBy:        sortBy,
		SortDir:       sortDir,
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

	// Get groups for this user
	groups, err := h.getGroupsForUser(ctx, r, userID, role)
	if err != nil {
		h.Log.Error("failed to load groups", zap.Error(err))
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	// Build headers for the grid
	unitHeaders, pointHeaders := h.buildHeaders(cfg)

	if len(groups) == 0 {
		data := GridData{
			UnitHeaders:  unitHeaders,
			PointHeaders: pointHeaders,
			Members:      nil,
			CSRFToken:    csrf.Token(r),
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

	memberRows := h.buildProgressRows(ctx, members, cfg)

	data := GridData{
		SelectedGroup: selectedGroup,
		GroupName:     groupName,
		MemberCount:   len(members),
		LastUpdated:   lastUpdated,
		UnitHeaders:   unitHeaders,
		PointHeaders:  pointHeaders,
		Members:       memberRows,
		CSRFToken:     csrf.Token(r),
		SortBy:        sortBy,
		SortDir:       sortDir,
	}

	templates.Render(w, r, "mhsdashboard_grid", data)
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
			ID:         unit.ID,
			Title:      unit.Title,
			Width:      len(unit.ProgressPoints) * 28,
			PointCount: len(unit.ProgressPoints),
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
	Game        string                       `bson:"game"`
	PlayerID    string                       `bson:"playerId"`
	Grades      map[string]ProgressGradeItem `bson:"grades"`
	LastUpdated time.Time                    `bson:"lastUpdated"`
}

// ProgressGradeItem represents a single grade within the grades map.
type ProgressGradeItem struct {
	Color      string         `bson:"color"`      // "green" or "yellow"
	ComputedAt time.Time      `bson:"computedAt"`
	RuleID     string         `bson:"ruleId"`
	ReasonCode string         `bson:"reasonCode,omitempty"` // Only for yellow grades
	Metrics    map[string]any `bson:"metrics,omitempty"`    // Only for yellow grades
}

// reasonCodeToMessage maps reason codes to human-readable messages.
var reasonCodeToMessage = map[string]string{
	"TOO_MANY_TARGETS":       "Student used more targets than allowed for efficient problem-solving.",
	"TOO_MANY_TESTS":         "Student ran more tests than expected, may need guidance on efficiency.",
	"TOO_MANY_NEGATIVE":      "Student received too many negative responses during the activity.",
	"BAD_FEEDBACK":           "Student's responses indicate areas needing improvement.",
	"SCORE_BELOW_THRESHOLD":  "Student's score was below the expected threshold.",
	"YELLOW_NODES_EXIST":     "Student completed with some areas still needing attention.",
	"INCOMPLETE":             "Student did not complete all required components.",
	"HINT_OR_TOO_MANY_GUESSES": "Student needed hints or made too many incorrect attempts.",
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

// buildProgressRows builds the progress rows for the given members using real grade data.
func (h *Handler) buildProgressRows(ctx context.Context, members []models.User, cfg *ProgressConfig) []MemberRow {
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

	for i, member := range members {
		cells := make([]CellData, totalPoints)
		var gradeDoc *ProgressGradeDoc
		if member.LoginID != nil {
			gradeDoc = grades[*member.LoginID]
		}

		pointIdx := 0
		for _, unit := range cfg.Units {
			for j, point := range unit.ProgressPoints {
				var value int
				var cellClass, borderClass, reviewReason string

				// Look up the grade for this progress point
				var gradeItem *ProgressGradeItem
				if gradeDoc != nil {
					if item, ok := gradeDoc.Grades[point.ID]; ok {
						gradeItem = &item
					}
				}

				if gradeItem == nil {
					// No grade - white (not started)
					value = 0
					cellClass = "mhs-cell-empty"
					borderClass = "border-gray-200 dark:border-gray-600"
				} else if gradeItem.Color == "green" {
					value = 2
					cellClass = "mhs-cell-success"
					borderClass = "border-green-300"
				} else if gradeItem.Color == "yellow" {
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

				cells[pointIdx] = CellData{
					Value:        value,
					IsUnitStart:  j == 0,
					CellClass:    cellClass,
					BorderClass:  borderClass,
					PointID:      point.ID,
					PointTitle:   point.ShortName,
					StudentName:  member.FullName,
					ReviewReason: reviewReason,
				}
				pointIdx++
			}
		}

		result[i] = MemberRow{
			ID:     member.ID.Hex(),
			Name:   member.FullName,
			IsEven: i%2 == 0,
			Cells:  cells,
		}
	}

	return result
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
