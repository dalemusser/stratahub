// internal/app/features/userinfo/routes.go
package userinfo

import "github.com/go-chi/chi/v5"

// MountRoutes registers GET /api/user on the supplied router.
// No auth-specific middleware is required because the handler itself
// checks the session via auth.CurrentUser.
func MountRoutes(r chi.Router, h *Handler) {
	r.Get("/api/user", h.ServeUserInfo)
}
