package resources

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/dalemusser/stratahub/internal/app/system/authz"
	"github.com/dalemusser/stratahub/internal/app/system/inputval"
	"github.com/dalemusser/stratahub/internal/app/system/timeouts"
	"github.com/dalemusser/stratahub/internal/domain/models"
	wafflemongo "github.com/dalemusser/waffle/pantry/mongo"
	"github.com/dalemusser/stratahub/internal/app/system/navigation"
	"github.com/dalemusser/waffle/pantry/text"
	"github.com/dalemusser/waffle/pantry/urlutil"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

// createResourceInput defines validation rules for creating a resource.
type createResourceInput struct {
	Title     string `validate:"required,max=200" label:"Title"`
	LaunchURL string `validate:"required,url" label:"Launch URL"`
	Status    string `validate:"required,oneof=active disabled" label:"Status"`
}

func (h *AdminHandler) ServeNew(w http.ResponseWriter, r *http.Request) {
	vm := resourceFormVM{}
	h.renderNewForm(w, r, vm, "")
}

func (h *AdminHandler) HandleCreate(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		h.ErrLog.LogBadRequest(w, r, "parse form failed", err, "Invalid form data.", "/resources")
		return
	}

	title := strings.TrimSpace(r.FormValue("title"))
	subject := strings.TrimSpace(r.FormValue("subject"))
	description := strings.TrimSpace(r.FormValue("description"))
	launchURL := strings.TrimSpace(r.FormValue("launch_url"))
	typeValue := strings.TrimSpace(r.FormValue("type"))
	status := strings.TrimSpace(r.FormValue("status"))
	showInLibrary := r.FormValue("show_in_library") != ""
	defaultInstructions := strings.TrimSpace(r.FormValue("default_instructions"))

	if typeValue == "" {
		typeValue = models.DefaultResourceType
	}
	if status == "" {
		status = "active"
	}

	// Helper to re-render the form with a message
	reRender := func(msg string) {
		vm := resourceFormVM{
			ResourceTitle:       title,
			Subject:             subject,
			Description:         description,
			LaunchURL:           launchURL,
			Type:                typeValue,
			Status:              status,
			ShowInLibrary:       showInLibrary,
			DefaultInstructions: defaultInstructions,
		}
		h.renderNewForm(w, r, vm, msg)
	}

	// Validate required fields using struct tags
	input := createResourceInput{Title: title, LaunchURL: launchURL, Status: status}
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

	_, uname, memberID, ok := authz.UserCtx(r)
	var createdByID primitive.ObjectID
	var createdByName string
	if ok {
		createdByID = memberID
		createdByName = uname
	}

	ctx, cancel := context.WithTimeout(r.Context(), timeouts.Medium())
	defer cancel()
	db := h.DB

	now := time.Now()
	doc := bson.M{
		"_id":                  primitive.NewObjectID(),
		"title":                title,
		"title_ci":             text.Fold(title),
		"subject":              subject,
		"subject_ci":           text.Fold(subject),
		"description":          description,
		"launch_url":           launchURL,
		"status":               status,
		"type":                 typeValue,
		"show_in_library":      showInLibrary,
		"default_instructions": defaultInstructions,
		"created_at":           now,
		"updated_at":           now,
		"created_by_id":        createdByID,
		"created_by_name":      createdByName,
	}

	_, err := db.Collection("resources").InsertOne(ctx, doc)
	if err != nil {
		msg := "Database error while creating resource."
		if wafflemongo.IsDup(err) {
			msg = "A resource with that title already exists."
		}
		reRender(msg)
		return
	}

	ret := navigation.SafeBackURL(r, navigation.ResourcesBackURL)
	http.Redirect(w, r, ret, http.StatusSeeOther)
}
