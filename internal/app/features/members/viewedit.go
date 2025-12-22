// internal/app/features/members/viewedit.go
package members

import (
	"context"
	"errors"
	"net/http"

	uierrors "github.com/dalemusser/stratahub/internal/app/features/errors"
	"github.com/dalemusser/stratahub/internal/app/policy/memberpolicy"
	userstore "github.com/dalemusser/stratahub/internal/app/store/users"
	"github.com/dalemusser/stratahub/internal/app/system/authz"
	"github.com/dalemusser/stratahub/internal/app/system/formutil"
	"github.com/dalemusser/stratahub/internal/app/system/inputval"
	"github.com/dalemusser/stratahub/internal/app/system/navigation"
	"github.com/dalemusser/stratahub/internal/app/system/normalize"
	"github.com/dalemusser/stratahub/internal/app/system/timeouts"
	"github.com/dalemusser/stratahub/internal/app/system/txn"
	"github.com/dalemusser/waffle/pantry/httpnav"
	"github.com/dalemusser/waffle/pantry/templates"
	"github.com/go-chi/chi/v5"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.uber.org/zap"
)

// ServeView – View Member (Back goes to /members or safe return)
// Authorization: Admin can view any member; Leader can only view members in their org.
func (h *Handler) ServeView(w http.ResponseWriter, r *http.Request) {
	role, uname, _, ok := authz.UserCtx(r)
	if !ok {
		uierrors.RenderUnauthorized(w, r, "/login")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), timeouts.Short())
	defer cancel()

	uidHex := chi.URLParam(r, "id")
	uid, err := primitive.ObjectIDFromHex(uidHex)
	if err != nil {
		uierrors.RenderBadRequest(w, r, "Invalid member ID.", httpnav.ResolveBackURL(r, "/members"))
		return
	}

	u, err := h.Users.GetMemberByID(ctx, uid)
	if err != nil {
		uierrors.RenderNotFound(w, r, "Member not found.", httpnav.ResolveBackURL(r, "/members"))
		return
	}

	// Check authorization: can this user view this member?
	canView, policyErr := memberpolicy.CanViewMember(ctx, h.DB, r, u.OrganizationID)
	if policyErr != nil {
		h.ErrLog.LogServerError(w, r, "policy check failed", policyErr, "A database error occurred.", "/members")
		return
	}
	if !canView {
		uierrors.RenderForbidden(w, r, "You don't have permission to view this member.", httpnav.ResolveBackURL(r, "/members"))
		return
	}

	orgName := ""
	if u.OrganizationID != nil {
		o, err := h.Orgs.GetByID(ctx, *u.OrganizationID)
		if err != nil {
			if err == mongo.ErrNoDocuments {
				h.Log.Warn("organization not found for member (may have been deleted)",
					zap.String("user_id", uid.Hex()),
					zap.String("org_id", u.OrganizationID.Hex()))
				orgName = "(Deleted)"
			} else {
				h.ErrLog.LogServerError(w, r, "database error loading organization for member", err, "A database error occurred.", "/members")
				return
			}
		} else {
			orgName = o.Name
		}
	}

	templates.Render(w, r, "member_view", viewData{
		Title:      "View Member",
		IsLoggedIn: true,
		Role:       role,
		UserName:   uname,
		ID:         u.ID.Hex(),
		FullName:   u.FullName,
		Email:      normalize.Email(u.Email),
		OrgName:    orgName,
		Status:     u.Status,
		Auth:       u.AuthMethod,
		BackURL:    httpnav.ResolveBackURL(r, "/members"),
	})
}

