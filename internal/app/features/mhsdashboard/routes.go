// internal/app/features/mhsdashboard/routes.go
package mhsdashboard

import (
	"github.com/dalemusser/stratahub/internal/app/system/auth"
	"github.com/go-chi/chi/v5"
)

// Routes returns the router for the MHS dashboard feature.
func Routes(h *Handler, sm *auth.SessionManager) chi.Router {
	r := chi.NewRouter()

	// All routes require authentication
	r.Group(func(pr chi.Router) {
		pr.Use(sm.RequireSignedIn)

		// Main dashboard page
		pr.Get("/", h.ServeDashboard)

		// HTMX endpoint for grid refresh
		pr.Get("/grid", h.ServeGrid)
	})

	return r
}
