package resources

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/dalemusser/stratahub/internal/app/system/authz"
	"github.com/dalemusser/stratahub/internal/domain/models"
	"github.com/dalemusser/waffle/toolkit/db/mongodb"
	"github.com/dalemusser/waffle/toolkit/http/webutil"
	"github.com/dalemusser/waffle/toolkit/text/textfold"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

func (h *AdminHandler) ServeNew(w http.ResponseWriter, r *http.Request) {
	vm := resourceFormVM{}
	h.renderNewForm(w, r, vm, "")
}

func (h *AdminHandler) HandleCreate(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad form", http.StatusBadRequest)
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
	if !isValidResourceType(typeValue) {
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
		h.renderNewForm(w, r, vm, "Type is invalid.")
		return
	}

	if status == "" {
		status = "active"
	}
	if status != "active" && status != "disabled" {
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
		h.renderNewForm(w, r, vm, "Status must be Active or Disabled.")
		return
	}

	if title == "" {
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
		h.renderNewForm(w, r, vm, "Title is required.")
		return
	}

	if launchURL != "" && !webutil.IsValidAbsHTTPURL(launchURL) {
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
		h.renderNewForm(w, r, vm, "Launch URL must be a valid absolute URL (e.g., https://example.com).")
		return
	}

	_, uname, memberID, ok := authz.UserCtx(r)
	var createdByID primitive.ObjectID
	var createdByName string
	if ok {
		createdByID = memberID
		createdByName = uname
	}

	ctx, cancel := context.WithTimeout(r.Context(), resourcesMedTimeout)
	defer cancel()
	db := h.DB

	now := time.Now()
	doc := bson.M{
		"_id":                  primitive.NewObjectID(),
		"title":                title,
		"title_ci":             textfold.Fold(title),
		"subject":              subject,
		"subject_ci":           textfold.Fold(subject),
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
		if mongodb.IsDup(err) {
			msg = "A resource with that title already exists."
		}
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
		return
	}

	ret := r.FormValue("return")
	if ret != "" && strings.HasPrefix(ret, "/") {
		http.Redirect(w, r, ret, http.StatusSeeOther)
		return
	}

	http.Redirect(w, r, "/resources", http.StatusSeeOther)
}
