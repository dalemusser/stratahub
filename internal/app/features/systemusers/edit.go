// internal/app/features/systemusers/edit.go
package systemusers

import (
	"context"
	"html/template"
	"net/http"
	"strings"

	uierrors "github.com/dalemusser/stratahub/internal/app/features/errors"
	userstore "github.com/dalemusser/stratahub/internal/app/store/users"
	"github.com/dalemusser/stratahub/internal/app/system/authz"
	"github.com/dalemusser/stratahub/internal/app/system/inputval"
	"github.com/dalemusser/stratahub/internal/app/system/navigation"
	"github.com/dalemusser/stratahub/internal/app/system/normalize"
	"github.com/dalemusser/stratahub/internal/app/system/timeouts"
	"github.com/dalemusser/stratahub/internal/app/system/viewdata"
	"github.com/dalemusser/waffle/pantry/templates"
	"github.com/go-chi/chi/v5"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
)

// editSystemUserInput defines validation rules for editing a system user.
type editSystemUserInput struct {
	FullName   string `validate:"required,max=200" label:"Full name"`
	Email      string `validate:"required,email,max=254" label:"Email"`
	Role       string `validate:"required,oneof=admin analyst" label:"Role"`
	AuthMethod string `validate:"required,authmethod" label:"Auth method"`
	Status     string `validate:"required,oneof=active disabled" label:"Status"`
}

// ServeEdit renders the Edit System User form.
func (h *Handler) ServeEdit(w http.ResponseWriter, r *http.Request) {
	// Viewer context for header/sidebar; actual edit is admin-gated in HandleEdit.
	_, _, uid, ok := authz.UserCtx(r)
	if !ok {
		uierrors.RenderUnauthorized(w, r, "/login")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), timeouts.Short())
	defer cancel()
	db := h.DB

	uidHex := chi.URLParam(r, "id")
	uidParam, err := primitive.ObjectIDFromHex(uidHex)
	if err != nil {
		uierrors.RenderBadRequest(w, r, "Invalid user ID.", "/system-users")
		return
	}

	usrStore := userstore.New(db)
	uptr, err := usrStore.GetByID(ctx, uidParam)
	if err != nil {
		uierrors.RenderNotFound(w, r, "User not found.", "/system-users")
		return
	}
	u := *uptr

	isSelf := uid == u.ID

	data := formData{
		BaseVM:   viewdata.NewBaseVM(r, h.DB, "Edit System User", "/system-users"),
		ID:       u.ID.Hex(),
		FullName: u.FullName,
		Email:    normalize.Email(u.Email),
		URole:    normalize.Role(u.Role),
		UserRole: normalize.Role(u.Role),
		Auth:     normalize.AuthMethod(u.AuthMethod),
		Status:   normalize.Status(u.Status),
		IsSelf:   isSelf,
	}

	templates.Render(w, r, "system_user_edit", data)
}

