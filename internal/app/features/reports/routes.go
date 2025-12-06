// internal/app/features/reports/routes.go
package reports

import (
	"github.com/dalemusser/stratahub/internal/app/system/auth"
	"github.com/go-chi/chi/v5"
)

func Routes(h *Handler) chi.Router {
	r := chi.NewRouter()

	r.Group(func(rr chi.Router) {
		rr.Use(auth.RequireSignedIn)
		// Admin / analyst / leader gating is enforced inside the handlers.
		rr.Get("/members", h.ServeMembersReport)
		rr.Get("/members.csv", h.ServeMembersCSV)
	})

	return r
}
