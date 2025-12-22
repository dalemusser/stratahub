// internal/app/features/systemusers/edit.go
package systemusers

import (
	"context"
	"html/template"
	"net/http"
	"strings"
	"time"

	uierrors "github.com/dalemusser/stratahub/internal/app/features/errors"
	"github.com/dalemusser/stratahub/internal/app/system/authz"
	"github.com/dalemusser/stratahub/internal/app/system/inputval"
	"github.com/dalemusser/stratahub/internal/app/system/navigation"
	"github.com/dalemusser/stratahub/internal/app/system/normalize"
	"github.com/dalemusser/stratahub/internal/app/system/timeouts"
	"github.com/dalemusser/stratahub/internal/domain/models"
	"github.com/dalemusser/waffle/pantry/httpnav"
	wafflemongo "github.com/dalemusser/waffle/pantry/mongo"
	"github.com/dalemusser/waffle/pantry/templates"
	"github.com/dalemusser/waffle/pantry/text"

	"github.com/go-chi/chi/v5"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
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
	role, uname, uid, ok := authz.UserCtx(r)
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

	var u models.User
	if err := db.Collection("users").FindOne(ctx, bson.M{"_id": uidParam}).Decode(&u); err != nil {
		uierrors.RenderNotFound(w, r, "User not found.", "/system-users")
		return
	}

	isSelf := uid == u.ID

	data := formData{
		Title:       "Edit System User",
		IsLoggedIn:  true,
		Role:        role,
		UserName:    uname,
		ID:          u.ID.Hex(),
		FullName:    u.FullName,
		Email:       normalize.Email(u.Email),
		URole:       normalize.Role(u.Role),
		UserRole:    normalize.Role(u.Role),
		Auth:        normalize.AuthMethod(u.AuthMethod),
		Status:      normalize.Status(u.Status),
		IsSelf:      isSelf,
		BackURL:     httpnav.ResolveBackURL(r, "/system-users"),
		CurrentPath: httpnav.CurrentPath(r),
	}

	templates.Render(w, r, "system_user_edit", data)
}

// HandleEdit processes the Edit System User form POST.
func (h *Handler) HandleEdit(w http.ResponseWriter, r *http.Request) {
	role, uname, who, ok := userContext(r) // who = current user's ObjectID
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
		renderEditForm(w, r, role, uname, uid.Hex(), full, email, userRole, authm, status, isSelf,
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
		renderEditForm(w, r, role, uname, uid.Hex(), full, email, userRole, authm, status, isSelf,
			template.HTML(result.First()))
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), timeouts.Medium())
	defer cancel()
	db := h.DB

	// Duplicate email check (exclude self)
	if err := db.Collection("users").FindOne(ctx, bson.M{
		"email": email,
		"_id":   bson.M{"$ne": uid},
	}).Err(); err == nil {
		renderEditForm(w, r, role, uname, uid.Hex(), full, email, userRole, authm, status, isSelf,
			template.HTML("A user with that email already exists."))
		return
	}

	up := bson.M{
		"full_name":    full,
		"full_name_ci": text.Fold(full),
		"email":        email,
		"role":         userRole,
		"auth_method":  authm,
		"status":       status,
		"updated_at":   time.Now(),
	}

	if _, err := db.Collection("users").UpdateOne(ctx, bson.M{"_id": uid}, bson.M{"$set": up}); err != nil {
		if wafflemongo.IsDup(err) {
			renderEditForm(w, r, role, uname, uid.Hex(), full, email, userRole, authm, status, isSelf,
				template.HTML("A user with that email already exists."))
			return
		}
		renderEditForm(w, r, role, uname, uid.Hex(), full, email, userRole, authm, status, isSelf,
			template.HTML("Database error while updating system user."))
		return
	}

	ret := navigation.SafeBackURL(r, navigation.SystemUsersBackURL)
	http.Redirect(w, r, ret, http.StatusSeeOther)
}

// HandleDelete deletes a system user, enforcing safety guards
// (cannot delete self, cannot delete last active admin).
func (h *Handler) HandleDelete(w http.ResponseWriter, r *http.Request) {
	role, uname, who, ok := userContext(r) // who = current user's ObjectID
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

	var u models.User
	if err := db.Collection("users").FindOne(ctx, bson.M{"_id": uid}).Decode(&u); err != nil {
		http.NotFound(w, r)
		return
	}

	isSelf := who == uid

	// Guard 1: prevent an admin from deleting themself.
	if isSelf && strings.EqualFold(u.Role, "admin") {
		renderEditForm(w, r, role, uname, idHex,
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
			renderEditForm(w, r, role, uname, idHex,
				u.FullName, normalize.Email(u.Email), normalize.Role(u.Role), normalize.AuthMethod(u.AuthMethod), normalize.Status(u.Status),
				isSelf, template.HTML("There must be at least one active admin in the system."))
			return
		}
	}

	if _, err := db.Collection("users").DeleteOne(ctx, bson.M{"_id": uid}); err != nil {
		h.ErrLog.LogServerError(w, r, "database error deleting system user", err, "A database error occurred.", "/system-users")
		return
	}

	http.Redirect(w, r, backToSystemUsersURL(r), http.StatusSeeOther)
}
