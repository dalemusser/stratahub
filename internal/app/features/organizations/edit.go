// internal/app/features/organizations/edit.go
package organizations

import (
	"context"
	"net/http"
	"strings"

	uierrors "github.com/dalemusser/stratahub/internal/app/features/errors"
	organizationstore "github.com/dalemusser/stratahub/internal/app/store/organizations"
	"github.com/dalemusser/stratahub/internal/app/system/formutil"
	"github.com/dalemusser/stratahub/internal/app/system/inputval"
	"github.com/dalemusser/stratahub/internal/app/system/navigation"
	"github.com/dalemusser/stratahub/internal/app/system/timeouts"
	"github.com/dalemusser/stratahub/internal/app/system/timezones"
	"github.com/dalemusser/stratahub/internal/domain/models"
	"github.com/dalemusser/waffle/pantry/templates"
	"github.com/dalemusser/waffle/pantry/text"

	"github.com/go-chi/chi/v5"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

// editOrgInput defines validation rules for editing an organization.
type editOrgInput struct {
	Name     string `validate:"required,max=200" label:"Organization name"`
	City     string `validate:"max=100" label:"City"`
	State    string `validate:"max=100" label:"State"`
	Contact  string `validate:"max=500" label:"Contact info"`
	TimeZone string `validate:"required,timezone" label:"Time zone"`
}

// ServeEdit renders the Edit Organization page.
// Authorization: RequireRole("admin") middleware in routes.go ensures only admins reach this handler.
func (h *Handler) ServeEdit(w http.ResponseWriter, r *http.Request) {
	idHex := chi.URLParam(r, "id")
	oid, err := primitive.ObjectIDFromHex(idHex)
	if err != nil {
		uierrors.RenderBadRequest(w, r, "Invalid organization ID.", "/organizations")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), timeouts.Short())
	defer cancel()
	db := h.DB

	orgStore := organizationstore.New(db)
	org, err := orgStore.GetByID(ctx, oid)
	if err != nil {
		uierrors.RenderNotFound(w, r, "Organization not found.", "/organizations")
		return
	}

	tzGroups, err := timezones.Groups()
	if err != nil {
		uierrors.RenderServerError(w, r, "Failed to load time zones.", "/organizations")
		return
	}

	data := editData{
		ID:             org.ID.Hex(),
		Name:           org.Name,
		City:           org.City,
		State:          org.State,
		TimeZone:       org.TimeZone,
		Contact:        org.ContactInfo,
		TimeZoneGroups: tzGroups,
	}
	formutil.SetBase(&data.Base, r, h.DB, "Edit Organization", "/organizations")

	templates.Render(w, r, "organization_edit", data)
}

// HandleEdit processes the Edit Organization form POST.
// Authorization: RequireRole("admin") middleware in routes.go ensures only admins reach this handler.
func (h *Handler) HandleEdit(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		h.ErrLog.LogBadRequest(w, r, "parse form failed", err, "Invalid form submission.", "/organizations")
		return
	}

	idHex := chi.URLParam(r, "id")
	oid, err := primitive.ObjectIDFromHex(idHex)
	if err != nil {
		uierrors.RenderBadRequest(w, r, "Invalid organization ID.", "/organizations")
		return
	}

	name := strings.TrimSpace(r.FormValue("name"))
	city := strings.TrimSpace(r.FormValue("city"))
	state := strings.TrimSpace(r.FormValue("state"))
	contact := strings.TrimSpace(r.FormValue("contact"))
	tz := strings.TrimSpace(r.FormValue("timezone"))

	tzGroups, err := timezones.Groups()
	if err != nil {
		uierrors.RenderServerError(w, r, "Failed to load time zones.", "/organizations")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), timeouts.Medium())
	defer cancel()
	db := h.DB

	orgStore := organizationstore.New(db)

	// Ensure the organization exists
	if _, err := orgStore.GetByID(ctx, oid); err != nil {
		uierrors.RenderNotFound(w, r, "Organization not found.", "/organizations")
		return
	}

	// Helper to re-render the form with an error and posted values.
	reRender := func(msg string) {
		data := editData{
			ID:             idHex,
			Name:           name,
			City:           city,
			State:          state,
			TimeZone:       tz,
			Contact:        contact,
			TimeZoneGroups: tzGroups,
		}
		formutil.SetBase(&data.Base, r, h.DB, "Edit Organization", "/organizations")
		data.SetError(msg)
		templates.Render(w, r, "organization_edit", data)
	}

	// Validate required fields and length limits using struct tags
	input := editOrgInput{Name: name, City: city, State: state, Contact: contact, TimeZone: tz}
	if result := inputval.Validate(input); result.HasErrors() {
		reRender(result.First())
		return
	}

	// Validate timezone is in the curated list
	if !timezones.Valid(tz) {
		reRender("Please select a valid time zone.")
		return
	}

	// Preflight duplicate by name_ci excluding self
	ci := text.Fold(name)
	exists, err := orgStore.NameExistsForOther(ctx, ci, oid)
	if err != nil {
		reRender("Database error checking organization name.")
		return
	}
	if exists {
		reRender("Another organization already uses that name.")
		return
	}

	// Update via store
	updateOrg := models.Organization{
		Name:        name,
		City:        city,
		State:       state,
		ContactInfo: contact,
		TimeZone:    tz,
	}
	if err := orgStore.Update(ctx, oid, updateOrg); err != nil {
		msg := "Database error while updating organization."
		if err == organizationstore.ErrDuplicateOrganization {
			msg = "Another organization already uses that name."
		}
		reRender(msg)
		return
	}

	// Redirect to explicit return (if safe), otherwise back to the list.
	ret := navigation.SafeBackURL(r, navigation.OrganizationsBackURL)
	http.Redirect(w, r, ret, http.StatusSeeOther)
}
