package leaders

import (
	"context"
	"net/http"
	"strings"
	"time"

	uierrors "github.com/dalemusser/stratahub/internal/app/features/errors"
	"github.com/dalemusser/stratahub/internal/app/system/authutil"
	"github.com/dalemusser/stratahub/internal/app/system/authz"
	"github.com/dalemusser/stratahub/internal/app/system/formutil"
	"github.com/dalemusser/stratahub/internal/app/system/inputval"
	"github.com/dalemusser/stratahub/internal/app/system/navigation"
	"github.com/dalemusser/stratahub/internal/app/system/normalize"
	"github.com/dalemusser/stratahub/internal/app/system/timeouts"
	"github.com/dalemusser/stratahub/internal/app/system/workspace"
	"github.com/dalemusser/stratahub/internal/domain/models"
	wafflemongo "github.com/dalemusser/waffle/pantry/mongo"
	"github.com/dalemusser/waffle/pantry/templates"
	"github.com/dalemusser/waffle/pantry/text"
	"github.com/go-chi/chi/v5"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
)

// editLeaderInput defines validation rules for editing a leader.
// Note: LoginID is validated conditionally in HandleEdit based on auth method.
type editLeaderInput struct {
	FullName string `validate:"required,max=200" label:"Full name"`
	Status   string `validate:"required,oneof=active disabled" label:"Status"`
}

// editData is the view model for the edit-leader page.
type editData struct {
	formutil.Base

	ID          string
	AuthMethods []models.AuthMethod

	FullName     string
	LoginID      string
	Email        string
	AuthReturnID string
	Auth         string
	TempPassword string
	Status       string

	OrgID   string // hidden field
	OrgName string // read-only display

	IsEdit bool // true for edit forms
}

// Template helper methods for auth field visibility
func (d editData) EmailIsLoginMethod() bool       { return authutil.EmailIsLogin(d.Auth) }
func (d editData) RequiresAuthReturnIDMethod() bool { return authutil.RequiresAuthReturnID(d.Auth) }
func (d editData) IsPasswordMethod() bool         { return d.Auth == "password" }

// ServeEdit renders the Edit Leader page.
func (h *Handler) ServeEdit(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), timeouts.Short())
	defer cancel()

	uidHex := chi.URLParam(r, "id")
	uid, err := primitive.ObjectIDFromHex(uidHex)
	if err != nil {
		uierrors.RenderBadRequest(w, r, "Invalid leader ID.", "/leaders")
		return
	}

	var usr models.User
	if err := h.DB.Collection("users").FindOne(ctx, bson.M{"_id": uid, "role": "leader"}).Decode(&usr); err != nil {
		uierrors.RenderNotFound(w, r, "Leader not found.", "/leaders")
		return
	}

	// Verify workspace ownership (prevent cross-workspace access)
	wsID := workspace.IDFromRequest(r)
	if wsID != primitive.NilObjectID && (usr.WorkspaceID == nil || *usr.WorkspaceID != wsID) {
		uierrors.RenderNotFound(w, r, "Leader not found.", "/leaders")
		return
	}

	// Coordinator access check: verify access to leader's organization
	if authz.IsCoordinator(r) && usr.OrganizationID != nil {
		if !authz.CanAccessOrg(r, *usr.OrganizationID) {
			uierrors.RenderForbidden(w, r, "You don't have access to this leader.", "/leaders")
			return
		}
	}

	orgHex := ""
	orgName := ""
	if usr.OrganizationID != nil {
		orgHex = usr.OrganizationID.Hex()
		var o models.Organization
		if err := h.DB.Collection("organizations").FindOne(ctx, bson.M{"_id": *usr.OrganizationID}).Decode(&o); err != nil {
			if err == mongo.ErrNoDocuments {
				orgName = "(Deleted)"
			} else {
				h.ErrLog.LogServerError(w, r, "database error loading organization for leader", err, "A database error occurred.", "/leaders")
				return
			}
		} else {
			orgName = o.Name
		}
	}

	loginID := ""
	if usr.LoginID != nil {
		loginID = *usr.LoginID
	}
	email := ""
	if usr.Email != nil {
		email = *usr.Email
	}
	authReturnID := ""
	if usr.AuthReturnID != nil {
		authReturnID = *usr.AuthReturnID
	}

	data := editData{
		ID:           usr.ID.Hex(),
		AuthMethods:  models.EnabledAuthMethods,
		FullName:     usr.FullName,
		LoginID:      loginID,
		Email:        email,
		AuthReturnID: authReturnID,
		Auth:         normalize.AuthMethod(usr.AuthMethod),
		Status:       usr.Status,
		OrgID:        orgHex,
		OrgName:      orgName,
		IsEdit:       true,
	}
	formutil.SetBase(&data.Base, r, h.DB, "Edit Leader", "/leaders")

	templates.Render(w, r, "admin_leader_edit", data)
}

