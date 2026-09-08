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
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/dalemusser/stratahub/internal/app/features/memberstatusapi"
	"github.com/dalemusser/stratahub/internal/app/store/mhsbuilds"
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
	HasLogo               bool
	LogoName              string
	LandingTitle          string // Title for landing page
	LandingContent        string // HTML content for landing page
	AllAuthMethods        []models.AuthMethod
	EnabledAuthMethods    map[string]bool
	CurrentUserMethod     string // Current user's auth method (for protection)
	MHSMemberAuth         string // trust, keyword, staffauth
	MHSMemberAuthKeyword  string // keyword value (only when mode is "keyword")
	MHSStaffUnlockMinutes int    // staff unlock duration for the manage page (minutes)
	EnableClaudeSummaries bool   // AI student summaries toggle
	ClaudeModel           string // Selected Claude model ID

	// MHS device test: the public route's switch, the chosen Unit 2 build
	// ("unit2|2.2.3" or "" for the active collection's), the options, and
	// the URL to hand a school.
	MHSDeviceTestEnabled bool
	MHSDeviceTestUnitKey string
	MHSDeviceTestBuilds  []deviceTestBuildVM
	MHSDeviceTestURL     string

	// Member Status API shared key (see docs/member-status-api/plan.md)
	MemberStatusAPIKey         string     // current key, rendered into a masked input
	MemberStatusAPIKeySetAt    *time.Time // when the current key was set; nil if none
	MemberStatusEndpoint       string     // absolute URL the provider posts to, for display
	MemberStatusAllowedOrigins string     // allowed browser origins (CORS), one per line, for the textarea

	Error string
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

	h.render(w, r, h.buildVM(ctx, r, userID, settings, ""))
}

// buildVM assembles the settings form view model from a settings document.
// Used by both the initial render and the re-render after a validation error.
// deviceTestBuildVM is one selectable Unit 2 build.
type deviceTestBuildVM struct {
	Key   string // "unit2|2.2.3"
	Label string
}

// deviceTestBuilds lists the Unit 2 builds already on the CDN, newest first.
func (h *Handler) deviceTestBuilds(ctx context.Context) []deviceTestBuildVM {
	builds, err := mhsbuilds.New(h.DB).ListByUnit(ctx, models.MHSDeviceTestUnitID)
	if err != nil {
		h.Log.Warn("settings: listing device-test builds failed", zap.Error(err))
		return nil
	}
	out := make([]deviceTestBuildVM, 0, len(builds))
	for _, b := range builds {
		label := "v" + b.Version
		if b.BuildIdentifier != "" {
			label += " · " + b.BuildIdentifier
		}
		label += fmt.Sprintf(" · %d MB", b.TotalSize/1048576)
		if !b.CreatedAt.IsZero() {
			label += " · uploaded " + b.CreatedAt.UTC().Format("2006-01-02")
		}
		out = append(out, deviceTestBuildVM{Key: models.MHSDeviceTestUnitID + "|" + b.Version, Label: label})
	}
	return out
}

// deviceTestURL is the public device-test URL for this workspace's host.
func deviceTestURL(r *http.Request) string {
	scheme := "https"
	if r.TLS == nil && r.Header.Get("X-Forwarded-Proto") != "https" &&
		(strings.HasPrefix(r.Host, "localhost") || strings.HasPrefix(r.Host, "127.0.0.1")) {
		scheme = "http"
	}
	return scheme + "://" + r.Host + "/missionhydrosci/devicetest"
}

func (h *Handler) buildVM(ctx context.Context, r *http.Request, userID primitive.ObjectID, settings models.SiteSettings, errMsg string) settingsVM {
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

	return settingsVM{
		BaseVM:                     viewdata.NewBaseVM(r, h.DB, "Settings", "/dashboard"),
		HasLogo:                    settings.HasLogo(),
		LogoName:                   settings.LogoName,
		LandingTitle:               landingTitle,
		LandingContent:             settings.LandingContent,
		AllAuthMethods:             models.AllAuthMethods,
		EnabledAuthMethods:         enabledMap,
		CurrentUserMethod:          currentUserMethod,
		MHSMemberAuth:              settings.GetMHSMemberAuth(),
		MHSMemberAuthKeyword:       settings.MHSMemberAuthKeyword,
		MHSStaffUnlockMinutes:      displayUnlockMinutes(settings.MHSStaffUnlockMinutes),
		EnableClaudeSummaries:      settings.EnableClaudeSummaries,
		ClaudeModel:                settings.ClaudeModel,
		MHSDeviceTestEnabled:       settings.MHSDeviceTestEnabled,
		MHSDeviceTestUnitKey:       deviceTestUnitKey(settings.MHSDeviceTestUnit),
		MHSDeviceTestBuilds:        h.deviceTestBuilds(ctx),
		MHSDeviceTestURL:           deviceTestURL(r),
		MemberStatusAPIKey:         settings.MemberStatusAPIKey,
		MemberStatusAPIKeySetAt:    settings.MemberStatusAPIKeySetAt,
		MemberStatusEndpoint:       memberStatusEndpoint(r),
		MemberStatusAllowedOrigins: strings.Join(settings.MemberStatusAPIAllowedOrigins, "\n"),
		Error:                      errMsg,
	}
}

