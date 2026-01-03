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

	// Picker endpoint - accessible by admin, coordinator, and leader roles
	r.Group(func(pr chi.Router) {
		pr.Use(sm.RequireSignedIn)
		pr.Get("/picker", h.ServeOrgPicker)
	})

	// Admin and coordinator routes (coordinators have filtered views, see handlers)
	r.Group(func(pr chi.Router) {
		pr.Use(sm.RequireSignedIn)
		pr.Use(sm.RequireRole("admin", "coordinator"))

		// LIST (admin sees all; coordinator sees only assigned orgs)
		pr.Get("/", h.ServeList)

		// VIEW (admin can view any; coordinator can view assigned orgs)
		pr.Get("/{id}/view", h.ServeView)

		// EDIT (admin can edit any; coordinator can edit assigned orgs)
		pr.Get("/{id}/edit", h.ServeEdit)
		pr.Post("/{id}/edit", h.HandleEdit)

		// MANAGE MODAL (HTMX) - same permissions as view
		pr.Get("/{id}/manage_modal", h.ServeManageModal)
	})

	// Admin-only routes (coordinators cannot create or delete)
	r.Group(func(pr chi.Router) {
		pr.Use(sm.RequireSignedIn)
		pr.Use(sm.RequireRole("admin"))

		// CREATE
		pr.Get("/new", h.ServeNew)
		pr.Post("/", h.HandleCreate)

		// DELETE
		pr.Post("/{id}/delete", h.HandleDelete)
	})

	return r
}
