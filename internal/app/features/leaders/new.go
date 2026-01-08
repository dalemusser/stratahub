// internal/app/features/leaders/new.go
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
	"github.com/dalemusser/stratahub/internal/app/system/orgutil"
	"github.com/dalemusser/stratahub/internal/app/system/timeouts"
	"github.com/dalemusser/stratahub/internal/app/system/workspace"
	"github.com/dalemusser/stratahub/internal/domain/models"
	wafflemongo "github.com/dalemusser/waffle/pantry/mongo"
	"github.com/dalemusser/waffle/pantry/templates"
	"github.com/dalemusser/waffle/pantry/text"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.uber.org/zap"
)

// createLeaderInput defines validation rules for creating a new leader.
// Note: LoginID is validated conditionally in HandleCreate based on auth method.
type createLeaderInput struct {
	FullName string `validate:"required,max=200" label:"Full name"`
	OrgID    string `validate:"required,objectid" label:"Organization"`
}

// orgOption is a type alias for organization dropdown options.
type orgOption = orgutil.OrgOption

// newData is the view model for the new-leader page.
type newData struct {
	formutil.Base

	Organizations []orgOption
	AuthMethods   []models.AuthMethod

	// Org is now always locked (passed via URL)
	OrgHex  string
	OrgName string

	FullName     string
	LoginID      string
	Email        string
	AuthReturnID string
	Auth         string
	TempPassword string
	Status       string

	IsEdit bool // false for new forms
}

// Template helper methods for auth field visibility
func (d newData) EmailIsLoginMethod() bool       { return authutil.EmailIsLogin(d.Auth) }
func (d newData) RequiresAuthReturnIDMethod() bool { return authutil.RequiresAuthReturnID(d.Auth) }
func (d newData) IsPasswordMethod() bool         { return d.Auth == "password" }

func (h *Handler) ServeNew(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), timeouts.Short())
	defer cancel()

	var data newData
	formutil.SetBase(&data.Base, r, h.DB, "New Leader", "/leaders")
	data.AuthMethods = models.EnabledAuthMethods
	data.Auth = "trust" // default auth method

	// Org can be passed via URL param (optional - can select via picker)
	selectedOrg := normalize.QueryParam(r.URL.Query().Get("org"))
	if selectedOrg != "" && selectedOrg != "all" {
		orgID, orgName, err := orgutil.ResolveActiveOrgFromHex(ctx, h.DB, selectedOrg)
		if err != nil {
			if orgutil.IsExpectedOrgError(err) {
				// Org not found - just show page without org selected
				h.Log.Warn("org not found or inactive", zap.String("org", selectedOrg))
			} else {
				h.ErrLog.LogServerError(w, r, "database error loading organization", err, "A database error occurred.", "/leaders")
				return
			}
		} else {
			// Coordinator access check: verify access to specified organization
			if authz.IsCoordinator(r) && !authz.CanAccessOrg(r, orgID) {
				uierrors.RenderForbidden(w, r, "You don't have access to this organization.", "/leaders")
				return
			}
			data.OrgHex = orgID.Hex()
			data.OrgName = orgName
		}
	}

	templates.Render(w, r, "admin_leader_new", data)
}

