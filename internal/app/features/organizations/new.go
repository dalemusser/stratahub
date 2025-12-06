// internal/app/features/organizations/new.go
package organizations

import (
	"context"
	"html/template"
	"net/http"
	"strings"
	"time"

	uierrors "github.com/dalemusser/stratahub/internal/app/features/errors"
	"github.com/dalemusser/stratahub/internal/app/system/authz"
	"github.com/dalemusser/stratahub/internal/app/system/timezones"
	"github.com/dalemusser/stratahub/internal/domain/models"
	"github.com/dalemusser/waffle/templates"
	mongodb "github.com/dalemusser/waffle/toolkit/db/mongodb"
	textfold "github.com/dalemusser/waffle/toolkit/text/textfold"
	nav "github.com/dalemusser/waffle/toolkit/ui/nav"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.uber.org/zap"
)

// ServeNew renders the "New Organization" form.
func (h *Handler) ServeNew(w http.ResponseWriter, r *http.Request) {
	role, uname, _, ok := authz.UserCtx(r)
	if !ok {
		uierrors.RenderUnauthorized(w, r, "/login")
		return
	}
	// Defensive: require admin even though routes should already gate this.
	if !authz.IsAdmin(r) {
		uierrors.RenderForbidden(w, r, "You do not have access to organizations.", nav.ResolveBackURL(r, "/dashboard"))
		return
	}

	tzGroups, err := timezones.Groups()
	if err != nil {
		h.Log.Error("failed to load time zones", zap.Error(err))
		http.Error(w, "failed to load time zones", http.StatusInternalServerError)
		return
	}

	data := newData{
		Title:          "New Organization",
		IsLoggedIn:     true,
		Role:           role,
		UserName:       uname,
		BackURL:        nav.ResolveBackURL(r, "/organizations"),
		CurrentPath:    nav.CurrentPath(r),
		TimeZoneGroups: tzGroups,
	}

	// Template name updated to "organization_new" (no admin_ prefix).
	templates.Render(w, r, "organization_new", data)
}

// HandleCreate processes the New Organization form submission.
func (h *Handler) HandleCreate(w http.ResponseWriter, r *http.Request) {
	role, uname, _, ok := authz.UserCtx(r)
	if !ok {
		uierrors.RenderUnauthorized(w, r, "/login")
		return
	}
	if !authz.IsAdmin(r) {
		uierrors.RenderForbidden(w, r, "You do not have access to organizations.", nav.ResolveBackURL(r, "/dashboard"))
		return
	}

	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad form", http.StatusBadRequest)
		return
	}

	name := strings.TrimSpace(r.FormValue("name"))
	city := strings.TrimSpace(r.FormValue("city"))
	state := strings.TrimSpace(r.FormValue("state"))
	contact := strings.TrimSpace(r.FormValue("contact"))
	tz := strings.TrimSpace(r.FormValue("timezone"))
	status := "active"

	ctx, cancel := context.WithTimeout(r.Context(), orgsMedTimeout)
	defer cancel()
	db := h.DB

	tzGroups, err := timezones.Groups()
	if err != nil {
		h.Log.Error("failed to load time zones", zap.Error(err))
		http.Error(w, "failed to load time zones", http.StatusInternalServerError)
		return
	}

	renderWithError := func(msg string) {
		data := newData{
			Title:          "New Organization",
			IsLoggedIn:     true,
			Role:           role,
			UserName:       uname,
			Name:           name,
			City:           city,
			State:          state,
			TimeZone:       tz,
			Contact:        contact,
			Error:          template.HTML(msg),
			BackURL:        nav.ResolveBackURL(r, "/organizations"),
			CurrentPath:    nav.CurrentPath(r),
			TimeZoneGroups: tzGroups,
		}
		templates.Render(w, r, "organization_new", data)
	}

	// Validation: name required
	if name == "" {
		renderWithError("Organization name is required.")
		return
	}

	// Validation: timezone required
	if tz == "" {
		renderWithError("Time zone is required.")
		return
	}

	// Preflight duplicate by name_ci
	ci := textfold.Fold(name)
	if err := db.Collection("organizations").
		FindOne(ctx, bson.M{"name_ci": ci}).Err(); err == nil {
		renderWithError("An organization with that name already exists.")
		return
	}

	doc := models.Organization{
		ID:          primitive.NewObjectID(),
		Name:        name,
		NameCI:      ci,
		City:        city,
		CityCI:      textfold.Fold(city),
		State:       state,
		StateCI:     textfold.Fold(state),
		ContactInfo: contact,
		TimeZone:    tz,
		Status:      status,
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}

	if _, err := db.Collection("organizations").InsertOne(ctx, doc); err != nil {
		msg := "Database error while creating organization."
		if mongodb.IsDup(err) {
			msg = "An organization with that name already exists."
		}
		h.Log.Warn("insert organization failed", zap.Error(err))
		renderWithError(msg)
		return
	}

	// Redirect (honor return; fallback to /organizations)
	ret := strings.TrimSpace(r.FormValue("return"))
	if ret == "" || !strings.HasPrefix(ret, "/") {
		ret = "/organizations"
	}
	http.Redirect(w, r, ret, http.StatusSeeOther)
}
