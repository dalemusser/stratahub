package resources

import (
	"context"
	"net/http"
	"strings"
	"time"

	uierrors "github.com/dalemusser/stratahub/internal/app/features/errors"
	"github.com/dalemusser/stratahub/internal/app/system/authz"
	"github.com/dalemusser/stratahub/internal/app/system/inputval"
	"github.com/dalemusser/stratahub/internal/app/system/timeouts"
	"github.com/dalemusser/stratahub/internal/domain/models"
	"github.com/dalemusser/waffle/pantry/httpnav"
	wafflemongo "github.com/dalemusser/waffle/pantry/mongo"
	"github.com/dalemusser/waffle/pantry/text"
	"github.com/dalemusser/waffle/pantry/urlutil"
	"github.com/go-chi/chi/v5"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

// editResourceInput defines validation rules for editing a resource.
type editResourceInput struct {
	Title     string `validate:"required,max=200" label:"Title"`
	LaunchURL string `validate:"required,url" label:"Launch URL"`
	Status    string `validate:"required,oneof=active disabled" label:"Status"`
}

// ServeEdit renders the Edit Resource form for admins.
// Authorization: RequireRole("admin") middleware in routes.go ensures only admins reach this handler.
func (h *AdminHandler) ServeEdit(w http.ResponseWriter, r *http.Request) {
	role, uname, _, _ := authz.UserCtx(r)

	ctx, cancel := context.WithTimeout(r.Context(), timeouts.Short())
	defer cancel()
	db := h.DB

	idHex := chi.URLParam(r, "id")
	oid, err := primitive.ObjectIDFromHex(idHex)
	if err != nil {
		uierrors.RenderBadRequest(w, r, "Invalid resource ID.", "/resources")
		return
	}

	var res models.Resource
	if err := db.Collection("resources").FindOne(ctx, bson.M{"_id": oid}).Decode(&res); err != nil {
		uierrors.RenderNotFound(w, r, "Resource not found.", "/resources")
		return
	}

	// Compute safe return targets for submit/delete actions
	deleteReturn := urlutil.SafeReturn(r.URL.Query().Get("return"), idHex /* filter out current id */, "/resources")
	submitReturn := urlutil.SafeReturn(r.URL.Query().Get("return"), "", "/resources")

	vm := resourceFormVM{
		Title:               "Edit Resource",
		IsLoggedIn:          true,
		Role:                role,
		UserName:            uname,
		ID:                  res.ID.Hex(),
		ResourceTitle:       res.Title,
		Subject:             res.Subject,
		Description:         res.Description,
		LaunchURL:           res.LaunchURL,
		Type:                res.Type,
		Status:              res.Status,
		ShowInLibrary:       res.ShowInLibrary,
		DefaultInstructions: res.DefaultInstructions,
		BackURL:             httpnav.ResolveBackURL(r, "/resources"),
		DeleteReturn:        deleteReturn,
		SubmitReturn:        submitReturn,
		CurrentPath:         httpnav.CurrentPath(r),
	}

	h.renderEditForm(w, r, vm, "")
}

// HandleEdit processes the Edit Resource form POST for admins.
// Authorization: RequireRole("admin") middleware in routes.go ensures only admins reach this handler.
func (h *AdminHandler) HandleEdit(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		h.ErrLog.LogBadRequest(w, r, "parse form failed", err, "Invalid form data.", "/resources")
		return
	}

	idHex := chi.URLParam(r, "id")
	rid, err := primitive.ObjectIDFromHex(idHex)
	if err != nil {
		uierrors.RenderBadRequest(w, r, "Invalid resource ID.", "/resources")
		return
	}

	title := strings.TrimSpace(r.FormValue("title"))
	subject := strings.TrimSpace(r.FormValue("subject"))
	description := strings.TrimSpace(r.FormValue("description"))
	launchURL := strings.TrimSpace(r.FormValue("launch_url"))

	typeValue := strings.TrimSpace(r.FormValue("type"))
	if typeValue == "" {
		typeValue = models.DefaultResourceType
	}

	status := strings.TrimSpace(r.FormValue("status"))
	if status == "" {
		status = "active"
	}

	showInLibrary := r.FormValue("show_in_library") != ""
	defaultInstructions := strings.TrimSpace(r.FormValue("default_instructions"))

	// Delete-return should never redirect back to a URL containing this id.
	delReturn := urlutil.SafeReturn(r.FormValue("return"), rid.Hex(), "/resources")

	// Helper to re-render the form with a message.
	reRender := func(msg string) {
		vm := resourceFormVM{
			ID:                  rid.Hex(),
			ResourceTitle:       title,
			Subject:             subject,
			Description:         description,
			LaunchURL:           launchURL,
			Type:                typeValue,
			Status:              status,
			ShowInLibrary:       showInLibrary,
			DefaultInstructions: defaultInstructions,
			DeleteReturn:        delReturn,
			SubmitReturn:        urlutil.SafeReturn(r.FormValue("return"), "", "/resources"),
		}
		h.renderEditForm(w, r, vm, msg)
	}

	// Validate required fields using struct tags
	input := editResourceInput{Title: title, LaunchURL: launchURL, Status: status}
	if result := inputval.Validate(input); result.HasErrors() {
		reRender(result.First())
		return
	}

	// Validate resource type
	if !inputval.IsValidResourceType(typeValue) {
		reRender("Type is invalid.")
		return
	}

	// Validate launch URL is an absolute HTTP/HTTPS URL
	if !urlutil.IsValidAbsHTTPURL(launchURL) {
		reRender("Launch URL must be a valid absolute URL (e.g., https://example.com).")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), timeouts.Medium())
	defer cancel()
	db := h.DB

	update := bson.M{
		"title":                title,
		"title_ci":             text.Fold(title),
		"subject":              subject,
		"subject_ci":           text.Fold(subject),
		"description":          description,
		"launch_url":           launchURL,
		"type":                 typeValue,
		"status":               status,
		"show_in_library":      showInLibrary,
		"default_instructions": defaultInstructions,
		"updated_at":           time.Now(),
	}

	if _, err := db.Collection("resources").UpdateOne(ctx, bson.M{"_id": rid}, bson.M{"$set": update}); err != nil {
		msg := "Database error while updating resource."
		if wafflemongo.IsDup(err) {
			msg = "A resource with that title already exists."
		}
		reRender(msg)
		return
	}

	// Success: redirect to provided ?return= (sanitized and MUST NOT reference this id)
	ret := urlutil.SafeReturn(r.FormValue("return"), rid.Hex(), "/resources")
	http.Redirect(w, r, ret, http.StatusSeeOther)
}
