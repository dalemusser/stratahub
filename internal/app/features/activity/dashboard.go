// internal/app/features/activity/dashboard.go
package activity

// Terminology: User Identifiers
//   - UserID / userID / user_id: The MongoDB ObjectID (_id) that uniquely identifies a user record
//   - LoginID / loginID / login_id: The human-readable string users type to log in

import (
	"context"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/dalemusser/stratahub/internal/app/system/authz"
	"github.com/dalemusser/stratahub/internal/app/system/paging"
	"github.com/dalemusser/stratahub/internal/app/system/timeouts"
	"github.com/dalemusser/stratahub/internal/app/system/viewdata"
	"github.com/dalemusser/stratahub/internal/app/system/workspace"
	"github.com/dalemusser/waffle/pantry/query"
	"github.com/dalemusser/waffle/pantry/templates"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
)

// ServeDashboard renders the real-time activity dashboard.
// GET /activity
func (h *Handler) ServeDashboard(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), timeouts.Medium())
	defer cancel()

	// Get viewer role for determining available roles
	viewerRole, _, _, _ := authz.UserCtx(r)

	// Parse query parameters
	selectedGroup := query.Get(r, "group")
	statusFilter := query.Get(r, "status")
	if statusFilter == "" {
		statusFilter = "all"
	}
	roleFilter := query.Get(r, "role")
	if roleFilter == "" {
		roleFilter = "all"
	}
	searchQuery := query.Get(r, "search")
	sortBy := query.Get(r, "sort")
	if sortBy == "" {
		sortBy = "name"
	}
	sortDir := query.Get(r, "dir")
	if sortDir == "" {
		sortDir = "asc"
	}
	page := 1
	if p := query.Get(r, "page"); p != "" {
		if n, err := strconv.Atoi(p); err == nil && n > 0 {
			page = n
		}
	}

	// Build role filter options based on viewer role
	availableRoles := getAvailableRoles(viewerRole)
	roleOptions := buildRoleOptions(availableRoles, roleFilter)
	showRoleFilter := len(availableRoles) > 1

	// Get groups and all users based on viewer role
	groups, allMembers, err := h.fetchGroupsAndMembers(ctx, r, selectedGroup, roleFilter)
	if err != nil {
		h.ErrLog.LogServerError(w, r, "failed to fetch groups and members", err, "A database error occurred.", "/")
		return
	}

	// Count statuses (before filtering)
	var onlineCount, idleCount, offlineCount int
	for _, m := range allMembers {
		switch m.Status {
		case StatusOnline:
			onlineCount++
		case StatusIdle:
			idleCount++
		case StatusOffline:
			offlineCount++
		}
	}

	// Filter by search query
	filteredMembers := filterMembersBySearch(allMembers, searchQuery)

	// Filter by role (for UI filtering when viewing multiple roles)
	filteredMembers = filterMembersByRole(filteredMembers, roleFilter)

	// Filter by status
	filteredMembers = filterMembersByStatus(filteredMembers, statusFilter)

	// Sort members
	sortMembers(filteredMembers, sortBy, sortDir)

	// Paginate
	total := len(filteredMembers)
	pageSize := paging.PageSize
	startIdx := (page - 1) * pageSize
	endIdx := startIdx + pageSize
	if startIdx > total {
		startIdx = total
	}
	if endIdx > total {
		endIdx = total
	}
	pagedMembers := filteredMembers[startIdx:endIdx]

	// Calculate pagination info
	rangeStart := 0
	rangeEnd := 0
	if total > 0 {
		rangeStart = startIdx + 1
		rangeEnd = endIdx
	}

	data := dashboardData{
		BaseVM:         viewdata.NewBaseVM(r, h.DB, "Activity Dashboard", "/activity"),
		SelectedGroup:  selectedGroup,
		Groups:         groups,
		StatusFilter:   statusFilter,
		SearchQuery:    searchQuery,
		RoleFilter:     roleFilter,
		Roles:          roleOptions,
		ShowRoleFilter: showRoleFilter,
		SortBy:         sortBy,
		SortDir:        sortDir,
		Page:           page,
		Total:          total,
		RangeStart:     rangeStart,
		RangeEnd:       rangeEnd,
		HasPrev:        page > 1,
		HasNext:        endIdx < total,
		PrevPage:       page - 1,
		NextPage:       page + 1,
		TotalMembers:   len(allMembers),
		OnlineCount:    onlineCount,
		IdleCount:      idleCount,
		OfflineCount:   offlineCount,
		Members:        pagedMembers,
	}

	templates.Render(w, r, "activity_dashboard", data)
}

