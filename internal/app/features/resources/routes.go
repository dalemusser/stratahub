// internal/app/features/resources/routes.go
package resources

import (
	"github.com/dalemusser/stratahub/internal/app/system/auth"
	"github.com/go-chi/chi/v5"
)

// AdminRoutes mounts all admin/library resource routes under whatever base
// path the caller chooses (typically "/resources" from bootstrap).
//
// Example from bootstrap:
//
//	admin := resources.NewAdminHandler(db, logger)
//	r.Mount("/resources", resources.AdminRoutes(admin, sessionMgr))
func AdminRoutes(h *AdminHandler, sm *auth.SessionManager) chi.Router {
	r := chi.NewRouter()

	r.Group(func(pr chi.Router) {
		// Admin-only feature; require a signed-in admin.
		pr.Use(sm.RequireSignedIn)
		pr.Use(sm.RequireRole("admin"))

		// LIST (live search + HTMX table swap)
		pr.Get("/", h.ServeList)

		// CREATE
		pr.Get("/new", h.ServeNew)
		pr.Post("/", h.HandleCreate)

		// VIEW
		pr.Get("/{id}/view", h.ServeView)

		// DOWNLOAD
		pr.Get("/{id}/download", h.HandleDownload)

		// EDIT
		pr.Get("/{id}/edit", h.ServeEdit)
		pr.Post("/{id}/edit", h.HandleEdit)

		// DELETE
		pr.Post("/{id}/delete", h.HandleDelete)

		// MANAGE MODAL (HTMX)
		pr.Get("/{id}/manage_modal", h.ServeManageModal)
	})

	return r
}

// MemberRoutes mounts the member-facing resource routes ("My Resources")
// under whatever base path the caller chooses (typically "/member/resources").
//
// Example from bootstrap:
//
//	member := resources.NewMemberHandler(db, logger)
//	r.Mount("/member/resources", resources.MemberRoutes(member, sessionMgr))
func MemberRoutes(h *MemberHandler, sm *auth.SessionManager) chi.Router {
	r := chi.NewRouter()

	r.Group(func(pr chi.Router) {
		// Member-only feature; require a signed-in member.
		pr.Use(sm.RequireSignedIn)
		pr.Use(sm.RequireRole("member"))

		// List all resources available to the current member.
		pr.Get("/", h.ServeListResources)

		// View a single resource (respecting assignment visibility windows).
		pr.Get("/{resourceID}", h.ServeViewResource)

		// Download a resource file (respecting assignment visibility windows).
		pr.Get("/{resourceID}/download", h.HandleDownload)
	})

	return r
}
