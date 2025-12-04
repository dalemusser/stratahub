// internal/app/features/logout/routes.go
package logout

import (
	"github.com/dalemusser/stratahub/internal/app/system/auth"
	"github.com/go-chi/chi/v5"
)

func Routes(h *Handler) chi.Router {
	r := chi.NewRouter()

	r.Group(func(pr chi.Router) {
		// Only allow logged-in users to hit /logout.
		pr.Use(auth.RequireSignedIn)
		pr.Get("/", h.ServeLogout)
	})

	return r
}
