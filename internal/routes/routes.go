package routes

import (
	gwcMid "github.com/dalemusser/gowebcore/middleware"
	"github.com/go-chi/chi/v5"
	chiMid "github.com/go-chi/chi/v5/middleware"

	"github.com/dalemusser/stratahub/internal/platform/handler"

	// slice routers
	"github.com/dalemusser/stratahub/internal/features/about"
	"github.com/dalemusser/stratahub/internal/features/contact"
	"github.com/dalemusser/stratahub/internal/features/login"
	"github.com/dalemusser/stratahub/internal/features/terms"
)

// RegisterAll mounts every feature’s routes on the given chi.Router.
func RegisterAll(r chi.Router, h *handler.Handler) {
	// ── global middleware ───────────────────────────────────────────────
	r.Use(chiMid.RequestID)
	r.Use(chiMid.RealIP)
	r.Use(chiMid.Logger)
	r.Use(gwcMid.CSRF) // SameSite cookie, HTMx-aware

	// ── public features ────────────────────────────────────────────────
	about.MountRoutes(r, h)
	contact.MountRoutes(r, h)
	login.MountRoutes(r, h)
	terms.MountRoutes(r, h)

	// ── auth-protected area ────────────────────────────────────────────
	authed := chi.NewRouter()
	authed.Use(h.Session.RequireAuth) // your new session middleware

	// add protected features here as you implement them…
	r.Mount("/", authed)

}
