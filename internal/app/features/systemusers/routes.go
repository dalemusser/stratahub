// internal/app/features/systemusers/routes.go
package systemusers

import (
	"github.com/dalemusser/stratahub/internal/app/system/auth"
	"github.com/go-chi/chi/v5"
)

// Routes mounts all system user routes under the path where this
// router is mounted (typically "/system-users" from bootstrap).
//
// Example mount from bootstrap:
//
//	h := systemusers.NewHandler(db, logger)
//	r.Mount("/system-users", systemusers.Routes(h, sessionMgr))
func Routes(h *Handler, sm *auth.SessionManager) chi.Router {
	r := chi.NewRouter()

	r.Group(func(pr chi.Router) {
		// Only signed-in admins can access system users.
		pr.Use(sm.RequireSignedIn)
		pr.Use(sm.RequireRole("admin"))

		// List all system users
		pr.Get("/", h.ServeList)

		// Create new system user
		pr.Get("/new", h.ServeNew)
		pr.Post("/", h.HandleCreate)

		// Organization picker for coordinator assignment
		pr.Get("/org-picker", h.ServeOrgPicker)

		// View / edit / delete specific system user
		pr.Get("/{id}/view", h.ServeView)
		pr.Get("/{id}/edit", h.ServeEdit)
		pr.Post("/{id}/edit", h.HandleEdit)
		pr.Post("/{id}/delete", h.HandleDelete)

		// Manage modal for a specific system user
		pr.Get("/{id}/manage_modal", h.ServeManageModal)
	})

	return r
}
