// internal/app/features/workspaces/settings.go
package workspaces

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"strings"
	"time"

	settingsstore "github.com/dalemusser/stratahub/internal/app/store/settings"
	workspacestore "github.com/dalemusser/stratahub/internal/app/store/workspaces"
	"github.com/dalemusser/stratahub/internal/app/system/authz"
	"github.com/dalemusser/stratahub/internal/app/system/htmlsanitize"
	"github.com/dalemusser/stratahub/internal/app/system/timeouts"
	"github.com/dalemusser/stratahub/internal/app/system/viewdata"
	"github.com/dalemusser/stratahub/internal/domain/models"
	"github.com/dalemusser/waffle/pantry/storage"
	"github.com/dalemusser/waffle/pantry/templates"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.uber.org/zap"
)

type workspaceSettingsVM struct {
	viewdata.BaseVM
	WorkspaceID        string
	WorkspaceName      string
	HasLogo            bool
	LogoName           string
	AllAuthMethods     []models.AuthMethod
	EnabledAuthMethods map[string]bool
	Error              string
}

// ServeSettings displays the settings form for a specific workspace.
// GET /workspaces/{id}/settings
func (h *Handler) ServeSettings(w http.ResponseWriter, r *http.Request) {
	wsID, err := primitive.ObjectIDFromHex(chi.URLParam(r, "id"))
	if err != nil {
		http.NotFound(w, r)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), timeouts.Medium())
	defer cancel()

	// Get workspace info for display
	wsStore := workspacestore.New(h.DB)
	ws, err := wsStore.GetByID(ctx, wsID)
	if err != nil {
		h.ErrLog.LogServerError(w, r, "workspace not found", err, "Workspace not found.", "/workspaces")
		return
	}

	// Get settings for this workspace
	store := settingsstore.New(h.DB)
	settings, err := store.Get(ctx, wsID)
	if err != nil {
		h.ErrLog.LogServerError(w, r, "load settings failed", err, "Failed to load settings.", "/workspaces")
		return
	}

	// Build enabled auth methods map for checkbox state
	enabledMap := make(map[string]bool)
	if len(settings.EnabledAuthMethods) == 0 {
		// Default: all methods enabled
		for _, m := range models.AllAuthMethods {
			enabledMap[m.Value] = true
		}
	} else {
		for _, m := range settings.EnabledAuthMethods {
			enabledMap[m] = true
		}
	}

	vm := workspaceSettingsVM{
		BaseVM:             viewdata.NewBaseVM(r, h.DB, "Workspace Settings", "/workspaces"),
		WorkspaceID:        wsID.Hex(),
		WorkspaceName:      ws.Name,
		HasLogo:            settings.HasLogo(),
		LogoName:           settings.LogoName,
		AllAuthMethods:     models.AllAuthMethods,
		EnabledAuthMethods: enabledMap,
	}

	templates.Render(w, r, "workspace_settings", vm)
}

