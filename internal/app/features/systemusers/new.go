// internal/app/features/systemusers/new.go
package systemusers

import (
	"context"
	"html/template"
	"net/http"
	"strings"
	"time"

	uierrors "github.com/dalemusser/stratahub/internal/app/features/errors"
	"github.com/dalemusser/waffle/templates"
	textfold "github.com/dalemusser/waffle/toolkit/text/textfold"
	nav "github.com/dalemusser/waffle/toolkit/ui/nav"
	validate "github.com/dalemusser/waffle/toolkit/validate"

	mongodb "github.com/dalemusser/waffle/toolkit/db/mongodb"

	"github.com/dalemusser/stratahub/internal/app/system/authz"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

// ServeNew renders the "Add System User" form.
//
// Note: This uses the viewer's context (role/name) for the header/sidebar,
// but does NOT itself enforce admin-only access. The list entry point and
// modal actions are admin-gated via requireAdmin().
func (h *Handler) ServeNew(w http.ResponseWriter, r *http.Request) {
	role, uname, _, ok := authz.UserCtx(r)
	if !ok {
		uierrors.RenderUnauthorized(w, r, "/login")
		return
	}

	data := formData{
		Title:       "Add System User",
		IsLoggedIn:  true,
		Role:        role,
		UserName:    uname,
		BackURL:     nav.ResolveBackURL(r, "/system-users"),
		CurrentPath: nav.CurrentPath(r),
		// Field values start empty; template will show sensible defaults.
	}

	templates.Render(w, r, "system_user_new", data)
}

// HandleCreate processes the Add System User form POST.
func (h *Handler) HandleCreate(w http.ResponseWriter, r *http.Request) {
	role, uname, _, ok := authz.UserCtx(r)
	if !ok {
		uierrors.RenderUnauthorized(w, r, "/login")
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad form", http.StatusBadRequest)
		return
	}

	full := strings.TrimSpace(r.FormValue("full_name"))
	email := strings.ToLower(strings.TrimSpace(r.FormValue("email")))
	userRole := strings.ToLower(strings.TrimSpace(r.FormValue("role")))
	authm := strings.ToLower(strings.TrimSpace(r.FormValue("auth_method")))

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

	reRender := func(msg string) {
		templates.Render(w, r, "system_user_new", formData{
			Title:       "Add System User",
			IsLoggedIn:  true,
			Role:        role,
			UserName:    uname,
			FullName:    full,
			Email:       email,
			URole:       userRole,
			UserRole:    userRole,
			Auth:        authm,
			Status:      "active",
			Error:       template.HTML(msg),
			BackURL:     nav.ResolveBackURL(r, "/system-users"),
			CurrentPath: nav.CurrentPath(r),
		})
	}

	if len(errs) > 0 {
		reRender(strings.Join(errs, " "))
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), sysUsersMedTimeout)
	defer cancel()
	db := h.DB

	// Duplicate email check
	if err := db.Collection("users").FindOne(ctx, bson.M{"email": email}).Err(); err == nil {
		reRender("A user with that email already exists.")
		return
	}

	now := time.Now()

	doc := bson.M{
		"_id":          primitive.NewObjectID(),
		"full_name":    full,
		"full_name_ci": textfold.Fold(full),
		"email":        email,
		"auth_method":  authm,
		"role":         userRole,
		"status":       "active",
		"created_at":   now,
		"updated_at":   now,
	}

	if _, err := db.Collection("users").InsertOne(ctx, doc); err != nil {
		if mongodb.IsDup(err) {
			reRender("A user with that email already exists.")
			return
		}
		reRender("Database error while creating system user.")
		return
	}

	if ret := r.FormValue("return"); ret != "" && strings.HasPrefix(ret, "/") {
		http.Redirect(w, r, ret, http.StatusSeeOther)
		return
	}
	http.Redirect(w, r, "/system-users", http.StatusSeeOther)
}
