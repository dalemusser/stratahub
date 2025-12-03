// internal/app/features/health/routes.go
package health

import "github.com/go-chi/chi/v5"

// Routes returns a subrouter that serves the health endpoints.
func Routes(h *Handler) chi.Router {
	r := chi.NewRouter()
	r.Get("/", h.Serve) // this will be mounted under /health
	return r
}