// HandleSettings processes the settings form submission for a specific workspace.
// POST /workspaces/{id}/settings
func (h *Handler) HandleSettings(w http.ResponseWriter, r *http.Request) {
	wsID, err := primitive.ObjectIDFromHex(chi.URLParam(r, "id"))
	if err != nil {
		http.NotFound(w, r)
		return
	}

	// Parse multipart form for file uploads (8MB max for logo)
	if err := r.ParseMultipartForm(8 << 20); err != nil {
		h.ErrLog.LogBadRequest(w, r, "parse form failed", err, "Invalid form data.", "/workspaces/"+wsID.Hex()+"/settings")
		return
	}

	workspaceName := strings.TrimSpace(r.FormValue("workspace_name"))
	siteName := strings.TrimSpace(r.FormValue("site_name"))
	footerHTML := htmlsanitize.Sanitize(strings.TrimSpace(r.FormValue("footer_html")))
	removeLogo := r.FormValue("remove_logo") != ""
	authMethods := r.Form["auth_methods"]

	// Validation
	if workspaceName == "" {
		h.renderSettingsWithError(w, r, wsID, "Workspace name is required.")
		return
	}
	if siteName == "" {
		h.renderSettingsWithError(w, r, wsID, "Site name is required.")
		return
	}
	if len(authMethods) == 0 {
		h.renderSettingsWithError(w, r, wsID, "At least one authentication method must be selected.")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), timeouts.Long())
	defer cancel()

	store := settingsstore.New(h.DB)
	current, err := store.Get(ctx, wsID)
	if err != nil {
		h.ErrLog.LogServerError(w, r, "load settings failed", err, "Failed to load settings.", "/workspaces/"+wsID.Hex()+"/settings")
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
			h.renderSettingsWithError(w, r, wsID, "Logo must be an image file.")
			return
		}

		// Delete old logo if exists
		if current.HasLogo() {
			if err := h.Storage.Delete(ctx, current.LogoPath); err != nil {
				h.Log.Warn("failed to delete old logo", zap.String("path", current.LogoPath), zap.Error(err))
			}
		}

		// Upload new logo
		info, err := uploadWorkspaceLogo(ctx, h.Storage, header.Filename, file, header.Size, contentType)
		if err != nil {
			h.Log.Error("logo upload failed", zap.Error(err))
			h.renderSettingsWithError(w, r, wsID, "Failed to upload logo. Please try again.")
			return
		}
		logoPath = info.Path
		logoName = header.Filename
	}

	// Get user info for audit
	_, uname, memberID, _ := authz.UserCtx(r)

	// Save settings
	settings := models.SiteSettings{
		SiteName:           siteName,
		LogoPath:           logoPath,
		LogoName:           logoName,
		FooterHTML:         footerHTML,
		EnabledAuthMethods: authMethods,
		UpdatedByID:        &memberID,
		UpdatedByName:      uname,
	}

	if err := store.Save(ctx, wsID, settings); err != nil {
		h.Log.Error("failed to save settings", zap.Error(err))
		h.renderSettingsWithError(w, r, wsID, "Failed to save settings.")
		return
	}

	// Update workspace name
	wsStore := workspacestore.New(h.DB)
	wsUpdate := models.Workspace{Name: workspaceName}
	if err := wsStore.Update(ctx, wsID, wsUpdate); err != nil {
		if err == workspacestore.ErrDuplicateName {
			h.renderSettingsWithError(w, r, wsID, "A workspace with that name already exists.")
			return
		}
		h.Log.Error("failed to update workspace name", zap.Error(err))
		h.renderSettingsWithError(w, r, wsID, "Failed to update workspace name.")
		return
	}

	http.Redirect(w, r, "/workspaces/"+wsID.Hex()+"/settings", http.StatusSeeOther)
}

func (h *Handler) renderSettingsWithError(w http.ResponseWriter, r *http.Request, wsID primitive.ObjectID, errMsg string) {
	ctx, cancel := context.WithTimeout(r.Context(), timeouts.Short())
	defer cancel()

	// Get workspace info for display
	wsStore := workspacestore.New(h.DB)
	ws, _ := wsStore.GetByID(ctx, wsID)

	store := settingsstore.New(h.DB)
	settings, _ := store.Get(ctx, wsID)

	// Build enabled auth methods map for checkbox state
	enabledMap := make(map[string]bool)
	if len(settings.EnabledAuthMethods) == 0 {
		// Default: all methods enabled
		for _, m := range models.AllAuthMethods {
			enabledMap[m.Value] = true
		}
	} else {
		for _, m := range settings.EnabledAuthMethods {
			enabledMap[m] = true
		}
	}

	vm := workspaceSettingsVM{
		BaseVM:             viewdata.NewBaseVM(r, h.DB, "Workspace Settings", "/workspaces"),
		WorkspaceID:        wsID.Hex(),
		WorkspaceName:      ws.Name,
		HasLogo:            settings.HasLogo(),
		LogoName:           settings.LogoName,
		AllAuthMethods:     models.AllAuthMethods,
		EnabledAuthMethods: enabledMap,
		Error:              errMsg,
	}

	templates.Render(w, r, "workspace_settings", vm)
}

// uploadWorkspaceLogo stores a logo file with a unique path and returns upload info.
func uploadWorkspaceLogo(ctx context.Context, store storage.Store, filename string, reader io.Reader, size int64, contentType string) (uploadInfo, error) {
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
		return uploadInfo{}, fmt.Errorf("failed to upload logo: %w", err)
	}

	return uploadInfo{
		Path: path,
		Size: size,
	}, nil
}

// uploadInfo contains metadata about an uploaded file.
type uploadInfo struct {
	Path string
	Size int64
}