func (h *Handler) HandleCreate(w http.ResponseWriter, r *http.Request) {
	actorRole, _, actorID, ok := authz.UserCtx(r)
	if !ok {
		uierrors.RenderUnauthorized(w, r, "/login")
		return
	}

	if err := r.ParseForm(); err != nil {
		h.renderNewWithError(w, r, "Bad request.")
		return
	}

	full := normalize.Name(r.FormValue("full_name"))
	loginID := normalize.Email(r.FormValue("login_id"))
	email := normalize.Email(r.FormValue("email"))
	authReturnID := strings.TrimSpace(r.FormValue("auth_return_id"))
	authm := normalize.AuthMethod(r.FormValue("auth_method"))
	orgHex := normalize.OrgID(r.FormValue("orgID"))
	tempPassword := strings.TrimSpace(r.FormValue("temp_password"))

	// New leaders always start as active
	status := "active"

	// Normalize defaults
	if authm == "" {
		authm = "trust"
	}

	// Validate required fields using struct tags
	input := createLeaderInput{FullName: full, OrgID: orgHex}
	if result := inputval.Validate(input); result.HasErrors() {
		h.renderNewWithError(w, r, result.First(),
			withNewEcho(full, loginID, email, authReturnID, orgHex, authm, tempPassword, status))
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
		h.renderNewWithError(w, r, err.Error(),
			withNewEcho(full, loginID, email, authReturnID, orgHex, authm, tempPassword, status))
		return
	}

	oid, err := primitive.ObjectIDFromHex(orgHex)
	if err != nil {
		h.renderNewWithError(w, r, "Organization is required.",
			withNewEcho(full, loginID, email, authReturnID, orgHex, authm, tempPassword, status))
		return
	}

	// Coordinator access check: verify access to specified organization
	if authz.IsCoordinator(r) && !authz.CanAccessOrg(r, oid) {
		uierrors.RenderForbidden(w, r, "You don't have access to this organization.", "/leaders")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), timeouts.Medium())
	defer cancel()

	// Build & insert (duplicate login_id_ci is caught by unique index)
	now := time.Now()
	effectiveLoginID := authResult.EffectiveLoginID
	loginIDCI := text.Fold(effectiveLoginID)
	doc := bson.M{
		"_id":             primitive.NewObjectID(),
		"full_name":       full,
		"full_name_ci":    text.Fold(full),
		"login_id":        effectiveLoginID,
		"login_id_ci":     loginIDCI,
		"auth_method":     authm,
		"role":            "leader",
		"status":          status,
		"organization_id": oid,
		"created_at":      now,
		"updated_at":      now,
	}

	// Add workspace ID if in workspace context
	wsID := workspace.IDFromRequest(r)
	if wsID != primitive.NilObjectID {
		doc["workspace_id"] = wsID
	}

	// Add optional email if provided
	if authResult.Email != nil {
		doc["email"] = *authResult.Email
	}

	// Add optional auth_return_id if provided
	if authResult.AuthReturnID != nil {
		doc["auth_return_id"] = *authResult.AuthReturnID
	}

	// Add password hash if provided
	if authResult.PasswordHash != nil {
		doc["password_hash"] = *authResult.PasswordHash
		doc["password_temp"] = *authResult.PasswordTemp
	}

	res, err := h.DB.Collection("users").InsertOne(ctx, doc)
	if err != nil {
		msg := "Database error while creating leader."
		if wafflemongo.IsDup(err) {
			msg = "A user with that login ID already exists."
		}
		h.renderNewWithError(w, r, msg, withNewEcho(full, loginID, email, authReturnID, orgHex, authm, tempPassword, status))
		return
	}

	// Audit log: leader created
	newUserID := res.InsertedID.(primitive.ObjectID)
	h.AuditLog.UserCreated(ctx, r, actorID, newUserID, &oid, actorRole, "leader", authm)

	// Success: honor optional return parameter, otherwise go back to leaders list.
	ret := navigation.SafeBackURL(r, navigation.LeadersBackURL)
	http.Redirect(w, r, ret, http.StatusSeeOther)
}

func (h *Handler) renderNewWithError(w http.ResponseWriter, r *http.Request, msg string, echo ...newData) {
	ctx, cancel := context.WithTimeout(r.Context(), timeouts.Short())
	defer cancel()

	var data newData
	formutil.SetBase(&data.Base, r, h.DB, "New Leader", "/leaders")
	data.AuthMethods = models.EnabledAuthMethods
	data.SetError(msg)

	if len(echo) > 0 {
		e := echo[0]
		data.FullName = e.FullName
		data.LoginID = e.LoginID
		data.Email = e.Email
		data.AuthReturnID = e.AuthReturnID
		data.OrgHex = e.OrgHex
		data.Auth = e.Auth
		data.TempPassword = e.TempPassword
		data.Status = e.Status
	}

	// Reload org name if we have the hex
	if data.OrgHex != "" {
		orgID, err := primitive.ObjectIDFromHex(data.OrgHex)
		if err == nil {
			orgName, err := orgutil.GetOrgName(ctx, h.DB, orgID)
			if err == nil {
				data.OrgName = orgName
			} else {
				h.Log.Warn("GetOrgName (re-render)", zap.Error(err))
			}
		}
	}

	templates.Render(w, r, "admin_leader_new", data)
}

func withNewEcho(full, loginID, email, authReturnID, orgHex, auth, tempPassword, status string) newData {
	return newData{
		FullName:     full,
		LoginID:      loginID,
		Email:        email,
		AuthReturnID: authReturnID,
		OrgHex:       orgHex,
		Auth:         auth,
		TempPassword: tempPassword,
		Status:       status,
	}
}
