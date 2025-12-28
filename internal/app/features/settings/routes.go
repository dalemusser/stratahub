// internal/app/features/settings/routes.go
package settings

import "github.com/go-chi/chi/v5"

// MountRoutes mounts all settings routes on the given router.
// All routes require admin authentication.
func (h *Handler) MountRoutes(r chi.Router) {
	r.Get("/", h.ServeSettings)
	r.Post("/", h.HandleSettings)
}
