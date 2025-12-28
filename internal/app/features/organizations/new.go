// internal/app/features/organizations/new.go
package organizations

import (
	"context"
	"net/http"
	"strings"

	organizationstore "github.com/dalemusser/stratahub/internal/app/store/organizations"
	"github.com/dalemusser/stratahub/internal/app/system/formutil"
	"github.com/dalemusser/stratahub/internal/app/system/inputval"
	"github.com/dalemusser/stratahub/internal/app/system/navigation"
	"github.com/dalemusser/stratahub/internal/app/system/timeouts"
	"github.com/dalemusser/stratahub/internal/app/system/timezones"
	"github.com/dalemusser/stratahub/internal/domain/models"
	"github.com/dalemusser/waffle/pantry/templates"
)

// createOrgInput defines validation rules for creating an organization.
type createOrgInput struct {
	Name     string `validate:"required,max=200" label:"Organization name"`
	City     string `validate:"max=100" label:"City"`
	State    string `validate:"max=100" label:"State"`
	Contact  string `validate:"max=500" label:"Contact info"`
	TimeZone string `validate:"required,timezone" label:"Time zone"`
}

// ServeNew renders the "New Organization" form.
// Authorization: RequireRole("admin") middleware in routes.go ensures only admins reach this handler.
func (h *Handler) ServeNew(w http.ResponseWriter, r *http.Request) {
	tzGroups, err := timezones.Groups()
	if err != nil {
		h.ErrLog.LogServerError(w, r, "failed to load time zones", err, "Failed to load time zones.", "/organizations")
		return
	}

	data := newData{TimeZoneGroups: tzGroups}
	formutil.SetBase(&data.Base, r, h.DB, "New Organization", "/organizations")

	// Template name updated to "organization_new" (no admin_ prefix).
	templates.Render(w, r, "organization_new", data)
}

// HandleCreate processes the New Organization form submission.
// Authorization: RequireRole("admin") middleware in routes.go ensures only admins reach this handler.
func (h *Handler) HandleCreate(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		h.ErrLog.LogBadRequest(w, r, "parse form failed", err, "Invalid form submission.", "/organizations")
		return
	}

	name := strings.TrimSpace(r.FormValue("name"))
	city := strings.TrimSpace(r.FormValue("city"))
	state := strings.TrimSpace(r.FormValue("state"))
	contact := strings.TrimSpace(r.FormValue("contact"))
	tz := strings.TrimSpace(r.FormValue("timezone"))

	ctx, cancel := context.WithTimeout(r.Context(), timeouts.Medium())
	defer cancel()
	db := h.DB

	tzGroups, err := timezones.Groups()
	if err != nil {
		h.ErrLog.LogServerError(w, r, "failed to load time zones", err, "Failed to load time zones.", "/organizations")
		return
	}

	renderWithError := func(msg string) {
		data := newData{
			Name:           name,
			City:           city,
			State:          state,
			TimeZone:       tz,
			Contact:        contact,
			TimeZoneGroups: tzGroups,
		}
		formutil.SetBase(&data.Base, r, h.DB, "New Organization", "/organizations")
		data.SetError(msg)
		templates.Render(w, r, "organization_new", data)
	}

	// Validate required fields and length limits using struct tags
	input := createOrgInput{Name: name, City: city, State: state, Contact: contact, TimeZone: tz}
	if result := inputval.Validate(input); result.HasErrors() {
		renderWithError(result.First())
		return
	}

	// Validate timezone is in the curated list
	if !timezones.Valid(tz) {
		renderWithError("Please select a valid time zone.")
		return
	}

	// Create organization via store (handles ID, CI fields, timestamps, and duplicate detection)
	orgStore := organizationstore.New(db)
	org := models.Organization{
		Name:        name,
		City:        city,
		State:       state,
		ContactInfo: contact,
		TimeZone:    tz,
	}

	if _, err := orgStore.Create(ctx, org); err != nil {
		msg := "Database error while creating organization."
		if err == organizationstore.ErrDuplicateOrganization {
			msg = "An organization with that name already exists."
		}
		renderWithError(msg)
		return
	}

	// Redirect (honor return; fallback to /organizations)
	ret := navigation.SafeBackURL(r, navigation.OrganizationsBackURL)
	http.Redirect(w, r, ret, http.StatusSeeOther)
}
