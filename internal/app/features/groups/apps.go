package groups

import (
	"context"
	"net/http"

	uierrors "github.com/dalemusser/stratahub/internal/app/features/errors"
	"github.com/dalemusser/stratahub/internal/app/policy/grouppolicy"
	groupappstore "github.com/dalemusser/stratahub/internal/app/store/groupapps"
	groupstore "github.com/dalemusser/stratahub/internal/app/store/groups"
	"github.com/dalemusser/stratahub/internal/app/store/mhscollections"
	"github.com/dalemusser/stratahub/internal/app/system/authz"
	"github.com/dalemusser/stratahub/internal/app/system/timeouts"
	"github.com/dalemusser/stratahub/internal/app/system/viewdata"
	"github.com/dalemusser/stratahub/internal/app/system/workspace"
	"github.com/dalemusser/stratahub/internal/domain/models"
	"github.com/dalemusser/waffle/pantry/httpnav"
	"github.com/dalemusser/waffle/pantry/templates"
	"github.com/go-chi/chi/v5"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.uber.org/zap"
)

// HandleSetMHSCollection sets or clears the MHS collection pin for a group.
func (h *Handler) HandleSetMHSCollection(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}

	gid := chi.URLParam(r, "id")
	groupOID, err := primitive.ObjectIDFromHex(gid)
	if err != nil {
		http.Error(w, "bad group id", http.StatusBadRequest)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), timeouts.Short())
	defer cancel()
	db := h.DB

	// Verify access
	group, err := groupstore.New(db).GetByID(ctx, groupOID)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	canManage, _ := grouppolicy.CanManageGroup(ctx, db, r, group.ID, group.OrganizationID)
	if !canManage {
		uierrors.RenderForbidden(w, r, "You do not have access to this group.", "/groups")
		return
	}

	collIDStr := r.FormValue("mhs_collection_id")
	appStore := groupappstore.New(db)

	if collIDStr == "" || collIDStr == "active" {
		// Clear the pin
		if err := appStore.SetMHSCollection(ctx, groupOID, nil); err != nil {
			h.ErrLog.LogServerError(w, r, "failed to clear MHS collection pin", err, "Failed to update.", "/groups/"+gid+"/apps")
			return
		}
	} else {
		collOID, err := primitive.ObjectIDFromHex(collIDStr)
		if err != nil {
			http.Error(w, "invalid collection id", http.StatusBadRequest)
			return
		}
		if err := appStore.SetMHSCollection(ctx, groupOID, &collOID); err != nil {
			h.ErrLog.LogServerError(w, r, "failed to set MHS collection pin", err, "Failed to update.", "/groups/"+gid+"/apps")
			return
		}
	}

	dest := "/groups/" + gid + "/apps"
	if ret := r.FormValue("return"); ret != "" {
		dest += "?return=" + ret
	}
	http.Redirect(w, r, dest, http.StatusSeeOther)
}

// ServeGroupApps renders the Manage Apps page for a group.
func (h *Handler) ServeGroupApps(w http.ResponseWriter, r *http.Request) {
	_, _, _, ok := authz.UserCtx(r)
	if !ok {
		uierrors.RenderUnauthorized(w, r, "/login")
		return
	}

	gid := chi.URLParam(r, "id")
	groupOID, err := primitive.ObjectIDFromHex(gid)
	if err != nil {
		uierrors.RenderForbidden(w, r, "Bad group id.", httpnav.ResolveBackURL(r, "/groups"))
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), timeouts.Short())
	defer cancel()
	db := h.DB

	group, err := groupstore.New(db).GetByID(ctx, groupOID)
	if err == mongo.ErrNoDocuments {
		uierrors.RenderForbidden(w, r, "Group not found.", httpnav.ResolveBackURL(r, "/groups"))
		return
	}
	if err != nil {
		h.Log.Warn("group GetByID(apps)", zap.Error(err))
		uierrors.RenderForbidden(w, r, "A database error occurred.", httpnav.ResolveBackURL(r, "/groups"))
		return
	}

	// Verify workspace ownership
	wsID := workspace.IDFromRequest(r)
	if wsID != primitive.NilObjectID && group.WorkspaceID != wsID {
		uierrors.RenderNotFound(w, r, "Group not found.", "/groups")
		return
	}

	canManage, policyErr := grouppolicy.CanManageGroup(ctx, db, r, group.ID, group.OrganizationID)
	if policyErr != nil {
		h.ErrLog.LogServerError(w, r, "database error checking group access", policyErr, "A database error occurred.", "/groups")
		return
	}
	if !canManage {
		uierrors.RenderForbidden(w, r, "You do not have access to this group.", httpnav.ResolveBackURL(r, "/groups"))
		return
	}

	// Load enabled apps for this group
	appStore := groupappstore.New(db)
	enabledSettings, err := appStore.ListByGroup(ctx, groupOID)
	if err != nil {
		h.ErrLog.LogServerError(w, r, "database error loading app settings", err, "A database error occurred.", "/groups")
		return
	}

	// Build set of enabled app IDs
	enabledSet := make(map[string]bool, len(enabledSettings))
	for _, s := range enabledSettings {
		enabledSet[s.AppID] = true
	}

	// Build toggle items from registry
	items := make([]appToggleItem, len(models.AvailableApps))
	for i, app := range models.AvailableApps {
		items[i] = appToggleItem{
			ID:          app.ID,
			Name:        app.Name,
			Description: app.Description,
			Enabled:     enabledSet[app.ID],
		}
	}

	data := groupAppsData{
		BaseVM:    viewdata.NewBaseVM(r, db, "Manage Apps — "+group.Name, "/groups"),
		GroupID:   gid,
		GroupName: group.Name,
		Apps:      items,
	}

	// Load MHS collection pin info if MHS is enabled
	if enabledSet["missionhydrosci"] {
		// Get current pin from the MHS app setting
		for _, s := range enabledSettings {
			if s.AppID == "missionhydrosci" && s.MHSCollectionID != nil {
				data.MHSCollectionID = s.MHSCollectionID.Hex()
				// Look up collection name
				collStore := mhscollections.New(db)
				if coll, err := collStore.GetByID(ctx, *s.MHSCollectionID); err == nil {
					data.MHSCollectionName = coll.Name
				}
				break
			}
		}

		// Load available collections for the dropdown
		collStore := mhscollections.New(db)
		if collections, err := collStore.List(ctx, 50); err == nil {
			opts := make([]mhsCollectionOption, len(collections))
			for i, c := range collections {
				opts[i] = mhsCollectionOption{ID: c.ID.Hex(), Name: c.Name}
			}
			data.MHSCollectionOptions = opts
		}
	}

	templates.Render(w, r, "group_apps", data)
}

