// internal/app/features/organizations/manage.go
package organizations

import (
	"context"
	"net/http"

	uierrors "github.com/dalemusser/stratahub/internal/app/features/errors"
	"github.com/dalemusser/stratahub/internal/app/system/navigation"
	"github.com/dalemusser/stratahub/internal/app/system/timeouts"
	"github.com/dalemusser/stratahub/internal/domain/models"
	"github.com/dalemusser/waffle/pantry/templates"
	"github.com/go-chi/chi/v5"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
)

// ServeManageModal renders the HTMX Manage Organization modal snippet.
// Authorization: RequireRole("admin") middleware in routes.go ensures only admins reach this handler.
func (h *Handler) ServeManageModal(w http.ResponseWriter, r *http.Request) {
	idHex := chi.URLParam(r, "id")
	oid, err := primitive.ObjectIDFromHex(idHex)
	if err != nil {
		uierrors.RenderBadRequest(w, r, "Invalid organization ID.", "/organizations")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), timeouts.Short())
	defer cancel()

	db := h.DB

	var org models.Organization
	if err := db.Collection("organizations").
		FindOne(ctx, bson.M{"_id": oid}).
		Decode(&org); err != nil {

		if err == mongo.ErrNoDocuments {
			uierrors.RenderNotFound(w, r, "Organization not found.", "/organizations")
			return
		}
		h.ErrLog.LogServerError(w, r, "find org for manage modal failed", err, "A database error occurred.", "/organizations")
		return
	}

	back := navigation.SafeBackURL(r, navigation.OrganizationsBackURL)

	data := orgManageModalData{
		ID:      org.ID.Hex(),
		Name:    org.Name,
		BackURL: back,
	}

	// Snippet is defined as {{ define "organization_manage_modal" }} ...
	templates.RenderSnippet(w, "organization_manage_modal", data)
}