// ServeOnlineTable renders just the members table for HTMX refresh.
// GET /activity/online-table
func (h *Handler) ServeOnlineTable(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), timeouts.Medium())
	defer cancel()

	// Get viewer role for determining available roles
	viewerRole, _, _, _ := authz.UserCtx(r)

	// Parse query parameters
	selectedGroup := query.Get(r, "group")
	statusFilter := query.Get(r, "status")
	if statusFilter == "" {
		statusFilter = "all"
	}
	roleFilter := query.Get(r, "role")
	if roleFilter == "" {
		roleFilter = "all"
	}
	searchQuery := query.Get(r, "search")
	sortBy := query.Get(r, "sort")
	if sortBy == "" {
		sortBy = "name"
	}
	sortDir := query.Get(r, "dir")
	if sortDir == "" {
		sortDir = "asc"
	}
	page := 1
	if p := query.Get(r, "page"); p != "" {
		if n, err := strconv.Atoi(p); err == nil && n > 0 {
			page = n
		}
	}

	// Build role filter options based on viewer role
	availableRoles := getAvailableRoles(viewerRole)
	roleOptions := buildRoleOptions(availableRoles, roleFilter)
	showRoleFilter := len(availableRoles) > 1

	// Get groups and all users based on viewer role
	groups, allMembers, err := h.fetchGroupsAndMembers(ctx, r, selectedGroup, roleFilter)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	// Count statuses (before filtering)
	var onlineCount, idleCount, offlineCount int
	for _, m := range allMembers {
		switch m.Status {
		case StatusOnline:
			onlineCount++
		case StatusIdle:
			idleCount++
		case StatusOffline:
			offlineCount++
		}
	}

	// Filter by search query
	filteredMembers := filterMembersBySearch(allMembers, searchQuery)

	// Filter by role (for UI filtering when viewing multiple roles)
	filteredMembers = filterMembersByRole(filteredMembers, roleFilter)

	// Filter by status
	filteredMembers = filterMembersByStatus(filteredMembers, statusFilter)

	// Sort members
	sortMembers(filteredMembers, sortBy, sortDir)

	// Paginate
	total := len(filteredMembers)
	pageSize := paging.PageSize
	startIdx := (page - 1) * pageSize
	endIdx := startIdx + pageSize
	if startIdx > total {
		startIdx = total
	}
	if endIdx > total {
		endIdx = total
	}
	pagedMembers := filteredMembers[startIdx:endIdx]

	// Calculate pagination info
	rangeStart := 0
	rangeEnd := 0
	if total > 0 {
		rangeStart = startIdx + 1
		rangeEnd = endIdx
	}

	data := dashboardData{
		BaseVM:         viewdata.NewBaseVM(r, h.DB, "Activity Dashboard", "/activity"),
		SelectedGroup:  selectedGroup,
		Groups:         groups,
		StatusFilter:   statusFilter,
		SearchQuery:    searchQuery,
		RoleFilter:     roleFilter,
		Roles:          roleOptions,
		ShowRoleFilter: showRoleFilter,
		SortBy:         sortBy,
		SortDir:        sortDir,
		Page:           page,
		Total:          total,
		RangeStart:     rangeStart,
		RangeEnd:       rangeEnd,
		HasPrev:        page > 1,
		HasNext:        endIdx < total,
		PrevPage:       page - 1,
		NextPage:       page + 1,
		TotalMembers:   len(allMembers),
		OnlineCount:    onlineCount,
		IdleCount:      idleCount,
		OfflineCount:   offlineCount,
		Members:        pagedMembers,
	}

	templates.Render(w, r, "activity_online_table", data)
}