// HandleToggleApp handles POST to enable or disable an app for a group.
func (h *Handler) HandleToggleApp(w http.ResponseWriter, r *http.Request) {
	role, actorName, actorID, ok := authz.UserCtx(r)
	if !ok {
		uierrors.RenderUnauthorized(w, r, "/login")
		return
	}

	gid := chi.URLParam(r, "id")
	groupOID, err := primitive.ObjectIDFromHex(gid)
	if err != nil {
		uierrors.RenderForbidden(w, r, "Bad group id.", httpnav.ResolveBackURL(r, "/groups"))
		return
	}

	appID := r.FormValue("app_id")
	action := r.FormValue("action")

	// Validate app exists
	if _, found := models.FindApp(appID); !found {
		uierrors.RenderBadRequest(w, r, "Unknown app.", "/groups/"+gid+"/apps")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), timeouts.Short())
	defer cancel()
	db := h.DB

	group, err := groupstore.New(db).GetByID(ctx, groupOID)
	if err == mongo.ErrNoDocuments {
		uierrors.RenderForbidden(w, r, "Group not found.", httpnav.ResolveBackURL(r, "/groups"))
		return
	}
	if err != nil {
		h.Log.Warn("group GetByID(toggle app)", zap.Error(err))
		uierrors.RenderForbidden(w, r, "A database error occurred.", httpnav.ResolveBackURL(r, "/groups"))
		return
	}

	// Verify workspace ownership
	wsID := workspace.IDFromRequest(r)
	if wsID != primitive.NilObjectID && group.WorkspaceID != wsID {
		uierrors.RenderNotFound(w, r, "Group not found.", "/groups")
		return
	}

	canManage, policyErr := grouppolicy.CanManageGroup(ctx, db, r, group.ID, group.OrganizationID)
	if policyErr != nil {
		h.ErrLog.LogServerError(w, r, "database error checking group access", policyErr, "A database error occurred.", "/groups")
		return
	}
	if !canManage {
		uierrors.RenderForbidden(w, r, "You do not have access to this group.", httpnav.ResolveBackURL(r, "/groups"))
		return
	}

	appStore := groupappstore.New(db)

	switch action {
	case "enable":
		if err := appStore.Enable(ctx, wsID, groupOID, actorID, appID, actorName); err != nil {
			h.ErrLog.LogServerError(w, r, "failed to enable app", err, "Failed to enable app.", "/groups/"+gid+"/apps")
			return
		}
		h.AuditLog.GroupAppEnabled(ctx, r, actorID, groupOID, &group.OrganizationID, role, group.Name, appID)

	case "disable":
		if err := appStore.Disable(ctx, groupOID, appID); err != nil {
			h.ErrLog.LogServerError(w, r, "failed to disable app", err, "Failed to disable app.", "/groups/"+gid+"/apps")
			return
		}
		h.AuditLog.GroupAppDisabled(ctx, r, actorID, groupOID, &group.OrganizationID, role, group.Name, appID)

	default:
		uierrors.RenderBadRequest(w, r, "Invalid action.", "/groups/"+gid+"/apps")
		return
	}

	dest := "/groups/" + gid + "/apps"
	if ret := r.FormValue("return"); ret != "" {
		dest += "?return=" + ret
	}
	http.Redirect(w, r, dest, http.StatusSeeOther)
}
