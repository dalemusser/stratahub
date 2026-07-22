// internal/app/features/missionhydrosci/routes.go
package missionhydrosci

import (
	"github.com/dalemusser/stratahub/internal/app/system/auth"
	"github.com/dalemusser/stratahub/internal/app/system/timeouts"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

// Routes returns the router for the Mission HydroSci feature, mounted at /missionhydrosci.
func Routes(h *Handler, sm *auth.SessionManager) chi.Router {
	r := chi.NewRouter()

	r.Group(func(pr chi.Router) {
		// Bound every handler's request context so a stalled DB primary (e.g.
		// during a DocumentDB failover) can't pin request goroutines. This
		// covers the DB calls in the handlers AND their shared helpers
		// (resolveManifest, checkMemberAuth, ...), which all read from
		// r.Context(). None of these routes stream or long-poll, so a single
		// group-level timeout is safe. Static content and the service worker
		// are served outside this group and are unaffected.
		pr.Use(middleware.Timeout(timeouts.Long()))
		pr.Use(sm.RequireSignedIn)
		pr.Use(sm.RequireRole("member", "leader", "admin", "coordinator", "superadmin"))
		pr.Use(RequireApp("missionhydrosci"))

		pr.Get("/units", h.ServeUnits)
		// Manage page: troubleshooting/testing tools. Entry-gated server-side
		// for members in keyword/staffauth workspaces (staff unlock session).
		pr.Get("/manage", h.ServeManage)
		pr.Post("/api/manage/unlock", h.HandleManageUnlock)
		pr.Post("/api/manage/lock", h.HandleManageLock)
		pr.Get("/api/manage/status", h.ServeManageStatus)
		pr.Get("/play/{unit}", h.ServePlay)
		// Redirect game-initiated unit transitions (MHSBridge URL-mode fallback).
		// The game navigates to ../unit2/index.html which resolves to /missionhydrosci/unit2/index.html.
		pr.Get("/{unit}/index.html", h.RedirectToPlay)
		pr.Get("/api/manifest", h.ServeContentManifest)
		pr.Get("/api/progress", h.ServeProgress)
		pr.Post("/api/progress/complete", h.HandleCompleteUnit)
		pr.Post("/api/progress/set-unit", h.HandleSetToUnit)

		// Delete online saved game data (state / settings) via stratasave.
		// Same member-auth rules as set-unit; used to clear save data before a
		// unit launches without going through the running game.
		pr.Post("/api/save-data/state/delete", h.HandleDeleteSavedState)
		pr.Post("/api/save-data/settings/delete", h.HandleDeleteSavedSettings)
		pr.Post("/api/device-status", h.HandleDeviceStatus)
		pr.Post("/api/auth/start", h.HandleStaffAuthStart)
		pr.Post("/api/auth/verify", h.HandleStaffAuthVerify)
		pr.Post("/api/auth/resend", h.HandleStaffAuthResend)

		// Collection override (per-user, all roles — members need staff-auth via the picker UI)
		pr.Get("/api/collections", h.ServeCollectionList)
		pr.Post("/api/collection-override", h.HandleSetCollectionOverride)
		pr.Post("/api/collection-override/clear", h.HandleClearCollectionOverride)
	})

	return r
}
