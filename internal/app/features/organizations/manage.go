// internal/app/features/organizations/manage.go
package organizations

import (
	"context"
	"net/http"

	uierrors "github.com/dalemusser/stratahub/internal/app/features/errors"
	organizationstore "github.com/dalemusser/stratahub/internal/app/store/organizations"
	"github.com/dalemusser/stratahub/internal/app/system/authz"
	"github.com/dalemusser/stratahub/internal/app/system/navigation"
	"github.com/dalemusser/stratahub/internal/app/system/timeouts"
	"github.com/dalemusser/waffle/pantry/templates"
	"github.com/go-chi/chi/v5"
	"github.com/gorilla/csrf"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
)

// ServeManageModal renders the HTMX Manage Organization modal snippet.
// Authorization: RequireRole("admin", "coordinator") middleware in routes.go.
// Coordinators can only manage organizations they are assigned to.
func (h *Handler) ServeManageModal(w http.ResponseWriter, r *http.Request) {
	role, _, _, _ := authz.UserCtx(r)

	idHex := chi.URLParam(r, "id")
	oid, err := primitive.ObjectIDFromHex(idHex)
	if err != nil {
		uierrors.RenderBadRequest(w, r, "Invalid organization ID.", "/organizations")
		return
	}

	// Verify coordinator has access to this org
	if role == "coordinator" && !coordinatorHasAccess(r, oid) {
		uierrors.RenderForbidden(w, r, "You don't have access to this organization.", "/organizations")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), timeouts.Short())
	defer cancel()

	db := h.DB

	orgStore := organizationstore.New(db)
	org, err := orgStore.GetByID(ctx, oid)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			uierrors.RenderNotFound(w, r, "Organization not found.", "/organizations")
			return
		}
		h.ErrLog.LogServerError(w, r, "find org for manage modal failed", err, "A database error occurred.", "/organizations")
		return
	}

	back := navigation.SafeBackURL(r, navigation.OrganizationsBackURL)

	data := orgManageModalData{
		ID:        org.ID.Hex(),
		Name:      org.Name,
		BackURL:   back,
		Role:      role,
		CSRFToken: csrf.Token(r),
	}

	// Snippet is defined as {{ define "organization_manage_modal" }} ...
	templates.RenderSnippet(w, "organization_manage_modal", data)
}