// fetchGroupsAndMembers gets the groups and user activity for the current user's scope.
// The roleFilter parameter is used for deciding which roles to fetch.
func (h *Handler) fetchGroupsAndMembers(ctx context.Context, r *http.Request, selectedGroup, roleFilter string) ([]groupOption, []memberRow, error) {
	viewerRole, _, userID, ok := authz.UserCtx(r)
	if !ok {
		return nil, nil, nil
	}

	db := h.DB
	now := time.Now().UTC()

	// Determine which target roles to fetch based on viewer role
	var targetRoles []string
	var includeNonGroupUsers bool

	switch viewerRole {
	case "superadmin", "admin":
		// Admin/SuperAdmin can see all roles
		targetRoles = []string{"member", "leader"}
		includeNonGroupUsers = true // Also fetch coordinators, admins, superadmins
	case "coordinator":
		// Coordinator can see members and leaders in their orgs
		targetRoles = []string{"member", "leader"}
		includeNonGroupUsers = false
	case "leader":
		// Leader can only see members in their groups
		targetRoles = []string{"member"}
		includeNonGroupUsers = false
	default:
		return nil, nil, nil
	}

	// Get groups based on viewer role
	var groupIDs []primitive.ObjectID
	var groupMap = make(map[primitive.ObjectID]string) // ID -> Name
	var orgIDs []primitive.ObjectID                     // For coordinator/admin org scoping

	switch viewerRole {
	case "superadmin", "admin":
		// SuperAdmin/Admin can see all groups
		groups, err := h.fetchAllGroups(ctx, db)
		if err != nil {
			return nil, nil, err
		}
		for _, g := range groups {
			groupIDs = append(groupIDs, g.ID)
			groupMap[g.ID] = g.Name
		}

	case "coordinator":
		// Coordinator sees groups in their assigned organizations
		orgIDs = authz.UserOrgIDs(r)
		groups, err := h.fetchGroupsByOrgs(ctx, db, orgIDs)
		if err != nil {
			return nil, nil, err
		}
		for _, g := range groups {
			groupIDs = append(groupIDs, g.ID)
			groupMap[g.ID] = g.Name
		}

	case "leader":
		// Leader sees only groups they lead
		groups, err := h.fetchLeaderGroups(ctx, db, userID)
		if err != nil {
			return nil, nil, err
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
	// Sort groups by name
	sort.Slice(groups[1:], func(i, j int) bool {
		return groups[i+1].Name < groups[j+1].Name
	})

	// Filter to selected group if specified
	var filterGroupIDs []primitive.ObjectID
	if selectedGroup != "" {
		if oid, err := primitive.ObjectIDFromHex(selectedGroup); err == nil {
			// Verify this group is in our allowed list
			if _, ok := groupMap[oid]; ok {
				filterGroupIDs = []primitive.ObjectID{oid}
			}
		}
	}
	if len(filterGroupIDs) == 0 {
		filterGroupIDs = groupIDs
	}

	// Get group-based users (members and leaders from group_memberships)
	var allUsers []memberRow
	if len(filterGroupIDs) > 0 {
		groupUsers, err := h.fetchGroupUsersWithActivity(ctx, db, filterGroupIDs, groupMap, targetRoles, now)
		if err != nil {
			return nil, nil, err
		}
		allUsers = append(allUsers, groupUsers...)
	}

	// For admins/superadmins, also fetch non-group users (coordinators, admins, superadmins)
	if includeNonGroupUsers {
		nonGroupRoles := []string{"coordinator", "admin", "superadmin"}
		nonGroupUsers, err := h.fetchNonGroupUsersWithActivity(ctx, db, nonGroupRoles, now)
		if err != nil {
			return nil, nil, err
		}
		allUsers = append(allUsers, nonGroupUsers...)
	}

	return groups, allUsers, nil
}

// fetchLeaderGroups gets groups where the user is a leader.
func (h *Handler) fetchLeaderGroups(ctx context.Context, db *mongo.Database, userID primitive.ObjectID) ([]leaderGroup, error) {
	// Step 1: Get group IDs from memberships where user is leader
	memCur, err := db.Collection("group_memberships").Find(ctx, bson.M{
		"user_id": userID,
		"role":    "leader",
	})
	if err != nil {
		return nil, err
	}
	defer memCur.Close(ctx)

	var groupIDs []primitive.ObjectID
	for memCur.Next(ctx) {
		var gm struct {
			GroupID primitive.ObjectID `bson:"group_id"`
		}
		if err := memCur.Decode(&gm); err != nil {
			return nil, err
		}
		groupIDs = append(groupIDs, gm.GroupID)
	}
	if err := memCur.Err(); err != nil {
		return nil, err
	}

	if len(groupIDs) == 0 {
		return nil, nil
	}

	// Step 2: Fetch group details
	grpCur, err := db.Collection("groups").Find(ctx, bson.M{
		"_id": bson.M{"$in": groupIDs},
	})
	if err != nil {
		return nil, err
	}
	defer grpCur.Close(ctx)

	var groups []leaderGroup
	for grpCur.Next(ctx) {
		var doc struct {
			ID   primitive.ObjectID `bson:"_id"`
			Name string             `bson:"name"`
		}
		if err := grpCur.Decode(&doc); err != nil {
			return nil, err
		}
		groups = append(groups, leaderGroup{ID: doc.ID, Name: doc.Name})
	}
	return groups, nil
}

// fetchGroupsByOrgs gets groups in the specified organizations.
func (h *Handler) fetchGroupsByOrgs(ctx context.Context, db *mongo.Database, orgIDs []primitive.ObjectID) ([]leaderGroup, error) {
	if len(orgIDs) == 0 {
		return nil, nil
	}

	filter := bson.M{
		"organization_id": bson.M{"$in": orgIDs},
		"status":          "active",
	}
	workspace.FilterCtx(ctx, filter)
	cur, err := db.Collection("groups").Find(ctx, filter)
	if err != nil {
		return nil, err
	}
	defer cur.Close(ctx)

	var groups []leaderGroup
	for cur.Next(ctx) {
		var doc struct {
			ID   primitive.ObjectID `bson:"_id"`
			Name string             `bson:"name"`
		}
		if err := cur.Decode(&doc); err != nil {
			return nil, err
		}
		groups = append(groups, leaderGroup{ID: doc.ID, Name: doc.Name})
	}
	return groups, nil
}

// fetchAllGroups gets all active groups (for admins).
func (h *Handler) fetchAllGroups(ctx context.Context, db *mongo.Database) ([]leaderGroup, error) {
	filter := bson.M{"status": "active"}
	workspace.FilterCtx(ctx, filter)
	cur, err := db.Collection("groups").Find(ctx, filter)
	if err != nil {
		return nil, err
	}
	defer cur.Close(ctx)

	var groups []leaderGroup
	for cur.Next(ctx) {
		var doc struct {
			ID   primitive.ObjectID `bson:"_id"`
			Name string             `bson:"name"`
		}
		if err := cur.Decode(&doc); err != nil {
			return nil, err
		}
		groups = append(groups, leaderGroup{ID: doc.ID, Name: doc.Name})
	}
	return groups, nil
}

// fetchNonGroupUsersWithActivity fetches users who don't appear in group_memberships
// (coordinators, admins, superadmins) with their activity status.
func (h *Handler) fetchNonGroupUsersWithActivity(ctx context.Context, db *mongo.Database, roles []string, now time.Time) ([]memberRow, error) {
	if len(roles) == 0 {
		return nil, nil
	}

	// Query users directly by role
	filter := bson.M{
		"role":   bson.M{"$in": roles},
		"status": "active",
	}
	workspace.FilterCtx(ctx, filter)

	cur, err := db.Collection("users").Find(ctx, filter)
	if err != nil {
		return nil, err
	}
	defer cur.Close(ctx)

	type userInfo struct {
		ID      primitive.ObjectID  `bson:"_id"`
		Name    string              `bson:"full_name"`
		LoginID string              `bson:"login_id"`
		Email   string              `bson:"email"`
		Role    string              `bson:"role"`
		OrgID   *primitive.ObjectID `bson:"organization_id"`
	}

	var usersInfo []userInfo
	var userIDs []primitive.ObjectID
	orgIDs := make(map[primitive.ObjectID]bool)

	for cur.Next(ctx) {
		var u userInfo
		if err := cur.Decode(&u); err != nil {
			return nil, err
		}
		usersInfo = append(usersInfo, u)
		userIDs = append(userIDs, u.ID)
		if u.OrgID != nil && !u.OrgID.IsZero() {
			orgIDs[*u.OrgID] = true
		}
	}

	if len(usersInfo) == 0 {
		return nil, nil
	}

	// Fetch organization names
	orgNames := make(map[primitive.ObjectID]string)
	if len(orgIDs) > 0 {
		var orgIDList []primitive.ObjectID
		for oid := range orgIDs {
			orgIDList = append(orgIDList, oid)
		}
		orgCur, err := db.Collection("organizations").Find(ctx, bson.M{"_id": bson.M{"$in": orgIDList}})
		if err == nil {
			defer orgCur.Close(ctx)
			for orgCur.Next(ctx) {
				var org struct {
					ID   primitive.ObjectID `bson:"_id"`
					Name string             `bson:"name"`
				}
				if orgCur.Decode(&org) == nil {
					orgNames[org.ID] = org.Name
				}
			}
		}
	}

	// Get active sessions for these users
	sessionMap, err := h.getActiveSessionsForUsers(ctx, userIDs, now)
	if err != nil {
		return nil, err
	}

	// Get today's activity for these users
	todayStart := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
	todayActivity, err := h.getTodayActivityForUsers(ctx, userIDs, todayStart, now)
	if err != nil {
		return nil, err
	}

	// Build user rows
	var users []memberRow
	for _, u := range usersInfo {
		// Get organization name
		orgName := ""
		if u.OrgID != nil && !u.OrgID.IsZero() {
			orgName = orgNames[*u.OrgID]
		}

		// Determine status
		status := StatusOffline
		statusLabel := "Offline"
		var lastActive *time.Time

		if sess, ok := sessionMap[u.ID]; ok {
			lastActive = &sess.LastActiveAt
			timeSince := now.Sub(sess.LastActiveAt)
			if timeSince < OnlineThreshold {
				status = StatusOnline
				statusLabel = "Active"
			} else if timeSince < IdleThreshold {
				status = StatusIdle
				statusLabel = "Idle"
			}
		}

		// Get current activity (only for online/idle users)
		currentActivity := ""
		if status != StatusOffline {
			sess, hasSession := sessionMap[u.ID]
			currentPage := ""
			if hasSession {
				currentPage = sess.CurrentPage
			}

			if currentPage != "" {
				currentActivity = formatPageName(currentPage)
			} else {
				currentActivity = "Dashboard"
			}
		}

		// Get time today
		timeTodayMins := 0
		if mins, ok := todayActivity[u.ID]; ok {
			timeTodayMins = mins
		}

		// Format time today
		timeTodayStr := "0 min"
		if timeTodayMins > 0 {
			if timeTodayMins >= 60 {
				timeTodayStr = fmt.Sprintf("%dh %dm", timeTodayMins/60, timeTodayMins%60)
			} else {
				timeTodayStr = fmt.Sprintf("%d min", timeTodayMins)
			}
		}

		users = append(users, memberRow{
			ID:              u.ID.Hex(),
			Name:            u.Name,
			LoginID:         u.LoginID,
			Email:           u.Email,
			OrgName:         orgName,
			GroupName:       "", // Non-group users don't have a group
			Role:            u.Role,
			Status:          status,
			StatusLabel:     statusLabel,
			CurrentActivity: currentActivity,
			TimeTodayMins:   timeTodayMins,
			TimeTodayStr:    timeTodayStr,
			LastActiveAt:    lastActive,
		})
	}

	return users, nil
}

// fetchGroupUsersWithActivity gets users in the specified groups with their activity status.
// targetRoles specifies which roles to fetch (e.g., ["member"] or ["member", "leader"]).
func (h *Handler) fetchGroupUsersWithActivity(ctx context.Context, db *mongo.Database, groupIDs []primitive.ObjectID, groupMap map[primitive.ObjectID]string, targetRoles []string, now time.Time) ([]memberRow, error) {
	if len(groupIDs) == 0 || len(targetRoles) == 0 {
		return nil, nil
	}

	// Get user IDs and their group associations for the target roles
	pipeline := []bson.M{
		{"$match": bson.M{"group_id": bson.M{"$in": groupIDs}, "role": bson.M{"$in": targetRoles}}},
		{"$lookup": bson.M{
			"from":         "users",
			"localField":   "user_id",
			"foreignField": "_id",
			"as":           "user",
		}},
		{"$unwind": "$user"},
		{"$match": bson.M{"user.status": "active"}},
		{"$project": bson.M{
			"user_id":         "$user._id",
			"name":            "$user.full_name",
			"login_id":        "$user.login_id",
			"email":           "$user.email",
			"organization_id": "$user.organization_id",
			"group_id":        1,
			"membership_role": "$role", // The role in the group (member or leader)
		}},
	}

	cur, err := db.Collection("group_memberships").Aggregate(ctx, pipeline)
	if err != nil {
		return nil, err
	}
	defer cur.Close(ctx)

	// Collect unique users (a user might be in multiple groups with different roles)
	type userInfo struct {
		UserID   primitive.ObjectID
		Name     string
		LoginID  string
		Email    string
		OrgID    primitive.ObjectID
		GroupIDs []primitive.ObjectID
		Role     string // Highest role: leader > member
	}
	userMap := make(map[primitive.ObjectID]*userInfo)

	for cur.Next(ctx) {
		var doc struct {
			UserID         primitive.ObjectID `bson:"user_id"`
			Name           string             `bson:"name"`
			LoginID        string             `bson:"login_id"`
			Email          string             `bson:"email"`
			OrgID          primitive.ObjectID `bson:"organization_id"`
			GroupID        primitive.ObjectID `bson:"group_id"`
			MembershipRole string             `bson:"membership_role"`
		}
		if err := cur.Decode(&doc); err != nil {
			return nil, err
		}

		if u, ok := userMap[doc.UserID]; ok {
			u.GroupIDs = append(u.GroupIDs, doc.GroupID)
			// Keep highest role (leader > member)
			if doc.MembershipRole == "leader" {
				u.Role = "leader"
			}
		} else {
			userMap[doc.UserID] = &userInfo{
				UserID:   doc.UserID,
				Name:     doc.Name,
				LoginID:  doc.LoginID,
				Email:    doc.Email,
				OrgID:    doc.OrgID,
				GroupIDs: []primitive.ObjectID{doc.GroupID},
				Role:     doc.MembershipRole,
			}
		}
	}

	if len(userMap) == 0 {
		return nil, nil
	}

	// Get user IDs for session lookup
	var userIDs []primitive.ObjectID
	for uid := range userMap {
		userIDs = append(userIDs, uid)
	}

	// Collect unique organization IDs and fetch org names
	orgIDs := make(map[primitive.ObjectID]bool)
	for _, u := range userMap {
		if !u.OrgID.IsZero() {
			orgIDs[u.OrgID] = true
		}
	}
	orgNames := make(map[primitive.ObjectID]string)
	if len(orgIDs) > 0 {
		var orgIDList []primitive.ObjectID
		for oid := range orgIDs {
			orgIDList = append(orgIDList, oid)
		}
		orgCur, err := db.Collection("organizations").Find(ctx, bson.M{"_id": bson.M{"$in": orgIDList}})
		if err == nil {
			defer orgCur.Close(ctx)
			for orgCur.Next(ctx) {
				var org struct {
					ID   primitive.ObjectID `bson:"_id"`
					Name string             `bson:"name"`
				}
				if orgCur.Decode(&org) == nil {
					orgNames[org.ID] = org.Name
				}
			}
		}
	}

	// Get active sessions for these users
	sessionMap, err := h.getActiveSessionsForUsers(ctx, userIDs, now)
	if err != nil {
		return nil, err
	}

	// Get today's activity for these users
	todayStart := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
	todayActivity, err := h.getTodayActivityForUsers(ctx, userIDs, todayStart, now)
	if err != nil {
		return nil, err
	}

	// Get current resource launches
	currentResources, err := h.getCurrentResourcesForUsers(ctx, userIDs)
	if err != nil {
		return nil, err
	}

	// Build user rows
	var users []memberRow
	for _, u := range userMap {
		// Determine group name (use first group for display)
		groupName := ""
		if len(u.GroupIDs) > 0 {
			groupName = groupMap[u.GroupIDs[0]]
			if len(u.GroupIDs) > 1 {
				groupName += " (+)"
			}
		}

		// Get organization name
		orgName := ""
		if !u.OrgID.IsZero() {
			orgName = orgNames[u.OrgID]
		}

		// Determine status
		status := StatusOffline
		statusLabel := "Offline"
		var lastActive *time.Time

		if sess, ok := sessionMap[u.UserID]; ok {
			lastActive = &sess.LastActiveAt
			timeSince := now.Sub(sess.LastActiveAt)
			if timeSince < OnlineThreshold {
				status = StatusOnline
				statusLabel = "Active"
			} else if timeSince < IdleThreshold {
				status = StatusIdle
				statusLabel = "Idle"
			}
		}

		// Get current activity (only for online/idle users)
		currentActivity := ""
		if status != StatusOffline {
			sess, hasSession := sessionMap[u.UserID]
			currentPage := ""
			if hasSession {
				currentPage = sess.CurrentPage
			}

			// Only show resource activity if user is still on a resource page
			if res, ok := currentResources[u.UserID]; ok && strings.HasPrefix(currentPage, "/member/resources/") {
				currentActivity = res
			} else if currentPage != "" {
				currentActivity = formatPageName(currentPage)
			} else {
				currentActivity = "Dashboard"
			}
		}

		// Get time today
		timeTodayMins := 0
		if mins, ok := todayActivity[u.UserID]; ok {
			timeTodayMins = mins
		}

		// Format time today
		timeTodayStr := "0 min"
		if timeTodayMins > 0 {
			if timeTodayMins >= 60 {
				timeTodayStr = fmt.Sprintf("%dh %dm", timeTodayMins/60, timeTodayMins%60)
			} else {
				timeTodayStr = fmt.Sprintf("%d min", timeTodayMins)
			}
		}

		users = append(users, memberRow{
			ID:              u.UserID.Hex(),
			Name:            u.Name,
			LoginID:         u.LoginID,
			Email:           u.Email,
			OrgName:         orgName,
			GroupName:       groupName,
			Role:            u.Role,
			Status:          status,
			StatusLabel:     statusLabel,
			CurrentActivity: currentActivity,
			TimeTodayMins:   timeTodayMins,
			TimeTodayStr:    timeTodayStr,
			LastActiveAt:    lastActive,
		})
	}

	// Return unsorted - caller will sort as needed
	return users, nil
}

// sessionInfo holds minimal session data for status calculation.
type sessionInfo struct {
	LastActiveAt time.Time
	CurrentPage  string
}

// getActiveSessionsForUsers gets the most recent active session for each user.
func (h *Handler) getActiveSessionsForUsers(ctx context.Context, userIDs []primitive.ObjectID, now time.Time) (map[primitive.ObjectID]sessionInfo, error) {
	if len(userIDs) == 0 {
		return nil, nil
	}

	// Find sessions that are either:
	// 1. Not logged out (logout_at is nil)
	// 2. Recently logged out (within idle threshold) - for showing "just went offline"
	pipeline := []bson.M{
		{"$match": bson.M{
			"user_id":   bson.M{"$in": userIDs},
			"logout_at": nil,
		}},
		{"$sort": bson.M{"last_activity": -1}},
		{"$group": bson.M{
			"_id":           "$user_id",
			"last_activity": bson.M{"$first": "$last_activity"},
			"current_page":  bson.M{"$first": "$current_page"},
		}},
	}

	cur, err := h.DB.Collection("sessions").Aggregate(ctx, pipeline)
	if err != nil {
		return nil, err
	}
	defer cur.Close(ctx)

	result := make(map[primitive.ObjectID]sessionInfo)
	for cur.Next(ctx) {
		var doc struct {
			ID           primitive.ObjectID `bson:"_id"`
			LastActivity time.Time          `bson:"last_activity"`
			CurrentPage  string             `bson:"current_page"`
		}
		if err := cur.Decode(&doc); err != nil {
			return nil, err
		}
		result[doc.ID] = sessionInfo{LastActiveAt: doc.LastActivity, CurrentPage: doc.CurrentPage}
	}

	return result, nil
}

// getTodayActivityForUsers calculates total active minutes for each user today.
func (h *Handler) getTodayActivityForUsers(ctx context.Context, userIDs []primitive.ObjectID, todayStart, now time.Time) (map[primitive.ObjectID]int, error) {
	if len(userIDs) == 0 {
		return nil, nil
	}

	// Sum session durations for today
	pipeline := []bson.M{
		{"$match": bson.M{
			"user_id":  bson.M{"$in": userIDs},
			"login_at": bson.M{"$gte": todayStart},
		}},
		{"$project": bson.M{
			"user_id": 1,
			"duration_mins": bson.M{
				"$cond": bson.M{
					"if": bson.M{"$ne": bson.A{"$logout_at", nil}},
					"then": bson.M{"$divide": bson.A{
						bson.M{"$subtract": bson.A{"$logout_at", "$login_at"}},
						60000, // ms to minutes
					}},
					"else": bson.M{"$divide": bson.A{
						bson.M{"$subtract": bson.A{now, "$login_at"}},
						60000,
					}},
				},
			},
		}},
		{"$group": bson.M{
			"_id":        "$user_id",
			"total_mins": bson.M{"$sum": "$duration_mins"},
		}},
	}

	cur, err := h.DB.Collection("sessions").Aggregate(ctx, pipeline)
	if err != nil {
		return nil, err
	}
	defer cur.Close(ctx)

	result := make(map[primitive.ObjectID]int)
	for cur.Next(ctx) {
		var doc struct {
			ID        primitive.ObjectID `bson:"_id"`
			TotalMins float64            `bson:"total_mins"`
		}
		if err := cur.Decode(&doc); err != nil {
			return nil, err
		}
		result[doc.ID] = int(doc.TotalMins)
	}

	return result, nil
}

// getCurrentResourcesForUsers gets the most recent resource activity for users.
// Returns formatted activity strings like "Viewing [name]" or "[name]" based on event type.
func (h *Handler) getCurrentResourcesForUsers(ctx context.Context, userIDs []primitive.ObjectID) (map[primitive.ObjectID]string, error) {
	if len(userIDs) == 0 || h.Activity == nil {
		return nil, nil
	}

	// Get the most recent resource event for each user
	// Include view and launch events
	pipeline := []bson.M{
		{"$match": bson.M{
			"user_id":    bson.M{"$in": userIDs},
			"event_type": bson.M{"$in": bson.A{"resource_launch", "resource_view"}},
			"timestamp":  bson.M{"$gte": time.Now().UTC().Add(-1 * time.Hour)}, // Within last hour
		}},
		{"$sort": bson.M{"timestamp": -1}},
		{"$group": bson.M{
			"_id":           "$user_id",
			"event_type":    bson.M{"$first": "$event_type"},
			"resource_name": bson.M{"$first": "$resource_name"},
		}},
	}

	cur, err := h.DB.Collection("activity_events").Aggregate(ctx, pipeline)
	if err != nil {
		return nil, err
	}
	defer cur.Close(ctx)

	result := make(map[primitive.ObjectID]string)
	for cur.Next(ctx) {
		var doc struct {
			ID           primitive.ObjectID `bson:"_id"`
			EventType    string             `bson:"event_type"`
			ResourceName string             `bson:"resource_name"`
		}
		if err := cur.Decode(&doc); err != nil {
			return nil, err
		}

		// Format based on event type
		switch doc.EventType {
		case "resource_view":
			result[doc.ID] = "Viewing " + doc.ResourceName
		case "resource_launch":
			result[doc.ID] = "Open " + doc.ResourceName
		}
	}

	return result, nil
}

// filterMembersBySearch filters members by name (case-insensitive prefix match).
func filterMembersBySearch(members []memberRow, searchQuery string) []memberRow {
	if searchQuery == "" {
		return members
	}

	query := strings.ToLower(searchQuery)
	var filtered []memberRow
	for _, m := range members {
		if strings.HasPrefix(strings.ToLower(m.Name), query) {
			filtered = append(filtered, m)
		}
	}
	return filtered
}

// filterMembersByStatus filters members by their online status.
func filterMembersByStatus(members []memberRow, statusFilter string) []memberRow {
	if statusFilter == "all" || statusFilter == "" {
		return members
	}

	var filtered []memberRow
	for _, m := range members {
		switch statusFilter {
		case "online":
			if m.Status == StatusOnline {
				filtered = append(filtered, m)
			}
		case "idle":
			if m.Status == StatusIdle {
				filtered = append(filtered, m)
			}
		case "offline":
			if m.Status == StatusOffline {
				filtered = append(filtered, m)
			}
		}
	}
	return filtered
}

// sortMembers sorts members by the specified field and direction.
func sortMembers(members []memberRow, sortBy, sortDir string) {
	sort.Slice(members, func(i, j int) bool {
		var less bool
		switch sortBy {
		case "group":
			// Sort by group name, then by name within group
			if members[i].GroupName != members[j].GroupName {
				less = strings.ToLower(members[i].GroupName) < strings.ToLower(members[j].GroupName)
			} else {
				less = strings.ToLower(members[i].Name) < strings.ToLower(members[j].Name)
			}
		case "time":
			// Sort by time today (default descending - longest first)
			if members[i].TimeTodayMins != members[j].TimeTodayMins {
				less = members[i].TimeTodayMins > members[j].TimeTodayMins // Note: reversed for desc default
			} else {
				less = strings.ToLower(members[i].Name) < strings.ToLower(members[j].Name)
			}
			// For time, the default is descending, so we flip the logic
			if sortDir == "asc" {
				return !less
			}
			return less
		default: // "name"
			less = strings.ToLower(members[i].Name) < strings.ToLower(members[j].Name)
		}

		// Apply direction (except for time which handles it specially)
		if sortBy != "time" && sortDir == "desc" {
			return !less
		}
		return less
	})
}

// filterMembersByRole filters members by their role.
func filterMembersByRole(members []memberRow, roleFilter string) []memberRow {
	if roleFilter == "all" || roleFilter == "" {
		return members
	}

	var filtered []memberRow
	for _, m := range members {
		if m.Role == roleFilter {
			filtered = append(filtered, m)
		}
	}
	return filtered
}

// getAvailableRoles returns the roles that a viewer can see based on their role.
func getAvailableRoles(viewerRole string) []string {
	switch viewerRole {
	case "superadmin", "admin":
		return []string{"all", "member", "leader", "coordinator", "admin", "superadmin"}
	case "coordinator":
		return []string{"all", "member", "leader"}
	case "leader":
		return []string{"member"} // Only one role, no filter shown
	default:
		return nil
	}
}

// buildRoleOptions builds the role filter options for the dropdown.
func buildRoleOptions(availableRoles []string, selectedRole string) []roleOption {
	labels := map[string]string{
		"all":        "All Roles",
		"member":     "Members",
		"leader":     "Leaders",
		"coordinator": "Coordinators",
		"admin":      "Admins",
		"superadmin": "Super Admins",
	}

	var options []roleOption
	for _, role := range availableRoles {
		options = append(options, roleOption{
			Value:    role,
			Label:    labels[role],
			Selected: role == selectedRole || (selectedRole == "" && role == "all"),
		})
	}
	return options
}

// formatPageName converts a URL path to a readable page name.
func formatPageName(path string) string {
	// Map of known paths to display names
	pageNames := map[string]string{
		"/":                  "Dashboard",
		"/dashboard":         "Dashboard",
		"/resources":         "Resources",
		"/member/resources":  "Resources",
		"/member/materials":  "Materials",
		"/materials":         "Materials",
		"/profile":           "Profile",
		"/settings":          "Settings",
		"/about":             "About",
		"/contact":           "Contact",
		"/terms":             "Terms",
		"/privacy":           "Privacy",
		"/activity":          "Activity",
		"/activity/summary":  "Activity Summary",
	}

	if name, ok := pageNames[path]; ok {
		return name
	}

	// Check for prefix matches (for paths with dynamic segments like /member/resources/123)
	prefixes := map[string]string{
		"/member/resources/": "Resources",
		"/member/materials/": "Materials",
		"/activity/member/":  "Activity",
		"/activity/export":   "Activity Export",
	}
	for prefix, name := range prefixes {
		if strings.HasPrefix(path, prefix) {
			return name
		}
	}

	// For unknown paths, try to make them readable
	// Remove leading slash and get the last meaningful segment
	if len(path) > 1 {
		path = path[1:] // Remove leading /
		// For paths like "member/resources", use the last segment
		if idx := strings.LastIndex(path, "/"); idx > 0 {
			path = path[idx+1:]
		}
		if len(path) > 0 {
			return strings.ToUpper(path[:1]) + path[1:]
		}
	}

	return "Dashboard"
}
