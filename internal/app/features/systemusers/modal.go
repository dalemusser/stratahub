// internal/app/features/systemusers/modal.go
package systemusers

import (
	"context"
	"net/http"
	"strings"

	"github.com/dalemusser/stratahub/internal/domain/models"
	"github.com/dalemusser/waffle/templates"
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
	_, _, _, ok := requireAdmin(w, r)
	if !ok {
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), sysUsersShortTimeout)
	defer cancel()
	db := h.DB

	idHex := chi.URLParam(r, "id")
	uid, err := primitive.ObjectIDFromHex(idHex)
	if err != nil {
		http.Error(w, "bad id", http.StatusBadRequest)
		return
	}

	var u models.User
	if err := db.Collection("users").
		FindOne(ctx, bson.M{"_id": uid}).
		Decode(&u); err != nil {

		http.NotFound(w, r)
		return
	}

	back := backToSystemUsersURL(r)

	data := manageModalData{
		ID:       idHex,
		FullName: u.FullName,
		Email:    strings.ToLower(u.Email),
		Role:     strings.ToLower(u.Role),
		Auth:     strings.ToLower(u.AuthMethod),
		Status:   strings.ToLower(u.Status),
		BackURL:  back,
	}

	templates.RenderSnippet(w, "system_user_manage_modal", data)
}
