// internal/app/features/members/create.go
package members

import (
	"context"
	"errors"
	"net/http"
	"strings"

	uierrors "github.com/dalemusser/stratahub/internal/app/features/errors"
	"github.com/dalemusser/stratahub/internal/app/policy/memberpolicy"
	userstore "github.com/dalemusser/stratahub/internal/app/store/users"
	"github.com/dalemusser/stratahub/internal/app/system/authutil"
	"github.com/dalemusser/stratahub/internal/app/system/authz"
	"github.com/dalemusser/stratahub/internal/app/system/formutil"
	"github.com/dalemusser/stratahub/internal/app/system/inputval"
	"github.com/dalemusser/stratahub/internal/app/system/navigation"
	"github.com/dalemusser/stratahub/internal/app/system/normalize"
	"github.com/dalemusser/stratahub/internal/app/system/orgutil"
	"github.com/dalemusser/stratahub/internal/app/system/status"
	"github.com/dalemusser/stratahub/internal/app/system/timeouts"
	"github.com/dalemusser/stratahub/internal/domain/models"
	"github.com/dalemusser/waffle/pantry/httpnav"
	wafflemongo "github.com/dalemusser/waffle/pantry/mongo"
	"github.com/dalemusser/waffle/pantry/templates"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.uber.org/zap"
)

// ServeNew renders the "Add Member" form.
func (h *Handler) ServeNew(w http.ResponseWriter, r *http.Request) {
	role, _, uid, ok := authz.UserCtx(r)
	if !ok {
		uierrors.RenderUnauthorized(w, r, "/login")
		return
	}

	// Check authorization using policy layer
	listScope := memberpolicy.CanListMembers(r)
	if !listScope.CanList {
		uierrors.RenderForbidden(w, r, "You don't have permission to add members.", httpnav.ResolveBackURL(r, "/"))
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), timeouts.Short())
	defer cancel()
	db := h.DB

	selectedOrg := normalize.QueryParam(r.URL.Query().Get("org"))
	data := newData{
		AuthMethods: models.EnabledAuthMethods,
		Auth:        "trust",
		Status:      status.Active,
	}
	formutil.SetBase(&data.Base, r, db, "Add Member", "/members")

	if role == "leader" {
		// Leader: org is always locked to their org
		orgID, orgName, err := orgutil.ResolveLeaderOrg(ctx, db, uid)
		if errors.Is(err, orgutil.ErrUserNotFound) {
			uierrors.RenderForbidden(w, r, "User not found.", httpnav.ResolveBackURL(r, "/members"))
			return
		}
		if errors.Is(err, orgutil.ErrNoOrganization) {
			uierrors.RenderForbidden(w, r, "Your account is not linked to an organization.", httpnav.ResolveBackURL(r, "/members"))
			return
		}
		if err != nil {
			h.ErrLog.LogServerError(w, r, "database error resolving leader org", err, "A database error occurred.", "/members")
			return
		}
		data.OrgLocked = true
		data.OrgHex = orgID.Hex()
		data.OrgName = orgName
	} else {
		// Admin/Coordinator: org can be passed via URL param (optional - can select via picker)
		if selectedOrg != "" && selectedOrg != "all" {
			orgID, orgName, err := orgutil.ResolveActiveOrgFromHex(ctx, db, selectedOrg)
			if err != nil {
				if orgutil.IsExpectedOrgError(err) {
					// Org not found - just show page without org selected
					h.Log.Warn("org not found or inactive", zap.String("org", selectedOrg))
				} else {
					h.ErrLog.LogServerError(w, r, "database error loading organization", err, "A database error occurred.", "/members")
					return
				}
			} else {
				// Coordinator access check: verify access to specified organization
				if role == "coordinator" && !authz.CanAccessOrg(r, orgID) {
					uierrors.RenderForbidden(w, r, "You don't have access to this organization.", "/members")
					return
				}
				data.OrgHex = orgID.Hex()
				data.OrgName = orgName
			}
		}
		// OrgLocked stays false for admin/coordinator - they can change via picker
	}

	templates.Render(w, r, "member_new", data)
}

