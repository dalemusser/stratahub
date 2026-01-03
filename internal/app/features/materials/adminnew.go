package materials

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"

	materialstore "github.com/dalemusser/stratahub/internal/app/store/materials"
	"github.com/dalemusser/stratahub/internal/app/system/authz"
	"github.com/dalemusser/stratahub/internal/app/system/htmlsanitize"
	"github.com/dalemusser/stratahub/internal/app/system/inputval"
	"github.com/dalemusser/stratahub/internal/app/system/navigation"
	"github.com/dalemusser/stratahub/internal/app/system/timeouts"
	"github.com/dalemusser/stratahub/internal/domain/models"
	"github.com/dalemusser/waffle/pantry/urlutil"
	"go.uber.org/zap"
)

// createMaterialInput defines validation rules for creating a material.
type createMaterialInput struct {
	Title  string `validate:"required,max=200" label:"Title"`
	Status string `validate:"required,oneof=active disabled" label:"Status"`
}

func (h *AdminHandler) ServeNew(w http.ResponseWriter, r *http.Request) {
	// Check if user can manage materials (admin or coordinator with permission)
	if !authz.CanManageMaterials(r) {
		http.Redirect(w, r, "/materials", http.StatusSeeOther)
		return
	}

	vm := materialFormVM{}
	h.renderNewForm(w, r, vm, "")
}

func (h *AdminHandler) HandleCreate(w http.ResponseWriter, r *http.Request) {
	// Check if user can manage materials (admin or coordinator with permission)
	if !authz.CanManageMaterials(r) {
		http.Redirect(w, r, "/materials", http.StatusSeeOther)
		return
	}

	// Parse multipart form for file uploads (32MB max)
	if err := r.ParseMultipartForm(32 << 20); err != nil {
		h.ErrLog.LogBadRequest(w, r, "parse form failed", err, "Invalid form data.", "/materials")
		return
	}

	title := strings.TrimSpace(r.FormValue("title"))
	subject := strings.TrimSpace(r.FormValue("subject"))
	description := strings.TrimSpace(r.FormValue("description"))
	launchURL := strings.TrimSpace(r.FormValue("launch_url"))
	typeValue := strings.TrimSpace(r.FormValue("type"))
	status := strings.TrimSpace(r.FormValue("status"))
	// Sanitize HTML content from rich text editor
	defaultInstructions := htmlsanitize.Sanitize(strings.TrimSpace(r.FormValue("default_instructions")))

	if typeValue == "" {
		typeValue = models.DefaultMaterialType
	}
	if status == "" {
		status = "active"
	}

	// Check for file upload
	var filePath, fileName, contentType string
	var fileSize int64
	file, header, fileErr := r.FormFile("file")
	hasFile := fileErr == nil && header != nil && header.Size > 0

	// Helper to re-render the form with a message
	reRender := func(msg string) {
		vm := materialFormVM{
			MaterialTitle:       title,
			Subject:             subject,
			Description:         description,
			LaunchURL:           launchURL,
			Type:                typeValue,
			Status:              status,
			DefaultInstructions: defaultInstructions,
		}
		h.renderNewForm(w, r, vm, msg)
	}

	// Validate required fields using struct tags
	input := createMaterialInput{Title: title, Status: status}
	if result := inputval.Validate(input); result.HasErrors() {
		reRender(result.First())
		return
	}

	// Validate material type
	if !inputval.IsValidMaterialType(typeValue) {
		reRender("Type is invalid.")
		return
	}

	// Must have either URL or file
	hasURL := launchURL != ""
	if !hasURL && !hasFile {
		reRender("Either Launch URL or File is required.")
		return
	}
	if hasURL && hasFile {
		reRender("Cannot have both Launch URL and File. Choose one.")
		return
	}

	// Validate launch URL if provided
	if hasURL && !urlutil.IsValidAbsHTTPURL(launchURL) {
		reRender("Launch URL must be a valid absolute URL (e.g., https://example.com).")
		return
	}

	// Handle file upload
	if hasFile {
		defer file.Close()

		fileName = header.Filename
		contentType = header.Header.Get("Content-Type")
		if contentType == "" {
			contentType = "application/octet-stream"
		}

		ctx, cancel := context.WithTimeout(r.Context(), timeouts.Long())
		defer cancel()

		// Upload file to storage
		info, err := uploadFile(ctx, h.Storage, fileName, file, header.Size, contentType)
		if err != nil {
			h.Log.Error("file upload failed", zap.Error(err))
			reRender("Failed to upload file. Please try again.")
			return
		}
		filePath = info.Path
		fileSize = info.Size
	}

	_, uname, memberID, ok := authz.UserCtx(r)

	ctx, cancel := context.WithTimeout(r.Context(), timeouts.Medium())
	defer cancel()
	db := h.DB

	// Create material via store (handles ID, CI fields, timestamps)
	matStore := materialstore.New(db)
	mat := models.Material{
		Title:               title,
		Subject:             subject,
		Description:         description,
		LaunchURL:           launchURL,
		Status:              status,
		Type:                typeValue,
		FilePath:            filePath,
		FileName:            fileName,
		FileSize:            fileSize,
		DefaultInstructions: defaultInstructions,
	}
	if ok {
		mat.CreatedByID = &memberID
		mat.CreatedByName = uname
	}

	if _, err := matStore.Create(ctx, mat); err != nil {
		msg := "Database error while creating material."
		if errors.Is(err, materialstore.ErrDuplicateTitle) {
			msg = "A material with that title already exists."
		} else {
			h.Log.Error("failed to create material", zap.Error(err))
		}
		// Clean up uploaded file on error
		if filePath != "" {
			if delErr := h.Storage.Delete(ctx, filePath); delErr != nil {
				h.Log.Warn("failed to clean up uploaded file after create error",
					zap.String("path", filePath),
					zap.Error(delErr))
			}
		}
		reRender(msg)
		return
	}

	ret := navigation.SafeBackURL(r, navigation.MaterialsBackURL)
	http.Redirect(w, r, ret, http.StatusSeeOther)
}

// detectContentType attempts to detect the content type from the file content.
func detectContentType(file io.ReadSeeker) string {
	// Read first 512 bytes for detection
	buf := make([]byte, 512)
	n, _ := file.Read(buf)
	file.Seek(0, io.SeekStart) // Reset for later reading

	if n == 0 {
		return "application/octet-stream"
	}

	return http.DetectContentType(buf[:n])
}
