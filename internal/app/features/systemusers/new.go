// internal/app/features/systemusers/new.go
package systemusers

import (
	"context"
	"html/template"
	"net/http"

	uierrors "github.com/dalemusser/stratahub/internal/app/features/errors"
	userstore "github.com/dalemusser/stratahub/internal/app/store/users"
	"github.com/dalemusser/stratahub/internal/app/system/authz"
	"github.com/dalemusser/stratahub/internal/app/system/inputval"
	"github.com/dalemusser/stratahub/internal/app/system/navigation"
	"github.com/dalemusser/stratahub/internal/app/system/normalize"
	"github.com/dalemusser/stratahub/internal/app/system/timeouts"
	"github.com/dalemusser/stratahub/internal/app/system/viewdata"
	"github.com/dalemusser/stratahub/internal/domain/models"
	"github.com/dalemusser/waffle/pantry/templates"
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
	_, _, _, ok := authz.UserCtx(r)
	if !ok {
		uierrors.RenderUnauthorized(w, r, "/login")
		return
	}

	data := formData{
		BaseVM: viewdata.NewBaseVM(r, h.DB, "Add System User", "/system-users"),
		// Field values start empty; template will show sensible defaults.
	}

	templates.Render(w, r, "system_user_new", data)
}

// HandleCreate processes the Add System User form POST.
func (h *Handler) HandleCreate(w http.ResponseWriter, r *http.Request) {
	_, _, _, ok := authz.UserCtx(r)
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
			BaseVM:   viewdata.NewBaseVM(r, h.DB, "Add System User", "/system-users"),
			FullName: full,
			Email:    email,
			URole:    userRole,
			UserRole: userRole,
			Auth:     authm,
			Status:   "active",
			Error:    template.HTML(msg),
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

	// Create user via store (handles ID, CI fields, timestamps, and duplicate detection)
	usrStore := userstore.New(db)
	user := models.User{
		FullName:   full,
		Email:      email,
		Role:       userRole,
		AuthMethod: authm,
		Status:     "active",
	}

	if _, err := usrStore.Create(ctx, user); err != nil {
		msg := "Database error while creating system user."
		if err == userstore.ErrDuplicateEmail {
			msg = "A user with that email already exists."
		}
		reRender(msg)
		return
	}

	ret := navigation.SafeBackURL(r, navigation.SystemUsersBackURL)
	http.Redirect(w, r, ret, http.StatusSeeOther)
}