// memberStatusEndpoint is the absolute URL an external provider posts member
// status events to for the workspace serving this request. HTTPS is assumed
// except for local development hosts.
func memberStatusEndpoint(r *http.Request) string {
	scheme := "https"
	host := r.Host
	if r.TLS == nil && r.Header.Get("X-Forwarded-Proto") != "https" &&
		(strings.HasPrefix(host, "localhost") || strings.HasPrefix(host, "127.0.0.1")) {
		scheme = "http"
	}
	return scheme + "://" + host + "/api/member-status"
}

// Member Status API key constraints. The key is a shared secret typed or
// pasted by an admin (or generated in the browser), so the rules are about
// rejecting obviously weak or malformed values, not about format.
const (
	memberStatusKeyMinLen = 16
	memberStatusKeyMaxLen = 128
)

// validateMemberStatusKey returns a user-facing message for an unacceptable
// key, or "" when the key is acceptable. An empty key is acceptable (it
// disables the API).
func validateMemberStatusKey(key string) string {
	if key == "" {
		return ""
	}
	if len(key) < memberStatusKeyMinLen || len(key) > memberStatusKeyMaxLen {
		return fmt.Sprintf("The Member Status API key must be between %d and %d characters.", memberStatusKeyMinLen, memberStatusKeyMaxLen)
	}
	if strings.ContainsAny(key, " \t\r\n") {
		return "The Member Status API key cannot contain spaces."
	}
	return ""
}

