// internal/app/features/workspaces/routes.go
package workspaces

import (
	"github.com/dalemusser/stratahub/internal/app/system/auth"
	"github.com/dalemusser/stratahub/internal/app/system/workspace"
	"github.com/go-chi/chi/v5"
)

// Routes mounts all workspace management routes.
// These routes are superadmin-only and should only be accessible at the apex domain.
func Routes(h *Handler, sm *auth.SessionManager) chi.Router {
	r := chi.NewRouter()

	// All routes require apex domain and superadmin role
	r.Use(workspace.RequireApex)
	r.Use(sm.RequireSignedIn)
	r.Use(sm.RequireRole("superadmin"))

	// LIST - view all workspaces
	r.Get("/", h.ServeList)

	// MANAGE MODAL - htmx modal for workspace actions
	r.Get("/{id}/manage_modal", h.ServeManageModal)

	// CREATE - new workspace form and handler
	r.Get("/new", h.ServeNew)
	r.Post("/", h.HandleCreate)

	// STATS - view workspace statistics
	r.Get("/{id}/stats", h.ServeStats)

	// STATUS CHANGE - suspend, activate, archive
	r.Post("/{id}/status", h.HandleStatusChange)

	// SETTINGS - workspace-specific site settings
	r.Get("/{id}/settings", h.ServeSettings)
	r.Post("/{id}/settings", h.HandleSettings)

	// DELETE - delete workspace (with confirmation)
	r.Get("/{id}/delete", h.ServeDeleteConfirm)
	r.Post("/{id}/delete", h.HandleDelete)

	return r
}
