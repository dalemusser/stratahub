package resources

import (
	"context"
	"net/http"
	"strings"

	uierrors "github.com/dalemusser/stratahub/internal/app/features/errors"
	resourcestore "github.com/dalemusser/stratahub/internal/app/store/resources"
	"github.com/dalemusser/stratahub/internal/app/system/authz"
	"github.com/dalemusser/stratahub/internal/app/system/htmlsanitize"
	"github.com/dalemusser/stratahub/internal/app/system/inputval"
	"github.com/dalemusser/stratahub/internal/app/system/timeouts"
	"github.com/dalemusser/stratahub/internal/domain/models"
	"github.com/dalemusser/waffle/pantry/urlutil"
	"github.com/go-chi/chi/v5"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.uber.org/zap"
)

// editResourceInput defines validation rules for editing a resource.
type editResourceInput struct {
	Title  string `validate:"required,max=200" label:"Title"`
	Status string `validate:"required,oneof=active disabled" label:"Status"`
}

// ServeEdit renders the Edit Resource form for admins.
func (h *AdminHandler) ServeEdit(w http.ResponseWriter, r *http.Request) {
	// Check if user can manage resources (admin or coordinator with permission)
	if !authz.CanManageResources(r) {
		http.Redirect(w, r, "/resources", http.StatusSeeOther)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), timeouts.Short())
	defer cancel()
	db := h.DB

	idHex := chi.URLParam(r, "id")
	oid, err := primitive.ObjectIDFromHex(idHex)
	if err != nil {
		uierrors.RenderBadRequest(w, r, "Invalid resource ID.", "/resources")
		return
	}

	resStore := resourcestore.New(db)
	res, err := resStore.GetByID(ctx, oid)
	if err != nil {
		uierrors.RenderNotFound(w, r, "Resource not found.", "/resources")
		return
	}

	// Compute safe return targets for submit/delete actions
	deleteReturn := urlutil.SafeReturn(r.URL.Query().Get("return"), idHex /* filter out current id */, "/resources")
	submitReturn := urlutil.SafeReturn(r.URL.Query().Get("return"), "", "/resources")

	vm := resourceFormVM{
		ID:                  res.ID.Hex(),
		ResourceTitle:       res.Title,
		Subject:             res.Subject,
		Description:         res.Description,
		LaunchURL:           res.LaunchURL,
		Type:                res.Type,
		Status:              res.Status,
		ShowInLibrary:       res.ShowInLibrary,
		HasFile:             res.HasFile(),
		FileName:            res.FileName,
		FileSize:            res.FileSize,
		DefaultInstructions: res.DefaultInstructions,
		DeleteReturn:        deleteReturn,
		SubmitReturn:        submitReturn,
	}

	h.renderEditForm(w, r, vm, "")
}

