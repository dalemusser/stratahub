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
		pr.Use(sm.RequireRole("member", "leader", "admin", "coordinator", "superadmin"))
		pr.Use(RequireApp("missionhydrosci"))

		pr.Get("/units", h.ServeUnits)
		pr.Get("/play/{unit}", h.ServePlay)
		// Redirect game-initiated unit transitions (MHSBridge URL-mode fallback).
		// The game navigates to ../unit2/index.html which resolves to /missionhydrosci/unit2/index.html.
		pr.Get("/{unit}/index.html", h.RedirectToPlay)
		pr.Get("/api/manifest", h.ServeContentManifest)
		pr.Get("/api/progress", h.ServeProgress)
		pr.Post("/api/progress/complete", h.HandleCompleteUnit)
		pr.Post("/api/progress/reset", h.HandleResetProgress)
		pr.Post("/api/device-status", h.HandleDeviceStatus)
	})

	return r
}
