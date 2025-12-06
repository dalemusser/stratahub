// internal/app/features/members/create.go
package members

import (
	"context"
	"html/template"
	"net/http"
	"strings"

	uierrors "github.com/dalemusser/stratahub/internal/app/features/errors"
	userstore "github.com/dalemusser/stratahub/internal/app/store/users"
	"github.com/dalemusser/stratahub/internal/app/system/orgutil"
	"github.com/dalemusser/stratahub/internal/domain/models"
	"github.com/dalemusser/waffle/templates"
	nav "github.com/dalemusser/waffle/toolkit/ui/nav"
	validate "github.com/dalemusser/waffle/toolkit/validate"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.uber.org/zap"
)

// ServeNew renders the "Add Member" form.
func (h *Handler) ServeNew(w http.ResponseWriter, r *http.Request) {
	role, uname, uid, ok := userCtx(r)
	if !ok {
		uierrors.RenderUnauthorized(w, r, "/login")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), membersShortTimeout)
	defer cancel()
	db := h.DB

	selectedOrg := strings.TrimSpace(r.URL.Query().Get("org"))
	data := newData{
		Title:       "Add Member",
		IsLoggedIn:  true,
		Role:        role,
		UserName:    uname,
		BackURL:     nav.ResolveBackURL(r, "/members"),
		CurrentPath: nav.CurrentPath(r),
		Auth:        "internal",
		Status:      "active",
	}

	if role == "leader" {
		var u models.User
		if err := db.Collection("users").
			FindOne(ctx, bson.M{"_id": uid}).
			Decode(&u); err != nil || u.OrganizationID == nil {

			h.Log.Warn("leader org resolve failed", zap.Error(err))
			uierrors.RenderForbidden(w, r, "Your account is not linked to an organization.", nav.ResolveBackURL(r, "/members"))
			return
		}

		data.OrgLocked = true
		data.OrgHex = u.OrganizationID.Hex()

		var org models.Organization
		_ = db.Collection("organizations").
			FindOne(ctx, bson.M{"_id": *u.OrganizationID}).
			Decode(&org)
		data.OrgName = org.Name
	} else {
		// Admin: either locked to selected org (if valid) or show org picker
		if selectedOrg != "" && selectedOrg != "all" {
			if oid, err := primitive.ObjectIDFromHex(selectedOrg); err == nil {
				data.OrgLocked = true
				data.OrgHex = selectedOrg

				var org models.Organization
				_ = db.Collection("organizations").
					FindOne(ctx, bson.M{"_id": oid}).
					Decode(&org)
				data.OrgName = org.Name
			}
		}
		if !data.OrgLocked {
			orgs, err := orgutil.ListActiveOrgs(ctx, db)
			if err != nil {
				h.Log.Warn("list active orgs", zap.Error(err))
				uierrors.RenderForbidden(w, r, "A database error occurred.", nav.ResolveBackURL(r, "/members"))
				return
			}
			for _, o := range orgs {
				data.Orgs = append(data.Orgs, orgOption{ID: o.ID, Name: o.Name})
			}
		}
	}

	templates.Render(w, r, "member_new", data)
}

