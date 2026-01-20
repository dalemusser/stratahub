// internal/app/features/announcements/routes.go
package announcements

import "github.com/go-chi/chi/v5"

// MountRoutes mounts all announcement routes on the given router.
// All routes require admin authentication.
func (h *Handler) MountRoutes(r chi.Router) {
	r.Get("/", h.List)
	r.Get("/new", h.ShowNew)
	r.Post("/new", h.Create)
	r.Get("/{id}", h.Show)
	r.Get("/{id}/manage_modal", h.ManageModal)
	r.Get("/{id}/edit", h.ShowEdit)
	r.Post("/{id}", h.Update)
	r.Post("/{id}/toggle", h.Toggle)
	r.Post("/{id}/delete", h.Delete)
}
