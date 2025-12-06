// internal/app/features/systemusers/edit.go
package systemusers

import (
	"context"
	"html/template"
	"net/http"
	"strings"
	"time"

	uierrors "github.com/dalemusser/stratahub/internal/app/features/errors"
	"github.com/dalemusser/stratahub/internal/domain/models"
	"github.com/dalemusser/waffle/templates"
	textfold "github.com/dalemusser/waffle/toolkit/text/textfold"
	nav "github.com/dalemusser/waffle/toolkit/ui/nav"
	validate "github.com/dalemusser/waffle/toolkit/validate"

	mongodb "github.com/dalemusser/waffle/toolkit/db/mongodb"

	"github.com/dalemusser/stratahub/internal/app/system/authz"
	"github.com/go-chi/chi/v5"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

// ServeEdit renders the Edit System User form.
func (h *Handler) ServeEdit(w http.ResponseWriter, r *http.Request) {
	// Viewer context for header/sidebar; actual edit is admin-gated in HandleEdit.
	role, uname, uid, ok := authz.UserCtx(r)
	if !ok {
		uierrors.RenderUnauthorized(w, r, "/login")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), sysUsersShortTimeout)
	defer cancel()
	db := h.DB

	uidHex := chi.URLParam(r, "id")
	uidParam, err := primitive.ObjectIDFromHex(uidHex)
	if err != nil {
		http.Error(w, "bad id", http.StatusBadRequest)
		return
	}

	var u models.User
	if err := db.Collection("users").FindOne(ctx, bson.M{"_id": uidParam}).Decode(&u); err != nil {
		http.NotFound(w, r)
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
		Email:       strings.ToLower(u.Email),
		URole:       strings.ToLower(u.Role),
		UserRole:    strings.ToLower(u.Role),
		Auth:        strings.ToLower(u.AuthMethod),
		Status:      strings.ToLower(u.Status),
		IsSelf:      isSelf,
		BackURL:     nav.ResolveBackURL(r, "/system-users"),
		CurrentPath: nav.CurrentPath(r),
	}

	templates.Render(w, r, "system_user_edit", data)
}

