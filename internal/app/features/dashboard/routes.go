// internal/app/features/dashboard/routes.go
package dashboard

import (
	"github.com/dalemusser/stratahub/internal/app/system/auth"
	"github.com/go-chi/chi/v5"
)

// Routes wires the dashboard feature under whatever mount point
// the top-level router chooses (e.g., "/dashboard").
//
// The handler will dispatch to the appropriate role-specific view
// based on the current user's role (admin, analyst, leader, member).
func Routes(h *Handler, sm *auth.SessionManager) chi.Router {
	r := chi.NewRouter()

	// All dashboards require the user to be signed in.
	r.Group(func(pr chi.Router) {
		pr.Use(sm.RequireSignedIn)
		// Final path will be /dashboard when mounted at "/dashboard".
		pr.Get("/", h.ServeDashboard)
	})

	return r
}
