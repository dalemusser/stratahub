package resources

import (
	"context"
	"net/http"
	"time"

	uierrors "github.com/dalemusser/stratahub/internal/app/features/errors"
	resourcestore "github.com/dalemusser/stratahub/internal/app/store/resources"
	"github.com/dalemusser/stratahub/internal/app/system/authz"
	"github.com/dalemusser/stratahub/internal/app/system/htmlsanitize"
	"github.com/dalemusser/stratahub/internal/app/system/timeouts"
	"github.com/dalemusser/stratahub/internal/app/system/viewdata"
	"github.com/dalemusser/stratahub/internal/app/system/workspace"
	"github.com/dalemusser/waffle/pantry/storage"
	"github.com/dalemusser/waffle/pantry/templates"

	"github.com/go-chi/chi/v5"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.uber.org/zap"
)

// ServeView renders the admin detail view for a single resource.
func (h *AdminHandler) ServeView(w http.ResponseWriter, r *http.Request) {
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
		// Treat not found as a 404 to match other admin handlers
		uierrors.RenderNotFound(w, r, "Resource not found.", "/resources")
		return
	}

	// Verify workspace ownership (prevent cross-workspace access)
	wsID := workspace.IDFromRequest(r)
	if wsID != primitive.NilObjectID && res.WorkspaceID != wsID {
		uierrors.RenderNotFound(w, r, "Resource not found.", "/resources")
		return
	}

	// Check if user can edit resources (admin or coordinator with permission)
	canEdit := authz.CanManageResources(r)

	data := viewData{
		BaseVM:              viewdata.NewBaseVM(r, h.DB, "View Resource", "/resources"),
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
		DefaultInstructions: htmlsanitize.PrepareForDisplay(res.DefaultInstructions),
		CanEdit:             canEdit,
	}

	templates.Render(w, r, "resource_view", data)
}

// HandleDownload serves the resource file directly (for local storage) or
// generates a signed URL and redirects (for S3). For URLs, it redirects directly.
func (h *AdminHandler) HandleDownload(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), timeouts.Short())
	defer cancel()

	idHex := chi.URLParam(r, "id")
	oid, err := primitive.ObjectIDFromHex(idHex)
	if err != nil {
		uierrors.RenderBadRequest(w, r, "Invalid resource ID.", "/resources")
		return
	}

	resStore := resourcestore.New(h.DB)
	res, err := resStore.GetByID(ctx, oid)
	if err != nil {
		uierrors.RenderNotFound(w, r, "Resource not found.", "/resources")
		return
	}

	// Verify workspace ownership (prevent cross-workspace access)
	wsID := workspace.IDFromRequest(r)
	if wsID != primitive.NilObjectID && res.WorkspaceID != wsID {
		uierrors.RenderNotFound(w, r, "Resource not found.", "/resources")
		return
	}

	// If resource has a file, serve it
	if res.HasFile() {
		// Build Content-Disposition header for proper filename
		filename := res.FileName
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
			fullPath, err := localStorage.GetFullPath(res.FilePath)
			if err != nil {
				h.Log.Error("error getting file path",
					zap.Error(err),
					zap.String("path", res.FilePath))
				uierrors.RenderServerError(w, r, "Failed to locate file.", "/resources")
				return
			}
			w.Header().Set("Content-Disposition", contentDisposition)
			http.ServeFile(w, r, fullPath)
			return
		}

		// For S3/other storage, generate signed URL and redirect
		signedURL, err := h.Storage.PresignedURL(ctx, res.FilePath, &storage.PresignOptions{
			Expires:            15 * time.Minute,
			ContentDisposition: contentDisposition,
		})
		if err != nil {
			h.Log.Error("error generating signed URL",
				zap.Error(err),
				zap.String("path", res.FilePath))
			uierrors.RenderServerError(w, r, "Failed to generate download link.", "/resources")
			return
		}
		http.Redirect(w, r, signedURL, http.StatusSeeOther)
		return
	}

	// If resource has a URL, redirect to it
	if res.LaunchURL != "" {
		http.Redirect(w, r, res.LaunchURL, http.StatusSeeOther)
		return
	}

	uierrors.RenderNotFound(w, r, "No file or URL available for this resource.", "/resources")
}
