// internal/app/features/missionhydrosci/routes.go
package missionhydrosci

import (
	"github.com/dalemusser/stratahub/internal/app/system/auth"
	"github.com/go-chi/chi/v5"
)

// Routes returns the router for the Mission HydroSci feature, mounted at /missionhydrosci.
func Routes(h *Handler, sm *auth.SessionManager) chi.Router {
	r := chi.NewRouter()

	r.Group(func(pr chi.Router) {
		pr.Use(sm.RequireSignedIn)
		pr.Use(sm.RequireRole("admin", "coordinator", "superadmin"))

		pr.Get("/units", h.ServeUnits)
		pr.Get("/play/{unit}", h.ServePlay)
		pr.Get("/api/manifest", h.ServeContentManifest)
	})

	return r
}