// HandleCreate processes the Add Member form POST.
func (h *Handler) HandleCreate(w http.ResponseWriter, r *http.Request) {
	role, _, uid, ok := authz.UserCtx(r)
	if !ok {
		uierrors.RenderUnauthorized(w, r, "/login")
		return
	}
	if err := r.ParseForm(); err != nil {
		uierrors.RenderForbidden(w, r, "Bad request.", httpnav.ResolveBackURL(r, "/members"))
		return
	}

	// Check authorization using policy layer
	listScope := memberpolicy.CanListMembers(r)
	if !listScope.CanList {
		uierrors.RenderForbidden(w, r, "You don't have permission to add members.", httpnav.ResolveBackURL(r, "/"))
		return
	}

	full := normalize.Name(r.FormValue("full_name"))
	loginID := normalize.Email(r.FormValue("login_id"))
	email := normalize.Email(r.FormValue("email"))
	authReturnID := strings.TrimSpace(r.FormValue("auth_return_id"))
	authm := normalize.AuthMethod(r.FormValue("auth_method"))
	tempPassword := strings.TrimSpace(r.FormValue("temp_password"))
	// New members always start as active
	stat := status.Active
	orgHex := normalize.OrgID(r.FormValue("orgID"))

	// Normalize defaults
	if authm == "" {
		authm = "trust"
	}

	// Helper to build echo data
	echoData := func() newData {
		return newData{
			FullName: full, LoginID: loginID, Email: email, AuthReturnID: authReturnID,
			Auth: authm, TempPassword: tempPassword, Status: stat, OrgHex: orgHex,
		}
	}

	// Validate required fields using struct tags
	input := memberInput{FullName: full}
	if result := inputval.Validate(input); result.HasErrors() {
		h.reRenderNewWithError(w, r, echoData(), result.First())
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
		h.reRenderNewWithError(w, r, echoData(), err.Error())
		return
	}

	// Organization required for admins/coordinators (leaders are locked to their org)
	if role != "leader" && orgHex == "" {
		h.reRenderNewWithError(w, r, echoData(), "Organization is required.")
		return
	}

	// Coordinator access check: verify access to specified organization
	if role == "coordinator" && orgHex != "" {
		if oid, err := primitive.ObjectIDFromHex(orgHex); err == nil {
			if !authz.CanAccessOrg(r, oid) {
				uierrors.RenderForbidden(w, r, "You don't have access to this organization.", "/members")
				return
			}
		}
	}

	ctx, cancel := context.WithTimeout(r.Context(), timeouts.Medium())
	defer cancel()
	db := h.DB

	us := userstore.New(db)

	var orgID primitive.ObjectID
	if role == "leader" {
		oid, _, err := orgutil.ResolveLeaderOrg(ctx, db, uid)
		if errors.Is(err, orgutil.ErrUserNotFound) {
			uierrors.RenderForbidden(w, r, "User not found.", httpnav.ResolveBackURL(r, "/members"))
			return
		}
		if errors.Is(err, orgutil.ErrNoOrganization) {
			uierrors.RenderForbidden(w, r, "Your account is not linked to an organization.", httpnav.ResolveBackURL(r, "/members"))
			return
		}
		if err != nil {
			h.ErrLog.LogServerError(w, r, "database error resolving leader org", err, "A database error occurred.", "/members")
			return
		}
		orgID = oid
	} else {
		oid, _, err := orgutil.ResolveActiveOrgFromHex(ctx, db, orgHex)
		switch {
		case err == nil:
			orgID = oid
		case errors.Is(err, orgutil.ErrBadOrgID):
			h.reRenderNewWithError(w, r, echoData(), "Invalid organization ID.")
			return
		case errors.Is(err, orgutil.ErrOrgNotFound), errors.Is(err, orgutil.ErrOrgNotActive):
			h.reRenderNewWithError(w, r, echoData(), "Organization not found or is not active.")
			return
		default:
			h.ErrLog.LogServerError(w, r, "database error validating organization", err, "A database error occurred.", "/members")
			return
		}
	}

	orgPtr := orgID
	effectiveLoginID := authResult.EffectiveLoginID
	doc := models.User{
		FullName:       full,
		LoginID:        &effectiveLoginID,
		AuthMethod:     authm,
		Role:           "member",
		Status:         stat,
		OrganizationID: &orgPtr,
	}

	// Add optional email if provided
	if authResult.Email != nil {
		doc.Email = authResult.Email
	}

	// Add optional auth_return_id if provided
	if authResult.AuthReturnID != nil {
		doc.AuthReturnID = authResult.AuthReturnID
	}

	// Add password hash if provided
	if authResult.PasswordHash != nil {
		doc.PasswordHash = authResult.PasswordHash
		doc.PasswordTemp = authResult.PasswordTemp
	}

	if _, err := us.Create(ctx, doc); err != nil {
		if wafflemongo.IsDup(err) {
			h.reRenderNewWithError(w, r, echoData(), "A user with that login ID already exists.")
			return
		}
		h.ErrLog.LogServerError(w, r, "database error creating user", err, "A database error occurred.", "/members")
		return
	}

	ret := navigation.SafeBackURL(r, navigation.MembersBackURL)
	http.Redirect(w, r, ret, http.StatusSeeOther)
}

// Helper to re-render form with an inline error message and keep org options for admins.
func (h *Handler) reRenderNewWithError(w http.ResponseWriter, r *http.Request, data newData, msg string) {
	formutil.SetBase(&data.Base, r, h.DB, "Add Member", "/members")
	data.AuthMethods = models.EnabledAuthMethods
	data.SetError(msg)

	ctx, cancel := context.WithTimeout(r.Context(), timeouts.Short())
	defer cancel()
	db := h.DB

	// Org is always locked now, populate OrgName so the template shows the name
	if data.OrgHex != "" {
		if oid, err := primitive.ObjectIDFromHex(data.OrgHex); err == nil {
			name, err := orgutil.GetOrgName(ctx, db, oid)
			if err != nil {
				h.ErrLog.LogServerError(w, r, "database error loading organization (re-render)", err, "A database error occurred.", "/members")
				return
			}
			data.OrgName = name
			data.OrgLocked = true
		}
	}

	templates.Render(w, r, "member_new", data)
}
