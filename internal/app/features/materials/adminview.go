package materials

import (
	"context"
	"html/template"
	"net/http"
	"time"

	uierrors "github.com/dalemusser/stratahub/internal/app/features/errors"
	materialassignstore "github.com/dalemusser/stratahub/internal/app/store/materialassign"
	materialstore "github.com/dalemusser/stratahub/internal/app/store/materials"
	"github.com/dalemusser/stratahub/internal/app/system/timeouts"
	"github.com/dalemusser/stratahub/internal/app/system/viewdata"
	"github.com/dalemusser/waffle/pantry/httpnav"
	"github.com/dalemusser/waffle/pantry/storage"
	"github.com/dalemusser/waffle/pantry/templates"
	"github.com/go-chi/chi/v5"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.uber.org/zap"
)

// ServeView renders the View Material page for admins.
func (h *AdminHandler) ServeView(w http.ResponseWriter, r *http.Request) {
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

	// Count assignments
	assignStore := materialassignstore.New(db)
	assignCount, _ := assignStore.CountByMaterial(ctx, oid)

	data := viewData{
		BaseVM:              viewdata.NewBaseVM(r, h.DB, "View Material", "/materials"),
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
		DefaultInstructions: template.HTML(mat.DefaultInstructions),
		AssignmentCount:     assignCount,
	}

	templates.Render(w, r, "material_view", data)
}

// ServeManageModal renders the Manage Material modal.
func (h *AdminHandler) ServeManageModal(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), timeouts.Short())
	defer cancel()
	db := h.DB

	idHex := chi.URLParam(r, "id")
	oid, err := primitive.ObjectIDFromHex(idHex)
	if err != nil {
		uierrors.HTMXBadRequest(w, r, "Invalid material ID.", "/materials")
		return
	}

	matStore := materialstore.New(db)
	mat, err := matStore.GetByID(ctx, oid)
	if err != nil {
		uierrors.HTMXError(w, r, http.StatusNotFound, "Material not found.", func() {
			uierrors.RenderNotFound(w, r, "Material not found.", "/materials")
		})
		return
	}

	data := manageModalData{
		ID:          mat.ID.Hex(),
		Title:       mat.Title,
		Subject:     mat.Subject,
		Type:        mat.Type,
		Status:      mat.Status,
		HasFile:     mat.HasFile(),
		FileName:    mat.FileName,
		Description: mat.Description,
		BackURL:     httpnav.ResolveBackURL(r, "/materials"),
	}

	templates.RenderSnippet(w, "material_manage_modal", data)
}

// HandleDownload serves the material file directly (for local storage) or
// generates a signed URL and redirects (for S3). For URLs, it redirects directly.
func (h *AdminHandler) HandleDownload(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), timeouts.Short())
	defer cancel()

	idHex := chi.URLParam(r, "id")
	oid, err := primitive.ObjectIDFromHex(idHex)
	if err != nil {
		uierrors.RenderBadRequest(w, r, "Invalid material ID.", "/materials")
		return
	}

	matStore := materialstore.New(h.DB)
	mat, err := matStore.GetByID(ctx, oid)
	if err != nil {
		uierrors.RenderNotFound(w, r, "Material not found.", "/materials")
		return
	}

	// If material has a file, serve it
	if mat.HasFile() {
		// Build Content-Disposition header for proper filename
		filename := mat.FileName
		if filename == "" {
			filename = "download"
		}
		contentDisposition := "attachment; filename=\"" + filename + "\""

		// Prevent browser caching of downloads (important when files are replaced)
		w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
		w.Header().Set("Pragma", "no-cache")
		w.Header().Set("Expires", "0")

		// Check if local storage - serve file directly
		if localStorage, ok := h.Storage.(*storage.Local); ok {
			fullPath, err := localStorage.GetFullPath(mat.FilePath)
			if err != nil {
				h.Log.Error("error getting file path",
					zap.Error(err),
					zap.String("path", mat.FilePath))
				uierrors.RenderServerError(w, r, "Failed to locate file.", "/materials")
				return
			}
			w.Header().Set("Content-Disposition", contentDisposition)
			http.ServeFile(w, r, fullPath)
			return
		}

		// For S3/other storage, generate signed URL and redirect
		signedURL, err := h.Storage.PresignedURL(ctx, mat.FilePath, &storage.PresignOptions{
			Expires:            15 * time.Minute,
			ContentDisposition: contentDisposition,
		})
		if err != nil {
			h.Log.Error("error generating signed URL",
				zap.Error(err),
				zap.String("path", mat.FilePath))
			uierrors.RenderServerError(w, r, "Failed to generate download link.", "/materials")
			return
		}
		http.Redirect(w, r, signedURL, http.StatusSeeOther)
		return
	}

	// If material has a URL, redirect to it
	if mat.LaunchURL != "" {
		http.Redirect(w, r, mat.LaunchURL, http.StatusSeeOther)
		return
	}

	uierrors.RenderNotFound(w, r, "No file or URL available for this material.", "/materials")
}