// HandleEdit processes the Edit System User form POST.
func (h *Handler) HandleEdit(w http.ResponseWriter, r *http.Request) {
	role, uname, who, ok := requireAdmin(w, r) // who = current user's ObjectID
	if !ok {
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad form", http.StatusBadRequest)
		return
	}

	uidHex := chi.URLParam(r, "id")
	uid, err := primitive.ObjectIDFromHex(uidHex)
	if err != nil {
		http.Error(w, "bad id", http.StatusBadRequest)
		return
	}

	full := strings.TrimSpace(r.FormValue("full_name"))
	email := strings.ToLower(strings.TrimSpace(r.FormValue("email")))
	userRole := strings.ToLower(strings.TrimSpace(r.FormValue("role")))
	authm := strings.ToLower(strings.TrimSpace(r.FormValue("auth_method")))
	status := strings.ToLower(strings.TrimSpace(r.FormValue("status")))

	reRender := func(msg string) {
		templates.Render(w, r, "system_user_edit", formData{
			Title:       "Edit System User",
			IsLoggedIn:  true,
			Role:        role,
			UserName:    uname,
			ID:          uid.Hex(),
			FullName:    full,
			Email:       email,
			URole:       userRole,
			UserRole:    userRole,
			Auth:        authm,
			Status:      status,
			IsSelf:      who == uid,
			Error:       template.HTML(msg),
			BackURL:     nav.ResolveBackURL(r, "/system-users"),
			CurrentPath: nav.CurrentPath(r),
		})
	}

	// Prevent an admin from changing their own role or status.
	if who == uid && (userRole != "admin" || status != "active") {
		reRender("You can’t change your own role or status. Ask another admin to make those changes.")
		return
	}

	// Validation (presence + whitelist)
	var errs []string
	if full == "" {
		errs = append(errs, "Full Name is required.")
	}
	if email == "" {
		errs = append(errs, "Email is required.")
	} else if !validate.SimpleEmailValid(email) {
		errs = append(errs, "Email format is invalid.")
	}
	switch userRole {
	case "admin", "analyst":
	default:
		if userRole == "" {
			errs = append(errs, "Role is required.")
		} else {
			errs = append(errs, "Role must be admin or analyst.")
		}
	}
	switch authm {
	case "internal", "google", "classlink", "clever", "microsoft":
	default:
		if authm == "" {
			errs = append(errs, "Auth Method is required.")
		} else {
			errs = append(errs, "Auth Method must be internal, google, classlink, clever, or microsoft.")
		}
	}
	switch status {
	case "active", "disabled":
	default:
		if status == "" {
			errs = append(errs, "Status is required.")
		} else {
			errs = append(errs, "Status must be active or disabled.")
		}
	}

	if len(errs) > 0 {
		reRender(strings.Join(errs, " "))
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), sysUsersMedTimeout)
	defer cancel()
	db := h.DB

	// Duplicate email check (exclude self)
	if err := db.Collection("users").FindOne(ctx, bson.M{
		"email": email,
		"_id":   bson.M{"$ne": uid},
	}).Err(); err == nil {
		reRender("A user with that email already exists.")
		return
	}

	up := bson.M{
		"full_name":    full,
		"full_name_ci": textfold.Fold(full),
		"email":        email,
		"role":         userRole,
		"auth_method":  authm,
		"status":       status,
		"updated_at":   time.Now(),
	}

	if _, err := db.Collection("users").UpdateOne(ctx, bson.M{"_id": uid}, bson.M{"$set": up}); err != nil {
		if mongodb.IsDup(err) {
			reRender("A user with that email already exists.")
			return
		}
		reRender("Database error while updating system user.")
		return
	}

	if ret := r.FormValue("return"); ret != "" && strings.HasPrefix(ret, "/") {
		http.Redirect(w, r, ret, http.StatusSeeOther)
		return
	}
	http.Redirect(w, r, "/system-users", http.StatusSeeOther)
}

// HandleDelete deletes a system user, enforcing safety guards
// (cannot delete self, cannot delete last active admin).
func (h *Handler) HandleDelete(w http.ResponseWriter, r *http.Request) {
	role, uname, who, ok := requireAdmin(w, r) // who = current user's ObjectID
	if !ok {
		return
	}

	idHex := chi.URLParam(r, "id")
	uid, err := primitive.ObjectIDFromHex(idHex)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), sysUsersMedTimeout)
	defer cancel()
	db := h.DB

	var u models.User
	if err := db.Collection("users").FindOne(ctx, bson.M{"_id": uid}).Decode(&u); err != nil {
		http.NotFound(w, r)
		return
	}

	// Guard 1: prevent an admin from deleting themself.
	if who == uid && strings.EqualFold(u.Role, "admin") {
		renderEditForm(w, r, role, uname, idHex,
			u.FullName, strings.ToLower(u.Email), strings.ToLower(u.Role), strings.ToLower(u.AuthMethod), strings.ToLower(u.Status),
			template.HTML("You can’t delete your own admin account. Ask another admin to remove it."))
		return
	}

	// Guard 2: do not allow deleting the last active admin.
	if strings.EqualFold(u.Role, "admin") && strings.EqualFold(u.Status, "active") {
		cnt, _ := countActiveAdmins(ctx, db)
		if cnt <= 1 {
			renderEditForm(w, r, role, uname, idHex,
				u.FullName, strings.ToLower(u.Email), strings.ToLower(u.Role), strings.ToLower(u.AuthMethod), strings.ToLower(u.Status),
				template.HTML("There must be at least one active admin in the system."))
			return
		}
	}

	_, _ = db.Collection("users").DeleteOne(ctx, bson.M{"_id": uid})

	http.Redirect(w, r, backToSystemUsersURL(r), http.StatusSeeOther)
}
