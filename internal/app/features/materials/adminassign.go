// internal/app/features/materials/adminassign.go
package materials

import (
	"context"
	"html/template"
	"maps"
	"net/http"
	"strings"
	"time"

	uierrors "github.com/dalemusser/stratahub/internal/app/features/errors"
	materialassignstore "github.com/dalemusser/stratahub/internal/app/store/materialassign"
	materialstore "github.com/dalemusser/stratahub/internal/app/store/materials"
	userstore "github.com/dalemusser/stratahub/internal/app/store/users"
	"github.com/dalemusser/stratahub/internal/app/system/authz"
	"github.com/dalemusser/stratahub/internal/app/system/viewdata"
	"github.com/dalemusser/stratahub/internal/app/system/orgutil"
	"github.com/dalemusser/stratahub/internal/app/system/paging"
	"github.com/dalemusser/stratahub/internal/app/system/timeouts"
	"github.com/dalemusser/stratahub/internal/domain/models"
	wafflemongo "github.com/dalemusser/waffle/pantry/mongo"
	"github.com/dalemusser/waffle/pantry/query"
	"github.com/dalemusser/waffle/pantry/text"
	"github.com/dalemusser/waffle/pantry/httpnav"
	"github.com/dalemusser/waffle/pantry/templates"
	"github.com/go-chi/chi/v5"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"go.uber.org/zap"
)

// ServeAssign renders the two-pane assignment picker page.
// Left pane: searchable list of organizations (with leader counts)
// Right pane: leaders in selected org, with "All" option above
func (h *AdminHandler) ServeAssign(w http.ResponseWriter, r *http.Request) {
	_, _, userID, _ := authz.UserCtx(r)
	_ = userID // reserved for audit

	ctx, cancel := context.WithTimeout(r.Context(), timeouts.Medium())
	defer cancel()

	// Get material ID from URL
	idStr := chi.URLParam(r, "id")
	matID, err := primitive.ObjectIDFromHex(idStr)
	if err != nil {
		uierrors.RenderNotFound(w, r, "Material not found.", "/materials")
		return
	}

	// Fetch material
	matStore := materialstore.New(h.DB)
	mat, err := matStore.GetByID(ctx, matID)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			uierrors.RenderNotFound(w, r, "Material not found.", "/materials")
			return
		}
		h.Log.Error("error fetching material", zap.Error(err))
		uierrors.RenderServerError(w, r, "Failed to load material.", "/materials")
		return
	}

	// Determine coordinator scope (if coordinator, limit to assigned orgs)
	var scopeOrgIDs []primitive.ObjectID
	if authz.IsCoordinator(r) {
		scopeOrgIDs = authz.UserOrgIDs(r)
	}

	// Parse query params for org pane
	orgQ := query.Search(r, "org_search")
	orgAfter := query.Get(r, "org_after")
	orgBefore := query.Get(r, "org_before")
	selectedOrg := query.Get(r, "org")

	// Parse target param (org or leader:ID)
	target := query.Get(r, "target")

	// Fetch org pane data
	orgData, err := h.fetchOrgPaneForAssign(ctx, orgQ, orgAfter, orgBefore, scopeOrgIDs)
	if err != nil {
		h.Log.Error("error fetching org pane", zap.Error(err))
		uierrors.RenderServerError(w, r, "Failed to load organizations.", "/materials")
		return
	}

	// Build view model
	data := assignData{
		BaseVM:        viewdata.NewBaseVM(r, h.DB, "Assign Material", "/materials"),
		MaterialID:    mat.ID.Hex(),
		MaterialTitle: mat.Title,

		// Org pane
		OrgRows:     make([]orgPaneRow, 0, len(orgData.Rows)),
		SelectedOrg: selectedOrg,
		OrgSearch:   orgQ,
		OrgShown:    len(orgData.Rows),
		OrgTotal:    orgData.Total,
		OrgHasPrev:  orgData.HasPrev,
		OrgHasNext:  orgData.HasNext,
		OrgPrevCur:  orgData.PrevCursor,
		OrgNextCur:  orgData.NextCursor,
	}

	// Convert org rows
	for _, o := range orgData.Rows {
		data.OrgRows = append(data.OrgRows, orgPaneRow{
			ID:          o.ID,
			Name:        o.Name,
			NameCI:      text.Fold(o.Name),
			LeaderCount: o.Count,
			Selected:    o.ID.Hex() == selectedOrg,
		})
	}

	// If an org is selected, fetch org name and leaders for right pane
	if selectedOrg != "" {
		orgID, err := primitive.ObjectIDFromHex(selectedOrg)
		if err == nil {
			// Coordinator access check: verify access to selected organization
			if authz.IsCoordinator(r) && !authz.CanAccessOrg(r, orgID) {
				uierrors.RenderForbidden(w, r, "You don't have access to this organization.", "/materials")
				return
			}

			// Get org name
			var org struct {
				Name string `bson:"name"`
			}
			err = h.DB.Collection("organizations").FindOne(ctx, bson.M{"_id": orgID}).Decode(&org)
			if err == nil {
				data.SelectedOrgName = org.Name
			}

			// By default when org is selected, "All" is selected
			// unless a specific leader is selected
			if target == "" || target == "org" {
				data.SelectedAll = true
			} else if strings.HasPrefix(target, "leader:") {
				// Parse leader ID
				leaderIDStr := strings.TrimPrefix(target, "leader:")
				leaderID, err := primitive.ObjectIDFromHex(leaderIDStr)
				if err == nil {
					data.SelectedLeaderID = leaderIDStr
					// Fetch leader name
					usrStore := userstore.New(h.DB)
					leader, err := usrStore.GetByID(ctx, leaderID)
					if err == nil {
						data.SelectedLeaderName = leader.FullName
					}
				}
			}

			// Fetch leaders
			leaderSearch := query.Search(r, "leader_search")
			leaderAfter := query.Get(r, "leader_after")
			leaderBefore := query.Get(r, "leader_before")

			leaderData, err := h.fetchLeaderPaneForAssign(ctx, orgID, leaderSearch, leaderAfter, leaderBefore)
			if err != nil {
				h.Log.Error("error fetching leaders", zap.Error(err))
				// Continue without leader data
			} else {
				data.LeaderRows = leaderData.Rows
				data.LeaderSearch = leaderSearch
				data.LeaderShown = leaderData.Shown
				data.LeaderTotal = leaderData.Total
				data.LeaderHasPrev = leaderData.HasPrev
				data.LeaderHasNext = leaderData.HasNext
				data.LeaderPrevCur = leaderData.PrevCursor
				data.LeaderNextCur = leaderData.NextCursor

				// Mark selected leader
				for i := range data.LeaderRows {
					if data.LeaderRows[i].ID.Hex() == data.SelectedLeaderID {
						data.LeaderRows[i].Selected = true
					}
				}
			}
		}
	}

	templates.RenderAutoMap(w, r, "admin_materials_assign", nil, data)
}