// ServeEdit – show edit form (Organization is read-only)
// Authorization: Admin can edit any member; Leader can only edit members in their org.
func (h *Handler) ServeEdit(w http.ResponseWriter, r *http.Request) {
	_, _, _, ok := authz.UserCtx(r)
	if !ok {
		uierrors.RenderUnauthorized(w, r, "/login")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), timeouts.Short())
	defer cancel()

	uidHex := chi.URLParam(r, "id")
	uid, err := primitive.ObjectIDFromHex(uidHex)
	if err != nil {
		uierrors.RenderBadRequest(w, r, "Invalid member ID.", httpnav.ResolveBackURL(r, "/members"))
		return
	}

	u, err := h.Users.GetMemberByID(ctx, uid)
	if err != nil {
		uierrors.RenderNotFound(w, r, "Member not found.", httpnav.ResolveBackURL(r, "/members"))
		return
	}

	// Check authorization: can this user manage this member?
	canManage, policyErr := memberpolicy.CanManageMember(ctx, h.DB, r, u.OrganizationID)
	if policyErr != nil {
		h.ErrLog.LogServerError(w, r, "policy check failed", policyErr, "A database error occurred.", "/members")
		return
	}
	if !canManage {
		uierrors.RenderForbidden(w, r, "You don't have permission to edit this member.", httpnav.ResolveBackURL(r, "/members"))
		return
	}

	orgHex := ""
	orgName := ""
	if u.OrganizationID != nil {
		orgHex = u.OrganizationID.Hex()
		o, err := h.Orgs.GetByID(ctx, *u.OrganizationID)
		if err != nil {
			if err == mongo.ErrNoDocuments {
				h.Log.Warn("organization not found for member (may have been deleted)",
					zap.String("user_id", uid.Hex()),
					zap.String("org_id", u.OrganizationID.Hex()))
				orgName = "(Deleted)"
			} else {
				h.ErrLog.LogServerError(w, r, "database error loading organization for member", err, "A database error occurred.", "/members")
				return
			}
		} else {
			orgName = o.Name
		}
	}

	data := editData{
		ID:       u.ID.Hex(),
		FullName: u.FullName,
		Email:    normalize.Email(u.Email),
		OrgID:    orgHex,  // hidden input will carry this
		OrgName:  orgName, // read-only display
		Status:   u.Status,
		Auth:     u.AuthMethod,
	}
	formutil.SetBase(&data.Base, r, "Edit Member", "/members")

	templates.Render(w, r, "member_edit", data)
}

// HandleEdit – update a member (re-render form on validation errors)
// Authorization: Admin can edit any member; Leader can only edit members in their org.
func (h *Handler) HandleEdit(w http.ResponseWriter, r *http.Request) {
	_, _, _, ok := authz.UserCtx(r)
	if !ok {
		uierrors.RenderUnauthorized(w, r, "/login")
		return
	}
	if err := r.ParseForm(); err != nil {
		uierrors.RenderBadRequest(w, r, "Invalid form data.", httpnav.ResolveBackURL(r, "/members"))
		return
	}

	uidHex := chi.URLParam(r, "id")
	uid, err := primitive.ObjectIDFromHex(uidHex)
	if err != nil {
		uierrors.RenderBadRequest(w, r, "Invalid member ID.", httpnav.ResolveBackURL(r, "/members"))
		return
	}

	// Check authorization before processing the edit
	ctx, cancel := context.WithTimeout(r.Context(), timeouts.Medium())
	defer cancel()

	memberInfo, canManage, policyErr := memberpolicy.CheckMemberAccess(ctx, h.DB, r, uid)
	if policyErr != nil {
		h.ErrLog.LogServerError(w, r, "policy check failed", policyErr, "A database error occurred.", "/members")
		return
	}
	if memberInfo == nil {
		uierrors.RenderNotFound(w, r, "Member not found.", httpnav.ResolveBackURL(r, "/members"))
		return
	}
	if !canManage {
		uierrors.RenderForbidden(w, r, "You don't have permission to edit this member.", httpnav.ResolveBackURL(r, "/members"))
		return
	}

	full := normalize.Name(r.FormValue("full_name"))
	email := normalize.Email(r.FormValue("email"))
	authm := normalize.AuthMethod(r.FormValue("auth_method"))
	status := normalize.Status(r.FormValue("status"))
	orgHex := normalize.QueryParam(r.FormValue("orgID"))

	// Normalize status to allowed values: active or disabled
	if status != "disabled" {
		status = "active"
	}

	// Helper to get org name for re-render
	getOrgName := func() string {
		if oid, e := primitive.ObjectIDFromHex(orgHex); e == nil {
			ctx, cancel := context.WithTimeout(r.Context(), timeouts.Short())
			defer cancel()
			o, err := h.Orgs.GetByID(ctx, oid)
			if err != nil {
				if err != mongo.ErrNoDocuments {
					h.Log.Error("database error loading organization for member (re-render)", zap.Error(err), zap.String("org_id", oid.Hex()))
				}
				return ""
			}
			return o.Name
		}
		return ""
	}

	reRender := func(msg string) {
		data := editData{
			ID:       uidHex,
			FullName: full,
			Email:    email,
			OrgID:    orgHex,
			OrgName:  getOrgName(),
			Status:   status,
			Auth:     authm,
		}
		formutil.SetBase(&data.Base, r, "Edit Member", "/members")
		data.SetError(msg)
		templates.Render(w, r, "member_edit", data)
	}

	// Validate required fields using struct tags
	input := memberInput{FullName: full, Email: email}
	if result := inputval.Validate(input); result.HasErrors() {
		reRender(result.First())
		return
	}

	// Org is required (carried from hidden field)
	if orgHex == "" {
		reRender("An unexpected error occurred. Please reload the page.")
		return
	}

	oid, err := primitive.ObjectIDFromHex(orgHex)
	if err != nil {
		h.ErrLog.LogForbidden(w, r, "bad org id on edit", err, "Bad organization id.", httpnav.ResolveBackURL(r, "/members"))
		return
	}

	// Check duplicate email (exclude this user)
	exists, err := h.Users.EmailExistsForOther(ctx, email, uid)
	if err != nil {
		h.ErrLog.LogServerError(w, r, "database error checking duplicate email", err, "A database error occurred.", "/members")
		return
	}
	if exists {
		reRender("A user with that email already exists.")
		return
	}

	upd := userstore.MemberUpdate{
		FullName:       full,
		Email:          email,
		AuthMethod:     authm,
		Status:         status,
		OrganizationID: oid,
	}
	if err := h.Users.UpdateMember(ctx, uid, upd); err != nil {
		msg := "Database error while updating the member."
		if errors.Is(err, userstore.ErrDuplicateEmail) {
			msg = "A user with that email already exists."
		}
		reRender(msg)
		return
	}

	ret := navigation.SafeBackURL(r, navigation.MembersBackURL)
	http.Redirect(w, r, ret, http.StatusSeeOther)
}

