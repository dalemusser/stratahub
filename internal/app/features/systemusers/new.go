// internal/app/features/systemusers/new.go
package systemusers

import (
	"context"
	"html/template"
	"net/http"
	"strings"
	"time"

	uierrors "github.com/dalemusser/stratahub/internal/app/features/errors"
	"github.com/dalemusser/stratahub/internal/app/store/coordinatorassign"
	userstore "github.com/dalemusser/stratahub/internal/app/store/users"
	"github.com/dalemusser/stratahub/internal/app/system/authutil"
	"github.com/dalemusser/stratahub/internal/app/system/authz"
	"github.com/dalemusser/stratahub/internal/app/system/inputval"
	"github.com/dalemusser/stratahub/internal/app/system/navigation"
	"github.com/dalemusser/stratahub/internal/app/system/normalize"
	"github.com/dalemusser/stratahub/internal/app/system/timeouts"
	"github.com/dalemusser/stratahub/internal/app/system/viewdata"
	"github.com/dalemusser/stratahub/internal/domain/models"
	"github.com/dalemusser/waffle/pantry/templates"
	"go.uber.org/zap"
)

// createSystemUserInput defines validation rules for creating a system user.
// Note: LoginID is validated conditionally in HandleCreate based on auth method.
type createSystemUserInput struct {
	FullName string `validate:"required,max=200" label:"Full name"`
	Role     string `validate:"required,oneof=admin analyst coordinator" label:"Role"`
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
		BaseVM:      viewdata.NewBaseVM(r, h.DB, "Add System User", "/system-users"),
		AuthMethods: models.EnabledAuthMethods,
		Auth:        "trust", // default auth method
		IsEdit:      false,
	}

	templates.Render(w, r, "system_user_new", data)
}

// HandleCreate processes the Add System User form POST.
func (h *Handler) HandleCreate(w http.ResponseWriter, r *http.Request) {
	_, creatorName, creatorOID, ok := authz.UserCtx(r)
	if !ok {
		uierrors.RenderUnauthorized(w, r, "/login")
		return
	}

	if err := r.ParseForm(); err != nil {
		h.ErrLog.LogBadRequest(w, r, "parse form failed", err, "Invalid form data.", "/system-users")
		return
	}

	full := normalize.Name(r.FormValue("full_name"))
	loginID := normalize.Email(r.FormValue("login_id"))
	email := normalize.Email(r.FormValue("email"))
	authReturnID := strings.TrimSpace(r.FormValue("auth_return_id"))
	userRole := normalize.Role(r.FormValue("role"))
	authm := normalize.AuthMethod(r.FormValue("auth_method"))
	tempPassword := strings.TrimSpace(r.FormValue("temp_password"))

	// Get org IDs if coordinator role
	orgIDStrs := r.Form["orgIDs"]
	orgIDs := parseOrgIDs(orgIDStrs)

	// Get coordinator permissions (checkboxes)
	canManageMaterials := r.FormValue("can_manage_materials") == "on"
	canManageResources := r.FormValue("can_manage_resources") == "on"

	reRender := func(msg string) {
		templates.Render(w, r, "system_user_new", formData{
			BaseVM:             viewdata.NewBaseVM(r, h.DB, "Add System User", "/system-users"),
			AuthMethods:        models.EnabledAuthMethods,
			FullName:           full,
			LoginID:            loginID,
			Email:              email,
			AuthReturnID:       authReturnID,
			URole:              userRole,
			UserRole:           userRole,
			Auth:               authm,
			TempPassword:       tempPassword,
			Status:             "active",
			IsEdit:             false,
			CanManageMaterials: canManageMaterials,
			CanManageResources: canManageResources,
			Error:              template.HTML(msg),
		})
	}

	// Validate required fields using struct tags
	input := createSystemUserInput{
		FullName: full,
		Role:     userRole,
	}
	if result := inputval.Validate(input); result.HasErrors() {
		reRender(result.First())
		return
	}

	// Validate auth fields using centralized logic
	authResult, err := authutil.ValidateAndResolve(authutil.AuthInput{
		Method:       authm,
		LoginID:      loginID,
		Email:        email,
		AuthReturnID: authReturnID,
		TempPassword: tempPassword,
		IsEdit:       false,
	})
	if err != nil {
		reRender(err.Error())
		return
	}

	// Coordinators must have at least one organization assigned
	if userRole == "coordinator" && len(orgIDs) == 0 {
		reRender("Coordinators must be assigned to at least one organization.")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), timeouts.Medium())
	defer cancel()
	db := h.DB

	// Create user via store (handles ID, CI fields, timestamps, and duplicate detection)
	usrStore := userstore.New(db)
	effectiveLoginID := authResult.EffectiveLoginID
	user := models.User{
		FullName:   full,
		LoginID:    &effectiveLoginID,
		Role:       userRole,
		AuthMethod: authm,
		Status:     "active",
	}

	// Add optional email if provided
	if authResult.Email != nil {
		user.Email = authResult.Email
	}

	// Add optional auth_return_id if provided
	if authResult.AuthReturnID != nil {
		user.AuthReturnID = authResult.AuthReturnID
	}

	// Add password hash if provided
	if authResult.PasswordHash != nil {
		user.PasswordHash = authResult.PasswordHash
		user.PasswordTemp = authResult.PasswordTemp
	}

	// Set coordinator-specific permissions (only relevant if role is coordinator)
	if userRole == "coordinator" {
		user.CanManageMaterials = canManageMaterials
		user.CanManageResources = canManageResources
	}

	createdUser, err := usrStore.Create(ctx, user)
	if err != nil {
		msg := "Database error while creating system user."
		if err == userstore.ErrDuplicateLoginID {
			msg = "A user with that login ID already exists."
		} else {
			h.Log.Error("failed to create system user",
				zap.Error(err),
				zap.String("role", userRole),
				zap.String("login_id", effectiveLoginID))
		}
		reRender(msg)
		return
	}

	// Create coordinator assignments if role is coordinator
	if userRole == "coordinator" && len(orgIDs) > 0 {
		coordStore := coordinatorassign.New(db)

		for _, orgID := range orgIDs {
			assignment := models.CoordinatorAssignment{
				UserID:         createdUser.ID,
				OrganizationID: orgID,
				CreatedAt:      time.Now().UTC(),
				CreatedByID:    creatorOID,
				CreatedByName:  creatorName,
			}
			if _, err := coordStore.Create(ctx, assignment); err != nil {
				h.Log.Error("failed to create coordinator assignment",
					zap.Error(err),
					zap.String("user_id", createdUser.ID.Hex()),
					zap.String("org_id", orgID.Hex()))
				// Continue with other assignments; log but don't fail
			}
		}
	}

	ret := navigation.SafeBackURL(r, navigation.SystemUsersBackURL)
	http.Redirect(w, r, ret, http.StatusSeeOther)
}