// HandleEdit processes the Edit Leader form submission.
func (h *Handler) HandleEdit(w http.ResponseWriter, r *http.Request) {
	actorRole, _, actorID, ok := authz.UserCtx(r)
	if !ok {
		uierrors.RenderUnauthorized(w, r, "/login")
		return
	}

	if err := r.ParseForm(); err != nil {
		h.ErrLog.LogBadRequest(w, r, "parse form failed", err, "Invalid form submission.", "/leaders")
		return
	}

	uidHex := chi.URLParam(r, "id")
	uid, err := primitive.ObjectIDFromHex(uidHex)
	if err != nil {
		uierrors.RenderBadRequest(w, r, "Invalid leader ID.", "/leaders")
		return
	}

	full := normalize.Name(r.FormValue("full_name"))
	loginID := normalize.Email(r.FormValue("login_id"))
	email := normalize.Email(r.FormValue("email"))
	authReturnID := strings.TrimSpace(r.FormValue("auth_return_id"))
	authm := normalize.AuthMethod(r.FormValue("auth_method"))
	tempPassword := strings.TrimSpace(r.FormValue("temp_password"))
	status := normalize.Status(r.FormValue("status"))
	orgHex := normalize.QueryParam(r.FormValue("orgID")) // carried as hidden; not changeable

	ctx, cancel := context.WithTimeout(r.Context(), timeouts.Medium())
	defer cancel()

	// Coordinator access check: verify access to leader's organization
	if authz.IsCoordinator(r) && orgHex != "" {
		if oid, err := primitive.ObjectIDFromHex(orgHex); err == nil {
			if !authz.CanAccessOrg(r, oid) {
				uierrors.RenderForbidden(w, r, "You don't have access to this leader.", "/leaders")
				return
			}
		}
	}

	// load org name for re-render convenience
	orgName := ""
	if oid, e := primitive.ObjectIDFromHex(orgHex); e == nil {
		var o models.Organization
		if err := h.DB.Collection("organizations").FindOne(ctx, bson.M{"_id": oid}).Decode(&o); err != nil {
			if err == mongo.ErrNoDocuments {
				orgName = "(Deleted)"
			} else {
				h.ErrLog.LogServerError(w, r, "database error loading organization", err, "A database error occurred.", "/leaders")
				return
			}
		} else {
			orgName = o.Name
		}
	}

	// Normalize defaults
	if status == "" {
		status = "active"
	}
	if authm == "" {
		authm = "trust"
	}

	reRender := func(msg string) {
		data := editData{
			ID:           uid.Hex(),
			AuthMethods:  models.EnabledAuthMethods,
			FullName:     full,
			LoginID:      loginID,
			Email:        email,
			AuthReturnID: authReturnID,
			Auth:         authm,
			TempPassword: tempPassword,
			Status:       status,
			OrgID:        orgHex,
			OrgName:      orgName,
			IsEdit:       true,
		}
		formutil.SetBase(&data.Base, r, h.DB, "Edit Leader", "/leaders")
		data.SetError(msg)
		templates.Render(w, r, "admin_leader_edit", data)
	}

	// Validate required fields using struct tags
	input := editLeaderInput{FullName: full, Status: status}
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
		IsEdit:       true,
	})
	if err != nil {
		reRender(err.Error())
		return
	}

	// Fetch current leader to get old status for audit logging
	var currentLeader models.User
	if err := h.DB.Collection("users").FindOne(ctx, bson.M{"_id": uid, "role": "leader"}).Decode(&currentLeader); err != nil {
		if err == mongo.ErrNoDocuments {
			uierrors.RenderNotFound(w, r, "Leader not found.", "/leaders")
		} else {
			h.ErrLog.LogServerError(w, r, "database error loading leader", err, "A database error occurred.", "/leaders")
		}
		return
	}

	// Verify workspace ownership (prevent cross-workspace access)
	wsID := workspace.IDFromRequest(r)
	if wsID != primitive.NilObjectID && (currentLeader.WorkspaceID == nil || *currentLeader.WorkspaceID != wsID) {
		uierrors.RenderNotFound(w, r, "Leader not found.", "/leaders")
		return
	}
	oldStatus := normalize.Status(currentLeader.Status)

	// Early uniqueness check: same login_id used by a different user?
	effectiveLoginID := authResult.EffectiveLoginID
	loginIDCI := text.Fold(effectiveLoginID)
	if err := h.DB.Collection("users").FindOne(ctx, bson.M{
		"login_id_ci": loginIDCI,
		"_id":         bson.M{"$ne": uid},
	}).Err(); err == nil {
		reRender("A user with that login ID already exists.")
		return
	}

	// Build update doc WITHOUT changing organization_id
	up := bson.M{
		"full_name":    full,
		"full_name_ci": text.Fold(full),
		"login_id":     effectiveLoginID,
		"login_id_ci":  loginIDCI,
		"auth_method":  authm,
		"status":       status,
		"updated_at":   time.Now(),
	}

	// Handle optional email
	if authResult.Email != nil {
		up["email"] = *authResult.Email
	}

	// Handle optional auth_return_id
	if authResult.AuthReturnID != nil {
		up["auth_return_id"] = *authResult.AuthReturnID
	}

	// Handle password reset if provided
	if authResult.PasswordHash != nil {
		up["password_hash"] = *authResult.PasswordHash
		up["password_temp"] = *authResult.PasswordTemp
	}

	if _, err := h.DB.Collection("users").UpdateOne(ctx, bson.M{"_id": uid, "role": "leader"}, bson.M{"$set": up}); err != nil {
		msg := "Database error while updating leader."
		if wafflemongo.IsDup(err) {
			msg = "A user with that login ID already exists."
		}
		reRender(msg)
		return
	}

	// Audit log: check for status change or general update
	if oldStatus != status {
		if status == "disabled" {
			h.AuditLog.UserDisabled(ctx, r, actorID, uid, currentLeader.OrganizationID, actorRole)
		} else if status == "active" {
			h.AuditLog.UserEnabled(ctx, r, actorID, uid, currentLeader.OrganizationID, actorRole)
		}
	} else {
		// General update - log changed fields
		h.AuditLog.UserUpdated(ctx, r, actorID, uid, currentLeader.OrganizationID, actorRole, "leader details")
	}

	ret := navigation.SafeBackURL(r, navigation.LeadersBackURL)
	http.Redirect(w, r, ret, http.StatusSeeOther)
}
