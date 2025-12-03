// internal/app/features/contact/routes.go
package contact

import "github.com/go-chi/chi/v5"

func Routes(h *Handler) chi.Router {
	r := chi.NewRouter()
	r.Get("/", h.ServeContact) // relative path
	return r
}
