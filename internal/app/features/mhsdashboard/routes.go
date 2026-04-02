// internal/app/features/mhsdashboard/routes.go
package mhsdashboard

import (
	"github.com/dalemusser/stratahub/internal/app/system/auth"
	"github.com/go-chi/chi/v5"
)

// Routes returns the router for the MHS dashboard 3 feature.
func Routes(h *Handler, sm *auth.SessionManager) chi.Router {
	r := chi.NewRouter()

	// All routes require authentication
	r.Group(func(pr chi.Router) {
		pr.Use(sm.RequireSignedIn)

		// Main dashboard page
		pr.Get("/", h.ServeDashboard)

		// HTMX endpoint for grid refresh
		pr.Get("/grid", h.ServeGrid)

		// Set student progress to a specific unit
		pr.Post("/set-progress", h.HandleSetProgress)

		// AI-powered student performance summary
		pr.Get("/summary", h.ServeSummary)

		// Debug tools (admin/coordinator only — enforced in handlers)
		pr.Get("/debug/students", h.ServeDebugStudents)
		pr.Get("/debug/student/{userID}", h.ServeDebugDetail)

		// Maps tab (admin/coordinator only — enforced in handlers)
		pr.Get("/maps/positions", h.ServeMapPositions)
		pr.Get("/maps/scenes", h.ServeMapScenes)
		pr.Get("/maps/members", h.ServeMapMembers)
	})

	return r
}
