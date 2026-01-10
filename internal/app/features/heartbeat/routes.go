// internal/app/features/heartbeat/routes.go
package heartbeat

import (
	"github.com/dalemusser/stratahub/internal/app/system/auth"
	"github.com/go-chi/chi/v5"
)

// Routes returns the router for heartbeat endpoints.
func Routes(h *Handler, sm *auth.SessionManager) chi.Router {
	r := chi.NewRouter()

	// Require user to be signed in
	r.Use(sm.RequireSignedIn)

	r.Post("/", h.ServeHeartbeat)

	return r
}
