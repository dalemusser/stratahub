// internal/app/features/login/routes.go
package login

import "github.com/go-chi/chi/v5"

func Routes(h *Handler) chi.Router {
	r := chi.NewRouter()
	r.Get("/", h.ServeLogin)
	r.Post("/", h.HandleLoginPost)
	return r
}
