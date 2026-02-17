// internal/app/features/mhsdelivery/routes.go
package mhsdelivery

import (
	"github.com/dalemusser/stratahub/internal/app/system/auth"
	"github.com/go-chi/chi/v5"
)

// Routes returns the router for the MHS content delivery feature, mounted at /mhs.
func Routes(h *Handler, sm *auth.SessionManager) chi.Router {
	r := chi.NewRouter()

	r.Group(func(pr chi.Router) {
		pr.Use(sm.RequireSignedIn)
		pr.Use(sm.RequireRole("member", "leader", "admin", "coordinator", "superadmin"))

		pr.Get("/units", h.ServeUnits)
		pr.Get("/play/{unit}", h.ServePlay)
		pr.Get("/api/manifest", h.ServeContentManifest)
	})

	return r
}
