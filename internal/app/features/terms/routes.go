// internal/app/features/terms/routes.go
package terms

import "github.com/go-chi/chi/v5"

func Routes(h *Handler) chi.Router {
	r := chi.NewRouter()
	r.Get("/", h.ServeTerms) // relative to the mount point
	return r
}
