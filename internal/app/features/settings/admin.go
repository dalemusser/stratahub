// internal/app/features/settings/admin.go
package settings

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"strings"
	"time"

	settingsstore "github.com/dalemusser/stratahub/internal/app/store/settings"
	"github.com/dalemusser/stratahub/internal/app/system/authz"
	"github.com/dalemusser/stratahub/internal/app/system/htmlsanitize"
	"github.com/dalemusser/stratahub/internal/app/system/timeouts"
	"github.com/dalemusser/stratahub/internal/app/system/viewdata"
	"github.com/dalemusser/stratahub/internal/app/system/workspace"
	"github.com/dalemusser/stratahub/internal/domain/models"
	"github.com/dalemusser/waffle/pantry/storage"
	"github.com/dalemusser/waffle/pantry/templates"
	"github.com/google/uuid"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.uber.org/zap"
)

type settingsVM struct {
	viewdata.BaseVM
	HasLogo  bool
	LogoName string
	Error    string
}

// ServeSettings displays the settings form.
func (h *Handler) ServeSettings(w http.ResponseWriter, r *http.Request) {
	// Get workspace ID from context
	wsID := workspace.IDFromRequest(r)
	if wsID == primitive.NilObjectID {
		// Superadmin on apex domain - redirect to workspaces management
		http.Redirect(w, r, "/workspaces", http.StatusSeeOther)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), timeouts.Medium())
	defer cancel()

	store := settingsstore.New(h.DB)
	settings, err := store.Get(ctx, wsID)
	if err != nil {
		h.ErrLog.LogServerError(w, r, "load settings failed", err, "Failed to load settings.", "/dashboard")
		return
	}

	vm := settingsVM{
		BaseVM:   viewdata.NewBaseVM(r, h.DB, "Settings", "/dashboard"),
		HasLogo:  settings.HasLogo(),
		LogoName: settings.LogoName,
	}

	h.render(w, r, vm)
}

// HandleSettings processes the settings form submission.
func (h *Handler) HandleSettings(w http.ResponseWriter, r *http.Request) {
	// Get workspace ID from context
	wsID := workspace.IDFromRequest(r)
	if wsID == primitive.NilObjectID {
		// Superadmin on apex domain - redirect to workspaces management
		http.Redirect(w, r, "/workspaces", http.StatusSeeOther)
		return
	}

	// Parse multipart form for file uploads (8MB max for logo)
	if err := r.ParseMultipartForm(8 << 20); err != nil {
		h.ErrLog.LogBadRequest(w, r, "parse form failed", err, "Invalid form data.", "/settings")
		return
	}

	siteName := strings.TrimSpace(r.FormValue("site_name"))
	footerHTML := htmlsanitize.Sanitize(strings.TrimSpace(r.FormValue("footer_html")))
	removeLogo := r.FormValue("remove_logo") != ""

	// Validation
	if siteName == "" {
		h.renderWithError(w, r, wsID, "Site name is required.")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), timeouts.Long())
	defer cancel()

	store := settingsstore.New(h.DB)
	current, err := store.Get(ctx, wsID)
	if err != nil {
		h.ErrLog.LogServerError(w, r, "load settings failed", err, "Failed to load settings.", "/settings")
		return
	}

	// Handle logo upload/removal
	logoPath := current.LogoPath
	logoName := current.LogoName

	if removeLogo {
		// Delete old logo if exists
		if current.HasLogo() {
			if err := h.Storage.Delete(ctx, current.LogoPath); err != nil {
				h.Log.Warn("failed to delete old logo", zap.String("path", current.LogoPath), zap.Error(err))
			}
		}
		logoPath = ""
		logoName = ""
	}

	// Check for new logo upload
	file, header, fileErr := r.FormFile("logo")
	hasNewLogo := fileErr == nil && header != nil && header.Size > 0
	if hasNewLogo {
		defer file.Close()

		// Validate file type (only images)
		contentType := header.Header.Get("Content-Type")
		if !strings.HasPrefix(contentType, "image/") {
			h.renderWithError(w, r, wsID, "Logo must be an image file.")
			return
		}

		// Delete old logo if exists
		if current.HasLogo() {
			if err := h.Storage.Delete(ctx, current.LogoPath); err != nil {
				h.Log.Warn("failed to delete old logo", zap.String("path", current.LogoPath), zap.Error(err))
			}
		}

		// Upload new logo
		info, err := uploadLogo(ctx, h.Storage, header.Filename, file, header.Size, contentType)
		if err != nil {
			h.Log.Error("logo upload failed", zap.Error(err))
			h.renderWithError(w, r, wsID, "Failed to upload logo. Please try again.")
			return
		}
		logoPath = info.Path
		logoName = header.Filename
	}

	// Get user info for audit
	_, uname, memberID, _ := authz.UserCtx(r)

	// Save settings
	settings := models.SiteSettings{
		SiteName:      siteName,
		LogoPath:      logoPath,
		LogoName:      logoName,
		FooterHTML:    footerHTML,
		UpdatedByID:   &memberID,
		UpdatedByName: uname,
	}

	if err := store.Save(ctx, wsID, settings); err != nil {
		h.Log.Error("failed to save settings", zap.Error(err))
		h.renderWithError(w, r, wsID, "Failed to save settings.")
		return
	}

	http.Redirect(w, r, "/settings", http.StatusSeeOther)
}

func (h *Handler) render(w http.ResponseWriter, r *http.Request, vm settingsVM) {
	templates.Render(w, r, "settings", vm)
}

func (h *Handler) renderWithError(w http.ResponseWriter, r *http.Request, wsID primitive.ObjectID, errMsg string) {
	ctx, cancel := context.WithTimeout(r.Context(), timeouts.Short())
	defer cancel()

	store := settingsstore.New(h.DB)
	settings, _ := store.Get(ctx, wsID)

	vm := settingsVM{
		BaseVM:   viewdata.NewBaseVM(r, h.DB, "Settings", "/dashboard"),
		HasLogo:  settings.HasLogo(),
		LogoName: settings.LogoName,
		Error:    errMsg,
	}

	h.render(w, r, vm)
}

// UploadInfo contains metadata about an uploaded file.
type UploadInfo struct {
	Path string
	Size int64
}

// uploadLogo stores a logo file with a unique path and returns upload info.
func uploadLogo(ctx context.Context, store storage.Store, filename string, reader io.Reader, size int64, contentType string) (UploadInfo, error) {
	// Generate unique path: logos/YYYY/MM/uuid.ext
	now := time.Now().UTC()
	dateDir := fmt.Sprintf("logos/%04d/%02d", now.Year(), now.Month())
	ext := filepath.Ext(filename)
	uniqueName := fmt.Sprintf("%s%s", uuid.New().String()[:8], ext)
	path := filepath.Join(dateDir, uniqueName)
	path = filepath.ToSlash(path)

	// Upload to storage
	opts := &storage.PutOptions{
		ContentType: contentType,
	}
	if err := store.Put(ctx, path, reader, opts); err != nil {
		return UploadInfo{}, fmt.Errorf("failed to upload logo: %w", err)
	}

	return UploadInfo{
		Path: path,
		Size: size,
	}, nil
}
