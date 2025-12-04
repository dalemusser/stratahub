// internal/app/features/leaders/new.go
package leaders

import (
	"context"
	"html/template"
	"net/http"
	"strings"
	"time"

	"github.com/dalemusser/stratahub/internal/app/system/auth"
	"github.com/dalemusser/stratahub/internal/app/system/authz"
	"github.com/dalemusser/stratahub/internal/domain/models"
	"github.com/dalemusser/waffle/templates"
	"github.com/dalemusser/waffle/toolkit/db/mongodb"
	"github.com/dalemusser/waffle/toolkit/text/textfold"
	"github.com/dalemusser/waffle/toolkit/ui/nav"
	"github.com/dalemusser/waffle/toolkit/validate"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

const (
	newTimeoutShort = 5 * time.Second
	newTimeoutMed   = 10 * time.Second
)

// orgOption represents an organization option in the new-leader form.
type orgOption struct {
	ID   primitive.ObjectID
	Name string
}

// newData is the view model for the new-leader page.
type newData struct {
	Title, Role, UserName string
	IsLoggedIn            bool
	BackURL, CurrentPath  string

	Organizations []orgOption

	Error template.HTML

	FullName string
	Email    string
	OrgHex   string
	Auth     string
	Status   string
}

func (h *Handler) ServeNew(w http.ResponseWriter, r *http.Request) {
	u, _ := auth.CurrentUser(r)

	ctx, cancel := context.WithTimeout(r.Context(), newTimeoutShort)
	defer cancel()

	cur, _ := h.DB.Collection("organizations").Find(ctx, bson.M{"status": "active"})
	defer cur.Close(ctx)

	var orgs []orgOption
	for cur.Next(ctx) {
		var o models.Organization
		_ = cur.Decode(&o)
		orgs = append(orgs, orgOption{ID: o.ID, Name: o.Name})
	}

	data := newData{
		Title:         "New Leader",
		IsLoggedIn:    true,
		Role:          "admin",
		UserName:      u.Name,
		BackURL:       nav.ResolveBackURL(r, "/leaders"),
		CurrentPath:   nav.CurrentPath(r),
		Organizations: orgs,
		Status:        "",
	}

	templates.Render(w, r, "admin_leader_new", data)
}

func (h *Handler) HandleCreate(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		h.renderNewWithError(w, r, "Bad request.")
		return
	}

	full := strings.TrimSpace(r.FormValue("full_name"))
	email := strings.ToLower(strings.TrimSpace(r.FormValue("email")))
	authm := strings.ToLower(strings.TrimSpace(r.FormValue("auth_method")))
	orgHex := strings.TrimSpace(r.FormValue("orgID"))

	// New leaders always start as active
	status := "active"

	// Normalize defaults
	if authm == "" {
		authm = "internal"
	}

	// Inline validation (same style as Members/Groups)
	if full == "" {
		h.renderNewWithError(w, r, "Full name is required.",
			withNewEcho(full, email, orgHex, authm, status))
		return
	}
	if email == "" || !validate.SimpleEmailValid(email) {
		h.renderNewWithError(w, r, "A valid email address is required.",
			withNewEcho(full, email, orgHex, authm, status))
		return
	}
	if orgHex == "" || orgHex == "all" {
		h.renderNewWithError(w, r, "Organization is required.",
			withNewEcho(full, email, orgHex, authm, status))
		return
	}

	oid, err := primitive.ObjectIDFromHex(orgHex)
	if err != nil {
		h.renderNewWithError(w, r, "Organization is required.",
			withNewEcho(full, email, orgHex, authm, status))
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), newTimeoutMed)
	defer cancel()

	// Early duplicate check
	if err := h.DB.Collection("users").FindOne(ctx, bson.M{"email": email}).Err(); err == nil {
		h.renderNewWithError(w, r, "A user with that email already exists.",
			withNewEcho(full, email, orgHex, authm, status))
		return
	}

	// Build & insert
	now := time.Now()
	doc := bson.M{
		"_id":             primitive.NewObjectID(),
		"full_name":       full,
		"full_name_ci":    textfold.Fold(full),
		"email":           email,
		"auth_method":     authm,
		"role":            "leader",
		"status":          status,
		"organization_id": oid,
		"created_at":      now,
		"updated_at":      now,
	}
	if _, err := h.DB.Collection("users").InsertOne(ctx, doc); err != nil {
		msg := "Database error while creating leader."
		if mongodb.IsDup(err) {
			msg = "A user with that email already exists."
		}
		h.renderNewWithError(w, r, msg, withNewEcho(full, email, orgHex, authm, status))
		return
	}

	// Success: honor optional return parameter, otherwise go back to leaders list.
	if ret := r.FormValue("return"); ret != "" && strings.HasPrefix(ret, "/") {
		http.Redirect(w, r, ret, http.StatusSeeOther)
		return
	}
	http.Redirect(w, r, nav.ResolveBackURL(r, "/leaders"), http.StatusSeeOther)
}

func (h *Handler) renderNewWithError(w http.ResponseWriter, r *http.Request, msg string, echo ...newData) {
	role, uname, _, _ := authz.UserCtx(r)

	ctx, cancel := context.WithTimeout(r.Context(), newTimeoutShort)
	defer cancel()

	cur, _ := h.DB.Collection("organizations").Find(ctx, bson.M{"status": "active"})
	defer cur.Close(ctx)

	var orgs []orgOption
	for cur.Next(ctx) {
		var o models.Organization
		_ = cur.Decode(&o)
		orgs = append(orgs, orgOption{ID: o.ID, Name: o.Name})
	}

	data := newData{
		Title:         "New Leader",
		IsLoggedIn:    true,
		Role:          role,
		UserName:      uname,
		BackURL:       nav.ResolveBackURL(r, "/leaders"),
		CurrentPath:   nav.CurrentPath(r),
		Organizations: orgs,
		Error:         template.HTML(msg),
	}
	if len(echo) > 0 {
		e := echo[0]
		data.FullName = e.FullName
		data.Email = e.Email
		data.OrgHex = e.OrgHex
		data.Auth = e.Auth
		data.Status = e.Status
	}

	templates.Render(w, r, "admin_leader_new", data)
}

func withNewEcho(full, email, orgHex, auth, status string) newData {
	return newData{
		FullName: full,
		Email:    email,
		OrgHex:   orgHex,
		Auth:     auth,
		Status:   status,
	}
}
