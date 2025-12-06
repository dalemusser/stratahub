package resources

import (
	"context"
	"net/http"
	"strings"
	"time"

	uierrors "github.com/dalemusser/stratahub/internal/app/features/errors"
	"github.com/dalemusser/stratahub/internal/app/system/authz"
	"github.com/dalemusser/stratahub/internal/domain/models"
	mongodb "github.com/dalemusser/waffle/toolkit/db/mongodb"
	webutil "github.com/dalemusser/waffle/toolkit/http/webutil"
	textfold "github.com/dalemusser/waffle/toolkit/text/textfold"
	nav "github.com/dalemusser/waffle/toolkit/ui/nav"
	"github.com/go-chi/chi/v5"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

// ServeEdit renders the Edit Resource form for admins.
func (h *AdminHandler) ServeEdit(w http.ResponseWriter, r *http.Request) {
	role, uname, _, ok := authz.UserCtx(r)
	if !ok {
		uierrors.RenderUnauthorized(w, r, "/login")
		return
	}
	if role != "admin" {
		uierrors.RenderForbidden(w, r, "You do not have access to edit resources.", nav.ResolveBackURL(r, "/resources"))
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), resourcesShortTimeout)
	defer cancel()
	db := h.DB

	idHex := chi.URLParam(r, "id")
	oid, err := primitive.ObjectIDFromHex(idHex)
	if err != nil {
		http.Error(w, "bad id", http.StatusBadRequest)
		return
	}

	var res models.Resource
	if err := db.Collection("resources").FindOne(ctx, bson.M{"_id": oid}).Decode(&res); err != nil {
		http.NotFound(w, r)
		return
	}

	// Compute safe return targets for submit/delete actions
	deleteReturn := webutil.SafeReturn(r.URL.Query().Get("return"), idHex /* filter out current id */, "/resources")
	submitReturn := webutil.SafeReturn(r.URL.Query().Get("return"), "", "/resources")

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
		BackURL:             nav.ResolveBackURL(r, "/resources"),
		DeleteReturn:        deleteReturn,
		SubmitReturn:        submitReturn,
		CurrentPath:         nav.CurrentPath(r),
	}

	h.renderEditForm(w, r, vm, "")
}

// HandleEdit processes the Edit Resource form POST for admins.
func (h *AdminHandler) HandleEdit(w http.ResponseWriter, r *http.Request) {
	role, _, _, ok := authz.UserCtx(r)
	if !ok {
		uierrors.RenderUnauthorized(w, r, "/login")
		return
	}
	if role != "admin" {
		uierrors.RenderForbidden(w, r, "You do not have access to edit resources.", nav.ResolveBackURL(r, "/resources"))
		return
	}

	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad form", http.StatusBadRequest)
		return
	}

	idHex := chi.URLParam(r, "id")
	rid, err := primitive.ObjectIDFromHex(idHex)
	if err != nil {
		http.Error(w, "bad id", http.StatusBadRequest)
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
	delReturn := webutil.SafeReturn(r.FormValue("return"), rid.Hex(), "/resources")

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
			SubmitReturn:        webutil.SafeReturn(r.FormValue("return"), "", "/resources"),
		}
		h.renderEditForm(w, r, vm, msg)
	}

	// Validate type
	if !isValidResourceType(typeValue) {
		reRender("Type is invalid.")
		return
	}

	// Validate status
	if status != "active" && status != "disabled" {
		reRender("Status must be Active or Disabled.")
		return
	}

	// Validate title
	if title == "" {
		reRender("Title is required.")
		return
	}

	// Validate launch URL (if provided)
	if launchURL != "" && !webutil.IsValidAbsHTTPURL(launchURL) {
		reRender("Launch URL must be a valid absolute URL (e.g., https://example.com).")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), resourcesMedTimeout)
	defer cancel()
	db := h.DB

	update := bson.M{
		"title":                title,
		"title_ci":             textfold.Fold(title),
		"subject":              subject,
		"subject_ci":           textfold.Fold(subject),
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
		if mongodb.IsDup(err) {
			msg = "A resource with that title already exists."
		}
		reRender(msg)
		return
	}

	// Success: redirect to provided ?return= (sanitized and MUST NOT reference this id)
	ret := webutil.SafeReturn(r.FormValue("return"), rid.Hex(), "/resources")
	http.Redirect(w, r, ret, http.StatusSeeOther)
}
