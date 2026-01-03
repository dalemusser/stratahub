// internal/app/features/groups/routes.go
package groups

import (
	"github.com/dalemusser/stratahub/internal/app/system/auth"
	"github.com/go-chi/chi/v5"
)

func Routes(h *Handler, sm *auth.SessionManager) chi.Router {
	r := chi.NewRouter()

	// Everything under /groups requires authentication
	r.Group(func(pr chi.Router) {
		pr.Use(sm.RequireSignedIn)

		// LIST
		pr.Get("/", h.ServeGroupsList)

		// PICKER (modal for selecting a group)
		pr.Get("/picker", h.ServeGroupPicker)

		// CREATE
		pr.Get("/new", h.ServeNewGroup)
		pr.Post("/", h.HandleCreateGroup)

		// EDIT
		pr.Get("/{id}/edit", h.ServeEditGroup)
		pr.Post("/{id}/edit", h.HandleEditGroup)

		// DELETE
		pr.Post("/{id}/delete", h.HandleDeleteGroup)

		// VIEW
		pr.Get("/{id}/view", h.ServeGroupView)

		// RESOURCE VIEW (group → single resource)
		pr.Get("/{id}/resources/{resourceID}", h.ServeGroupResourceView)

		// MANAGE MODAL
		pr.Get("/{id}/manage_modal", h.ServeGroupManageModal)

		// MANAGE (leaders/members)
		pr.Get("/{id}/manage", h.ServeManageGroup)
		pr.Post("/{id}/manage/add-leader", h.HandleAddLeader)
		pr.Post("/{id}/manage/remove-leader", h.HandleRemoveLeader)
		pr.Post("/{id}/manage/add-member", h.HandleAddMember)
		pr.Post("/{id}/manage/remove-member", h.HandleRemoveMember)
		pr.Get("/{id}/manage/search-members", h.ServeSearchMembers)

		// ASSIGN RESOURCES — THIS IS WHAT YOUR 404 NEEDS
		pr.Get("/{id}/assign_resources", h.ServeAssignResources)
		pr.Get("/{id}/assign_resources/new", h.ServeAssignResourceModal)
		pr.Get("/{id}/assign_resources/search-resources", h.ServeSearchResources)
		pr.Get("/{id}/assign_resources/create", h.ServeAssignResourcePage)
		pr.Get("/{id}/assign_resources/{assignmentID}/view", h.ServeViewResourceAssignmentPage)
		pr.Get("/{id}/assign_resources/{assignmentID}/edit", h.ServeEditResourceAssignmentPage)
		pr.Post("/{id}/assign_resources/{assignmentID}/update", h.HandleUpdateResourceAssignment)
		pr.Post("/{id}/assign_resources/add", h.HandleAssignResource)
		pr.Post("/{id}/assign_resources/remove", h.HandleRemoveAssignment)
	})

	return r
}
