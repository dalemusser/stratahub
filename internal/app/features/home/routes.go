package home

import "github.com/go-chi/chi/v5"

func Routes(h *Handler) chi.Router {
	r := chi.NewRouter()
	r.Get("/", h.ServeRoot)
	// We'll bring over the static handler once we port that logic.
	// r.Handle("/static/*", h.serveStatic())
	return r
}
