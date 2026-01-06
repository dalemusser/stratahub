package materials

import (
	"context"
	"net/http"
	"strings"

	uierrors "github.com/dalemusser/stratahub/internal/app/features/errors"
	materialstore "github.com/dalemusser/stratahub/internal/app/store/materials"
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

// editMaterialInput defines validation rules for editing a material.
type editMaterialInput struct {
	Title  string `validate:"required,max=200" label:"Title"`
	Status string `validate:"required,oneof=active disabled" label:"Status"`
}

// ServeEdit renders the Edit Material form for admins.
func (h *AdminHandler) ServeEdit(w http.ResponseWriter, r *http.Request) {
	// Check if user can manage materials (admin or coordinator with permission)
	if !authz.CanManageMaterials(r) {
		http.Redirect(w, r, "/materials", http.StatusSeeOther)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), timeouts.Short())
	defer cancel()
	db := h.DB

	idHex := chi.URLParam(r, "id")
	oid, err := primitive.ObjectIDFromHex(idHex)
	if err != nil {
		uierrors.RenderBadRequest(w, r, "Invalid material ID.", "/materials")
		return
	}

	matStore := materialstore.New(db)
	mat, err := matStore.GetByID(ctx, oid)
	if err != nil {
		uierrors.RenderNotFound(w, r, "Material not found.", "/materials")
		return
	}

	// Compute safe return targets for submit/delete actions
	deleteReturn := urlutil.SafeReturn(r.URL.Query().Get("return"), idHex, "/materials")
	submitReturn := urlutil.SafeReturn(r.URL.Query().Get("return"), "", "/materials")

	vm := materialFormVM{
		ID:                  mat.ID.Hex(),
		MaterialTitle:       mat.Title,
		Subject:             mat.Subject,
		Description:         mat.Description,
		LaunchURL:           mat.LaunchURL,
		Type:                mat.Type,
		Status:              mat.Status,
		HasFile:             mat.HasFile(),
		FileName:            mat.FileName,
		FileSize:            mat.FileSize,
		DefaultInstructions: mat.DefaultInstructions,
		DeleteReturn:        deleteReturn,
		SubmitReturn:        submitReturn,
	}

	h.renderEditForm(w, r, vm, "")
}

// HandleEdit processes the Edit Material form POST for admins.
func (h *AdminHandler) HandleEdit(w http.ResponseWriter, r *http.Request) {
	// Check if user can manage materials (admin or coordinator with permission)
	if !authz.CanManageMaterials(r) {
		http.Redirect(w, r, "/materials", http.StatusSeeOther)
		return
	}

	actorRole, _, actorID, _ := authz.UserCtx(r)

	// Parse multipart form for file uploads (32MB max)
	if err := r.ParseMultipartForm(32 << 20); err != nil {
		h.ErrLog.LogBadRequest(w, r, "parse form failed", err, "Invalid form data.", "/materials")
		return
	}

	idHex := chi.URLParam(r, "id")
	mid, err := primitive.ObjectIDFromHex(idHex)
	if err != nil {
		uierrors.RenderBadRequest(w, r, "Invalid material ID.", "/materials")
		return
	}

	title := strings.TrimSpace(r.FormValue("title"))
	subject := strings.TrimSpace(r.FormValue("subject"))
	description := strings.TrimSpace(r.FormValue("description"))
	launchURL := strings.TrimSpace(r.FormValue("launch_url"))

	typeValue := strings.TrimSpace(r.FormValue("type"))
	if typeValue == "" {
		typeValue = models.DefaultMaterialType
	}

	status := strings.TrimSpace(r.FormValue("status"))
	if status == "" {
		status = "active"
	}

	// Sanitize HTML content from rich text editor
	defaultInstructions := htmlsanitize.Sanitize(strings.TrimSpace(r.FormValue("default_instructions")))

	// Check for new file upload
	file, header, fileErr := r.FormFile("file")
	hasNewFile := fileErr == nil && header != nil && header.Size > 0

	// Check if user wants to remove file
	removeFile := r.FormValue("remove_file") != ""

	// Delete-return should never redirect back to a URL containing this id.
	delReturn := urlutil.SafeReturn(r.FormValue("return"), mid.Hex(), "/materials")

	ctx, cancel := context.WithTimeout(r.Context(), timeouts.Medium())
	defer cancel()
	db := h.DB

	// Get existing material
	matStore := materialstore.New(db)
	existing, err := matStore.GetByID(ctx, mid)
	if err != nil {
		uierrors.RenderNotFound(w, r, "Material not found.", "/materials")
		return
	}

	// Helper to re-render the form with a message.
	reRender := func(msg string) {
		vm := materialFormVM{
			ID:                  mid.Hex(),
			MaterialTitle:       title,
			Subject:             subject,
			Description:         description,
			LaunchURL:           launchURL,
			Type:                typeValue,
			Status:              status,
			HasFile:             existing.HasFile(),
			FileName:            existing.FileName,
			FileSize:            existing.FileSize,
			DefaultInstructions: defaultInstructions,
			DeleteReturn:        delReturn,
			SubmitReturn:        urlutil.SafeReturn(r.FormValue("return"), "", "/materials"),
		}
		h.renderEditForm(w, r, vm, msg)
	}

	// Validate required fields using struct tags
	input := editMaterialInput{Title: title, Status: status}
	if result := inputval.Validate(input); result.HasErrors() {
		reRender(result.First())
		return
	}

	// Validate material type
	if !inputval.IsValidMaterialType(typeValue) {
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
	mat := models.Material{
		Title:               title,
		Subject:             subject,
		Description:         description,
		Type:                typeValue,
		Status:              status,
		DefaultInstructions: defaultInstructions,
	}

	if hasNewFile {
		mat.FilePath = newFilePath
		mat.FileName = newFileName
		mat.FileSize = newFileSize
		mat.LaunchURL = "" // Clear URL when adding file
	} else if keepExistingFile {
		mat.FilePath = existing.FilePath
		mat.FileName = existing.FileName
		mat.FileSize = existing.FileSize
		mat.LaunchURL = "" // Keep URL empty
	} else {
		mat.LaunchURL = launchURL
		mat.FilePath = "" // Clear file fields
		mat.FileName = ""
		mat.FileSize = 0
	}

	if err := matStore.Update(ctx, mid, mat); err != nil {
		msg := "Database error while updating material."
		if err == materialstore.ErrDuplicateTitle {
			msg = "A material with that title already exists."
		}
		reRender(msg)
		return
	}

	// Audit log: material updated
	h.AuditLog.MaterialUpdated(ctx, r, actorID, mid, actorRole, title)

	// Success: redirect
	ret := urlutil.SafeReturn(r.FormValue("return"), mid.Hex(), "/materials")
	http.Redirect(w, r, ret, http.StatusSeeOther)
}