// ServeAssignLeadersPane renders the leaders pane for a selected organization.
// This is an HTMX partial loaded when an org is selected.
func (h *AdminHandler) ServeAssignLeadersPane(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), timeouts.Medium())
	defer cancel()

	// Get material ID from URL
	idStr := chi.URLParam(r, "id")
	matID, err := primitive.ObjectIDFromHex(idStr)
	if err != nil {
		http.Error(w, "Invalid material ID", http.StatusBadRequest)
		return
	}

	// Get org ID from query
	orgIDStr := query.Get(r, "org")
	if orgIDStr == "" {
		http.Error(w, "Organization required", http.StatusBadRequest)
		return
	}
	orgID, err := primitive.ObjectIDFromHex(orgIDStr)
	if err != nil {
		http.Error(w, "Invalid organization ID", http.StatusBadRequest)
		return
	}

	// Coordinator access check: verify access to selected organization
	if authz.IsCoordinator(r) && !authz.CanAccessOrg(r, orgID) {
		http.Error(w, "You don't have access to this organization", http.StatusForbidden)
		return
	}

	// Fetch org name
	orgName := ""
	var org struct {
		Name string `bson:"name"`
	}
	err = h.DB.Collection("organizations").FindOne(ctx, bson.M{"_id": orgID}).Decode(&org)
	if err == nil {
		orgName = org.Name
	}

	// Parse leader query params
	leaderSearch := query.Search(r, "leader_search")
	leaderAfter := query.Get(r, "after")
	leaderBefore := query.Get(r, "before")

	// Fetch leaders
	leaderData, err := h.fetchLeaderPaneForAssign(ctx, orgID, leaderSearch, leaderAfter, leaderBefore)
	if err != nil {
		h.Log.Error("error fetching leaders", zap.Error(err))
		http.Error(w, "Failed to load leaders", http.StatusInternalServerError)
		return
	}

	data := leadersPaneData{
		MaterialID:    matID.Hex(),
		SelectedOrg:   orgIDStr,
		OrgName:       orgName,
		LeaderRows:    leaderData.Rows,
		SelectedAll:   true, // Default to All selected
		LeaderSearch:  leaderSearch,
		LeaderShown:   leaderData.Shown,
		LeaderTotal:   leaderData.Total,
		LeaderHasPrev: leaderData.HasPrev,
		LeaderHasNext: leaderData.HasNext,
		LeaderPrevCur: leaderData.PrevCursor,
		LeaderNextCur: leaderData.NextCursor,
	}

	templates.RenderAutoMap(w, r, "admin_materials_assign_leaders_pane", nil, data)
}

// ServeAssignForm renders the assignment form page (step 2).
func (h *AdminHandler) ServeAssignForm(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), timeouts.Medium())
	defer cancel()

	// Get material ID from URL
	idStr := chi.URLParam(r, "id")
	matID, err := primitive.ObjectIDFromHex(idStr)
	if err != nil {
		uierrors.RenderNotFound(w, r, "Material not found.", "/materials")
		return
	}

	// Fetch material
	matStore := materialstore.New(h.DB)
	mat, err := matStore.GetByID(ctx, matID)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			uierrors.RenderNotFound(w, r, "Material not found.", "/materials")
			return
		}
		h.Log.Error("error fetching material", zap.Error(err))
		uierrors.RenderServerError(w, r, "Failed to load material.", "/materials")
		return
	}

	// Get org and optional leader from query
	orgIDStr := query.Get(r, "org")
	leaderIDStr := query.Get(r, "leader")

	if orgIDStr == "" {
		uierrors.RenderBadRequest(w, r, "Organization is required.", "/materials/"+matID.Hex()+"/assign")
		return
	}

	orgID, err := primitive.ObjectIDFromHex(orgIDStr)
	if err != nil {
		uierrors.RenderBadRequest(w, r, "Invalid organization.", "/materials/"+matID.Hex()+"/assign")
		return
	}

	// Coordinator access check: verify access to selected organization
	if authz.IsCoordinator(r) && !authz.CanAccessOrg(r, orgID) {
		uierrors.RenderForbidden(w, r, "You don't have access to this organization.", "/materials")
		return
	}

	// Fetch org name
	var org struct {
		Name string `bson:"name"`
	}
	err = h.DB.Collection("organizations").FindOne(ctx, bson.M{"_id": orgID}).Decode(&org)
	if err != nil {
		uierrors.RenderNotFound(w, r, "Organization not found.", "/materials/"+matID.Hex()+"/assign")
		return
	}

	// Resolve org timezone for date interpretation
	loc, tzLabel := orgutil.ResolveOrgLocation(ctx, h.DB, orgID)

	// Default back URL
	backURL := "/materials/" + matID.Hex() + "/assign?org=" + orgIDStr

	// Build view model
	data := assignFormData{
		BaseVM:        viewdata.NewBaseVM(r, h.DB, "Complete Material Assignment", backURL),
		MaterialID:    matID.Hex(),
		MaterialTitle: mat.Title,
		HasFile:       mat.HasFile(),
		LaunchURL:     mat.LaunchURL,
		OrgID:         orgIDStr,
		Directions:    mat.DefaultInstructions,
		VisibleFrom:   time.Now().In(loc).Format("2006-01-02T15:04"), // Default to now in org timezone
		TimeZone:      tzLabel,
	}

	if leaderIDStr != "" {
		// Specific leader assignment
		leaderID, err := primitive.ObjectIDFromHex(leaderIDStr)
		if err != nil {
			uierrors.RenderBadRequest(w, r, "Invalid leader.", "/materials/"+matID.Hex()+"/assign")
			return
		}

		usrStore := userstore.New(h.DB)
		leader, err := usrStore.GetByID(ctx, leaderID)
		if err != nil {
			uierrors.RenderNotFound(w, r, "Leader not found.", "/materials/"+matID.Hex()+"/assign")
			return
		}

		data.LeaderID = leaderIDStr
		data.TargetName = leader.FullName
		data.IsOrgWide = false
		data.BaseVM.BackURL = "/materials/" + matID.Hex() + "/assign?org=" + orgIDStr + "&target=leader:" + leaderIDStr
	} else {
		// Organization-wide assignment
		data.TargetName = org.Name
		data.IsOrgWide = true
		data.BaseVM.BackURL = "/materials/" + matID.Hex() + "/assign?org=" + orgIDStr + "&target=org"
	}

	templates.RenderAutoMap(w, r, "admin_materials_assign_form", nil, data)
}