// HandleEdit processes the Edit Resource form POST for admins.
func (h *AdminHandler) HandleEdit(w http.ResponseWriter, r *http.Request) {
	// Check if user can manage resources (admin or coordinator with permission)
	if !authz.CanManageResources(r) {
		http.Redirect(w, r, "/resources", http.StatusSeeOther)
		return
	}

	actorRole, _, actorID, _ := authz.UserCtx(r)

	// Parse multipart form for file uploads (32MB max)
	if err := r.ParseMultipartForm(32 << 20); err != nil {
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
	// Sanitize HTML content from rich text editor
	defaultInstructions := htmlsanitize.Sanitize(strings.TrimSpace(r.FormValue("default_instructions")))

	// Check for new file upload
	file, header, fileErr := r.FormFile("file")
	hasNewFile := fileErr == nil && header != nil && header.Size > 0

	// Check if user wants to remove file
	removeFile := r.FormValue("remove_file") != ""

	// Delete-return should never redirect back to a URL containing this id.
	delReturn := urlutil.SafeReturn(r.FormValue("return"), rid.Hex(), "/resources")

	ctx, cancel := context.WithTimeout(r.Context(), timeouts.Medium())
	defer cancel()
	db := h.DB

	// Get existing resource
	resStore := resourcestore.New(db)
	existing, err := resStore.GetByID(ctx, rid)
	if err != nil {
		uierrors.RenderNotFound(w, r, "Resource not found.", "/resources")
		return
	}

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
			HasFile:             existing.HasFile(),
			FileName:            existing.FileName,
			FileSize:            existing.FileSize,
			DefaultInstructions: defaultInstructions,
			DeleteReturn:        delReturn,
			SubmitReturn:        urlutil.SafeReturn(r.FormValue("return"), "", "/resources"),
		}
		h.renderEditForm(w, r, vm, msg)
	}

	// Validate required fields using struct tags
	input := editResourceInput{Title: title, Status: status}
	if result := inputval.Validate(input); result.HasErrors() {
		reRender(result.First())
		return
	}

	// Validate resource type
	if !inputval.IsValidResourceType(typeValue) {
		reRender("Type is invalid.")
		return
	}

	// Determine final content state
	hasURL := launchURL != ""
	keepExistingFile := existing.HasFile() && !removeFile && !hasNewFile && !hasURL
	finalHasFile := hasNewFile || keepExistingFile
	finalHasURL := hasURL && !hasNewFile

	if !finalHasURL && !finalHasFile {
		reRender("Either Launch URL or File is required.")
		return
	}
	if finalHasURL && finalHasFile {
		reRender("Cannot have both Launch URL and File. Choose one.")
		return
	}

	// Validate launch URL if provided
	if finalHasURL && !urlutil.IsValidAbsHTTPURL(launchURL) {
		reRender("Launch URL must be a valid absolute URL (e.g., https://example.com).")
		return
	}

	// Handle file operations
	var newFilePath, newFileName string
	var newFileSize int64

	if hasNewFile {
		defer file.Close()

		newFileName = header.Filename
		contentType := header.Header.Get("Content-Type")
		if contentType == "" {
			contentType = "application/octet-stream"
		}

		// Upload new file
		info, err := uploadFile(ctx, h.Storage, newFileName, file, header.Size, contentType)
		if err != nil {
			h.Log.Error("file upload failed", zap.Error(err))
			reRender("Failed to upload file. Please try again.")
			return
		}
		newFilePath = info.Path
		newFileSize = info.Size

		// Delete old file if exists
		if existing.HasFile() {
			if err := h.Storage.Delete(ctx, existing.FilePath); err != nil {
				h.Log.Warn("failed to delete old file during update",
					zap.String("path", existing.FilePath),
					zap.Error(err))
			}
		}
	} else if removeFile && existing.HasFile() {
		// Delete existing file
		if err := h.Storage.Delete(ctx, existing.FilePath); err != nil {
			h.Log.Warn("failed to delete file on removal",
				zap.String("path", existing.FilePath),
				zap.Error(err))
		}
	}

	// Build update model
	res := models.Resource{
		Title:               title,
		Subject:             subject,
		Description:         description,
		Type:                typeValue,
		Status:              status,
		ShowInLibrary:       showInLibrary,
		DefaultInstructions: defaultInstructions,
	}

	if hasNewFile {
		res.FilePath = newFilePath
		res.FileName = newFileName
		res.FileSize = newFileSize
		res.LaunchURL = "" // Clear URL when adding file
	} else if keepExistingFile {
		res.FilePath = existing.FilePath
		res.FileName = existing.FileName
		res.FileSize = existing.FileSize
		res.LaunchURL = "" // Keep URL empty
	} else {
		res.LaunchURL = launchURL
		res.FilePath = "" // Clear file fields
		res.FileName = ""
		res.FileSize = 0
	}

	if err := resStore.Update(ctx, rid, res); err != nil {
		msg := "Database error while updating resource."
		if err == resourcestore.ErrDuplicateTitle {
			msg = "A resource with that title already exists."
		}
		reRender(msg)
		return
	}

	// Audit log: resource updated
	h.AuditLog.ResourceUpdated(ctx, r, actorID, rid, actorRole, title)

	// Success: redirect to provided ?return= (sanitized and MUST NOT reference this id)
	ret := urlutil.SafeReturn(r.FormValue("return"), rid.Hex(), "/resources")
	http.Redirect(w, r, ret, http.StatusSeeOther)
}
