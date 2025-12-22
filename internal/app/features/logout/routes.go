// internal/app/features/logout/routes.go
package logout

import (
	"github.com/dalemusser/stratahub/internal/app/system/auth"
	"github.com/go-chi/chi/v5"
)

func Routes(h *Handler, sm *auth.SessionManager) chi.Router {
	r := chi.NewRouter()

	r.Group(func(pr chi.Router) {
		// Only allow logged-in users to hit /logout.
		pr.Use(sm.RequireSignedIn)
		pr.Get("/", h.ServeLogout)
	})

	return r
}
