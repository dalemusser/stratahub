// internal/app/features/resources/modal.go
package resources

import (
	"context"
	"net/http"

	uierrors "github.com/dalemusser/stratahub/internal/app/features/errors"
	"github.com/dalemusser/stratahub/internal/app/system/authz"
	"github.com/dalemusser/stratahub/internal/app/system/timeouts"
	"github.com/dalemusser/waffle/pantry/httpnav"
	"github.com/dalemusser/waffle/pantry/templates"
	"github.com/go-chi/chi/v5"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
)

// ServeManageModal renders the Manage Resource modal for a single resource.
// It is used by the admin library UI to offer View/Edit/Delete actions.
// Coordinators can view resources but cannot edit/delete them.
func (h *AdminHandler) ServeManageModal(w http.ResponseWriter, r *http.Request) {
	role, _, _, ok := authz.UserCtx(r)
	if !ok {
		uierrors.RenderUnauthorized(w, r, "/login")
		return
	}
	// SuperAdmin, Admin, and Coordinator can access; others are forbidden
	if role != "superadmin" && role != "admin" && role != "coordinator" {
		uierrors.HTMXForbidden(w, r, "You do not have access to manage resources.", "/resources")
		return
	}

	idHex := chi.URLParam(r, "id")
	oid, err := primitive.ObjectIDFromHex(idHex)
	if err != nil {
		uierrors.HTMXBadRequest(w, r, "Invalid resource ID.", "/resources")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), timeouts.Short())
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
			uierrors.HTMXNotFound(w, r, "Resource not found.", "/resources")
			return
		}
		h.ErrLog.HTMXLogServerError(w, r, "resource FindOne(manage modal)", err, "A database error occurred.", "/resources")
		return
	}

	back := r.URL.Query().Get("return")
	if back == "" {
		back = httpnav.ResolveBackURL(r, "/resources")
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
		CanEdit:       authz.CanManageResources(r),
	}

	templates.RenderSnippet(w, "resource_manage_modal", vm)
}
