// internal/app/features/members/viewedit.go
package members

import (
	"context"
	"html/template"
	"net/http"
	"strings"
	"time"

	uierrors "github.com/dalemusser/stratahub/internal/app/features/errors"
	"github.com/dalemusser/stratahub/internal/domain/models"
	"github.com/dalemusser/waffle/templates"
	mongodb "github.com/dalemusser/waffle/toolkit/db/mongodb"
	textfold "github.com/dalemusser/waffle/toolkit/text/textfold"
	nav "github.com/dalemusser/waffle/toolkit/ui/nav"
	"github.com/dalemusser/waffle/toolkit/validate"
	"github.com/go-chi/chi/v5"
	"go.uber.org/zap"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

// ServeView – View Member (Back goes to /members or safe return)
func (h *Handler) ServeView(w http.ResponseWriter, r *http.Request) {
	role, uname, _, ok := userCtx(r)
	if !ok {
		uierrors.RenderUnauthorized(w, r, "/login")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), membersShortTimeout)
	defer cancel()
	db := h.DB

	uidHex := chi.URLParam(r, "id")
	uid, err := primitive.ObjectIDFromHex(uidHex)
	if err != nil {
		uierrors.RenderForbidden(w, r, "Bad member id.", nav.ResolveBackURL(r, "/members"))
		return
	}

	var u models.User
	if err := db.Collection("users").FindOne(ctx, bson.M{"_id": uid, "role": "member"}).Decode(&u); err != nil {
		uierrors.RenderForbidden(w, r, "Member not found.", nav.ResolveBackURL(r, "/members"))
		return
	}

	orgName := ""
	if u.OrganizationID != nil {
		var o models.Organization
		_ = db.Collection("organizations").FindOne(ctx, bson.M{"_id": *u.OrganizationID}).Decode(&o)
		orgName = o.Name
	}

	templates.Render(w, r, "member_view", viewData{
		Title:      "View Member",
		IsLoggedIn: true,
		Role:       role,
		UserName:   uname,
		ID:         u.ID.Hex(),
		FullName:   u.FullName,
		Email:      strings.ToLower(u.Email),
		OrgName:    orgName,
		Status:     u.Status,
		Auth:       u.AuthMethod,
		BackURL:    nav.ResolveBackURL(r, "/members"),
	})
}

// ServeEdit – show edit form (Organization is read-only)
func (h *Handler) ServeEdit(w http.ResponseWriter, r *http.Request) {
	role, uname, _, ok := userCtx(r)
	if !ok {
		uierrors.RenderUnauthorized(w, r, "/login")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), membersShortTimeout)
	defer cancel()
	db := h.DB

	uidHex := chi.URLParam(r, "id")
	uid, err := primitive.ObjectIDFromHex(uidHex)
	if err != nil {
		uierrors.RenderForbidden(w, r, "Bad member id.", nav.ResolveBackURL(r, "/members"))
		return
	}

	var u models.User
	if err := db.Collection("users").FindOne(ctx, bson.M{"_id": uid, "role": "member"}).Decode(&u); err != nil {
		uierrors.RenderForbidden(w, r, "Member not found.", nav.ResolveBackURL(r, "/members"))
		return
	}

	orgHex := ""
	orgName := ""
	if u.OrganizationID != nil {
		orgHex = u.OrganizationID.Hex()
		var o models.Organization
		_ = db.Collection("organizations").FindOne(ctx, bson.M{"_id": *u.OrganizationID}).Decode(&o)
		orgName = o.Name
	}

	templates.Render(w, r, "member_edit", editData{
		Title:       "Edit Member",
		IsLoggedIn:  true,
		Role:        role,
		UserName:    uname,
		ID:          u.ID.Hex(),
		FullName:    u.FullName,
		Email:       strings.ToLower(u.Email),
		OrgID:       orgHex,  // hidden input will carry this
		OrgName:     orgName, // read-only display
		Status:      u.Status,
		Auth:        u.AuthMethod,
		BackURL:     nav.ResolveBackURL(r, "/members"),
		CurrentPath: nav.CurrentPath(r),
	})
}

