// internal/app/features/systemusers/view.go
package systemusers

import (
	"context"
	"net/http"
	"strings"

	"github.com/dalemusser/stratahub/internal/domain/models"
	"github.com/dalemusser/waffle/templates"
	nav "github.com/dalemusser/waffle/toolkit/ui/nav"

	"github.com/go-chi/chi/v5"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

// ServeView renders a read-only view of a single system user.
//
// This endpoint is admin-only, enforced via requireAdmin. It is
// typically linked from the system users list and the Manage modal.
func (h *Handler) ServeView(w http.ResponseWriter, r *http.Request) {
	role, uname, _, ok := requireAdmin(w, r)
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
	if err := db.Collection("users").FindOne(ctx, bson.M{"_id": uid}).Decode(&u); err != nil {
		http.NotFound(w, r)
		return
	}

	templates.Render(w, r, "system_user_view", viewData{
		Title:       "View User",
		IsLoggedIn:  true,
		Role:        role,
		UserName:    uname,
		ID:          idHex,
		FullName:    u.FullName,
		Email:       strings.ToLower(u.Email),
		URole:       strings.ToLower(u.Role),
		UserRole:    strings.ToLower(u.Role),
		Auth:        strings.ToLower(u.AuthMethod),
		Status:      strings.ToLower(u.Status),
		BackURL:     backToSystemUsersURL(r),
		CurrentPath: nav.CurrentPath(r),
	})
}
