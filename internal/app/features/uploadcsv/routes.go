// internal/app/features/uploadcsv/routes.go
package uploadcsv

import (
	"github.com/dalemusser/stratahub/internal/app/system/auth"
	"github.com/go-chi/chi/v5"
)

// Routes mounts all CSV upload routes under the path where the caller mounts it.
// Typically: r.Mount("/upload_csv", uploadcsv.Routes(handler, sessionMgr))
func Routes(h *Handler, sm *auth.SessionManager) chi.Router {
	r := chi.NewRouter()

	r.Group(func(pr chi.Router) {
		pr.Use(sm.RequireSignedIn)

		pr.Get("/", h.ServeUploadCSV)
		pr.Post("/", h.HandleUploadCSV)
		pr.Get("/confirm", h.RedirectConfirm)
		pr.Post("/confirm", h.HandleConfirm)
	})

	return r
}