// HandleDelete – remove memberships then delete the user
// Authorization: Admin can delete any member; Leader can only delete members in their org.
func (h *Handler) HandleDelete(w http.ResponseWriter, r *http.Request) {
	_, _, _, ok := authz.UserCtx(r)
	if !ok {
		uierrors.RenderUnauthorized(w, r, "/login")
		return
	}

	uidHex := chi.URLParam(r, "id")
	uid, err := primitive.ObjectIDFromHex(uidHex)
	if err != nil {
		uierrors.RenderBadRequest(w, r, "Invalid member ID.", httpnav.ResolveBackURL(r, "/members"))
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), timeouts.Medium())
	defer cancel()

	// Check authorization before deleting
	memberInfo, canManage, policyErr := memberpolicy.CheckMemberAccess(ctx, h.DB, r, uid)
	if policyErr != nil {
		h.ErrLog.LogServerError(w, r, "policy check failed", policyErr, "A database error occurred.", "/members")
		return
	}
	if memberInfo == nil {
		uierrors.RenderNotFound(w, r, "Member not found.", httpnav.ResolveBackURL(r, "/members"))
		return
	}
	if !canManage {
		uierrors.RenderForbidden(w, r, "You don't have permission to delete this member.", httpnav.ResolveBackURL(r, "/members"))
		return
	}

	// Use transaction for atomic deletion of memberships and user.
	if err := txn.Run(ctx, h.DB, h.Log, func(ctx context.Context) error {
		// 1) Remove ALL memberships for this user (defensive: any role)
		res, err := h.DB.Collection("group_memberships").DeleteMany(ctx, bson.M{"user_id": uid})
		if err != nil {
			return err
		}
		h.Log.Info("memberships deleted for user",
			zap.String("user_id", uid.Hex()),
			zap.Int64("deleted_count", res.DeletedCount))

		// 2) Delete the member user itself (guard on role to be safe)
		deletedCount, err := h.Users.DeleteMember(ctx, uid)
		if err != nil {
			return err
		}
		if deletedCount == 0 {
			h.Log.Info("delete user: no document found (idempotent)", zap.String("user_id", uid.Hex()))
		}
		return nil
	}); err != nil {
		h.ErrLog.LogServerError(w, r, "database error deleting member", err, "A database error occurred.", "/members")
		return
	}

	ret := navigation.SafeBackURL(r, navigation.MembersBackURL)
	http.Redirect(w, r, ret, http.StatusSeeOther)
}
