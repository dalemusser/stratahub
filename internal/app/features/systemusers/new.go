// internal/app/features/systemusers/new.go
package systemusers

import (
	"context"
	"html/template"
	"net/http"
	"time"

	uierrors "github.com/dalemusser/stratahub/internal/app/features/errors"
	"github.com/dalemusser/stratahub/internal/app/system/authz"
	"github.com/dalemusser/stratahub/internal/app/system/inputval"
	"github.com/dalemusser/stratahub/internal/app/system/navigation"
	"github.com/dalemusser/stratahub/internal/app/system/normalize"
	"github.com/dalemusser/stratahub/internal/app/system/timeouts"
	"github.com/dalemusser/waffle/pantry/httpnav"
	wafflemongo "github.com/dalemusser/waffle/pantry/mongo"
	"github.com/dalemusser/waffle/pantry/templates"
	"github.com/dalemusser/waffle/pantry/text"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

// createSystemUserInput defines validation rules for creating a system user.
type createSystemUserInput struct {
	FullName   string `validate:"required,max=200" label:"Full name"`
	Email      string `validate:"required,email,max=254" label:"Email"`
	Role       string `validate:"required,oneof=admin analyst" label:"Role"`
	AuthMethod string `validate:"required,authmethod" label:"Auth method"`
}

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
		BackURL:     httpnav.ResolveBackURL(r, "/system-users"),
		CurrentPath: httpnav.CurrentPath(r),
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
		h.ErrLog.LogBadRequest(w, r, "parse form failed", err, "Invalid form data.", "/system-users")
		return
	}

	full := normalize.Name(r.FormValue("full_name"))
	email := normalize.Email(r.FormValue("email"))
	userRole := normalize.Role(r.FormValue("role"))
	authm := normalize.AuthMethod(r.FormValue("auth_method"))

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
			BackURL:     httpnav.ResolveBackURL(r, "/system-users"),
			CurrentPath: httpnav.CurrentPath(r),
		})
	}

	// Validate using struct tags (required, email, oneof)
	input := createSystemUserInput{
		FullName:   full,
		Email:      email,
		Role:       userRole,
		AuthMethod: authm,
	}
	if result := inputval.Validate(input); result.HasErrors() {
		reRender(result.First())
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), timeouts.Medium())
	defer cancel()
	db := h.DB

	// Insert (duplicate email is caught by unique index)
	now := time.Now()

	doc := bson.M{
		"_id":          primitive.NewObjectID(),
		"full_name":    full,
		"full_name_ci": text.Fold(full),
		"email":        email,
		"auth_method":  authm,
		"role":         userRole,
		"status":       "active",
		"created_at":   now,
		"updated_at":   now,
	}

	if _, err := db.Collection("users").InsertOne(ctx, doc); err != nil {
		if wafflemongo.IsDup(err) {
			reRender("A user with that email already exists.")
			return
		}
		reRender("Database error while creating system user.")
		return
	}

	ret := navigation.SafeBackURL(r, navigation.SystemUsersBackURL)
	http.Redirect(w, r, ret, http.StatusSeeOther)
}