// HandleEdit processes the Edit System User form POST.
func (h *Handler) HandleEdit(w http.ResponseWriter, r *http.Request) {
	_, _, who, ok := userContext(r) // who = current user's ObjectID
	if !ok {
		return
	}

	if err := r.ParseForm(); err != nil {
		h.ErrLog.LogBadRequest(w, r, "parse form failed", err, "Invalid form data.", "/system-users")
		return
	}

	uidHex := chi.URLParam(r, "id")
	uid, err := primitive.ObjectIDFromHex(uidHex)
	if err != nil {
		uierrors.RenderBadRequest(w, r, "Invalid user ID.", "/system-users")
		return
	}

	full := normalize.Name(r.FormValue("full_name"))
	email := normalize.Email(r.FormValue("email"))
	userRole := normalize.Role(r.FormValue("role"))
	authm := normalize.AuthMethod(r.FormValue("auth_method"))
	status := normalize.Status(r.FormValue("status"))

	isSelf := who == uid

	// Prevent an admin from changing their own role or status.
	if isSelf && (userRole != "admin" || status != "active") {
		renderEditForm(w, r, h.DB, uid.Hex(), full, email, userRole, authm, status, isSelf,
			template.HTML("You can't change your own role or status. Ask another admin to make those changes."))
		return
	}

	// Validate required fields using struct tags
	input := editSystemUserInput{
		FullName:   full,
		Email:      email,
		Role:       userRole,
		AuthMethod: authm,
		Status:     status,
	}
	if result := inputval.Validate(input); result.HasErrors() {
		renderEditForm(w, r, h.DB, uid.Hex(), full, email, userRole, authm, status, isSelf,
			template.HTML(result.First()))
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), timeouts.Medium())
	defer cancel()
	db := h.DB

	// Update system user via store (handles CI fields, timestamps, and duplicate detection)
	usrStore := userstore.New(db)

	// Check for duplicate email (exclude self)
	exists, err := usrStore.EmailExistsForOther(ctx, email, uid)
	if err != nil {
		renderEditForm(w, r, h.DB, uid.Hex(), full, email, userRole, authm, status, isSelf,
			template.HTML("Database error while checking email."))
		return
	}
	if exists {
		renderEditForm(w, r, h.DB, uid.Hex(), full, email, userRole, authm, status, isSelf,
			template.HTML("A user with that email already exists."))
		return
	}

	upd := userstore.SystemUserUpdate{
		FullName:   full,
		Email:      email,
		AuthMethod: authm,
		Role:       userRole,
		Status:     status,
	}

	if err := usrStore.UpdateSystemUser(ctx, uid, upd); err != nil {
		msg := "Database error while updating system user."
		if err == userstore.ErrDuplicateEmail {
			msg = "A user with that email already exists."
		}
		renderEditForm(w, r, h.DB, uid.Hex(), full, email, userRole, authm, status, isSelf,
			template.HTML(msg))
		return
	}

	ret := navigation.SafeBackURL(r, navigation.SystemUsersBackURL)
	http.Redirect(w, r, ret, http.StatusSeeOther)
}

// HandleDelete deletes a system user, enforcing safety guards
// (cannot delete self, cannot delete last active admin).
func (h *Handler) HandleDelete(w http.ResponseWriter, r *http.Request) {
	_, _, who, ok := userContext(r) // who = current user's ObjectID
	if !ok {
		return
	}

	idHex := chi.URLParam(r, "id")
	uid, err := primitive.ObjectIDFromHex(idHex)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), timeouts.Medium())
	defer cancel()
	db := h.DB

	usrStore := userstore.New(db)
	uptr, err := usrStore.GetByID(ctx, uid)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			http.NotFound(w, r)
			return
		}
		h.ErrLog.LogServerError(w, r, "database error loading user", err, "A database error occurred.", "/system-users")
		return
	}
	u := *uptr

	isSelf := who == uid

	// Guard 1: prevent an admin from deleting themself.
	if isSelf && strings.EqualFold(u.Role, "admin") {
		renderEditForm(w, r, h.DB, idHex,
			u.FullName, normalize.Email(u.Email), normalize.Role(u.Role), normalize.AuthMethod(u.AuthMethod), normalize.Status(u.Status),
			isSelf, template.HTML("You can't delete your own admin account. Ask another admin to remove it."))
		return
	}

	// Guard 2: do not allow deleting the last active admin.
	if strings.EqualFold(u.Role, "admin") && strings.EqualFold(u.Status, "active") {
		cnt, err := countActiveAdmins(ctx, db)
		if err != nil {
			h.ErrLog.LogServerError(w, r, "database error counting active admins", err, "A database error occurred.", "/system-users")
			return
		}
		if cnt <= 1 {
			renderEditForm(w, r, h.DB, idHex,
				u.FullName, normalize.Email(u.Email), normalize.Role(u.Role), normalize.AuthMethod(u.AuthMethod), normalize.Status(u.Status),
				isSelf, template.HTML("There must be at least one active admin in the system."))
			return
		}
	}

	if _, err := usrStore.DeleteSystemUser(ctx, uid); err != nil {
		h.ErrLog.LogServerError(w, r, "database error deleting system user", err, "A database error occurred.", "/system-users")
		return
	}

	http.Redirect(w, r, backToSystemUsersURL(r), http.StatusSeeOther)
}
