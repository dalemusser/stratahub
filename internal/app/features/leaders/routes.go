// internal/app/features/leaders/routes.go
package leaders

import (
	"github.com/dalemusser/stratahub/internal/app/system/auth"
	"github.com/go-chi/chi/v5"
)

func Routes(h *Handler, sm *auth.SessionManager) chi.Router {
	r := chi.NewRouter()

	r.Group(func(pr chi.Router) {
		pr.Use(sm.RequireSignedIn)
		pr.Use(sm.RequireRole("admin"))

		pr.Get("/", h.ServeList)
		pr.Get("/new", h.ServeNew)
		pr.Post("/", h.HandleCreate)
		pr.Get("/{id}/view", h.ServeView)
		pr.Get("/{id}/edit", h.ServeEdit)
		pr.Post("/{id}/edit", h.HandleEdit)
		pr.Post("/{id}/delete", h.HandleDelete)
		pr.Get("/{id}/manage_modal", h.ServeLeaderManageModal)
	})

	return r
}
