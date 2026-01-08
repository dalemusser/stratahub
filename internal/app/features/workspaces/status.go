// internal/app/features/workspaces/status.go
package workspaces

import (
	"context"
	"net/http"

	workspacestore "github.com/dalemusser/stratahub/internal/app/store/workspaces"
	"github.com/dalemusser/stratahub/internal/app/system/authz"
	"github.com/dalemusser/stratahub/internal/app/system/timeouts"
	"github.com/dalemusser/stratahub/internal/app/system/viewdata"
	"github.com/dalemusser/stratahub/internal/domain/models"
	"github.com/dalemusser/waffle/pantry/templates"
	"github.com/go-chi/chi/v5"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.uber.org/zap"
)

// ServeStats renders the workspace statistics page.
func (h *Handler) ServeStats(w http.ResponseWriter, r *http.Request) {
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

	// Count users by role
	userFilter := bson.M{"workspace_id": wsID}
	userCount, _ := h.DB.Collection("users").CountDocuments(ctx, userFilter)
	adminCount, _ := h.DB.Collection("users").CountDocuments(ctx, bson.M{"workspace_id": wsID, "role": "admin"})
	analystCount, _ := h.DB.Collection("users").CountDocuments(ctx, bson.M{"workspace_id": wsID, "role": "analyst"})
	coordCount, _ := h.DB.Collection("users").CountDocuments(ctx, bson.M{"workspace_id": wsID, "role": "coordinator"})
	leaderCount, _ := h.DB.Collection("users").CountDocuments(ctx, bson.M{"workspace_id": wsID, "role": "leader"})
	memberCount, _ := h.DB.Collection("users").CountDocuments(ctx, bson.M{"workspace_id": wsID, "role": "member"})

	// Count other entities
	orgCount, _ := h.DB.Collection("organizations").CountDocuments(ctx, bson.M{"workspace_id": wsID})
	groupCount, _ := h.DB.Collection("groups").CountDocuments(ctx, bson.M{"workspace_id": wsID})
	resourceCount, _ := h.DB.Collection("resources").CountDocuments(ctx, bson.M{"workspace_id": wsID})
	materialCount, _ := h.DB.Collection("materials").CountDocuments(ctx, bson.M{"workspace_id": wsID})

	data := statsData{
		BaseVM:        viewdata.NewBaseVM(r, h.DB, "Workspace Stats", "/workspaces"),
		WorkspaceID:   ws.ID.Hex(),
		WorkspaceName: ws.Name,
		Subdomain:     ws.Subdomain,
		Status:        ws.Status,
		CreatedAt:     ws.CreatedAt,
		UserCount:     userCount,
		AdminCount:    adminCount,
		AnalystCount:  analystCount,
		CoordCount:    coordCount,
		LeaderCount:   leaderCount,
		MemberCount:   memberCount,
		OrgCount:      orgCount,
		GroupCount:    groupCount,
		ResourceCount: resourceCount,
		MaterialCount: materialCount,
	}

	templates.Render(w, r, "workspace_stats", data)
}

// HandleStatusChange processes workspace status changes (suspend, activate, archive).
func (h *Handler) HandleStatusChange(w http.ResponseWriter, r *http.Request) {
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

	newStatus := r.FormValue("status")
	if newStatus != "active" && newStatus != "suspended" && newStatus != "archived" {
		h.ErrLog.LogBadRequest(w, r, "invalid status", nil, "Invalid status value.", "/workspaces")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), timeouts.Medium())
	defer cancel()

	store := workspacestore.New(h.DB)

	// Get current workspace
	ws, err := store.GetByID(ctx, wsID)
	if err != nil {
		if err == workspacestore.ErrNotFound {
			http.NotFound(w, r)
			return
		}
		h.ErrLog.LogServerError(w, r, "database error loading workspace", err, "A database error occurred.", "/workspaces")
		return
	}

	oldStatus := ws.Status

	// Update status
	if err := store.Update(ctx, wsID, models.Workspace{Status: newStatus}); err != nil {
		h.ErrLog.LogServerError(w, r, "database error updating workspace", err, "A database error occurred.", "/workspaces")
		return
	}

	// Audit log
	h.Log.Info("workspace status changed",
		zap.String("workspace_id", wsID.Hex()),
		zap.String("old_status", oldStatus),
		zap.String("new_status", newStatus),
		zap.String("changed_by", actorID.Hex()))

	http.Redirect(w, r, "/workspaces", http.StatusSeeOther)
}
