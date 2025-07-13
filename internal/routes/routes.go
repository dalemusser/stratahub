package routes

import (
	gwcMid "github.com/dalemusser/gowebcore/middleware"
	"github.com/go-chi/chi/v5"
	chiMid "github.com/go-chi/chi/v5/middleware" // <-- Chi-built-ins

	"github.com/dalemusser/stratahub/internal/platform/handler"
	// one import per slice
	"github.com/dalemusser/stratahub/internal/features/about"
	/*
		"github.com/dalemusser/stratahub/internal/features/home"

		"github.com/dalemusser/stratahub/internal/features/contact"
		"github.com/dalemusser/stratahub/internal/features/login"
		"github.com/dalemusser/stratahub/internal/features/logout"
		"github.com/dalemusser/stratahub/internal/features/dashboard"
		"github.com/dalemusser/stratahub/internal/features/games"
		"github.com/dalemusser/stratahub/internal/features/game"

		// health-check & auth helpers
		"github.com/dalemusser/stratahub/internal/features/health"
		"github.com/dalemusser/stratahub/internal/features/oauth"
	*/)

// RegisterAll mounts every feature’s routes onto r.
func RegisterAll(r chi.Router, h *handler.Handler) {
	//--------------------------------------------------------------------
	// GLOBAL MIDDLEWARE
	//--------------------------------------------------------------------
	r.Use(chiMid.RequestID) // χ helpers
	r.Use(chiMid.RealIP)
	r.Use(chiMid.Logger)
	r.Use(gwcMid.CSRF) // gowebcore helper (SameSite + HTMx)

	//--------------------------------------------------------------------
	// PUBLIC FEATURES
	//--------------------------------------------------------------------
	about.MountRoutes(r, h)
	/*
		home.MountRoutes(r, h)

		contact.MountRoutes(r, h)
		login.MountRoutes(r, h)
		logout.MountRoutes(r, h)
		health.MountRoutes(r, h)
		oauth.MountRoutes(r, h)
	*/

	//--------------------------------------------------------------------
	// AUTH-PROTECTED AREA
	//--------------------------------------------------------------------
	authed := chi.NewRouter()
	authed.Use(gwcMid.RequireAuth(h.Session))

	/*
		dashboard.MountRoutes(authed, h)
		games.MountRoutes(authed, h)
		game.MountRoutes(authed, h)
	*/

	r.Mount("/", authed)
}
