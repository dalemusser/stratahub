// internal/app/features/organizations/routes.go
package organizations

import (
	"github.com/dalemusser/stratahub/internal/app/system/auth"
	"github.com/go-chi/chi/v5"
)

// Routes mounts all Organization routes under the base path
// (typically "/organizations" from bootstrap).
func Routes(h *Handler, sm *auth.SessionManager) chi.Router {
	r := chi.NewRouter()

	// Picker endpoint - accessible by admin and leader roles
	r.Group(func(pr chi.Router) {
		pr.Use(sm.RequireSignedIn)
		pr.Get("/picker", h.ServeOrgPicker)
	})

	// Admin-only routes
	r.Group(func(pr chi.Router) {
		pr.Use(sm.RequireSignedIn)
		pr.Use(sm.RequireRole("admin"))

		// LIST (live search + HTMX swap)
		pr.Get("/", h.ServeList)

		// CREATE
		pr.Get("/new", h.ServeNew)
		pr.Post("/", h.HandleCreate)

		// VIEW
		pr.Get("/{id}/view", h.ServeView)

		// EDIT
		pr.Get("/{id}/edit", h.ServeEdit)
		pr.Post("/{id}/edit", h.HandleEdit)

		// DELETE
		pr.Post("/{id}/delete", h.HandleDelete)

		// MANAGE MODAL (HTMX)
		pr.Get("/{id}/manage_modal", h.ServeManageModal)
	})

	return r
}