// HandleAssign processes the assignment form POST.
func (h *AdminHandler) HandleAssign(w http.ResponseWriter, r *http.Request) {
	_, userName, userID, _ := authz.UserCtx(r)

	ctx, cancel := context.WithTimeout(r.Context(), timeouts.Medium())
	defer cancel()

	// Get material ID from URL
	idStr := chi.URLParam(r, "id")
	matID, err := primitive.ObjectIDFromHex(idStr)
	if err != nil {
		uierrors.RenderNotFound(w, r, "Material not found.", "/materials")
		return
	}

	// Verify material exists
	matStore := materialstore.New(h.DB)
	_, err = matStore.GetByID(ctx, matID)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			uierrors.RenderNotFound(w, r, "Material not found.", "/materials")
			return
		}
		h.Log.Error("error fetching material", zap.Error(err))
		uierrors.RenderServerError(w, r, "Failed to load material.", "/materials")
		return
	}

	// Parse form
	if err := r.ParseForm(); err != nil {
		h.Log.Error("error parsing form", zap.Error(err))
		uierrors.RenderBadRequest(w, r, "Invalid form data.", "/materials/"+matID.Hex()+"/assign")
		return
	}

	// Get assignment target
	orgIDStr := strings.TrimSpace(r.FormValue("org"))
	leaderIDStr := strings.TrimSpace(r.FormValue("leader"))

	if orgIDStr == "" {
		uierrors.RenderBadRequest(w, r, "Organization is required.", "/materials/"+matID.Hex()+"/assign")
		return
	}

	// Validate org
	var orgID *primitive.ObjectID
	var leaderID *primitive.ObjectID

	oid, err := primitive.ObjectIDFromHex(orgIDStr)
	if err != nil {
		uierrors.RenderBadRequest(w, r, "Invalid organization.", "/materials/"+matID.Hex()+"/assign")
		return
	}

	// Coordinator access check: verify access to selected organization
	if authz.IsCoordinator(r) && !authz.CanAccessOrg(r, oid) {
		uierrors.RenderForbidden(w, r, "You don't have access to this organization.", "/materials")
		return
	}

	if leaderIDStr != "" {
		// Leader assignment
		lid, err := primitive.ObjectIDFromHex(leaderIDStr)
		if err != nil {
			uierrors.RenderBadRequest(w, r, "Invalid leader.", "/materials/"+matID.Hex()+"/assign")
			return
		}
		leaderID = &lid
	} else {
		// Organization-wide assignment
		orgID = &oid
	}

	// Resolve the organization's timezone so we can interpret the submitted
	// dates in the organization's local timezone.
	loc, _ := orgutil.ResolveOrgLocation(ctx, h.DB, oid)

	// Parse visibility dates (datetime-local format: 2006-01-02T15:04)
	var visibleFrom, visibleUntil *time.Time
	if vf := strings.TrimSpace(r.FormValue("visible_from")); vf != "" {
		if t, err := time.ParseInLocation("2006-01-02T15:04", vf, loc); err == nil {
			visibleFrom = &t
		}
	}
	if vu := strings.TrimSpace(r.FormValue("visible_until")); vu != "" {
		if t, err := time.ParseInLocation("2006-01-02T15:04", vu, loc); err == nil {
			visibleUntil = &t
		}
	}

	// Get directions
	directions := strings.TrimSpace(r.FormValue("directions"))

	// Create assignment
	assignment := models.MaterialAssignment{
		ID:             primitive.NewObjectID(),
		MaterialID:     matID,
		OrganizationID: orgID,
		LeaderID:       leaderID,
		VisibleFrom:    visibleFrom,
		VisibleUntil:   visibleUntil,
		Directions:     directions,
		CreatedAt:      time.Now().UTC(),
		CreatedByID:    &userID,
		CreatedByName:  userName,
	}

	assignStore := materialassignstore.New(h.DB)
	_, err = assignStore.Create(ctx, assignment)
	if err != nil {
		h.Log.Error("error creating assignment", zap.Error(err))
		uierrors.RenderServerError(w, r, "Failed to create assignment.", "/materials/"+matID.Hex()+"/assign")
		return
	}

	// Redirect to assignments list
	http.Redirect(w, r, "/materials/"+matID.Hex()+"/assignments", http.StatusSeeOther)
}

