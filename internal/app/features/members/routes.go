// internal/app/features/members/routes.go
package members

import (
	"github.com/dalemusser/stratahub/internal/app/system/auth"
	"github.com/go-chi/chi/v5"
)

// Routes mounts all member routes under the path where the caller mounts it.
// Typically: r.Mount("/members", members.Routes(handler))
func Routes(h *Handler) chi.Router {
	r := chi.NewRouter()

	r.Group(func(pr chi.Router) {
		pr.Use(auth.RequireSignedIn)

		// Unified screen (admin + leader; role-aware inside)
		pr.Get("/", h.ServeList)
		pr.Get("/{id}/manage_modal", h.ServeManageMemberModal)

		// Add / Upload
		pr.Get("/new", h.ServeNew)
		pr.Post("/", h.HandleCreate)
		pr.Get("/upload_csv", h.ServeUploadCSV)
		pr.Post("/upload_csv", h.HandleUploadCSV)

		// View / Edit / Delete single member
		pr.Get("/{id}/view", h.ServeView)
		pr.Get("/{id}/edit", h.ServeEdit)
		pr.Post("/{id}/edit", h.HandleEdit)
		pr.Post("/{id}/delete", h.HandleDelete)
	})

	return r
}
