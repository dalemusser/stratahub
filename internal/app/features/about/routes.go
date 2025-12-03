// internal/app/features/about/routes.go
package about

import "github.com/go-chi/chi/v5"

func Routes(h *Handler) chi.Router {
	r := chi.NewRouter()
	r.Get("/", h.ServeAbout)
	return r
}
