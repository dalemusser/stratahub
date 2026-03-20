// internal/app/features/gameconfig/routes.go
package gameconfig

import "github.com/go-chi/chi/v5"

// MountRoutes registers GET /api/game-config on the supplied router.
// No auth-specific middleware is required because the handler itself
// checks the session via auth.CurrentUser.
func MountRoutes(r chi.Router, h *Handler) {
	r.Get("/api/game-config", h.ServeGameConfig)
}
