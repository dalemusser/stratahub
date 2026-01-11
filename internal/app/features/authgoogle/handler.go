// internal/app/features/authgoogle/handler.go
package authgoogle

// Terminology: User Identifiers
//   - UserID / userID / user_id: The MongoDB ObjectID (_id) that uniquely identifies a user record
//   - LoginID / loginID / login_id: The human-readable string users type to log in

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	uierrors "github.com/dalemusser/stratahub/internal/app/features/errors"
	"github.com/dalemusser/stratahub/internal/app/store/oauthstate"
	"github.com/dalemusser/stratahub/internal/app/store/sessions"
	workspacestore "github.com/dalemusser/stratahub/internal/app/store/workspaces"
	"github.com/dalemusser/stratahub/internal/app/system/auth"
	"github.com/dalemusser/stratahub/internal/app/system/auditlog"
	"github.com/dalemusser/stratahub/internal/app/system/normalize"
	"github.com/dalemusser/stratahub/internal/app/system/timeouts"
	"github.com/dalemusser/stratahub/internal/app/system/workspace"
	"github.com/dalemusser/stratahub/internal/domain/models"
	"github.com/dalemusser/waffle/pantry/query"
	"github.com/dalemusser/waffle/pantry/urlutil"
	"github.com/gorilla/securecookie"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.uber.org/zap"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
)

// Handler handles Google OAuth authentication.
type Handler struct {
	DB         *mongo.Database
	Log        *zap.Logger
	SessionMgr *auth.SessionManager
	ErrLog     *uierrors.ErrorLogger
	AuditLog   *auditlog.Logger
	Sessions   *sessions.Store // Activity session tracking
	StateStore *oauthstate.Store
	Workspaces *workspacestore.Store

	// OAuth configuration
	ClientID     string
	ClientSecret string
	RedirectURL  string // e.g., "https://stratahub.com/auth/google/callback"

	// Multi-workspace configuration
	MultiWorkspace bool   // true = subdomain-based workspaces
	PrimaryDomain  string // Apex domain for redirects (e.g., "adroit.games")
}

// NewHandler creates a new Google OAuth handler.
func NewHandler(
	db *mongo.Database,
	sessionMgr *auth.SessionManager,
	errLog *uierrors.ErrorLogger,
	audit *auditlog.Logger,
	sessStore *sessions.Store,
	stateStore *oauthstate.Store,
	wsStore *workspacestore.Store,
	clientID, clientSecret, baseURL string,
	multiWorkspace bool,
	primaryDomain string,
	logger *zap.Logger,
) *Handler {
	return &Handler{
		DB:             db,
		Log:            logger,
		SessionMgr:     sessionMgr,
		ErrLog:         errLog,
		AuditLog:       audit,
		Sessions:       sessStore,
		StateStore:     stateStore,
		Workspaces:     wsStore,
		ClientID:       clientID,
		ClientSecret:   clientSecret,
		RedirectURL:    baseURL + "/auth/google/callback",
		MultiWorkspace: multiWorkspace,
		PrimaryDomain:  primaryDomain,
	}
}

// oauth2Config returns the Google OAuth2 configuration.
func (h *Handler) oauth2Config() *oauth2.Config {
	return &oauth2.Config{
		ClientID:     h.ClientID,
		ClientSecret: h.ClientSecret,
		RedirectURL:  h.RedirectURL,
		Scopes: []string{
			"openid",
			"https://www.googleapis.com/auth/userinfo.email",
			"https://www.googleapis.com/auth/userinfo.profile",
		},
		Endpoint: google.Endpoint,
	}
}

// IsConfigured returns true if Google OAuth is configured.
func (h *Handler) IsConfigured() bool {
	return h.ClientID != "" && h.ClientSecret != ""
}

/*─────────────────────────────────────────────────────────────────────────────*
| GET /auth/google                                                             |
| Initiates the Google OAuth flow by redirecting to Google's consent screen.   |
*─────────────────────────────────────────────────────────────────────────────*/