// HandleEdit – update a member (re-render form on validation errors)
func (h *Handler) HandleEdit(w http.ResponseWriter, r *http.Request) {
	role, uname, _, ok := userCtx(r)
	if !ok {
		uierrors.RenderUnauthorized(w, r, "/login")
		return
	}
	if err := r.ParseForm(); err != nil {
		uierrors.RenderForbidden(w, r, "Bad request.", nav.ResolveBackURL(r, "/members"))
		return
	}

	uidHex := chi.URLParam(r, "id")
	uid, err := primitive.ObjectIDFromHex(uidHex)
	if err != nil {
		uierrors.RenderForbidden(w, r, "Bad member id.", nav.ResolveBackURL(r, "/members"))
		return
	}

	full := strings.TrimSpace(r.FormValue("full_name"))
	email := strings.ToLower(strings.TrimSpace(r.FormValue("email")))
	authm := strings.ToLower(strings.TrimSpace(r.FormValue("auth_method")))
	status := strings.ToLower(strings.TrimSpace(r.FormValue("status")))
	orgHex := strings.TrimSpace(r.FormValue("orgID"))

	// Normalize status to allowed values: active or disabled
	if status != "disabled" {
		status = "active"
	}

	// Specific validation messages
	if full == "" || email == "" || !validate.SimpleEmailValid(email) || orgHex == "" {
		ctx, cancel := context.WithTimeout(r.Context(), membersShortTimeout)
		defer cancel()
		db := h.DB

		orgName := ""
		if oid, e := primitive.ObjectIDFromHex(orgHex); e == nil {
			var o models.Organization
			_ = db.Collection("organizations").FindOne(ctx, bson.M{"_id": oid}).Decode(&o)
			orgName = o.Name
		}

		var msg string
		switch {
		case full == "":
			msg = "Full name is required."
		case email == "" || !validate.SimpleEmailValid(email):
			msg = "A valid email address is required."
		default:
			msg = "An unexpected error occurred. Please reload the page."
		}

		templates.Render(w, r, "member_edit", editData{
			Title:       "Edit Member",
			IsLoggedIn:  true,
			Role:        role,
			UserName:    uname,
			ID:          uidHex,
			FullName:    full,
			Email:       email,
			OrgID:       orgHex,
			OrgName:     orgName,
			Status:      status,
			Auth:        authm,
			BackURL:     nav.ResolveBackURL(r, "/members"),
			CurrentPath: nav.CurrentPath(r),
			Error:       template.HTML(msg),
		})
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), membersMedTimeout)
	defer cancel()
	db := h.DB

	oid, err := primitive.ObjectIDFromHex(orgHex)
	if err != nil {
		h.Log.Warn("bad org id on edit", zap.String("orgID", orgHex), zap.Error(err))
		uierrors.RenderForbidden(w, r, "Bad organization id.", nav.ResolveBackURL(r, "/members"))
		return
	}

	// Check duplicate email (exclude this user)
	if err := db.Collection("users").FindOne(ctx, bson.M{
		"email": email,
		"_id":   bson.M{"$ne": uid},
	}).Err(); err == nil {
		orgName := ""
		if oid2, e := primitive.ObjectIDFromHex(orgHex); e == nil {
			var o models.Organization
			_ = db.Collection("organizations").FindOne(ctx, bson.M{"_id": oid2}).Decode(&o)
			orgName = o.Name
		}
		templates.Render(w, r, "member_edit", editData{
			Title:       "Edit Member",
			IsLoggedIn:  true,
			Role:        role,
			UserName:    uname,
			ID:          uid.Hex(),
			FullName:    full,
			Email:       email,
			OrgID:       oid.Hex(),
			OrgName:     orgName,
			Status:      status,
			Auth:        authm,
			BackURL:     nav.ResolveBackURL(r, "/members"),
			CurrentPath: nav.CurrentPath(r),
			Error:       template.HTML("A user with that email already exists."),
		})
		return
	}

	set := bson.M{
		"full_name":       full,
		"full_name_ci":    textfold.Fold(full),
		"email":           email,
		"auth_method":     authm,
		"status":          status,
		"organization_id": oid,
		"updated_at":      time.Now(),
	}
	if _, err := db.Collection("users").UpdateOne(ctx, bson.M{"_id": uid, "role": "member"}, bson.M{"$set": set}); err != nil {
		orgName := ""
		if oid2, e := primitive.ObjectIDFromHex(orgHex); e == nil {
			var o models.Organization
			_ = db.Collection("organizations").FindOne(ctx, bson.M{"_id": oid2}).Decode(&o)
			orgName = o.Name
		}
		msg := template.HTML("Database error while updating the member.")
		if mongodb.IsDup(err) {
			msg = template.HTML("A user with that email already exists.")
		}
		templates.Render(w, r, "member_edit", editData{
			Title:       "Edit Member",
			IsLoggedIn:  true,
			Role:        role,
			UserName:    uname,
			ID:          uid.Hex(),
			FullName:    full,
			Email:       email,
			OrgID:       oid.Hex(),
			OrgName:     orgName,
			Status:      status,
			Auth:        authm,
			BackURL:     nav.ResolveBackURL(r, "/members"),
			CurrentPath: nav.CurrentPath(r),
			Error:       msg,
		})
		return
	}

	if ret := r.FormValue("return"); ret != "" && strings.HasPrefix(ret, "/") {
		http.Redirect(w, r, ret, http.StatusSeeOther)
		return
	}
	http.Redirect(w, r, "/members", http.StatusSeeOther)
}

