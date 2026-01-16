// internal/app/features/systemusers/modal.go
package systemusers

// Terminology: User Identifiers
//   - UserID / userID / user_id: The MongoDB ObjectID (_id) that uniquely identifies a user record
//   - LoginID / loginID / login_id: The human-readable string users type to log in

import (
	"context"
	"net/http"

	uierrors "github.com/dalemusser/stratahub/internal/app/features/errors"
	userstore "github.com/dalemusser/stratahub/internal/app/store/users"
	"github.com/dalemusser/stratahub/internal/app/system/normalize"
	"github.com/dalemusser/stratahub/internal/app/system/timeouts"
	"github.com/dalemusser/waffle/pantry/templates"
	"github.com/go-chi/chi/v5"
	"github.com/gorilla/csrf"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

// ServeManageModal renders the Manage modal for a single system user.
//
// This is an admin-only endpoint. It is typically invoked via HTMX from
// the system users list page and returns only the modal snippet
// (system_user_manage_modal).
func (h *Handler) ServeManageModal(w http.ResponseWriter, r *http.Request) {
	// Only admins can manage system users.
	_, _, currentUserID, ok := userContext(r)
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

	usrStore := userstore.New(db)
	u, err := usrStore.GetByID(ctx, uid)
	if err != nil {
		uierrors.HTMXNotFound(w, r, "User not found.", "/system-users")
		return
	}

	back := backToSystemUsersURL(r)

	loginID := ""
	if u.LoginID != nil {
		loginID = *u.LoginID
	}

	data := manageModalData{
		ID:        idHex,
		FullName:  u.FullName,
		LoginID:   loginID,
		Role:      normalize.Role(u.Role),
		Auth:      normalize.AuthMethod(u.AuthMethod),
		Status:    normalize.Status(u.Status),
		BackURL:   back,
		CSRFToken: csrf.Token(r),
		IsSelf:    currentUserID == uid,
	}

	templates.RenderSnippet(w, "system_user_manage_modal", data)
}