// ServeAssignmentList renders the list of assignments for a material.
func (h *AdminHandler) ServeAssignmentList(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), timeouts.Medium())
	defer cancel()

	// Get material ID from URL
	idStr := chi.URLParam(r, "id")
	matID, err := primitive.ObjectIDFromHex(idStr)
	if err != nil {
		uierrors.RenderNotFound(w, r, "Material not found.", "/materials")
		return
	}

	// Fetch material
	matStore := materialstore.New(h.DB)
	mat, err := matStore.GetByID(ctx, matID)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			uierrors.RenderNotFound(w, r, "Material not found.", "/materials")
			return
		}
		h.Log.Error("error fetching material", zap.Error(err))
		uierrors.RenderServerError(w, r, "Failed to load material.", "/materials")
		return
	}

	// Determine coordinator scope (if coordinator, limit to assigned orgs)
	var scopeOrgIDs []primitive.ObjectID
	isCoordinator := authz.IsCoordinator(r)
	if isCoordinator {
		scopeOrgIDs = authz.UserOrgIDs(r)
	}

	// Fetch assignments
	assignStore := materialassignstore.New(h.DB)
	assignments, err := assignStore.ListByMaterial(ctx, matID)
	if err != nil {
		h.Log.Error("error fetching assignments", zap.Error(err))
		uierrors.RenderServerError(w, r, "Failed to load assignments.", "/materials")
		return
	}

	// Collect org and leader IDs for name lookup
	orgIDs := make([]primitive.ObjectID, 0)
	leaderIDs := make([]primitive.ObjectID, 0)
	for _, a := range assignments {
		if a.OrganizationID != nil {
			orgIDs = append(orgIDs, *a.OrganizationID)
		}
		if a.LeaderID != nil {
			leaderIDs = append(leaderIDs, *a.LeaderID)
		}
	}

	// Fetch org names
	orgNames := make(map[primitive.ObjectID]string)
	if len(orgIDs) > 0 {
		names, err := orgutil.FetchOrgNames(ctx, h.DB, orgIDs)
		if err == nil {
			orgNames = names
		}
	}

	// Fetch leader info (name and org)
	leaderNames := make(map[primitive.ObjectID]string)
	leaderOrgs := make(map[primitive.ObjectID]primitive.ObjectID) // leader ID -> org ID
	if len(leaderIDs) > 0 {
		usrStore := userstore.New(h.DB)
		users, err := usrStore.Find(ctx, bson.M{"_id": bson.M{"$in": leaderIDs}})
		if err == nil {
			for _, u := range users {
				leaderNames[u.ID] = u.FullName
				if u.OrganizationID != nil {
					leaderOrgs[u.ID] = *u.OrganizationID
				}
			}
		}
	}

	// Build scope org set for quick lookup (coordinators only)
	scopeOrgSet := make(map[primitive.ObjectID]bool)
	for _, oid := range scopeOrgIDs {
		scopeOrgSet[oid] = true
	}

	// Build list items, filtering by coordinator scope if applicable
	items := make([]assignmentListItem, 0, len(assignments))
	for _, a := range assignments {
		// For coordinators, filter to only accessible assignments
		if isCoordinator {
			if a.OrganizationID != nil {
				// Org-wide assignment: check if org is in coordinator's scope
				if !scopeOrgSet[*a.OrganizationID] {
					continue
				}
			} else if a.LeaderID != nil {
				// Leader assignment: check if leader's org is in coordinator's scope
				leaderOrgID, ok := leaderOrgs[*a.LeaderID]
				if !ok || !scopeOrgSet[leaderOrgID] {
					continue
				}
			}
		}

		item := assignmentListItem{
			ID:        a.ID.Hex(),
			CreatedAt: a.CreatedAt.Format("Jan 2, 2006"),
		}

		if a.OrganizationID != nil {
			item.TargetType = "organization"
			item.TargetName = orgNames[*a.OrganizationID]
			if item.TargetName == "" {
				item.TargetName = "(unknown org)"
			}
		} else if a.LeaderID != nil {
			item.TargetType = "leader"
			item.TargetName = leaderNames[*a.LeaderID]
			if item.TargetName == "" {
				item.TargetName = "(unknown leader)"
			}
		}

		if a.VisibleFrom != nil {
			item.VisibleFrom = a.VisibleFrom.Format("Jan 2, 2006")
		}
		if a.VisibleUntil != nil {
			item.VisibleUntil = a.VisibleUntil.Format("Jan 2, 2006")
		}

		items = append(items, item)
	}

	data := assignmentListData{
		BaseVM:        viewdata.NewBaseVM(r, h.DB, "Assignments", "/materials/"+matID.Hex()+"/view"),
		MaterialID:    matID.Hex(),
		MaterialTitle: mat.Title,
		Items:         items,
		Shown:         len(items),
	}

	templates.RenderAutoMap(w, r, "admin_materials_assignments", nil, data)
}

// HandleUnassign deletes an assignment.
func (h *AdminHandler) HandleUnassign(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), timeouts.Medium())
	defer cancel()

	// Get assignment ID from URL
	assignIDStr := chi.URLParam(r, "assignID")
	assignID, err := primitive.ObjectIDFromHex(assignIDStr)
	if err != nil {
		uierrors.RenderNotFound(w, r, "Assignment not found.", "/materials")
		return
	}

	// Fetch assignment to get material ID for redirect
	assignStore := materialassignstore.New(h.DB)
	assignment, err := assignStore.GetByID(ctx, assignID)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			uierrors.RenderNotFound(w, r, "Assignment not found.", "/materials")
			return
		}
		h.Log.Error("error fetching assignment", zap.Error(err))
		uierrors.RenderServerError(w, r, "Failed to load assignment.", "/materials")
		return
	}

	// Coordinator access check
	if authz.IsCoordinator(r) {
		if !h.canAccessAssignment(ctx, r, assignment) {
			uierrors.RenderForbidden(w, r, "You don't have access to this assignment.", "/materials")
			return
		}
	}

	// Delete assignment
	if err := assignStore.Delete(ctx, assignID); err != nil {
		h.Log.Error("error deleting assignment", zap.Error(err))
		uierrors.RenderServerError(w, r, "Failed to delete assignment.", "/materials")
		return
	}

	// Redirect back - use return param or fall back to material's assignments
	returnURL := r.FormValue("return")
	if returnURL == "" {
		returnURL = "/materials/" + assignment.MaterialID.Hex() + "/assignments"
	}
	http.Redirect(w, r, returnURL, http.StatusSeeOther)
}

