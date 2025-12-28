// internal/app/features/materials/routes.go
package materials

import (
	"github.com/dalemusser/stratahub/internal/app/system/auth"
	"github.com/go-chi/chi/v5"
)

// AdminRoutes mounts all admin material routes under whatever base
// path the caller chooses (typically "/materials" from bootstrap).
//
// Example from bootstrap:
//
//	admin := materials.NewAdminHandler(db, storage, errLog, logger)
//	r.Mount("/materials", materials.AdminRoutes(admin, sessionMgr))
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
		pr.Get("/{id}/download", h.HandleDownload)

		// EDIT
		pr.Get("/{id}/edit", h.ServeEdit)
		pr.Post("/{id}/edit", h.HandleEdit)

		// DELETE
		pr.Post("/{id}/delete", h.HandleDelete)

		// ASSIGNMENT (per material)
		pr.Get("/{id}/assign", h.ServeAssign)
		pr.Get("/{id}/assign/form", h.ServeAssignForm)
		pr.Get("/{id}/assign/leaders", h.ServeAssignLeadersPane)
		pr.Post("/{id}/assign", h.HandleAssign)
		pr.Get("/{id}/assignments", h.ServeAssignmentList)

		// GLOBAL ASSIGNMENTS
		pr.Get("/assignments", h.ServeAllAssignments)
		pr.Get("/assignments/{assignID}/manage_modal", h.ServeAssignmentManageModal)
		pr.Get("/assignments/{assignID}/view", h.ServeAssignmentView)
		pr.Get("/assignments/{assignID}/edit", h.ServeAssignmentEdit)
		pr.Post("/assignments/{assignID}/edit", h.HandleAssignmentEdit)
		pr.Post("/assignments/{assignID}/delete", h.HandleUnassign)

		// MANAGE MODAL (HTMX)
		pr.Get("/{id}/manage_modal", h.ServeManageModal)
	})

	return r
}

// LeaderRoutes mounts the leader-facing material routes ("My Materials")
// under whatever base path the caller chooses (typically "/leader/materials").
//
// Example from bootstrap:
//
//	leader := materials.NewLeaderHandler(db, storage, errLog, logger)
//	r.Mount("/leader/materials", materials.LeaderRoutes(leader, sessionMgr))
func LeaderRoutes(h *LeaderHandler, sm *auth.SessionManager) chi.Router {
	r := chi.NewRouter()

	r.Group(func(pr chi.Router) {
		// Leader-only feature; require a signed-in leader.
		pr.Use(sm.RequireSignedIn)
		pr.Use(sm.RequireRole("leader"))

		// List all materials available to the current leader.
		pr.Get("/", h.ServeListMaterials)

		// View a single material (respecting assignment visibility windows).
		pr.Get("/{materialID}", h.ServeViewMaterial)

		// Download/redirect to file
		pr.Get("/{materialID}/download", h.HandleDownload)
	})

	return r
}
