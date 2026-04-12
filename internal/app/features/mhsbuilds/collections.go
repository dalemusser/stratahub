// internal/app/features/mhsbuilds/collections.go
package mhsbuilds

import (
	"context"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/dalemusser/stratahub/internal/app/store/mhsbuilds"
	"github.com/dalemusser/stratahub/internal/app/system/format"
	"github.com/dalemusser/stratahub/internal/app/system/viewdata"
	"github.com/dalemusser/stratahub/internal/app/system/workspace"
	"github.com/dalemusser/stratahub/internal/domain/models"
	"github.com/dalemusser/waffle/pantry/templates"
	"github.com/go-chi/chi/v5"
	"github.com/gorilla/csrf"
	"go.mongodb.org/mongo-driver/bson"
	"go.uber.org/zap"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

// ServeCollections renders the collection list page.
func (h *Handler) ServeCollections(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()

	collections, err := h.CollectionStore.List(ctx, 100)
	if err != nil {
		h.ErrLog.LogServerError(w, r, "Failed to load collections", err, "Failed to load collections", "/mhsbuilds/collections")
		return
	}

	// Get active collection for this workspace
	wsID := workspace.IDFromRequest(r)
	settings, _ := h.SettingsStore.Get(ctx, wsID)
	activeID := ""
	if settings.MHSActiveCollectionID != nil {
		activeID = settings.MHSActiveCollectionID.Hex()
	}

	vms := make([]CollectionVM, len(collections))
	for i, c := range collections {
		vms[i] = CollectionVM{
			ID:            c.ID.Hex(),
			Name:          c.Name,
			Description:   c.Description,
			UnitsSummary:  unitsSummary(c.Units),
			CreatedAt:     c.CreatedAt,
			CreatedByName: c.CreatedByName,
			IsActive:      c.ID.Hex() == activeID,
		}
	}

	data := CollectionsData{
		BaseVM:      viewdata.LoadBase(r, h.DB),
		Collections: vms,
		ActiveID:    activeID,
	}
	data.Title = "MHS Collections"
	templates.Render(w, r, "mhsbuilds_collections", data)
}

// ServeCollectionDetail renders the detail page for a single collection.
func (h *Handler) ServeCollectionDetail(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	oid, err := primitive.ObjectIDFromHex(idStr)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()

	coll, err := h.CollectionStore.GetByID(ctx, oid)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	wsID := workspace.IDFromRequest(r)
	settings, _ := h.SettingsStore.Get(ctx, wsID)
	activeID := ""
	if settings.MHSActiveCollectionID != nil {
		activeID = settings.MHSActiveCollectionID.Hex()
	}

	// Look up file data from mhs_builds for display
	pairs := make([]mhsbuilds.UnitVersionPair, len(coll.Units))
	for i, u := range coll.Units {
		pairs[i] = mhsbuilds.UnitVersionPair{UnitID: u.UnitID, Version: u.Version}
	}
	buildMap, err := h.BuildStore.GetByUnitVersionBatch(ctx, pairs)
	if err != nil {
		h.Log.Error("failed to load build records for collection detail", zap.Error(err))
		buildMap = make(map[string]models.MHSBuild)
	}

	unitVMs := make([]CollectionUnitVM, len(coll.Units))
	for i, u := range coll.Units {
		build := buildMap[u.UnitID+":"+u.Version]
		unitVMs[i] = CollectionUnitVM{
			UnitID:          u.UnitID,
			Title:           u.Title,
			Version:         u.Version,
			BuildIdentifier: u.BuildIdentifier,
			FileCount:       len(build.Files),
			TotalSize:       build.TotalSize,
			SizeLabel:       format.Bytes(build.TotalSize),
		}
	}

	data := CollectionDetailData{
		BaseVM:        viewdata.LoadBase(r, h.DB),
		ID:            coll.ID.Hex(),
		Name:          coll.Name,
		Description:   coll.Description,
		Units:         unitVMs,
		CreatedAt:     coll.CreatedAt,
		CreatedByName: coll.CreatedByName,
		IsActive:      coll.ID.Hex() == activeID,
		ActiveID:      activeID,
	}
	data.Title = coll.Name
	templates.Render(w, r, "mhsbuilds_collection_detail", data)
}

// HandleActivateCollection sets a collection as the active collection for the workspace.
func (h *Handler) HandleActivateCollection(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	oid, err := primitive.ObjectIDFromHex(idStr)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()

	// Verify collection exists
	_, err = h.CollectionStore.GetByID(ctx, oid)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	// Update workspace settings
	wsID := workspace.IDFromRequest(r)
	settings, err := h.SettingsStore.Get(ctx, wsID)
	if err != nil {
		h.ErrLog.LogServerError(w, r, "Failed to load settings", err, "Failed to load settings", "/mhsbuilds/collections")
		return
	}

	settings.MHSActiveCollectionID = &oid
	if err := h.SettingsStore.Save(ctx, wsID, settings); err != nil {
		h.ErrLog.LogServerError(w, r, "Failed to activate collection", err, "Failed to activate collection", "/mhsbuilds/collections")
		return
	}

	http.Redirect(w, r, "/mhsbuilds/collections/"+idStr, http.StatusSeeOther)
}

// HandleDeactivateCollection clears the active collection for the workspace.
func (h *Handler) HandleDeactivateCollection(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()

	wsID := workspace.IDFromRequest(r)
	settings, err := h.SettingsStore.Get(ctx, wsID)
	if err != nil {
		h.ErrLog.LogServerError(w, r, "Failed to load settings", err, "Failed to load settings", "/mhsbuilds/collections")
		return
	}

	settings.MHSActiveCollectionID = nil
	if err := h.SettingsStore.Save(ctx, wsID, settings); err != nil {
		h.ErrLog.LogServerError(w, r, "Failed to deactivate collection", err, "Failed to deactivate collection", "/mhsbuilds/collections")
		return
	}

	http.Redirect(w, r, "/mhsbuilds/collections", http.StatusSeeOther)
}

// ServeManageModal renders the manage modal for a collection (HTMX snippet).
func (h *Handler) ServeManageModal(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	oid, err := primitive.ObjectIDFromHex(idStr)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()

	coll, err := h.CollectionStore.GetByID(ctx, oid)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	wsID := workspace.IDFromRequest(r)
	settings, _ := h.SettingsStore.Get(ctx, wsID)
	isActive := settings.MHSActiveCollectionID != nil && *settings.MHSActiveCollectionID == oid

	// Check if collection can be deleted (not in use anywhere across all workspaces)
	canDelete := !h.isCollectionInUse(ctx, oid)

	data := ManageModalData{
		ID:        coll.ID.Hex(),
		Name:      coll.Name,
		IsActive:  isActive,
		CanDelete: canDelete,
		CSRFToken: csrf.Token(r),
	}

	templates.RenderSnippet(w, "mhsbuilds_manage_modal", data)
}

// ServeEdit renders the edit form for a collection.
func (h *Handler) ServeEdit(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	oid, err := primitive.ObjectIDFromHex(idStr)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()

	coll, err := h.CollectionStore.GetByID(ctx, oid)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	wsID := workspace.IDFromRequest(r)
	settings, _ := h.SettingsStore.Get(ctx, wsID)
	isActive := settings.MHSActiveCollectionID != nil && *settings.MHSActiveCollectionID == oid

	// Load all builds grouped by unit for version dropdowns
	allBuilds, _ := h.BuildStore.ListAll(ctx)
	buildsByUnit := make(map[string][]models.MHSBuild)
	for _, b := range allBuilds {
		buildsByUnit[b.UnitID] = append(buildsByUnit[b.UnitID], b)
	}

	units := make([]EditCollectionUnitRow, len(coll.Units))
	for i, u := range coll.Units {
		row := EditCollectionUnitRow{
			UnitID:          u.UnitID,
			Title:           u.Title,
			Version:         u.Version,
			BuildIdentifier: u.BuildIdentifier,
		}
		if builds, ok := buildsByUnit[u.UnitID]; ok {
			for _, b := range builds {
				row.AvailableVersions = append(row.AvailableVersions, ManualVersionOption{
					Version:         b.Version,
					BuildIdentifier: b.BuildIdentifier,
					Selected:        b.Version == u.Version,
				})
			}
		}
		units[i] = row
	}

	data := EditCollectionData{
		BaseVM:      viewdata.NewBaseVM(r, h.DB, "Edit Collection", "/mhsbuilds/collections"),
		ID:          coll.ID.Hex(),
		Name:        coll.Name,
		Description: coll.Description,
		Units:       units,
		IsActive:    isActive,
	}

	templates.Render(w, r, "mhsbuilds_collection_edit", data)
}

// HandleEdit processes the edit form submission.
func (h *Handler) HandleEdit(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	oid, err := primitive.ObjectIDFromHex(idStr)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	if err := r.ParseForm(); err != nil {
		h.ErrLog.LogBadRequest(w, r, "Invalid form", err, "Invalid form data", "/mhsbuilds/collections/"+idStr+"/edit")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Minute)
	defer cancel()

	coll, err := h.CollectionStore.GetByID(ctx, oid)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	newName := strings.TrimSpace(r.FormValue("collection_name"))
	newDesc := strings.TrimSpace(r.FormValue("collection_description"))
	if newName == "" {
		newName = coll.Name
	}

	// Update unit versions from dropdowns — build identifier comes from the build record
	for i, u := range coll.Units {
		newVersion := strings.TrimSpace(r.FormValue("version_" + u.UnitID))
		if newVersion == "" {
			newVersion = u.Version
		}
		coll.Units[i].Version = newVersion

		// Look up build identifier from the build record
		build, err := h.BuildStore.GetByUnitVersion(ctx, u.UnitID, newVersion)
		if err != nil {
			h.renderEditError(w, r, oid, fmt.Sprintf("No build record found for %s v%s. Try syncing from S3 first.", u.UnitID, newVersion))
			return
		}
		coll.Units[i].BuildIdentifier = build.BuildIdentifier
	}

	// Sort units
	sort.Slice(coll.Units, func(i, j int) bool {
		return coll.Units[i].UnitID < coll.Units[j].UnitID
	})

	coll.Name = newName
	coll.Description = newDesc

	if err := h.CollectionStore.Update(ctx, oid, coll); err != nil {
		h.ErrLog.LogServerError(w, r, "Failed to update collection", err, "Failed to save changes", "/mhsbuilds/collections/"+idStr+"/edit")
		return
	}

	http.Redirect(w, r, "/mhsbuilds/collections/"+idStr, http.StatusSeeOther)
}

func (h *Handler) renderEditError(w http.ResponseWriter, r *http.Request, id primitive.ObjectID, msg string) {
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()

	coll, _ := h.CollectionStore.GetByID(ctx, id)

	// Load builds for version dropdowns
	allBuilds, _ := h.BuildStore.ListAll(ctx)
	buildsByUnit := make(map[string][]models.MHSBuild)
	for _, b := range allBuilds {
		buildsByUnit[b.UnitID] = append(buildsByUnit[b.UnitID], b)
	}

	units := make([]EditCollectionUnitRow, len(coll.Units))
	for i, u := range coll.Units {
		selectedVersion := r.FormValue("version_" + u.UnitID)
		if selectedVersion == "" {
			selectedVersion = u.Version
		}
		row := EditCollectionUnitRow{
			UnitID:          u.UnitID,
			Title:           u.Title,
			Version:         selectedVersion,
			BuildIdentifier: u.BuildIdentifier,
		}
		if builds, ok := buildsByUnit[u.UnitID]; ok {
			for _, b := range builds {
				row.AvailableVersions = append(row.AvailableVersions, ManualVersionOption{
					Version:         b.Version,
					BuildIdentifier: b.BuildIdentifier,
					Selected:        b.Version == selectedVersion,
				})
			}
		}
		units[i] = row
	}

	data := EditCollectionData{
		BaseVM:      viewdata.NewBaseVM(r, h.DB, "Edit Collection", "/mhsbuilds/collections"),
		ID:          id.Hex(),
		Name:        r.FormValue("collection_name"),
		Description: r.FormValue("collection_description"),
		Units:       units,
		Error:       msg,
	}
	templates.Render(w, r, "mhsbuilds_collection_edit", data)
}

// ServeAssignments renders the assignments report for a collection.
func (h *Handler) ServeAssignments(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	oid, err := primitive.ObjectIDFromHex(idStr)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()

	coll, err := h.CollectionStore.GetByID(ctx, oid)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	// Find workspaces where this collection is active
	var workspaces []AssignmentWorkspace
	wsCursor, err := h.DB.Collection("site_settings").Find(ctx, bson.M{"mhs_active_collection_id": oid})
	if err == nil {
		defer wsCursor.Close(ctx)
		for wsCursor.Next(ctx) {
			var ss struct {
				WorkspaceID primitive.ObjectID `bson:"workspace_id"`
			}
			if wsCursor.Decode(&ss) == nil {
				// Look up workspace name
				var ws struct {
					Name      string `bson:"name"`
					Subdomain string `bson:"subdomain"`
				}
				if h.DB.Collection("workspaces").FindOne(ctx, bson.M{"_id": ss.WorkspaceID}).Decode(&ws) == nil {
					workspaces = append(workspaces, AssignmentWorkspace{Name: ws.Name, Subdomain: ws.Subdomain})
				}
			}
		}
	}

	// Find groups pinned to this collection
	var groups []AssignmentGroup
	grpCursor, err := h.DB.Collection("group_app_settings").Find(ctx, bson.M{
		"app_id":            "missionhydrosci",
		"mhs_collection_id": oid,
	})
	if err == nil {
		defer grpCursor.Close(ctx)
		for grpCursor.Next(ctx) {
			var gas struct {
				GroupID     primitive.ObjectID `bson:"group_id"`
				WorkspaceID primitive.ObjectID `bson:"workspace_id"`
			}
			if grpCursor.Decode(&gas) != nil {
				continue
			}
			var grp struct {
				Name string `bson:"name"`
			}
			if h.DB.Collection("groups").FindOne(ctx, bson.M{"_id": gas.GroupID}).Decode(&grp) != nil {
				continue // group deleted — skip
			}
			var ws struct {
				Name string `bson:"name"`
			}
			h.DB.Collection("workspaces").FindOne(ctx, bson.M{"_id": gas.WorkspaceID}).Decode(&ws)
			groups = append(groups, AssignmentGroup{GroupName: grp.Name, WorkspaceName: ws.Name})
		}
	}

	// Find users with this collection as their override
	var users []AssignmentUser
	userCursor, err := h.DB.Collection("mhs_user_progress").Find(ctx, bson.M{"collection_override_id": oid})
	if err == nil {
		defer userCursor.Close(ctx)
		for userCursor.Next(ctx) {
			var prog struct {
				UserID      primitive.ObjectID `bson:"user_id"`
				WorkspaceID primitive.ObjectID `bson:"workspace_id"`
				LoginID     string             `bson:"login_id"`
			}
			if userCursor.Decode(&prog) != nil {
				continue
			}
			var usr struct {
				FullName string `bson:"full_name"`
			}
			if h.DB.Collection("users").FindOne(ctx, bson.M{"_id": prog.UserID}).Decode(&usr) != nil {
				continue // user deleted — skip
			}
			var ws struct {
				Name string `bson:"name"`
			}
			h.DB.Collection("workspaces").FindOne(ctx, bson.M{"_id": prog.WorkspaceID}).Decode(&ws)
			users = append(users, AssignmentUser{
				UserName:      usr.FullName,
				LoginID:       prog.LoginID,
				WorkspaceName: ws.Name,
			})
		}
	}

	// Load all workspaces for the filter dropdown and name lookup
	selectedWS := r.URL.Query().Get("ws")
	if selectedWS == "" {
		selectedWS = "all"
	}

	type wsInfo struct {
		Name      string
		Subdomain string
	}
	wsMap := make(map[string]wsInfo) // workspace ID hex → info
	var wsOptions []WorkspaceOption
	wsCursorAll, _ := h.DB.Collection("workspaces").Find(ctx, bson.M{})
	if wsCursorAll != nil {
		defer wsCursorAll.Close(ctx)
		for wsCursorAll.Next(ctx) {
			var w struct {
				ID        primitive.ObjectID `bson:"_id"`
				Name      string             `bson:"name"`
				Subdomain string             `bson:"subdomain"`
			}
			if wsCursorAll.Decode(&w) == nil {
				wsMap[w.ID.Hex()] = wsInfo{Name: w.Name, Subdomain: w.Subdomain}
				wsOptions = append(wsOptions, WorkspaceOption{
					ID:        w.ID.Hex(),
					Name:      w.Name,
					Subdomain: w.Subdomain,
					Selected:  w.ID.Hex() == selectedWS,
				})
			}
		}
	}

	// Find all groups with Mission HydroSci enabled
	var enabledGroups []MHSEnabledGroup
	allMHSCursor, err := h.DB.Collection("group_app_settings").Find(ctx, bson.M{"app_id": "missionhydrosci"})
	if err == nil {
		defer allMHSCursor.Close(ctx)

		// Build a map of workspace active collections for lookup
		wsActiveMap := make(map[string]string) // workspace ID hex → collection name
		ssCursor, _ := h.DB.Collection("site_settings").Find(ctx, bson.M{"mhs_active_collection_id": bson.M{"$exists": true, "$ne": nil}})
		if ssCursor != nil {
			defer ssCursor.Close(ctx)
			for ssCursor.Next(ctx) {
				var ss struct {
					WorkspaceID           primitive.ObjectID  `bson:"workspace_id"`
					MHSActiveCollectionID *primitive.ObjectID `bson:"mhs_active_collection_id"`
				}
				if ssCursor.Decode(&ss) == nil && ss.MHSActiveCollectionID != nil {
					if c, err := h.CollectionStore.GetByID(ctx, *ss.MHSActiveCollectionID); err == nil {
						wsActiveMap[ss.WorkspaceID.Hex()] = c.Name
					}
				}
			}
		}

		for allMHSCursor.Next(ctx) {
			var gas struct {
				GroupID         primitive.ObjectID  `bson:"group_id"`
				WorkspaceID     primitive.ObjectID  `bson:"workspace_id"`
				MHSCollectionID *primitive.ObjectID `bson:"mhs_collection_id"`
			}
			if allMHSCursor.Decode(&gas) != nil {
				continue
			}

			wsIDHex := gas.WorkspaceID.Hex()

			// Filter by selected workspace
			if selectedWS != "all" && wsIDHex != selectedWS {
				continue
			}

			var grp struct {
				Name string `bson:"name"`
			}
			if h.DB.Collection("groups").FindOne(ctx, bson.M{"_id": gas.GroupID}).Decode(&grp) != nil {
				continue
			}

			info := wsMap[wsIDHex]
			wsLabel := info.Name
			if info.Subdomain != "" {
				wsLabel = info.Name + " (" + info.Subdomain + ")"
			}

			collUsed := "None"
			if gas.MHSCollectionID != nil && !gas.MHSCollectionID.IsZero() {
				if c, err := h.CollectionStore.GetByID(ctx, *gas.MHSCollectionID); err == nil {
					collUsed = "Pinned: " + c.Name
				}
			} else if name, ok := wsActiveMap[wsIDHex]; ok {
				collUsed = "Workspace active: " + name
			}

			enabledGroups = append(enabledGroups, MHSEnabledGroup{
				GroupName:      grp.Name,
				WorkspaceID:    wsIDHex,
				WorkspaceName:  wsLabel,
				CollectionUsed: collUsed,
			})
		}
	}

	data := AssignmentsData{
		BaseVM:            viewdata.NewBaseVM(r, h.DB, "Assignments — "+coll.Name, "/mhsbuilds/collections"),
		CollectionID:      idStr,
		CollectionName:    coll.Name,
		Workspaces:        workspaces,
		Groups:            groups,
		Users:             users,
		EnabledGroups:     enabledGroups,
		WorkspaceOptions:  wsOptions,
		SelectedWorkspace: selectedWS,
		IsUnused:          len(workspaces) == 0 && len(groups) == 0 && len(users) == 0,
	}

	templates.Render(w, r, "mhsbuilds_assignments", data)
}

// isCollectionInUse checks if a collection is active in any workspace,
// pinned to any group, or set as a user override — across ALL workspaces.
func (h *Handler) isCollectionInUse(ctx context.Context, collID primitive.ObjectID) bool {
	// Check if active in any workspace's site settings
	count, err := h.DB.Collection("site_settings").CountDocuments(ctx, bson.M{
		"mhs_active_collection_id": collID,
	})
	if err == nil && count > 0 {
		return true
	}

	// Check group pins across all workspaces
	count, err = h.DB.Collection("group_app_settings").CountDocuments(ctx, bson.M{
		"app_id":            "missionhydrosci",
		"mhs_collection_id": collID,
	})
	if err == nil && count > 0 {
		return true
	}

	// Check user overrides across all workspaces
	count, err = h.DB.Collection("mhs_user_progress").CountDocuments(ctx, bson.M{
		"collection_override_id": collID,
	})
	if err == nil && count > 0 {
		return true
	}

	return false
}

// HandleDelete deletes a collection if it's not in use.
func (h *Handler) HandleDelete(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	oid, err := primitive.ObjectIDFromHex(idStr)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()

	// Verify collection exists
	_, err = h.CollectionStore.GetByID(ctx, oid)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	// Check it's not in use anywhere (active in any workspace, pinned to groups, user overrides)
	if h.isCollectionInUse(ctx, oid) {
		http.Redirect(w, r, "/mhsbuilds/collections", http.StatusSeeOther)
		return
	}

	if err := h.CollectionStore.Delete(ctx, oid); err != nil {
		h.ErrLog.LogServerError(w, r, "Failed to delete collection", err, "Failed to delete collection", "/mhsbuilds/collections")
		return
	}

	http.Redirect(w, r, "/mhsbuilds/collections", http.StatusSeeOther)
}

// unitsSummary creates a short summary string of unit versions.
func unitsSummary(units []models.MHSCollectionUnit) string {
	parts := make([]string, len(units))
	for i, u := range units {
		parts[i] = fmt.Sprintf("%s:v%s", u.UnitID, u.Version)
	}
	return strings.Join(parts, ", ")
}
