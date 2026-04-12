// internal/app/features/mhsbuilds/routes.go
package mhsbuilds

import (
	"github.com/dalemusser/stratahub/internal/app/system/auth"
	"github.com/go-chi/chi/v5"
)

// Routes returns the router for the MHS build management feature, mounted at /mhsbuilds.
func Routes(h *Handler, sm *auth.SessionManager) chi.Router {
	r := chi.NewRouter()
	r.Use(sm.RequireSignedIn)
	r.Use(sm.RequireRole("admin", "superadmin"))

	r.Get("/upload", h.ServeUpload)
	r.Post("/upload", h.HandleUpload)
	r.Post("/upload/confirm", h.HandleUploadConfirm)

	r.Get("/manual", h.ServeManual)
	r.Post("/manual", h.HandleManual)

	r.Get("/storage", h.ServeStorage)
	r.Post("/storage/sync", h.HandleSync)
	r.Post("/storage/delete/{unit}/{version}", h.HandleDeleteBuild)
	r.Post("/storage/build/{id}/identifier", h.HandleEditBuildIdentifier)
	r.Get("/storage/build/{id}/collections_modal", h.ServeBuildCollectionsModal)

	r.Get("/collections", h.ServeCollections)
	r.Get("/collections/{id}", h.ServeCollectionDetail)
	r.Get("/collections/{id}/manage_modal", h.ServeManageModal)
	r.Get("/collections/{id}/assignments", h.ServeAssignments)
	r.Get("/collections/{id}/edit", h.ServeEdit)
	r.Post("/collections/{id}/edit", h.HandleEdit)
	r.Post("/collections/{id}/activate", h.HandleActivateCollection)
	r.Post("/collections/{id}/delete", h.HandleDelete)
	r.Post("/collections/deactivate", h.HandleDeactivateCollection)

	return r
}