// HandleDelete – remove memberships then delete the user
func (h *Handler) HandleDelete(w http.ResponseWriter, r *http.Request) {
	_, _, _, ok := userCtx(r)
	if !ok {
		uierrors.RenderUnauthorized(w, r, "/login")
		return
	}

	uidHex := chi.URLParam(r, "id")
	uid, err := primitive.ObjectIDFromHex(uidHex)
	if err != nil {
		uierrors.RenderForbidden(w, r, "Bad member id.", nav.ResolveBackURL(r, "/members"))
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), membersMedTimeout)
	defer cancel()
	db := h.DB

	// 1) Remove ALL memberships for this user (defensive: any role)
	res, delErr := db.Collection("group_memberships").DeleteMany(ctx, bson.M{"user_id": uid})
	if delErr != nil {
		h.Log.Warn("delete memberships failed", zap.Error(delErr), zap.String("user_id", uid.Hex()))
		uierrors.RenderForbidden(w, r, "Delete failed.", nav.ResolveBackURL(r, "/members"))
		return
	}
	h.Log.Info("memberships deleted for user",
		zap.String("user_id", uid.Hex()),
		zap.Int64("deleted_count", res.DeletedCount))

	// 2) Delete the member user itself (guard on role to be safe)
	ur, uErr := db.Collection("users").DeleteOne(ctx, bson.M{"_id": uid, "role": "member"})
	if uErr != nil {
		h.Log.Warn("delete user failed", zap.Error(uErr), zap.String("user_id", uid.Hex()))
		uierrors.RenderForbidden(w, r, "Delete failed.", nav.ResolveBackURL(r, "/members"))
		return
	}
	if ur.DeletedCount == 0 {
		h.Log.Info("delete user: no document found (idempotent)", zap.String("user_id", uid.Hex()))
	}

	if ret := r.FormValue("return"); ret != "" && strings.HasPrefix(ret, "/") {
		http.Redirect(w, r, ret, http.StatusSeeOther)
		return
	}
	http.Redirect(w, r, "/members", http.StatusSeeOther)
}
