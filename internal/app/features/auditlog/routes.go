// internal/app/features/auditlog/routes.go
package auditlog

import (
	"github.com/dalemusser/stratahub/internal/app/system/auth"
	"github.com/go-chi/chi/v5"
)

// Routes mounts all audit log routes under the path where this
// router is mounted (typically "/audit" from bootstrap).
//
// Access is restricted to admins and coordinators.
// Admins see all events; coordinators see only events for their assigned orgs.
func Routes(h *Handler, sm *auth.SessionManager) chi.Router {
	r := chi.NewRouter()

	r.Group(func(pr chi.Router) {
		pr.Use(sm.RequireSignedIn)
		pr.Use(sm.RequireRole("admin", "coordinator"))

		pr.Get("/", h.ServeList)
	})

	return r
}