// canAccessAssignment checks if the current user (coordinator) can access a specific assignment.
// For org-wide assignments, checks if org is in coordinator's scope.
// For leader assignments, checks if leader's org is in coordinator's scope.
func (h *AdminHandler) canAccessAssignment(ctx context.Context, r *http.Request, assignment models.MaterialAssignment) bool {
	scopeOrgIDs := authz.UserOrgIDs(r)
	scopeOrgSet := make(map[primitive.ObjectID]bool)
	for _, oid := range scopeOrgIDs {
		scopeOrgSet[oid] = true
	}

	if assignment.OrganizationID != nil {
		return scopeOrgSet[*assignment.OrganizationID]
	}

	if assignment.LeaderID != nil {
		usrStore := userstore.New(h.DB)
		leader, err := usrStore.GetByID(ctx, *assignment.LeaderID)
		if err != nil || leader.OrganizationID == nil {
			return false
		}
		return scopeOrgSet[*leader.OrganizationID]
	}

	return false
}

// Helper functions

// fetchOrgPaneForAssign fetches paginated organizations with leader counts.
// scopeOrgIDs limits results to specific orgs (for coordinators); nil means all orgs.
func (h *AdminHandler) fetchOrgPaneForAssign(
	ctx context.Context,
	orgQ, orgAfter, orgBefore string,
	scopeOrgIDs []primitive.ObjectID,
) (orgutil.OrgPaneData, error) {
	return orgutil.FetchOrgPane(ctx, h.DB, h.Log, "leader", orgQ, orgAfter, orgBefore, scopeOrgIDs)
}

// leaderPaneResult holds the result of fetching leaders for the assignment pane.
type leaderPaneResult struct {
	Rows       []leaderPaneRow
	Total      int64
	Shown      int
	HasPrev    bool
	HasNext    bool
	PrevCursor string
	NextCursor string
}

// fetchLeaderPaneForAssign fetches paginated leaders for a given organization.
func (h *AdminHandler) fetchLeaderPaneForAssign(
	ctx context.Context,
	orgID primitive.ObjectID,
	searchQuery, after, before string,
) (leaderPaneResult, error) {
	var result leaderPaneResult

	// Build base filter
	base := bson.M{
		"role":            "leader",
		"status":          "active",
		"organization_id": orgID,
	}

	// Add search condition
	var searchOr []bson.M
	if searchQuery != "" {
		s := text.Fold(searchQuery)
		hi := s + "\uffff"
		sEmail := strings.ToLower(searchQuery)
		hiEmail := sEmail + "\uffff"
		searchOr = []bson.M{
			{"full_name_ci": bson.M{"$gte": s, "$lt": hi}},
			{"email": bson.M{"$gte": sEmail, "$lt": hiEmail}},
		}
		base["$or"] = searchOr
	}

	// Count total
	usrStore := userstore.New(h.DB)
	total, err := usrStore.Count(ctx, base)
	if err != nil {
		return result, err
	}
	result.Total = total

	// Build pagination filter
	f := maps.Clone(base)
	find := options.Find()
	sortField := "full_name_ci"

	cfg := paging.ConfigureKeyset(before, after)
	cfg.ApplyToFind(find, sortField)

	if ks := cfg.KeysetWindow(sortField); ks != nil {
		if searchQuery != "" {
			f["$and"] = []bson.M{{"$or": searchOr}, ks}
			delete(f, "$or")
		} else {
			maps.Copy(f, ks)
		}
	}

	// Fetch leaders
	urows, err := usrStore.Find(ctx, f, find)
	if err != nil {
		return result, err
	}

	// Reverse if paging backwards
	if cfg.Direction == paging.Backward {
		paging.Reverse(urows)
	}

	// Apply pagination trimming
	page := paging.TrimPage(&urows, before, after)
	result.HasPrev = page.HasPrev
	result.HasNext = page.HasNext
	result.Shown = len(urows)

	// Build leader rows
	result.Rows = make([]leaderPaneRow, 0, len(urows))
	for _, u := range urows {
		loginID := ""
		if u.LoginID != nil {
			loginID = *u.LoginID
		}
		result.Rows = append(result.Rows, leaderPaneRow{
			ID:       u.ID,
			FullName: u.FullName,
			Email:    strings.ToLower(loginID),
		})
	}

	// Build cursors
	if len(urows) > 0 {
		result.PrevCursor = wafflemongo.EncodeCursor(urows[0].FullNameCI, urows[0].ID)
		result.NextCursor = wafflemongo.EncodeCursor(urows[len(urows)-1].FullNameCI, urows[len(urows)-1].ID)
	}

	return result, nil
}

// ========================= GLOBAL ASSIGNMENTS LIST =========================

