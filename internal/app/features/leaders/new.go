// internal/app/features/leaders/new.go
package leaders

import (
	"context"
	"net/http"
	"time"

	"github.com/dalemusser/stratahub/internal/app/system/formutil"
	"github.com/dalemusser/stratahub/internal/app/system/inputval"
	"github.com/dalemusser/stratahub/internal/app/system/navigation"
	"github.com/dalemusser/stratahub/internal/app/system/normalize"
	"github.com/dalemusser/stratahub/internal/app/system/orgutil"
	"github.com/dalemusser/stratahub/internal/app/system/timeouts"
	wafflemongo "github.com/dalemusser/waffle/pantry/mongo"
	"github.com/dalemusser/waffle/pantry/templates"
	"github.com/dalemusser/waffle/pantry/text"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

// createLeaderInput defines validation rules for creating a new leader.
type createLeaderInput struct {
	FullName string `validate:"required,max=200" label:"Full name"`
	Email    string `validate:"required,email,max=254" label:"Email"`
	OrgID    string `validate:"required,objectid" label:"Organization"`
}

// orgOption is a type alias for organization dropdown options.
type orgOption = orgutil.OrgOption

// newData is the view model for the new-leader page.
type newData struct {
	formutil.Base

	Organizations []orgOption

	FullName string
	Email    string
	OrgHex   string
	Auth     string
	Status   string
}

func (h *Handler) ServeNew(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), timeouts.Short())
	defer cancel()

	orgs, _, err := orgutil.LoadActiveOrgOptions(ctx, h.DB)
	if err != nil {
		h.ErrLog.LogServerError(w, r, "database error loading organizations", err, "A database error occurred.", "/leaders")
		return
	}

	data := newData{Organizations: orgs}
	formutil.SetBase(&data.Base, r, "New Leader", "/leaders")

	templates.Render(w, r, "admin_leader_new", data)
}

func (h *Handler) HandleCreate(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		h.renderNewWithError(w, r, "Bad request.")
		return
	}

	full := normalize.Name(r.FormValue("full_name"))
	email := normalize.Email(r.FormValue("email"))
	authm := normalize.AuthMethod(r.FormValue("auth_method"))
	orgHex := normalize.OrgID(r.FormValue("orgID"))

	// New leaders always start as active
	status := "active"

	// Normalize defaults
	if authm == "" {
		authm = "internal"
	}

	// Validate required fields using struct tags
	input := createLeaderInput{FullName: full, Email: email, OrgID: orgHex}
	if result := inputval.Validate(input); result.HasErrors() {
		h.renderNewWithError(w, r, result.First(),
			withNewEcho(full, email, orgHex, authm, status))
		return
	}

	oid, err := primitive.ObjectIDFromHex(orgHex)
	if err != nil {
		h.renderNewWithError(w, r, "Organization is required.",
			withNewEcho(full, email, orgHex, authm, status))
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), timeouts.Medium())
	defer cancel()

	// Build & insert (duplicate email is caught by unique index)
	now := time.Now()
	doc := bson.M{
		"_id":             primitive.NewObjectID(),
		"full_name":       full,
		"full_name_ci":    text.Fold(full),
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
		if wafflemongo.IsDup(err) {
			msg = "A user with that email already exists."
		}
		h.renderNewWithError(w, r, msg, withNewEcho(full, email, orgHex, authm, status))
		return
	}

	// Success: honor optional return parameter, otherwise go back to leaders list.
	ret := navigation.SafeBackURL(r, navigation.LeadersBackURL)
	http.Redirect(w, r, ret, http.StatusSeeOther)
}

func (h *Handler) renderNewWithError(w http.ResponseWriter, r *http.Request, msg string, echo ...newData) {
	ctx, cancel := context.WithTimeout(r.Context(), timeouts.Short())
	defer cancel()

	orgs, _, err := orgutil.LoadActiveOrgOptions(ctx, h.DB)
	if err != nil {
		h.ErrLog.LogServerError(w, r, "database error loading organizations", err, "A database error occurred.", "/leaders")
		return
	}

	data := newData{Organizations: orgs}
	formutil.SetBase(&data.Base, r, "New Leader", "/leaders")
	data.SetError(msg)

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
