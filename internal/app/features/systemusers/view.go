// internal/app/features/systemusers/view.go
package systemusers

import (
	"context"
	"net/http"

	uierrors "github.com/dalemusser/stratahub/internal/app/features/errors"
	"github.com/dalemusser/stratahub/internal/app/system/normalize"
	"github.com/dalemusser/stratahub/internal/app/system/timeouts"
	"github.com/dalemusser/stratahub/internal/domain/models"
	"github.com/dalemusser/waffle/pantry/httpnav"
	"github.com/dalemusser/waffle/pantry/templates"

	"github.com/go-chi/chi/v5"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

// ServeView renders a read-only view of a single system user.
//
// This endpoint is admin-only, enforced via requireAdmin. It is
// typically linked from the system users list and the Manage modal.
func (h *Handler) ServeView(w http.ResponseWriter, r *http.Request) {
	role, uname, _, ok := userContext(r)
	if !ok {
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), timeouts.Short())
	defer cancel()
	db := h.DB

	idHex := chi.URLParam(r, "id")
	uid, err := primitive.ObjectIDFromHex(idHex)
	if err != nil {
		uierrors.RenderBadRequest(w, r, "Invalid user ID.", "/system-users")
		return
	}

	var u models.User
	if err := db.Collection("users").FindOne(ctx, bson.M{"_id": uid}).Decode(&u); err != nil {
		uierrors.RenderNotFound(w, r, "User not found.", "/system-users")
		return
	}

	templates.Render(w, r, "system_user_view", viewData{
		Title:       "View User",
		IsLoggedIn:  true,
		Role:        role,
		UserName:    uname,
		ID:          idHex,
		FullName:    u.FullName,
		Email:       normalize.Email(u.Email),
		URole:       normalize.Role(u.Role),
		UserRole:    normalize.Role(u.Role),
		Auth:        normalize.AuthMethod(u.AuthMethod),
		Status:      normalize.Status(u.Status),
		BackURL:     backToSystemUsersURL(r),
		CurrentPath: httpnav.CurrentPath(r),
	})
}
