// internal/app/features/organizations/edit.go
package organizations

import (
	"context"
	"html/template"
	"net/http"
	"strings"
	"time"

	"github.com/dalemusser/stratahub/internal/app/system/auth"
	"github.com/dalemusser/stratahub/internal/app/system/timezones"
	"github.com/dalemusser/stratahub/internal/domain/models"
	"github.com/dalemusser/waffle/templates"
	mongodb "github.com/dalemusser/waffle/toolkit/db/mongodb"
	textfold "github.com/dalemusser/waffle/toolkit/text/textfold"
	nav "github.com/dalemusser/waffle/toolkit/ui/nav"

	"github.com/go-chi/chi/v5"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

// ServeEdit renders the Edit Organization page.
func (h *Handler) ServeEdit(w http.ResponseWriter, r *http.Request) {
	// Get viewer context for header/sidebar (we still enforce admin in routing).
	role, uname := "admin", ""
	if u, ok := auth.CurrentUser(r); ok {
		role = strings.ToLower(u.Role)
		uname = u.Name
	}

	idHex := chi.URLParam(r, "id")
	oid, err := primitive.ObjectIDFromHex(idHex)
	if err != nil {
		http.Error(w, "bad organization id", http.StatusBadRequest)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), orgsShortTimeout)
	defer cancel()
	db := h.DB

	var org models.Organization
	if err := db.Collection("organizations").
		FindOne(ctx, bson.M{"_id": oid}).
		Decode(&org); err != nil {

		http.NotFound(w, r)
		return
	}

	tzGroups, err := timezones.Groups()
	if err != nil {
		http.Error(w, "failed to load time zones", http.StatusInternalServerError)
		return
	}

	data := editData{
		Title:          "Edit Organization",
		IsLoggedIn:     true,
		Role:           role,
		UserName:       uname,
		ID:             org.ID.Hex(),
		Name:           org.Name,
		City:           org.City,
		State:          org.State,
		TimeZone:       org.TimeZone,
		Contact:        org.ContactInfo,
		BackURL:        "/organizations",   // canonical “back to list”
		CurrentPath:    nav.CurrentPath(r), // for ?return propagation
		TimeZoneGroups: tzGroups,
	}

	templates.Render(w, r, "organization_edit", data)
}

// HandleEdit processes the Edit Organization form POST.
func (h *Handler) HandleEdit(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad form", http.StatusBadRequest)
		return
	}

	idHex := chi.URLParam(r, "id")
	oid, err := primitive.ObjectIDFromHex(idHex)
	if err != nil {
		http.Error(w, "bad organization id", http.StatusBadRequest)
		return
	}

	name := strings.TrimSpace(r.FormValue("name"))
	city := strings.TrimSpace(r.FormValue("city"))
	state := strings.TrimSpace(r.FormValue("state"))
	contact := strings.TrimSpace(r.FormValue("contact"))
	tz := strings.TrimSpace(r.FormValue("timezone"))

	// Viewer context for re-rendering (header, role, name).
	role, uname := "admin", ""
	if u, ok := auth.CurrentUser(r); ok {
		role = strings.ToLower(u.Role)
		uname = u.Name
	}

	tzGroups, err := timezones.Groups()
	if err != nil {
		http.Error(w, "failed to load time zones", http.StatusInternalServerError)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), orgsMedTimeout)
	defer cancel()
	db := h.DB

	// Ensure the organization exists
	var cur models.Organization
	if err := db.Collection("organizations").
		FindOne(ctx, bson.M{"_id": oid}).
		Decode(&cur); err != nil {

		http.NotFound(w, r)
		return
	}

	// Helper to re-render the form with an error and posted values.
	reRender := func(msg string) {
		data := editData{
			Title:          "Edit Organization",
			IsLoggedIn:     true,
			Role:           role,
			UserName:       uname,
			ID:             idHex,
			Name:           name,
			City:           city,
			State:          state,
			TimeZone:       tz,
			Contact:        contact,
			Error:          template.HTML(msg),
			BackURL:        "/organizations",
			CurrentPath:    nav.CurrentPath(r),
			TimeZoneGroups: tzGroups,
		}
		templates.Render(w, r, "organization_edit", data)
	}

	// Validation: name required
	if name == "" {
		reRender("Organization name is required.")
		return
	}

	// Validation: timezone required
	if tz == "" {
		reRender("Time zone is required.")
		return
	}

	// Preflight duplicate by name_ci excluding self
	ci := textfold.Fold(name)
	if err := db.Collection("organizations").
		FindOne(ctx, bson.M{
			"name_ci": ci,
			"_id":     bson.M{"$ne": oid},
		}).Err(); err == nil {

		reRender("Another organization already uses that name.")
		return
	}

	// Build update doc
	up := bson.M{
		"name":         name,
		"name_ci":      ci,
		"city":         city,
		"city_ci":      textfold.Fold(city),
		"state":        state,
		"state_ci":     textfold.Fold(state),
		"contact_info": contact,
		"time_zone":    tz,
		"updated_at":   time.Now(),
	}

	if _, err := db.Collection("organizations").
		UpdateOne(ctx, bson.M{"_id": oid}, bson.M{"$set": up}); err != nil {

		msg := "Database error while updating organization."
		if mongodb.IsDup(err) {
			msg = "Another organization already uses that name."
		}
		reRender(msg)
		return
	}

	// Redirect to explicit return (if safe), otherwise back to the list.
	if ret := strings.TrimSpace(r.FormValue("return")); ret != "" && strings.HasPrefix(ret, "/") {
		http.Redirect(w, r, ret, http.StatusSeeOther)
		return
	}
	http.Redirect(w, r, "/organizations", http.StatusSeeOther)
}