// ServeAllAssignments renders a list of all material assignments.
func (h *AdminHandler) ServeAllAssignments(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), timeouts.Medium())
	defer cancel()

	// Determine coordinator scope (if coordinator, limit to assigned orgs)
	var scopeOrgIDs []primitive.ObjectID
	isCoordinator := authz.IsCoordinator(r)
	if isCoordinator {
		scopeOrgIDs = authz.UserOrgIDs(r)
	}

	// Fetch all assignments
	assignStore := materialassignstore.New(h.DB)
	assignments, err := assignStore.ListAll(ctx)
	if err != nil {
		h.Log.Error("error fetching all assignments", zap.Error(err))
		uierrors.RenderServerError(w, r, "Failed to load assignments.", "/materials")
		return
	}

	// Collect material, org, and leader IDs for name lookup
	materialIDs := make([]primitive.ObjectID, 0)
	orgIDs := make([]primitive.ObjectID, 0)
	leaderIDs := make([]primitive.ObjectID, 0)
	for _, a := range assignments {
		materialIDs = append(materialIDs, a.MaterialID)
		if a.OrganizationID != nil {
			orgIDs = append(orgIDs, *a.OrganizationID)
		}
		if a.LeaderID != nil {
			leaderIDs = append(leaderIDs, *a.LeaderID)
		}
	}

	// Fetch material names
	materialNames := make(map[primitive.ObjectID]string)
	if len(materialIDs) > 0 {
		matStore := materialstore.New(h.DB)
		mats, err := matStore.GetByIDs(ctx, materialIDs)
		if err == nil {
			for _, m := range mats {
				materialNames[m.ID] = m.Title
			}
		}
	}

	// Fetch org names
	orgNames := make(map[primitive.ObjectID]string)
	if len(orgIDs) > 0 {
		names, err := orgutil.FetchOrgNames(ctx, h.DB, orgIDs)
		if err == nil {
			orgNames = names
		}
	}

	// Fetch leader info (name and org)
	leaderNames := make(map[primitive.ObjectID]string)
	leaderOrgs := make(map[primitive.ObjectID]primitive.ObjectID) // leader ID -> org ID
	if len(leaderIDs) > 0 {
		usrStore := userstore.New(h.DB)
		users, err := usrStore.Find(ctx, bson.M{"_id": bson.M{"$in": leaderIDs}})
		if err == nil {
			for _, u := range users {
				leaderNames[u.ID] = u.FullName
				if u.OrganizationID != nil {
					leaderOrgs[u.ID] = *u.OrganizationID
				}
			}
		}
	}

	// Build scope org set for quick lookup (coordinators only)
	scopeOrgSet := make(map[primitive.ObjectID]bool)
	for _, oid := range scopeOrgIDs {
		scopeOrgSet[oid] = true
	}

	// Build list items, filtering by coordinator scope if applicable
	items := make([]assignmentListItem, 0, len(assignments))
	for _, a := range assignments {
		// For coordinators, filter to only accessible assignments
		if isCoordinator {
			if a.OrganizationID != nil {
				// Org-wide assignment: check if org is in coordinator's scope
				if !scopeOrgSet[*a.OrganizationID] {
					continue
				}
			} else if a.LeaderID != nil {
				// Leader assignment: check if leader's org is in coordinator's scope
				leaderOrgID, ok := leaderOrgs[*a.LeaderID]
				if !ok || !scopeOrgSet[leaderOrgID] {
					continue
				}
			}
		}

		item := assignmentListItem{
			ID:            a.ID.Hex(),
			MaterialID:    a.MaterialID.Hex(),
			MaterialTitle: materialNames[a.MaterialID],
			CreatedAt:     a.CreatedAt.Format("Jan 2, 2006"),
		}

		if item.MaterialTitle == "" {
			item.MaterialTitle = "(unknown material)"
		}

		if a.OrganizationID != nil {
			item.TargetType = "organization"
			item.TargetName = orgNames[*a.OrganizationID]
			if item.TargetName == "" {
				item.TargetName = "(unknown org)"
			}
		} else if a.LeaderID != nil {
			item.TargetType = "leader"
			item.TargetName = leaderNames[*a.LeaderID]
			if item.TargetName == "" {
				item.TargetName = "(unknown leader)"
			}
		}

		if a.VisibleFrom != nil {
			item.VisibleFrom = a.VisibleFrom.Format("Jan 2, 2006 3:04 PM")
		}
		if a.VisibleUntil != nil {
			item.VisibleUntil = a.VisibleUntil.Format("Jan 2, 2006 3:04 PM")
		}

		items = append(items, item)
	}

	data := allAssignmentsListData{
		BaseVM: viewdata.NewBaseVM(r, h.DB, "All Material Assignments", "/materials"),
		Items:  items,
		Shown:  len(items),
	}

	templates.RenderAutoMap(w, r, "admin_all_material_assignments", nil, data)
}

// ServeAssignmentManageModal renders the manage modal for an assignment.
func (h *AdminHandler) ServeAssignmentManageModal(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), timeouts.Short())
	defer cancel()

	assignIDStr := chi.URLParam(r, "assignID")
	assignID, err := primitive.ObjectIDFromHex(assignIDStr)
	if err != nil {
		uierrors.HTMXBadRequest(w, r, "Invalid assignment ID.", "/materials/assignments")
		return
	}

	assignStore := materialassignstore.New(h.DB)
	assignment, err := assignStore.GetByID(ctx, assignID)
	if err != nil {
		uierrors.HTMXError(w, r, http.StatusNotFound, "Assignment not found.", func() {
			uierrors.RenderNotFound(w, r, "Assignment not found.", "/materials/assignments")
		})
		return
	}

	// Coordinator access check
	if authz.IsCoordinator(r) {
		if !h.canAccessAssignment(ctx, r, assignment) {
			uierrors.HTMXError(w, r, http.StatusForbidden, "You don't have access to this assignment.", func() {
				uierrors.RenderForbidden(w, r, "You don't have access to this assignment.", "/materials")
			})
			return
		}
	}

	// Fetch material name
	matStore := materialstore.New(h.DB)
	mat, _ := matStore.GetByID(ctx, assignment.MaterialID)

	// Fetch target name
	var targetName, targetType string
	if assignment.OrganizationID != nil {
		targetType = "organization"
		names, _ := orgutil.FetchOrgNames(ctx, h.DB, []primitive.ObjectID{*assignment.OrganizationID})
		targetName = names[*assignment.OrganizationID]
	} else if assignment.LeaderID != nil {
		targetType = "leader"
		usrStore := userstore.New(h.DB)
		user, err := usrStore.GetByID(ctx, *assignment.LeaderID)
		if err == nil {
			targetName = user.FullName
		}
	}

	data := assignmentManageModalData{
		ID:            assignment.ID.Hex(),
		MaterialID:    assignment.MaterialID.Hex(),
		MaterialTitle: mat.Title,
		TargetName:    targetName,
		TargetType:    targetType,
		BackURL:       httpnav.ResolveBackURL(r, "/materials/assignments"),
	}

	if assignment.VisibleFrom != nil {
		data.VisibleFrom = assignment.VisibleFrom.Format("Jan 2, 2006 3:04 PM")
	}
	if assignment.VisibleUntil != nil {
		data.VisibleUntil = assignment.VisibleUntil.Format("Jan 2, 2006 3:04 PM")
	}

	templates.RenderSnippet(w, "material_assignment_manage_modal", data)
}

