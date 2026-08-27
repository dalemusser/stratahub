// internal/app/features/viewers/routes.go
package viewers

import (
	"github.com/dalemusser/stratahub/internal/app/system/auth"
	"github.com/go-chi/chi/v5"
)

// Routes returns the router for the viewers feature, mounted at /views inside
// the workspace group. Every route requires a signed-in user; the per-viewer
// role gate is applied in the handlers from Viewer.Roles().
//
//	GET /views                       index of viewers available to this role
//	GET /views/{slug}                page with filters and the first rows
//	GET /views/{slug}/table          HTMX partial: table, or rows-only with ?after=
//	GET /views/{slug}/rows/{id}      HTMX partial: one row's detail
//	GET /views/{slug}/export.csv     CSV of the current filter within scope
func Routes(h *Handler, sm *auth.SessionManager) chi.Router {
	r := chi.NewRouter()
	r.Use(sm.RequireSignedIn)

	r.Get("/", h.ServeIndex)
	r.Get("/{slug}", h.ServePage)
	r.Get("/{slug}/table", h.ServeTable)
	r.Get("/{slug}/rows/{id}", h.ServeDetail)
	r.Get("/{slug}/export.csv", h.ServeExport)

	return r
}