func (h *Handler) ServeLogin(w http.ResponseWriter, r *http.Request) {
	if !h.IsConfigured() {
		h.Log.Warn("Google OAuth not configured")
		http.Redirect(w, r, "/login?error=google_not_configured", http.StatusSeeOther)
		return
	}

	// Generate cryptographically secure state
	state, err := generateState()
	if err != nil {
		h.Log.Error("failed to generate OAuth state", zap.Error(err))
		http.Redirect(w, r, "/login?error=internal", http.StatusSeeOther)
		return
	}

	// Get return URL from query params
	returnURL := query.Get(r, "return")

	// Get workspace subdomain from context (set by workspace middleware)
	wsSubdomain := ""
	if ws := workspace.FromRequest(r); ws != nil && !ws.IsApex {
		wsSubdomain = ws.Subdomain
	}

	// Store state with 10-minute expiry (including workspace for redirect)
	ctx, cancel := context.WithTimeout(r.Context(), timeouts.Short())
	defer cancel()

	expiresAt := time.Now().UTC().Add(10 * time.Minute)
	if err := h.StateStore.Save(ctx, state, returnURL, wsSubdomain, expiresAt); err != nil {
		h.Log.Error("failed to save OAuth state", zap.Error(err))
		http.Redirect(w, r, "/login?error=internal", http.StatusSeeOther)
		return
	}

	// Redirect to Google
	url := h.oauth2Config().AuthCodeURL(state, oauth2.AccessTypeOffline)

	h.Log.Debug("initiating Google OAuth flow",
		zap.String("redirect_url", url),
		zap.String("return_url", returnURL),
		zap.String("workspace", wsSubdomain))

	http.Redirect(w, r, url, http.StatusTemporaryRedirect)
}

/*─────────────────────────────────────────────────────────────────────────────*
| GET /auth/google/callback                                                    |
| Handles the OAuth callback from Google, exchanges code for tokens,           |
| fetches user info, looks up user in database, and creates session.           |
*─────────────────────────────────────────────────────────────────────────────*/

func (h *Handler) ServeCallback(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// Check for errors from Google
	if errParam := r.URL.Query().Get("error"); errParam != "" {
		errDesc := r.URL.Query().Get("error_description")
		h.Log.Warn("Google OAuth error",
			zap.String("error", errParam),
			zap.String("description", errDesc))
		http.Redirect(w, r, "/login?error=google_denied", http.StatusSeeOther)
		return
	}

	// Validate state parameter
	state := r.URL.Query().Get("state")
	if state == "" {
		h.Log.Warn("missing OAuth state parameter")
		http.Redirect(w, r, "/login?error=invalid_state", http.StatusSeeOther)
		return
	}

	ctxTimeout, cancel := context.WithTimeout(ctx, timeouts.Short())
	defer cancel()

	returnURL, wsSubdomain, valid, err := h.StateStore.Validate(ctxTimeout, state)
	if err != nil {
		h.Log.Error("failed to validate OAuth state", zap.Error(err))
		http.Redirect(w, r, "/login?error=internal", http.StatusSeeOther)
		return
	}
	if !valid {
		h.Log.Warn("invalid or expired OAuth state")
		http.Redirect(w, r, "/login?error=invalid_state", http.StatusSeeOther)
		return
	}

	// Exchange code for token
	code := r.URL.Query().Get("code")
	if code == "" {
		h.Log.Warn("missing OAuth code parameter")
		http.Redirect(w, r, "/login?error=invalid_code", http.StatusSeeOther)
		return
	}

	token, err := h.oauth2Config().Exchange(ctx, code)
	if err != nil {
		h.Log.Error("failed to exchange OAuth code", zap.Error(err))
		http.Redirect(w, r, "/login?error=token_exchange", http.StatusSeeOther)
		return
	}

	// Fetch user info from Google
	googleUser, err := fetchGoogleUserInfo(ctx, token)
	if err != nil {
		h.Log.Error("failed to fetch Google user info", zap.Error(err))
		http.Redirect(w, r, "/login?error=user_info", http.StatusSeeOther)
		return
	}

	h.Log.Debug("Google user info fetched",
		zap.String("google_id", googleUser.ID),
		zap.String("email", googleUser.Email),
		zap.String("name", googleUser.Name),
		zap.String("workspace", wsSubdomain))

	// Determine workspace ID for user lookup
	var workspaceID *primitive.ObjectID
	if h.MultiWorkspace && wsSubdomain != "" {
		// Multi-workspace mode: look up workspace by subdomain
		ws, err := h.Workspaces.GetBySubdomain(ctxTimeout, wsSubdomain)
		if err != nil {
			h.Log.Warn("workspace not found for OAuth callback",
				zap.String("subdomain", wsSubdomain),
				zap.Error(err))
			http.Redirect(w, r, "/login?error=workspace_not_found", http.StatusSeeOther)
			return
		}
		workspaceID = &ws.ID
	} else if !h.MultiWorkspace {
		// Single workspace mode: get default workspace
		ws, err := h.Workspaces.GetFirst(ctxTimeout)
		if err == nil {
			workspaceID = &ws.ID
		}
	}

	// Look up user in database by Google ID (stored in auth_return_id) or email
	user, err := h.findUserInWorkspace(ctx, r, googleUser, workspaceID)
	if err != nil {
		if err == errUserNotFound {
			h.Log.Info("Google OAuth: user not found",
				zap.String("google_id", googleUser.ID),
				zap.String("email", googleUser.Email),
				zap.String("workspace", wsSubdomain))
			h.AuditLog.LoginFailedUserNotFound(ctx, r, googleUser.Email)
			h.redirectToLogin(w, r, wsSubdomain, "no_account")
			return
		}
		if err == errUserDisabled {
			h.Log.Info("Google OAuth: user disabled",
				zap.String("google_id", googleUser.ID),
				zap.String("email", googleUser.Email))
			// Note: Audit logging is done in findUserInWorkspace where we have the user object
			h.redirectToLogin(w, r, wsSubdomain, "account_disabled")
			return
		}
		h.Log.Error("failed to look up user", zap.Error(err))
		h.redirectToLogin(w, r, wsSubdomain, "internal")
		return
	}

	// Create session and redirect
	h.createSessionAndRedirect(w, r, user, wsSubdomain, returnURL)
}