// displayUnlockMinutes substitutes the default when the setting is unset so
// the form shows the effective value.
func displayUnlockMinutes(minutes int) int {
	if minutes <= 0 {
		return models.DefaultMHSStaffUnlockMinutes
	}
	return minutes
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
	mhsMemberAuth := strings.TrimSpace(r.FormValue("mhs_member_auth"))
	mhsMemberAuthKeyword := strings.TrimSpace(r.FormValue("mhs_member_auth_keyword"))
	mhsStaffUnlockStr := strings.TrimSpace(r.FormValue("mhs_staff_unlock_minutes"))
	enableClaudeSummaries := r.FormValue("enable_claude_summaries") != ""
	claudeModel := strings.TrimSpace(r.FormValue("claude_model"))
	mhsDeviceTestEnabled := r.FormValue("mhs_device_test_enabled") != ""
	mhsDeviceTestUnitKey := strings.TrimSpace(r.FormValue("mhs_device_test_unit"))
	memberStatusAPIKey := strings.TrimSpace(r.FormValue("member_status_api_key"))
	memberStatusOriginsText := r.FormValue("member_status_api_allowed_origins")

	// Validation
	if siteName == "" {
		h.renderWithError(w, r, wsID, "Site name is required.")
		return
	}
	if len(authMethods) == 0 {
		h.renderWithError(w, r, wsID, "At least one authentication method must be selected.")
		return
	}
	if msg := validateMemberStatusKey(memberStatusAPIKey); msg != "" {
		h.renderWithError(w, r, wsID, msg)
		return
	}
	memberStatusOrigins, err := memberstatusapi.ParseAllowedOrigins(memberStatusOriginsText)
	if err != nil {
		h.renderWithError(w, r, wsID, "Allowed browser origins: "+err.Error()+".")
		return
	}

	// Validate Claude model selection
	switch claudeModel {
	case "", "claude-haiku-4-5-20251001", "claude-sonnet-4-20250514", "claude-opus-4-20250514":
		// valid
	default:
		h.renderWithError(w, r, wsID, "Invalid AI model selection.")
		return
	}

	// Validate MHS member auth setting
	switch mhsMemberAuth {
	case "trust", "staffauth", "":
		// valid
	case "keyword":
		if mhsMemberAuthKeyword == "" {
			h.renderWithError(w, r, wsID, "A keyword is required when Keyword mode is selected.")
			return
		}
	default:
		h.renderWithError(w, r, wsID, "Invalid MHS member authorization mode.")
		return
	}

	// Validate MHS staff unlock duration (minutes). Empty keeps the default.
	mhsStaffUnlockMinutes := 0
	if mhsStaffUnlockStr != "" {
		n, err := strconv.Atoi(mhsStaffUnlockStr)
		if err != nil || n < 1 || n > 120 {
			h.renderWithError(w, r, wsID, "Staff unlock duration must be a number of minutes between 1 and 120.")
			return
		}
		mhsStaffUnlockMinutes = n
	}

	// Get current user's auth method for protection check
	_, _, userID, _ := authz.UserCtx(r)

	ctx, cancel := context.WithTimeout(r.Context(), timeouts.Long())
	defer cancel()

	// Device-test build: must be a Unit 2 build that still exists.
	var mhsDeviceTestUnit *models.MHSUnitVersionRef
	if mhsDeviceTestUnitKey != "" {
		unitID, version, ok := strings.Cut(mhsDeviceTestUnitKey, "|")
		if !ok || unitID != models.MHSDeviceTestUnitID || strings.TrimSpace(version) == "" {
			h.renderWithError(w, r, wsID, "Invalid device-test build selection.")
			return
		}
		exists, err := mhsbuilds.New(h.DB).Exists(ctx, unitID, version)
		if err != nil {
			h.ErrLog.LogServerError(w, r, "check device-test build failed", err, "Couldn't validate the device-test build.", "/settings")
			return
		}
		if !exists {
			h.renderWithError(w, r, wsID, "The selected device-test build no longer exists.")
			return
		}
		mhsDeviceTestUnit = &models.MHSUnitVersionRef{UnitID: unitID, Version: version}
	}

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

	// Save settings. Start from the current document and overlay only the
	// fields this form carries: Save writes every whitelisted field, so a
	// fresh struct here would blank settings managed elsewhere (for example
	// the active MHS collection, which is set from the MHS Builds pages).
	settings := current
	settings.SiteName = siteName
	settings.LogoPath = logoPath
	settings.LogoName = logoName
	settings.LandingTitle = landingTitle
	settings.LandingContent = landingContent
	settings.FooterHTML = footerHTML
	settings.EnabledAuthMethods = authMethods
	settings.MHSMemberAuth = mhsMemberAuth
	settings.MHSMemberAuthKeyword = mhsMemberAuthKeyword
	settings.MHSStaffUnlockMinutes = mhsStaffUnlockMinutes
	settings.EnableClaudeSummaries = enableClaudeSummaries
	settings.ClaudeModel = claudeModel
	settings.MHSDeviceTestEnabled = mhsDeviceTestEnabled
	settings.MHSDeviceTestUnit = mhsDeviceTestUnit
	settings.UpdatedByID = &memberID
	settings.UpdatedByName = uname

	// Member Status API key: the form always round-trips the current key, so
	// only a genuine change refreshes the set-at time and is audited.
	keyChanged := memberStatusAPIKey != current.MemberStatusAPIKey
	settings.MemberStatusAPIKey = memberStatusAPIKey
	if keyChanged {
		if memberStatusAPIKey == "" {
			settings.MemberStatusAPIKeySetAt = nil
		} else {
			now := time.Now().UTC()
			settings.MemberStatusAPIKeySetAt = &now
		}
	}
	// Allowed browser origins (CORS): already normalized by the parser, so a
	// re-saved unchanged list compares equal and is not audited again.
	originsChanged := !slices.Equal(memberStatusOrigins, current.MemberStatusAPIAllowedOrigins)
	settings.MemberStatusAPIAllowedOrigins = memberStatusOrigins

	if err := store.Save(ctx, wsID, settings); err != nil {
		h.Log.Error("failed to save settings", zap.Error(err))
		h.renderWithError(w, r, wsID, "Failed to save settings.")
		return
	}

	if keyChanged || originsChanged {
		role, _, actorID, _ := authz.UserCtx(r)
		if keyChanged {
			h.AuditLog.MemberStatusKeyChanged(ctx, r, actorID, role, memberStatusAPIKey == "")
		}
		if originsChanged {
			h.AuditLog.MemberStatusOriginsChanged(ctx, r, actorID, role, memberStatusOrigins)
		}
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

	h.render(w, r, h.buildVM(ctx, r, userID, settings, errMsg))
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

// deviceTestUnitKey renders the stored build reference as the select value.
func deviceTestUnitKey(ref *models.MHSUnitVersionRef) string {
	if ref == nil || ref.Version == "" {
		return ""
	}
	return ref.UnitID + "|" + ref.Version
}
