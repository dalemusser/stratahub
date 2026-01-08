// internal/app/features/workspaces/delete.go
package workspaces

import (
	"context"
	"net/http"

	workspacestore "github.com/dalemusser/stratahub/internal/app/store/workspaces"
	"github.com/dalemusser/stratahub/internal/app/system/authz"
	"github.com/dalemusser/stratahub/internal/app/system/timeouts"
	"github.com/dalemusser/stratahub/internal/app/system/viewdata"
	"github.com/dalemusser/waffle/pantry/templates"
	"github.com/go-chi/chi/v5"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.uber.org/zap"
)

// ServeDeleteConfirm renders the delete confirmation page.
func (h *Handler) ServeDeleteConfirm(w http.ResponseWriter, r *http.Request) {
	idHex := chi.URLParam(r, "id")
	wsID, err := primitive.ObjectIDFromHex(idHex)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), timeouts.Short())
	defer cancel()

	store := workspacestore.New(h.DB)
	ws, err := store.GetByID(ctx, wsID)
	if err != nil {
		if err == workspacestore.ErrNotFound {
			http.NotFound(w, r)
			return
		}
		h.ErrLog.LogServerError(w, r, "database error loading workspace", err, "A database error occurred.", "/workspaces")
		return
	}

	// Get counts for warning
	userCount, _ := h.DB.Collection("users").CountDocuments(ctx, bson.M{"workspace_id": wsID})
	orgCount, _ := h.DB.Collection("organizations").CountDocuments(ctx, bson.M{"workspace_id": wsID})

	data := deleteConfirmData{
		BaseVM:        viewdata.NewBaseVM(r, h.DB, "Delete Workspace", "/workspaces"),
		WorkspaceID:   ws.ID.Hex(),
		WorkspaceName: ws.Name,
		Subdomain:     ws.Subdomain,
		UserCount:     userCount,
		OrgCount:      orgCount,
	}

	templates.Render(w, r, "workspace_delete", data)
}

// HandleDelete processes the workspace deletion.
func (h *Handler) HandleDelete(w http.ResponseWriter, r *http.Request) {
	_, _, actorID, ok := authz.UserCtx(r)
	if !ok {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}

	idHex := chi.URLParam(r, "id")
	wsID, err := primitive.ObjectIDFromHex(idHex)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	if err := r.ParseForm(); err != nil {
		h.ErrLog.LogBadRequest(w, r, "parse form failed", err, "Invalid form data.", "/workspaces")
		return
	}

	// Require confirmation by typing workspace name
	confirmName := r.FormValue("confirm_name")

	ctx, cancel := context.WithTimeout(r.Context(), timeouts.Long())
	defer cancel()

	store := workspacestore.New(h.DB)
	ws, err := store.GetByID(ctx, wsID)
	if err != nil {
		if err == workspacestore.ErrNotFound {
			http.NotFound(w, r)
			return
		}
		h.ErrLog.LogServerError(w, r, "database error loading workspace", err, "A database error occurred.", "/workspaces")
		return
	}

	// Verify confirmation
	if confirmName != ws.Name {
		userCount, _ := h.DB.Collection("users").CountDocuments(ctx, bson.M{"workspace_id": wsID})
		orgCount, _ := h.DB.Collection("organizations").CountDocuments(ctx, bson.M{"workspace_id": wsID})

		data := deleteConfirmData{
			BaseVM:        viewdata.NewBaseVM(r, h.DB, "Delete Workspace", "/workspaces"),
			WorkspaceID:   ws.ID.Hex(),
			WorkspaceName: ws.Name,
			Subdomain:     ws.Subdomain,
			UserCount:     userCount,
			OrgCount:      orgCount,
		}
		data.Error = "Workspace name does not match. Please type the exact name to confirm deletion."

		templates.Render(w, r, "workspace_delete", data)
		return
	}

	// Delete all associated data
	h.deleteWorkspaceData(ctx, wsID)

	// Delete the workspace itself
	if _, err := store.Delete(ctx, wsID); err != nil {
		h.ErrLog.LogServerError(w, r, "database error deleting workspace", err, "A database error occurred.", "/workspaces")
		return
	}

	// Audit log
	h.Log.Info("workspace deleted",
		zap.String("workspace_id", wsID.Hex()),
		zap.String("workspace_name", ws.Name),
		zap.String("subdomain", ws.Subdomain),
		zap.String("deleted_by", actorID.Hex()))

	http.Redirect(w, r, "/workspaces", http.StatusSeeOther)
}

// deleteWorkspaceData removes all data associated with a workspace.
func (h *Handler) deleteWorkspaceData(ctx context.Context, wsID primitive.ObjectID) {
	filter := bson.M{"workspace_id": wsID}

	// Delete in order of dependencies
	collections := []string{
		"group_memberships",
		"coordinator_assignments",
		"material_assignments",
		"sessions",
		"activity_events",
		"audit_events",
		"groups",
		"users",
		"organizations",
		"resources",
		"materials",
	}

	for _, coll := range collections {
		result, err := h.DB.Collection(coll).DeleteMany(ctx, filter)
		if err != nil {
			h.Log.Warn("error deleting workspace data",
				zap.String("collection", coll),
				zap.String("workspace_id", wsID.Hex()),
				zap.Error(err))
		} else if result.DeletedCount > 0 {
			h.Log.Info("deleted workspace data",
				zap.String("collection", coll),
				zap.String("workspace_id", wsID.Hex()),
				zap.Int64("count", result.DeletedCount))
		}
	}
}
