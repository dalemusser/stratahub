// internal/app/features/organizations/manage.go
package organizations

import (
	"context"
	"net/http"

	"github.com/dalemusser/stratahub/internal/domain/models"
	"github.com/dalemusser/waffle/templates"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.uber.org/zap"

	"github.com/go-chi/chi/v5"
)

// ServeManageModal renders the HTMX Manage Organization modal snippet.
//
// Route: GET /organizations/{id}/manage_modal
func (h *Handler) ServeManageModal(w http.ResponseWriter, r *http.Request) {
	// Router should already have auth + RequireRole("admin"), so we
	// don’t re-check here. If you want belt-and-suspenders, you can
	// call authz.UserCtx and ensure role == "admin".

	idHex := chi.URLParam(r, "id")
	oid, err := primitive.ObjectIDFromHex(idHex)
	if err != nil {
		http.Error(w, "bad organization id", http.StatusBadRequest)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), orgsShortTimeout)
	defer cancel()

	db := h.DB

	var org models.Organization
	if err := db.Collection("organizations").
		FindOne(ctx, bson.M{"_id": oid}).
		Decode(&org); err != nil {

		if err == mongo.ErrNoDocuments {
			http.NotFound(w, r)
			return
		}
		h.Log.Error("find org for manage modal failed", zap.Error(err))
		http.Error(w, "database error", http.StatusInternalServerError)
		return
	}

	back := r.URL.Query().Get("return")
	if back == "" {
		back = "/organizations"
	}

	data := orgManageModalData{
		ID:      org.ID.Hex(),
		Name:    org.Name,
		BackURL: back,
	}

	// Snippet is defined as {{ define "organization_manage_modal" }} ...
	templates.RenderSnippet(w, "organization_manage_modal", data)
}