// ServeAssignmentView renders the view page for an assignment.
func (h *AdminHandler) ServeAssignmentView(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), timeouts.Short())
	defer cancel()

	assignIDStr := chi.URLParam(r, "assignID")
	assignID, err := primitive.ObjectIDFromHex(assignIDStr)
	if err != nil {
		uierrors.RenderBadRequest(w, r, "Invalid assignment ID.", "/materials/assignments")
		return
	}

	assignStore := materialassignstore.New(h.DB)
	assignment, err := assignStore.GetByID(ctx, assignID)
	if err != nil {
		uierrors.RenderNotFound(w, r, "Assignment not found.", "/materials/assignments")
		return
	}

	// Coordinator access check
	if authz.IsCoordinator(r) {
		if !h.canAccessAssignment(ctx, r, assignment) {
			uierrors.RenderForbidden(w, r, "You don't have access to this assignment.", "/materials")
			return
		}
	}

	// Fetch material name
	matStore := materialstore.New(h.DB)
	mat, _ := matStore.GetByID(ctx, assignment.MaterialID)

	// Fetch target name and resolve org ID for timezone
	var targetName, targetType string
	var resolveOrgID primitive.ObjectID
	if assignment.OrganizationID != nil {
		targetType = "Organization"
		resolveOrgID = *assignment.OrganizationID
		names, _ := orgutil.FetchOrgNames(ctx, h.DB, []primitive.ObjectID{*assignment.OrganizationID})
		targetName = names[*assignment.OrganizationID]
	} else if assignment.LeaderID != nil {
		targetType = "Leader"
		usrStore := userstore.New(h.DB)
		user, err := usrStore.GetByID(ctx, *assignment.LeaderID)
		if err == nil {
			targetName = user.FullName
			if user.OrganizationID != nil {
				resolveOrgID = *user.OrganizationID
			}
		}
	}

	// Resolve timezone for date display
	loc, tzLabel := orgutil.ResolveOrgLocation(ctx, h.DB, resolveOrgID)

	data := assignmentViewData{
		BaseVM:        viewdata.NewBaseVM(r, h.DB, "View Assignment", "/materials/assignments"),
		ID:            assignment.ID.Hex(),
		MaterialID:    assignment.MaterialID.Hex(),
		MaterialTitle: mat.Title,
		HasFile:       mat.HasFile(),
		LaunchURL:     mat.LaunchURL,
		TargetName:    targetName,
		TargetType:    targetType,
		CreatedAt:     assignment.CreatedAt.In(loc).Format("Jan 2, 2006 3:04 PM"),
		TimeZone:      tzLabel,
	}

	if assignment.VisibleFrom != nil {
		data.VisibleFrom = assignment.VisibleFrom.In(loc).Format("Jan 2, 2006 3:04 PM")
	}
	if assignment.VisibleUntil != nil {
		data.VisibleUntil = assignment.VisibleUntil.In(loc).Format("Jan 2, 2006 3:04 PM")
	}
	if assignment.Directions != "" {
		data.Directions = template.HTML(assignment.Directions)
	}

	templates.RenderAutoMap(w, r, "admin_material_assignment_view", nil, data)
}

// ServeAssignmentEdit renders the edit form for an assignment.
func (h *AdminHandler) ServeAssignmentEdit(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), timeouts.Short())
	defer cancel()

	assignIDStr := chi.URLParam(r, "assignID")
	assignID, err := primitive.ObjectIDFromHex(assignIDStr)
	if err != nil {
		uierrors.RenderBadRequest(w, r, "Invalid assignment ID.", "/materials/assignments")
		return
	}

	assignStore := materialassignstore.New(h.DB)
	assignment, err := assignStore.GetByID(ctx, assignID)
	if err != nil {
		uierrors.RenderNotFound(w, r, "Assignment not found.", "/materials/assignments")
		return
	}

	// Coordinator access check
	if authz.IsCoordinator(r) {
		if !h.canAccessAssignment(ctx, r, assignment) {
			uierrors.RenderForbidden(w, r, "You don't have access to this assignment.", "/materials")
			return
		}
	}

	// Fetch material name
	matStore := materialstore.New(h.DB)
	mat, _ := matStore.GetByID(ctx, assignment.MaterialID)

	// Fetch target name and resolve org ID for timezone
	var targetName, targetType string
	var resolveOrgID primitive.ObjectID
	if assignment.OrganizationID != nil {
		targetType = "Organization"
		resolveOrgID = *assignment.OrganizationID
		names, _ := orgutil.FetchOrgNames(ctx, h.DB, []primitive.ObjectID{*assignment.OrganizationID})
		targetName = names[*assignment.OrganizationID]
	} else if assignment.LeaderID != nil {
		targetType = "Leader"
		usrStore := userstore.New(h.DB)
		user, err := usrStore.GetByID(ctx, *assignment.LeaderID)
		if err == nil {
			targetName = user.FullName
			if user.OrganizationID != nil {
				resolveOrgID = *user.OrganizationID
			}
		}
	}

	// Resolve timezone for date display
	loc, tzLabel := orgutil.ResolveOrgLocation(ctx, h.DB, resolveOrgID)

	data := assignmentEditData{
		BaseVM:        viewdata.NewBaseVM(r, h.DB, "Edit Assignment", "/materials/assignments"),
		ID:            assignment.ID.Hex(),
		MaterialID:    assignment.MaterialID.Hex(),
		MaterialTitle: mat.Title,
		HasFile:       mat.HasFile(),
		LaunchURL:     mat.LaunchURL,
		TargetName:    targetName,
		TargetType:    targetType,
		Directions:    assignment.Directions,
		TimeZone:      tzLabel,
	}

	if assignment.VisibleFrom != nil {
		data.VisibleFrom = assignment.VisibleFrom.In(loc).Format("2006-01-02T15:04")
	}
	if assignment.VisibleUntil != nil {
		data.VisibleUntil = assignment.VisibleUntil.In(loc).Format("2006-01-02T15:04")
	}

	templates.RenderAutoMap(w, r, "admin_material_assignment_edit", nil, data)
}

