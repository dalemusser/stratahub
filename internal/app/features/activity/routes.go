// internal/app/features/activity/routes.go
package activity

import (
	"github.com/dalemusser/stratahub/internal/app/system/auth"
	"github.com/go-chi/chi/v5"
)

// Routes returns the router for activity dashboard endpoints.
// Leaders can view activity for members in their groups.
// Coordinators can view activity for members in their assigned organizations.
// Admins can view all activity.
func Routes(h *Handler, sm *auth.SessionManager) chi.Router {
	r := chi.NewRouter()

	// Teacher dashboard routes (admin, coordinator, leader)
	r.Group(func(pr chi.Router) {
		pr.Use(sm.RequireSignedIn)
		pr.Use(sm.RequireRole("admin", "coordinator", "leader"))

		// Real-time dashboard ("Who's Online")
		pr.Get("/", h.ServeDashboard)

		// Weekly summary view
		pr.Get("/summary", h.ServeSummary)

		// Member detail view
		pr.Get("/member/{memberID}", h.ServeMemberDetail)

		// HTMX partial for refreshing member detail content
		pr.Get("/member/{memberID}/content", h.ServeMemberDetailContent)

		// HTMX partial for refreshing the online status table
		pr.Get("/online-table", h.ServeOnlineTable)
	})

	// Researcher export routes (admin, coordinator only)
	r.Group(func(pr chi.Router) {
		pr.Use(sm.RequireSignedIn)
		pr.Use(sm.RequireRole("admin", "coordinator"))

		// Export UI
		pr.Get("/export", h.ServeExport)

		// CSV/JSON exports
		pr.Get("/export/sessions.csv", h.ServeSessionsCSV)
		pr.Get("/export/sessions.json", h.ServeSessionsJSON)
		pr.Get("/export/events.csv", h.ServeEventsCSV)
		pr.Get("/export/events.json", h.ServeEventsJSON)
	})

	return r
}
