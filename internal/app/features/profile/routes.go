// internal/app/features/profile/routes.go
package profile

import "github.com/go-chi/chi/v5"

func Routes(h *Handler) chi.Router {
	r := chi.NewRouter()
	r.Get("/", h.ServeProfile)
	r.Post("/password", h.HandleChangePassword)
	r.Post("/preferences", h.HandleUpdatePreferences)
	return r
}