/*─────────────────────────────────────────────────────────────────────────────*
| User lookup                                                                  |
*─────────────────────────────────────────────────────────────────────────────*/

var (
	errUserNotFound  = fmt.Errorf("user not found")
	errUserDisabled  = fmt.Errorf("user disabled")
	errAuthMismatch  = fmt.Errorf("auth method mismatch")
)

// googleUserInfo represents user info returned from Google.
type googleUserInfo struct {
	ID            string `json:"id"`
	Email         string `json:"email"`
	EmailVerified bool   `json:"verified_email"`
	Name          string `json:"name"`
	GivenName     string `json:"given_name"`
	FamilyName    string `json:"family_name"`
	Picture       string `json:"picture"`
}

// fetchGoogleUserInfo retrieves user information from Google's userinfo endpoint.
func fetchGoogleUserInfo(ctx context.Context, token *oauth2.Token) (*googleUserInfo, error) {
	client := oauth2.NewClient(ctx, oauth2.StaticTokenSource(token))

	resp, err := client.Get("https://www.googleapis.com/oauth2/v2/userinfo")
	if err != nil {
		return nil, fmt.Errorf("failed to fetch user info: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	var info googleUserInfo
	if err := json.NewDecoder(resp.Body).Decode(&info); err != nil {
		return nil, fmt.Errorf("failed to decode user info: %w", err)
	}

	return &info, nil
}

// findUserInWorkspace looks up a user by Google ID or email within a specific workspace.
// For Google auth, users are identified by:
// 1. auth_return_id = Google's user ID (if already linked) + workspace_id
// 2. login_id (email) + auth_method = "google" + workspace_id
func (h *Handler) findUserInWorkspace(ctx context.Context, r *http.Request, googleUser *googleUserInfo, workspaceID *primitive.ObjectID) (*models.User, error) {
	userColl := h.DB.Collection("users")

	var u models.User

	// Build base filter with workspace constraint
	baseFilter := bson.M{
		"auth_method": "google",
	}
	if workspaceID != nil {
		baseFilter["workspace_id"] = *workspaceID
	}

	// First, try to find by Google ID (auth_return_id for existing Google users)
	googleIDFilter := bson.M{
		"auth_return_id": googleUser.ID,
	}
	for k, v := range baseFilter {
		googleIDFilter[k] = v
	}

	err := userColl.FindOne(ctx, googleIDFilter).Decode(&u)

	if err == nil {
		// Found by Google ID
		if normalize.Status(u.Status) == "disabled" {
			h.AuditLog.LoginFailedUserDisabled(ctx, r, u.ID, u.OrganizationID, googleUser.Email)
			return nil, errUserDisabled
		}
		return &u, nil
	}

	if err != mongo.ErrNoDocuments {
		return nil, err
	}

	// Not found by Google ID - try by email (login_id) with auth_method = google
	emailFilter := bson.M{
		"login_id_ci": strings.ToLower(googleUser.Email),
	}
	for k, v := range baseFilter {
		emailFilter[k] = v
	}

	err = userColl.FindOne(ctx, emailFilter).Decode(&u)

	if err == nil {
		// Found by email - update auth_return_id if not set
		if u.AuthReturnID == nil || *u.AuthReturnID == "" {
			_, updateErr := userColl.UpdateOne(ctx,
				bson.M{"_id": u.ID},
				bson.M{"$set": bson.M{"auth_return_id": googleUser.ID}},
			)
			if updateErr != nil {
				h.Log.Warn("failed to update auth_return_id",
					zap.Error(updateErr),
					zap.String("user_id", u.ID.Hex()))
			}
		}

		if normalize.Status(u.Status) == "disabled" {
			h.AuditLog.LoginFailedUserDisabled(ctx, r, u.ID, u.OrganizationID, googleUser.Email)
			return nil, errUserDisabled
		}
		return &u, nil
	}

	if err != mongo.ErrNoDocuments {
		return nil, err
	}

	// User not found with Google auth method
	return nil, errUserNotFound
}

// redirectToLogin redirects to the login page, handling multi-workspace subdomain routing.
func (h *Handler) redirectToLogin(w http.ResponseWriter, r *http.Request, wsSubdomain, errorCode string) {
	loginURL := "/login?error=" + errorCode

	if h.MultiWorkspace && wsSubdomain != "" {
		// Redirect to the workspace's subdomain
		dest := fmt.Sprintf("https://%s.%s%s", wsSubdomain, h.PrimaryDomain, loginURL)
		http.Redirect(w, r, dest, http.StatusSeeOther)
		return
	}

	http.Redirect(w, r, loginURL, http.StatusSeeOther)
}

/*─────────────────────────────────────────────────────────────────────────────*
| Session creation                                                             |
*─────────────────────────────────────────────────────────────────────────────*/

// createSessionAndRedirect creates an authenticated session and redirects to the destination.
func (h *Handler) createSessionAndRedirect(w http.ResponseWriter, r *http.Request, u *models.User, wsSubdomain, returnURL string) {
	sess, err := h.SessionMgr.GetSession(r)
	if err != nil {
		if scErr, ok := err.(securecookie.Error); ok && scErr.IsDecode() {
			h.Log.Warn("session cookie invalid, using fresh session",
				zap.Error(err),
				zap.String("user_id", u.ID.Hex()))
		} else {
			h.Log.Error("session store error during login, using fresh session",
				zap.Error(err),
				zap.String("user_id", u.ID.Hex()))
		}
	}

	// Set authenticated state
	sess.Values["is_authenticated"] = true
	sess.Values["user_id"] = u.ID.Hex()

	loginID := ""
	if u.LoginID != nil {
		loginID = *u.LoginID
	}

	// Create activity session for tracking
	if h.Sessions != nil {
		ctx, cancel := context.WithTimeout(r.Context(), timeouts.Short())
		defer cancel()

		ip := extractIP(r)
		activitySess, err := h.Sessions.Create(ctx, u.ID, u.OrganizationID, ip, r.UserAgent(), sessions.CreatedByLogin)
		if err != nil {
			h.Log.Warn("failed to create activity session", zap.Error(err), zap.String("user_id", u.ID.Hex()))
		} else {
			sess.Values["activity_session_id"] = activitySess.ID.Hex()
		}
	}

	if err := sess.Save(r, w); err != nil {
		h.Log.Error("save session failed", zap.Error(err), zap.String("login_id", loginID))
		h.redirectToLogin(w, r, wsSubdomain, "session")
		return
	}

	// Audit log: login success
	h.AuditLog.LoginSuccess(r.Context(), r, u.ID, u.OrganizationID, "google", loginID)

	h.Log.Info("user logged in via Google OAuth",
		zap.String("user_id", u.ID.Hex()),
		zap.String("login_id", loginID),
		zap.String("workspace", wsSubdomain))

	// Set theme preference cookie (needs to work across subdomains)
	themePref := u.ThemePreference
	if themePref == "" {
		themePref = "system"
	}
	themeCookie := &http.Cookie{
		Name:     "theme_pref",
		Value:    themePref,
		Path:     "/",
		MaxAge:   60,
		HttpOnly: false,
		SameSite: http.SameSiteLaxMode,
	}
	// Set domain for cross-subdomain access in multi-workspace mode
	if h.MultiWorkspace && h.PrimaryDomain != "" {
		themeCookie.Domain = "." + h.PrimaryDomain
	}
	http.SetCookie(w, themeCookie)

	// Build destination URL
	safePath := urlutil.SafeReturn(returnURL, "", "/dashboard")

	// In multi-workspace mode with a subdomain, redirect to the subdomain
	if h.MultiWorkspace && wsSubdomain != "" {
		dest := fmt.Sprintf("https://%s.%s%s", wsSubdomain, h.PrimaryDomain, safePath)
		http.Redirect(w, r, dest, http.StatusSeeOther)
		return
	}

	http.Redirect(w, r, safePath, http.StatusSeeOther)
}

/*─────────────────────────────────────────────────────────────────────────────*
| Helpers                                                                      |
*─────────────────────────────────────────────────────────────────────────────*/

// generateState creates a cryptographically secure random state string.
func generateState() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.URLEncoding.EncodeToString(b), nil
}

// extractIP extracts the client IP address from the request.
func extractIP(r *http.Request) string {
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		if idx := strings.Index(xff, ","); idx != -1 {
			return strings.TrimSpace(xff[:idx])
		}
		return strings.TrimSpace(xff)
	}
	if xri := r.Header.Get("X-Real-IP"); xri != "" {
		return strings.TrimSpace(xri)
	}
	ip := r.RemoteAddr
	if idx := strings.LastIndex(ip, ":"); idx != -1 {
		ip = ip[:idx]
	}
	return ip
}