// HandleCreate processes the Add Member form POST.
func (h *Handler) HandleCreate(w http.ResponseWriter, r *http.Request) {
	role, uname, uid, ok := userCtx(r)
	if !ok {
		uierrors.RenderUnauthorized(w, r, "/login")
		return
	}
	if err := r.ParseForm(); err != nil {
		uierrors.RenderForbidden(w, r, "Bad request.", nav.ResolveBackURL(r, "/members"))
		return
	}

	full := strings.TrimSpace(r.FormValue("full_name"))
	email := strings.ToLower(strings.TrimSpace(r.FormValue("email")))
	authm := strings.ToLower(strings.TrimSpace(r.FormValue("auth_method")))
	// New members always start as active
	status := "active"
	orgHex := strings.TrimSpace(r.FormValue("orgID"))

	// Inline validation with specific messages
	if full == "" {
		h.reRenderNewWithError(w, r, role, uname, newData{
			FullName: full, Email: email, Auth: authm, Status: status, OrgHex: orgHex,
			Error: template.HTML("Full name is required."),
		})
		return
	}
	if email == "" || !validate.SimpleEmailValid(email) {
		h.reRenderNewWithError(w, r, role, uname, newData{
			FullName: full, Email: email, Auth: authm, Status: status, OrgHex: orgHex,
			Error: template.HTML("A valid email address is required."),
		})
		return
	}
	// Organization required for admins (leaders are locked to their org)
	if role != "leader" && (orgHex == "" || orgHex == "all") {
		h.reRenderNewWithError(w, r, role, uname, newData{
			FullName: full, Email: email, Auth: authm, Status: status, OrgHex: orgHex,
			Error: template.HTML("Organization is required."),
		})
		return
	}

	if authm == "" {
		authm = "internal"
	}

	ctx, cancel := context.WithTimeout(r.Context(), membersMedTimeout)
	defer cancel()
	db := h.DB

	us := userstore.New(db)
	if u, err := us.GetByEmail(ctx, email); err == nil && u != nil {
		h.reRenderNewWithError(w, r, role, uname, newData{
			FullName: full, Email: email, Auth: authm, Status: status, OrgHex: orgHex,
			Error: template.HTML("A user with that email already exists."),
		})
		return
	}

	var orgID primitive.ObjectID
	if role == "leader" {
		var me models.User
		if err := db.Collection("users").
			FindOne(ctx, bson.M{"_id": uid}).
			Decode(&me); err != nil || me.OrganizationID == nil {

			h.Log.Warn("leader org resolve failed", zap.Error(err))
			uierrors.RenderForbidden(w, r, "Your account is not linked to an organization.", nav.ResolveBackURL(r, "/members"))
			return
		}
		orgID = *me.OrganizationID
	} else {
		oid, err := primitive.ObjectIDFromHex(orgHex)
		if err != nil {
			h.reRenderNewWithError(w, r, role, uname, newData{
				FullName: full, Email: email, Auth: authm, Status: status, OrgHex: orgHex,
				Error: template.HTML("Bad organization id."),
			})
			return
		}
		orgID = oid
	}

	orgPtr := orgID
	doc := models.User{
		FullName:       full,
		Email:          email,
		AuthMethod:     authm,
		Role:           "member",
		Status:         status,
		OrganizationID: &orgPtr,
	}
	if _, err := us.Create(ctx, doc); err != nil {
		h.Log.Warn("user create failed", zap.Error(err))
		h.reRenderNewWithError(w, r, role, uname, newData{
			FullName: full, Email: email, Auth: authm, Status: status, OrgHex: orgHex,
			Error: template.HTML("Database error while creating member."),
		})
		return
	}

	if ret := r.FormValue("return"); ret != "" && strings.HasPrefix(ret, "/") {
		http.Redirect(w, r, ret, http.StatusSeeOther)
		return
	}
	http.Redirect(w, r, "/members", http.StatusSeeOther)
}

// Helper to re-render form with an inline error message and keep org options for admins.
func (h *Handler) reRenderNewWithError(w http.ResponseWriter, r *http.Request, role, uname string, data newData) {
	data.Title = "Add Member"
	data.IsLoggedIn = true
	data.Role = role
	data.UserName = uname
	data.BackURL = nav.ResolveBackURL(r, "/members")
	data.CurrentPath = nav.CurrentPath(r)

	ctx, cancel := context.WithTimeout(r.Context(), membersShortTimeout)
	defer cancel()
	db := h.DB

	// If OrgLocked and we know the org hex, populate OrgName so the template
	// can show a friendly label when re-rendering.
	if data.OrgLocked && data.OrgHex != "" {
		if oid, err := primitive.ObjectIDFromHex(data.OrgHex); err == nil {
			var org models.Organization
			if err := db.Collection("organizations").
				FindOne(ctx, bson.M{"_id": oid}).
				Decode(&org); err == nil {
				data.OrgName = org.Name
			}
		}
	}

	// Load org choices for admins whenever the org is NOT locked,
	// regardless of whether OrgHex is currently set to something.
	if role == "admin" && !data.OrgLocked {
		orgs, err := orgutil.ListActiveOrgs(ctx, db)
		if err != nil {
			h.Log.Warn("list active orgs (re-render)", zap.Error(err))
		} else {
			for _, o := range orgs {
				data.Orgs = append(data.Orgs, orgOption{ID: o.ID, Name: o.Name})
			}
		}
	}

	templates.Render(w, r, "member_new", data)
}
