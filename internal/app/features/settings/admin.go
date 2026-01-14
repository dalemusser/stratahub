// internal/app/features/settings/admin.go
package settings

// Terminology: User Identifiers
//   - UserID / userID / user_id: The MongoDB ObjectID (_id) that uniquely identifies a user record
//   - LoginID / loginID / login_id: The human-readable string users type to log in

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
	HasLogo            bool
	LogoName           string
	LandingTitle       string // Title for landing page
	LandingContent     string // HTML content for landing page
	AllAuthMethods     []models.AuthMethod
	EnabledAuthMethods map[string]bool
	CurrentUserMethod  string // Current user's auth method (for protection)
	Error              string
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

	_, _, userID, ok := authz.UserCtx(r)
	if !ok {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
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

	// Get current user's auth method for protection
	var currentUserMethod string
	var user struct {
		AuthMethod string `bson:"auth_method"`
	}
	if err := h.DB.Collection("users").FindOne(ctx, map[string]interface{}{"_id": userID}).Decode(&user); err == nil {
		currentUserMethod = user.AuthMethod
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

	// Use default landing title if empty so admin has something to work with
	landingTitle := settings.LandingTitle
	if landingTitle == "" {
		landingTitle = models.DefaultLandingTitle
	}

	vm := settingsVM{
		BaseVM:             viewdata.NewBaseVM(r, h.DB, "Settings", "/dashboard"),
		HasLogo:            settings.HasLogo(),
		LogoName:           settings.LogoName,
		LandingTitle:       landingTitle,
		LandingContent:     settings.LandingContent,
		AllAuthMethods:     models.AllAuthMethods,
		EnabledAuthMethods: enabledMap,
		CurrentUserMethod:  currentUserMethod,
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

	// Limit request body size to prevent memory exhaustion
	// Use 8MB to allow for logo uploads, but prevent excessive payloads
	r.Body = http.MaxBytesReader(w, r.Body, 8<<20)

	// Parse multipart form for file uploads (8MB max for logo)
	if err := r.ParseMultipartForm(8 << 20); err != nil {
		if err.Error() == "http: request body too large" {
			h.ErrLog.LogBadRequest(w, r, "request too large", err, "Request is too large. Maximum size is 8 MB.", "/settings")
			return
		}
		h.ErrLog.LogBadRequest(w, r, "parse form failed", err, "Invalid form data.", "/settings")
		return
	}

	siteName := strings.TrimSpace(r.FormValue("site_name"))
	landingTitle := strings.TrimSpace(r.FormValue("landing_title"))
	landingContent := htmlsanitize.Sanitize(strings.TrimSpace(r.FormValue("landing_content")))
	footerHTML := htmlsanitize.Sanitize(strings.TrimSpace(r.FormValue("footer_html")))
	removeLogo := r.FormValue("remove_logo") != ""
	authMethods := r.Form["auth_methods"]

	// Validation
	if siteName == "" {
		h.renderWithError(w, r, wsID, "Site name is required.")
		return
	}
	if len(authMethods) == 0 {
		h.renderWithError(w, r, wsID, "At least one authentication method must be selected.")
		return
	}

	// Get current user's auth method for protection check
	_, _, userID, _ := authz.UserCtx(r)

	ctx, cancel := context.WithTimeout(r.Context(), timeouts.Long())
	defer cancel()

	// Check if current user's auth method is in the selected list
	var currentUserMethod string
	var user struct {
		AuthMethod string `bson:"auth_method"`
	}
	if err := h.DB.Collection("users").FindOne(ctx, map[string]interface{}{"_id": userID}).Decode(&user); err == nil {
		currentUserMethod = user.AuthMethod
	}
	if currentUserMethod != "" {
		found := false
		for _, m := range authMethods {
			if m == currentUserMethod {
				found = true
				break
			}
		}
		if !found {
			h.renderWithError(w, r, wsID, "You cannot disable the authentication method you are currently using.")
			return
		}
	}

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
		SiteName:           siteName,
		LogoPath:           logoPath,
		LogoName:           logoName,
		LandingTitle:       landingTitle,
		LandingContent:     landingContent,
		FooterHTML:         footerHTML,
		EnabledAuthMethods: authMethods,
		UpdatedByID:        &memberID,
		UpdatedByName:      uname,
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

	_, _, userID, _ := authz.UserCtx(r)

	store := settingsstore.New(h.DB)
	settings, _ := store.Get(ctx, wsID)

	// Get current user's auth method for protection
	var currentUserMethod string
	var user struct {
		AuthMethod string `bson:"auth_method"`
	}
	if err := h.DB.Collection("users").FindOne(ctx, map[string]interface{}{"_id": userID}).Decode(&user); err == nil {
		currentUserMethod = user.AuthMethod
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

	// Use default landing title if empty so admin has something to work with
	landingTitle := settings.LandingTitle
	if landingTitle == "" {
		landingTitle = models.DefaultLandingTitle
	}

	vm := settingsVM{
		BaseVM:             viewdata.NewBaseVM(r, h.DB, "Settings", "/dashboard"),
		HasLogo:            settings.HasLogo(),
		LogoName:           settings.LogoName,
		LandingTitle:       landingTitle,
		LandingContent:     settings.LandingContent,
		AllAuthMethods:     models.AllAuthMethods,
		EnabledAuthMethods: enabledMap,
		CurrentUserMethod:  currentUserMethod,
		Error:              errMsg,
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
