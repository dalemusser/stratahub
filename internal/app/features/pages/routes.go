// internal/app/features/pages/routes.go
package pages

import (
	"github.com/dalemusser/stratahub/internal/app/system/auth"
	"github.com/go-chi/chi/v5"
)

// PublicRoutes returns routers for each public page.
// Call this once and mount each page at its respective path.
func (h *Handler) AboutRouter() chi.Router {
	r := chi.NewRouter()
	r.Get("/", h.ServeAbout)
	return r
}

func (h *Handler) ContactRouter() chi.Router {
	r := chi.NewRouter()
	r.Get("/", h.ServeContact)
	return r
}

func (h *Handler) TermsRouter() chi.Router {
	r := chi.NewRouter()
	r.Get("/", h.ServeTerms)
	return r
}

func (h *Handler) PrivacyRouter() chi.Router {
	r := chi.NewRouter()
	r.Get("/", h.ServePrivacy)
	return r
}

// EditRoutes returns a router for admin page editing.
// Mount this at /pages with admin-only middleware.
func EditRoutes(h *Handler, sessionMgr *auth.SessionManager) chi.Router {
	r := chi.NewRouter()
	r.Use(sessionMgr.RequireRole("admin"))
	r.Get("/{slug}/edit", h.ServeEdit)
	r.Post("/{slug}/edit", h.HandleEdit)
	return r
}
