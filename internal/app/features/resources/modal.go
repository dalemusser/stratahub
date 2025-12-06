// internal/app/features/resources/modal.go
package resources

import (
	"context"
	"net/http"
	"strings"

	uierrors "github.com/dalemusser/stratahub/internal/app/features/errors"
	"github.com/dalemusser/waffle/templates"
	nav "github.com/dalemusser/waffle/toolkit/ui/nav"
	"github.com/go-chi/chi/v5"

	"github.com/dalemusser/stratahub/internal/app/system/authz"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.uber.org/zap"
)

// ServeManageModal renders the Manage Resource modal for a single resource.
// It is used by the admin library UI to offer View/Edit/Delete actions.
func (h *AdminHandler) ServeManageModal(w http.ResponseWriter, r *http.Request) {
	role, _, _, ok := authz.UserCtx(r)
	if !ok {
		uierrors.RenderUnauthorized(w, r, "/login")
		return
	}
	if strings.ToLower(role) != "admin" {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}

	idHex := chi.URLParam(r, "id")
	oid, err := primitive.ObjectIDFromHex(idHex)
	if err != nil {
		http.Error(w, "bad id", http.StatusBadRequest)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), resourcesShortTimeout)
	defer cancel()

	var row struct {
		ID            primitive.ObjectID `bson:"_id"`
		Title         string             `bson:"title"`
		Subject       string             `bson:"subject"`
		Type          string             `bson:"type"`
		Status        string             `bson:"status"`
		ShowInLibrary bool               `bson:"show_in_library"`
		Description   string             `bson:"description"`
	}

	if err := h.DB.Collection("resources").FindOne(ctx, bson.M{"_id": oid}).Decode(&row); err != nil {
		if err == mongo.ErrNoDocuments {
			http.NotFound(w, r)
			return
		}
		h.Log.Error("resource FindOne(manage modal)", zap.Error(err))
		http.Error(w, "database error", http.StatusInternalServerError)
		return
	}

	back := r.URL.Query().Get("return")
	if back == "" {
		back = nav.ResolveBackURL(r, "/resources")
	}

	vm := manageModalData{
		ID:            row.ID.Hex(),
		Title:         row.Title,
		Subject:       row.Subject,
		Type:          row.Type,
		Status:        row.Status,
		ShowInLibrary: row.ShowInLibrary,
		Description:   row.Description,
		BackURL:       back,
	}

	templates.RenderSnippet(w, "resource_manage_modal", vm)
}
