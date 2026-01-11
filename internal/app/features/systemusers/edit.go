// internal/app/features/systemusers/edit.go
package systemusers

// Terminology: User Identifiers
//   - UserID / userID / user_id: The MongoDB ObjectID (_id) that uniquely identifies a user record
//   - LoginID / loginID / login_id: The human-readable string users type to log in

import (
	"context"
	"html/template"
	"net/http"
	"strings"
	"time"

	uierrors "github.com/dalemusser/stratahub/internal/app/features/errors"
	"github.com/dalemusser/stratahub/internal/app/store/coordinatorassign"
	orgstore "github.com/dalemusser/stratahub/internal/app/store/organizations"
	userstore "github.com/dalemusser/stratahub/internal/app/store/users"
	"github.com/dalemusser/stratahub/internal/app/system/authutil"
	"github.com/dalemusser/stratahub/internal/app/system/authz"
	"github.com/dalemusser/stratahub/internal/app/system/inputval"
	"github.com/dalemusser/stratahub/internal/app/system/navigation"
	"github.com/dalemusser/stratahub/internal/app/system/normalize"
	"github.com/dalemusser/stratahub/internal/app/system/timeouts"
	"github.com/dalemusser/stratahub/internal/app/system/viewdata"
	"github.com/dalemusser/stratahub/internal/app/system/workspace"
	"github.com/dalemusser/stratahub/internal/domain/models"
	"github.com/dalemusser/waffle/pantry/templates"
	"github.com/go-chi/chi/v5"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.uber.org/zap"
)

// editSystemUserInput defines validation rules for editing a system user.
// Note: LoginID is validated conditionally in HandleEdit based on auth method.
type editSystemUserInput struct {
	FullName string `validate:"required,max=200" label:"Full name"`
	Role     string `validate:"required,oneof=admin analyst coordinator" label:"Role"`
	Status   string `validate:"required,oneof=active disabled" label:"Status"`
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

	// Verify workspace ownership (prevent cross-workspace access)
	wsID := workspace.IDFromRequest(r)
	if wsID != primitive.NilObjectID {
		// User has nil workspace_id (superadmin) OR different workspace
		if u.WorkspaceID == nil || *u.WorkspaceID != wsID {
			uierrors.RenderNotFound(w, r, "User not found.", "/system-users")
			return
		}
	}

	isSelf := uid == u.ID
	userRole := normalize.Role(u.Role)

	// Safely dereference pointer fields
	loginID := ""
	if u.LoginID != nil {
		loginID = *u.LoginID
	}
	email := ""
	if u.Email != nil {
		email = *u.Email
	}
	authReturnID := ""
	if u.AuthReturnID != nil {
		authReturnID = *u.AuthReturnID
	}

	data := formData{
		BaseVM:             viewdata.NewBaseVM(r, h.DB, "Edit System User", "/system-users"),
		AuthMethods:        models.EnabledAuthMethods,
		ID:                 u.ID.Hex(),
		FullName:           u.FullName,
		LoginID:            loginID,
		Email:              email,
		AuthReturnID:       authReturnID,
		URole:              userRole,
		UserRole:           userRole,
		Auth:               normalize.AuthMethod(u.AuthMethod),
		Status:             normalize.Status(u.Status),
		IsEdit:             true,
		IsSelf:             isSelf,
		PreviousRole:       userRole,
		CanManageMaterials: u.CanManageMaterials,
		CanManageResources: u.CanManageResources,
	}

	// Load coordinator org assignments if this is a coordinator
	if userRole == "coordinator" {
		coordStore := coordinatorassign.New(db)
		oStore := orgstore.New(db)

		assignments, err := coordStore.ListByUser(ctx, u.ID)
		if err == nil {
			for _, a := range assignments {
				org, err := oStore.GetByID(ctx, a.OrganizationID)
				if err == nil {
					data.CoordinatorOrgs = append(data.CoordinatorOrgs, CoordinatorOrg{
						ID:   a.OrganizationID.Hex(),
						Name: org.Name,
					})
				}
			}
		}
	}

	templates.Render(w, r, "system_user_edit", data)
}

