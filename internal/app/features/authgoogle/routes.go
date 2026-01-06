// internal/app/features/authgoogle/routes.go
package authgoogle

import "github.com/go-chi/chi/v5"

// Routes returns the router for Google OAuth endpoints.
// These routes are public (no authentication required).
func Routes(h *Handler) chi.Router {
	r := chi.NewRouter()

	// GET /auth/google - Initiate Google OAuth flow
	r.Get("/", h.ServeLogin)

	// GET /auth/google/callback - Handle Google OAuth callback
	r.Get("/callback", h.ServeCallback)

	return r
}
