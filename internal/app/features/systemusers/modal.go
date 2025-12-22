// internal/app/features/systemusers/modal.go
package systemusers

import (
	"context"
	"net/http"

	uierrors "github.com/dalemusser/stratahub/internal/app/features/errors"
	"github.com/dalemusser/stratahub/internal/app/system/normalize"
	"github.com/dalemusser/stratahub/internal/app/system/timeouts"
	"github.com/dalemusser/stratahub/internal/domain/models"
	"github.com/dalemusser/waffle/pantry/templates"
	"github.com/go-chi/chi/v5"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

// ServeManageModal renders the Manage modal for a single system user.
//
// This is an admin-only endpoint. It is typically invoked via HTMX from
// the system users list page and returns only the modal snippet
// (system_user_manage_modal).
func (h *Handler) ServeManageModal(w http.ResponseWriter, r *http.Request) {
	// Only admins can manage system users.
	_, _, _, ok := userContext(r)
	if !ok {
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), timeouts.Short())
	defer cancel()
	db := h.DB

	idHex := chi.URLParam(r, "id")
	uid, err := primitive.ObjectIDFromHex(idHex)
	if err != nil {
		uierrors.HTMXBadRequest(w, r, "Invalid user ID.", "/system-users")
		return
	}

	var u models.User
	if err := db.Collection("users").
		FindOne(ctx, bson.M{"_id": uid}).
		Decode(&u); err != nil {

		uierrors.HTMXNotFound(w, r, "User not found.", "/system-users")
		return
	}

	back := backToSystemUsersURL(r)

	data := manageModalData{
		ID:       idHex,
		FullName: u.FullName,
		Email:    normalize.Email(u.Email),
		Role:     normalize.Role(u.Role),
		Auth:     normalize.AuthMethod(u.AuthMethod),
		Status:   normalize.Status(u.Status),
		BackURL:  back,
	}

	templates.RenderSnippet(w, "system_user_manage_modal", data)
}