// HandleEdit processes the Edit System User form POST.
func (h *Handler) HandleEdit(w http.ResponseWriter, r *http.Request) {
	actorRole, editorName, editorOID, ok := authz.UserCtx(r)
	if !ok {
		uierrors.RenderUnauthorized(w, r, "/login")
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

	isSelf := editorOID == uid

	// Fetch existing user to get current role/status (needed when editing yourself,
	// since those fields are disabled and won't submit values)
	ctx, cancel := context.WithTimeout(r.Context(), timeouts.Short())
	defer cancel()

	usrStore := userstore.New(h.DB)
	existingUser, err := usrStore.GetByID(ctx, uid)
	if err != nil {
		uierrors.RenderNotFound(w, r, "User not found.", "/system-users")
		return
	}

	// Verify workspace ownership (prevent cross-workspace access)
	wsID := workspace.IDFromRequest(r)
	if wsID != primitive.NilObjectID {
		// User has nil workspace_id (superadmin) OR different workspace
		if existingUser.WorkspaceID == nil || *existingUser.WorkspaceID != wsID {
			uierrors.RenderNotFound(w, r, "User not found.", "/system-users")
			return
		}
	}

	full := normalize.Name(r.FormValue("full_name"))
	loginID := normalize.Email(r.FormValue("login_id"))
	email := normalize.Email(r.FormValue("email"))
	authReturnID := strings.TrimSpace(r.FormValue("auth_return_id"))
	userRole := normalize.Role(r.FormValue("role"))
	authm := normalize.AuthMethod(r.FormValue("auth_method"))
	tempPassword := strings.TrimSpace(r.FormValue("temp_password"))
	status := normalize.Status(r.FormValue("status"))
	previousRole := normalize.Role(r.FormValue("previous_role"))

	// When editing yourself, role and status fields are disabled and won't submit.
	// Use the existing values from the database.
	if isSelf {
		userRole = normalize.Role(existingUser.Role)
		status = normalize.Status(existingUser.Status)
	}

	// Get org IDs if coordinator role
	orgIDStrs := r.Form["orgIDs"]
	orgIDs := parseOrgIDs(orgIDStrs)

	// Get coordinator permissions (checkboxes)
	canManageMaterials := r.FormValue("can_manage_materials") == "on"
	canManageResources := r.FormValue("can_manage_resources") == "on"

	// Helper for re-rendering with error
	formParams := func(msg string) editFormParams {
		return editFormParams{
			ID:           uid.Hex(),
			FullName:     full,
			LoginID:      loginID,
			Email:        email,
			AuthReturnID: authReturnID,
			TempPassword: tempPassword,
			Role:         userRole,
			Auth:         authm,
			Status:       status,
			IsSelf:       isSelf,
			ErrMsg:       template.HTML(msg),
		}
	}

	// Prevent an admin from changing their own role or status.
	if isSelf && (userRole != "admin" || status != "active") {
		renderEditForm(w, r, h.DB, formParams("You can't change your own role or status. Ask another admin to make those changes."))
		return
	}

	// Validate required fields using struct tags
	input := editSystemUserInput{
		FullName: full,
		Role:     userRole,
		Status:   status,
	}
	if result := inputval.Validate(input); result.HasErrors() {
		renderEditForm(w, r, h.DB, formParams(result.First()))
		return
	}

	// Validate auth fields using centralized logic
	authResult, err := authutil.ValidateAndResolve(authutil.AuthInput{
		Method:       authm,
		LoginID:      loginID,
		Email:        email,
		AuthReturnID: authReturnID,
		TempPassword: tempPassword,
		IsEdit:       true,
	})
	if err != nil {
		renderEditForm(w, r, h.DB, formParams(err.Error()))
		return
	}

	// Coordinators must have at least one organization assigned
	if userRole == "coordinator" && len(orgIDs) == 0 {
		renderEditForm(w, r, h.DB, formParams("Coordinators must be assigned to at least one organization."))
		return
	}

	ctx, cancel = context.WithTimeout(r.Context(), timeouts.Medium())
	defer cancel()
	db := h.DB

	// Update system user via store (handles CI fields, timestamps, and duplicate detection)
	usrStore = userstore.New(db)

	// Check for duplicate login_id (exclude self)
	effectiveLoginID := authResult.EffectiveLoginID
	exists, err := usrStore.LoginIDExistsForOther(ctx, effectiveLoginID, uid)
	if err != nil {
		renderEditForm(w, r, h.DB, formParams("Database error while checking login ID."))
		return
	}
	if exists {
		renderEditForm(w, r, h.DB, formParams("A user with that login ID already exists."))
		return
	}

	upd := userstore.SystemUserUpdate{
		FullName:           full,
		LoginID:            effectiveLoginID,
		AuthMethod:         authm,
		Role:               userRole,
		Status:             status,
		CanManageMaterials: canManageMaterials,
		CanManageResources: canManageResources,
	}

	// Set optional email if provided
	if authResult.Email != nil {
		upd.Email = authResult.Email
	}

	// Set optional auth_return_id if provided
	if authResult.AuthReturnID != nil {
		upd.AuthReturnID = authResult.AuthReturnID
	}

	// Handle password reset if provided
	if authResult.PasswordHash != nil {
		upd.PasswordHash = authResult.PasswordHash
		upd.PasswordTemp = authResult.PasswordTemp
	}

	if err := usrStore.UpdateSystemUser(ctx, uid, upd); err != nil {
		msg := "Database error while updating system user."
		if err == userstore.ErrDuplicateLoginID {
			msg = "A user with that login ID already exists."
		}
		renderEditForm(w, r, h.DB, formParams(msg))
		return
	}

	// Audit log: check for status change or general update
	oldStatus := normalize.Status(existingUser.Status)
	if oldStatus != status {
		if status == "disabled" {
			h.AuditLog.UserDisabled(ctx, r, editorOID, uid, nil, actorRole)
		} else if status == "active" {
			h.AuditLog.UserEnabled(ctx, r, editorOID, uid, nil, actorRole)
		}
	} else {
		// General update - log changed fields
		h.AuditLog.UserUpdated(ctx, r, editorOID, uid, nil, actorRole, "system user details")
	}

	// Handle coordinator assignment changes
	coordStore := coordinatorassign.New(db)

	// If role changed FROM coordinator to something else, delete all assignments
	if previousRole == "coordinator" && userRole != "coordinator" {
		// Fetch existing assignments for audit logging before deleting
		existingAssigns, listErr := coordStore.ListByUser(ctx, uid)
		if listErr != nil {
			h.Log.Error("failed to list coordinator assignments for audit",
				zap.Error(listErr),
				zap.String("user_id", uid.Hex()))
		}

		if _, err := coordStore.DeleteByUser(ctx, uid); err != nil {
			h.Log.Error("failed to delete coordinator assignments on role change",
				zap.Error(err),
				zap.String("user_id", uid.Hex()))
		} else if listErr == nil {
			// Audit log: coordinator unassigned from each organization
			oStore := orgstore.New(db)
			for _, a := range existingAssigns {
				orgName := ""
				if org, err := oStore.GetByID(ctx, a.OrganizationID); err == nil {
					orgName = org.Name
				}
				h.AuditLog.CoordinatorUnassignedFromOrg(ctx, r, editorOID, uid, a.OrganizationID, actorRole, orgName)
			}
		}
	}

	// If role is coordinator, sync assignments
	if userRole == "coordinator" {
		// Get existing assignments
		existingAssignments, err := coordStore.ListByUser(ctx, uid)
		if err != nil {
			h.Log.Error("failed to list coordinator assignments",
				zap.Error(err),
				zap.String("user_id", uid.Hex()))
		}

		// Build sets for comparison
		existingOrgIDs := make(map[string]primitive.ObjectID)
		for _, a := range existingAssignments {
			existingOrgIDs[a.OrganizationID.Hex()] = a.OrganizationID
		}

		newOrgIDs := make(map[string]primitive.ObjectID)
		for _, oid := range orgIDs {
			newOrgIDs[oid.Hex()] = oid
		}

		// Delete assignments that are no longer selected
		oStore := orgstore.New(db)
		for hexID := range existingOrgIDs {
			if _, exists := newOrgIDs[hexID]; !exists {
				orgOID := existingOrgIDs[hexID]
				// Find and delete the assignment
				for _, a := range existingAssignments {
					if a.OrganizationID == orgOID {
						if err := coordStore.Delete(ctx, a.ID); err != nil {
							h.Log.Error("failed to delete coordinator assignment",
								zap.Error(err),
								zap.String("assignment_id", a.ID.Hex()))
						} else {
							// Audit log: coordinator unassigned from organization
							orgName := ""
							if org, err := oStore.GetByID(ctx, orgOID); err == nil {
								orgName = org.Name
							}
							h.AuditLog.CoordinatorUnassignedFromOrg(ctx, r, editorOID, uid, orgOID, actorRole, orgName)
						}
						break
					}
				}
			}
		}

		// Add new assignments that don't exist yet
		for hexID, orgOID := range newOrgIDs {
			if _, exists := existingOrgIDs[hexID]; !exists {
				assignment := models.CoordinatorAssignment{
					UserID:         uid,
					OrganizationID: orgOID,
					CreatedAt:      time.Now().UTC(),
					CreatedByID:    editorOID,
					CreatedByName:  editorName,
				}
				if _, err := coordStore.Create(ctx, assignment); err != nil {
					h.Log.Error("failed to create coordinator assignment",
						zap.Error(err),
						zap.String("user_id", uid.Hex()),
						zap.String("org_id", orgOID.Hex()))
				} else {
					// Audit log: coordinator assigned to organization
					orgName := ""
					if org, err := oStore.GetByID(ctx, orgOID); err == nil {
						orgName = org.Name
					}
					h.AuditLog.CoordinatorAssignedToOrg(ctx, r, editorOID, uid, orgOID, actorRole, orgName)
				}
			}
		}
	}

	ret := navigation.SafeBackURL(r, navigation.SystemUsersBackURL)
	http.Redirect(w, r, ret, http.StatusSeeOther)
}

// HandleDelete deletes a system user, enforcing safety guards
// (cannot delete self, cannot delete last active admin).
func (h *Handler) HandleDelete(w http.ResponseWriter, r *http.Request) {
	actorRole, _, who, ok := userContext(r) // who = current user's ObjectID
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

	// Safely dereference pointer fields for error rendering
	loginID := ""
	if u.LoginID != nil {
		loginID = *u.LoginID
	}
	email := ""
	if u.Email != nil {
		email = *u.Email
	}
	authReturnID := ""
	if u.AuthReturnID != nil {
		authReturnID = *u.AuthReturnID
	}

	// Helper for re-rendering with error
	makeFormParams := func(msg string) editFormParams {
		return editFormParams{
			ID:           idHex,
			FullName:     u.FullName,
			LoginID:      loginID,
			Email:        email,
			AuthReturnID: authReturnID,
			Role:         normalize.Role(u.Role),
			Auth:         normalize.AuthMethod(u.AuthMethod),
			Status:       normalize.Status(u.Status),
			IsSelf:       isSelf,
			ErrMsg:       template.HTML(msg),
		}
	}

	// Guard 1: prevent an admin from deleting themself.
	if isSelf && strings.EqualFold(u.Role, "admin") {
		renderEditForm(w, r, h.DB, makeFormParams("You can't delete your own admin account. Ask another admin to remove it."))
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
			renderEditForm(w, r, h.DB, makeFormParams("There must be at least one active admin in the system."))
			return
		}
	}

	// Delete coordinator assignments first (if any)
	coordStore := coordinatorassign.New(db)
	if _, err := coordStore.DeleteByUser(ctx, uid); err != nil {
		h.Log.Error("failed to delete coordinator assignments on user delete",
			zap.Error(err),
			zap.String("user_id", uid.Hex()))
		// Continue with user deletion even if assignment cleanup fails
	}

	if _, err := usrStore.DeleteSystemUser(ctx, uid); err != nil {
		h.ErrLog.LogServerError(w, r, "database error deleting system user", err, "A database error occurred.", "/system-users")
		return
	}

	// Audit log: system user deleted
	h.AuditLog.UserDeleted(ctx, r, who, uid, nil, actorRole, normalize.Role(u.Role))

	http.Redirect(w, r, backToSystemUsersURL(r), http.StatusSeeOther)
}