// HandleAssignmentEdit processes the edit form for an assignment.
func (h *AdminHandler) HandleAssignmentEdit(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), timeouts.Medium())
	defer cancel()

	assignIDStr := chi.URLParam(r, "assignID")
	assignID, err := primitive.ObjectIDFromHex(assignIDStr)
	if err != nil {
		uierrors.RenderBadRequest(w, r, "Invalid assignment ID.", "/materials/assignments")
		return
	}

	assignStore := materialassignstore.New(h.DB)
	assignment, err := assignStore.GetByID(ctx, assignID)
	if err != nil {
		uierrors.RenderNotFound(w, r, "Assignment not found.", "/materials/assignments")
		return
	}

	// Coordinator access check
	if authz.IsCoordinator(r) {
		if !h.canAccessAssignment(ctx, r, assignment) {
			uierrors.RenderForbidden(w, r, "You don't have access to this assignment.", "/materials")
			return
		}
	}

	// Resolve org ID for timezone (from org assignment or leader's org)
	var resolveOrgID primitive.ObjectID
	if assignment.OrganizationID != nil {
		resolveOrgID = *assignment.OrganizationID
	} else if assignment.LeaderID != nil {
		usrStore := userstore.New(h.DB)
		user, err := usrStore.GetByID(ctx, *assignment.LeaderID)
		if err == nil && user.OrganizationID != nil {
			resolveOrgID = *user.OrganizationID
		}
	}

	// Resolve the organization's timezone so we can interpret the submitted
	// dates in the organization's local timezone.
	loc, tzLabel := orgutil.ResolveOrgLocation(ctx, h.DB, resolveOrgID)

	// Parse form values
	visibleFromStr := strings.TrimSpace(r.FormValue("visible_from"))
	visibleUntilStr := strings.TrimSpace(r.FormValue("visible_until"))
	directions := strings.TrimSpace(r.FormValue("directions"))

	// Parse dates in org timezone
	var visibleFrom, visibleUntil *time.Time
	if visibleFromStr != "" {
		if t, err := time.ParseInLocation("2006-01-02T15:04", visibleFromStr, loc); err == nil {
			visibleFrom = &t
		}
	}
	if visibleUntilStr != "" {
		if t, err := time.ParseInLocation("2006-01-02T15:04", visibleUntilStr, loc); err == nil {
			visibleUntil = &t
		}
	}

	// Helper to re-render with error
	reRender := func(errMsg string) {
		// Fetch material and target names for re-render
		matStore := materialstore.New(h.DB)
		mat, _ := matStore.GetByID(ctx, assignment.MaterialID)

		var targetName, targetType string
		if assignment.OrganizationID != nil {
			targetType = "Organization"
			names, _ := orgutil.FetchOrgNames(ctx, h.DB, []primitive.ObjectID{*assignment.OrganizationID})
			targetName = names[*assignment.OrganizationID]
		} else if assignment.LeaderID != nil {
			targetType = "Leader"
			usrStore := userstore.New(h.DB)
			user, err := usrStore.GetByID(ctx, *assignment.LeaderID)
			if err == nil {
				targetName = user.FullName
			}
		}

		data := assignmentEditData{
			BaseVM:        viewdata.NewBaseVM(r, h.DB, "Edit Assignment", "/materials/assignments"),
			ID:            assignment.ID.Hex(),
			MaterialID:    assignment.MaterialID.Hex(),
			MaterialTitle: mat.Title,
			HasFile:       mat.HasFile(),
			LaunchURL:     mat.LaunchURL,
			TargetName:    targetName,
			TargetType:    targetType,
			VisibleFrom:   visibleFromStr,
			VisibleUntil:  visibleUntilStr,
			Directions:    directions,
			TimeZone:      tzLabel,
			Error:         template.HTML(errMsg),
		}
		templates.RenderAutoMap(w, r, "admin_material_assignment_edit", nil, data)
	}

	// Validate: visible_until must be after visible_from if both are set
	if visibleUntil != nil && visibleFrom != nil && visibleUntil.Before(*visibleFrom) {
		reRender("Visible Until must be after Visible From.")
		return
	}

	// Update assignment
	assignment.VisibleFrom = visibleFrom
	assignment.VisibleUntil = visibleUntil
	assignment.Directions = directions

	if _, err := assignStore.Update(ctx, assignment); err != nil {
		h.Log.Error("error updating assignment", zap.Error(err))
		reRender("Failed to update assignment.")
		return
	}

	// Redirect back
	returnURL := r.FormValue("return")
	if returnURL == "" {
		returnURL = "/materials/assignments/" + assignID.Hex() + "/view"
	}
	http.Redirect(w, r, returnURL, http.StatusSeeOther)
}
