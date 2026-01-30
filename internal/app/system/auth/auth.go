package auth

// Terminology: User Identifiers
//   - UserID / userID / user_id: The MongoDB ObjectID (_id) that uniquely identifies a user record
//   - LoginID / loginID / login_id: The human-readable string users type to log in

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/dalemusser/stratahub/internal/app/system/normalize"
	"github.com/gorilla/securecookie"
	"github.com/gorilla/sessions"
	"go.uber.org/zap"
)

// Session error classification for logging and monitoring.
type sessionErrorType int

const (
	sessionErrUnknown sessionErrorType = iota
	sessionErrExpired                  // timestamp expired - normal
	sessionErrTampered                 // MAC invalid - potential attack
	sessionErrCorrupted                // decode/decrypt failed - corruption or key rotation
	sessionErrBackend                  // store/backend failure
)

/*─────────────────────────────────────────────────────────────────────────────*
| Session constants                                                           |
*─────────────────────────────────────────────────────────────────────────────*/

const (
	isAuthKey       = "is_authenticated"
	userIDKey       = "user_id"
	userName        = "user_name"
	userLoginID     = "user_login_id"
	userRole        = "user_role"
	userOrgID       = "organization_id"
	sessionTokenKey = "session_token"
)

/*─────────────────────────────────────────────────────────────────────────────*
| SessionManager - injectable session management                              |
*─────────────────────────────────────────────────────────────────────────────*/

// SessionManager encapsulates session store and configuration.
// It provides middleware and utilities for session-based authentication.
// Use NewSessionManager to create an instance.
type SessionManager struct {
	store       *sessions.CookieStore
	logger      *zap.Logger
	name        string
	userFetcher UserFetcher
}

// NewSessionManager creates a new SessionManager with the provided configuration.
//
// Parameters:
//   - sessionKey: signing key for cookies (must be ≥32 chars in production)
//   - name: session cookie name (defaults to "stratahub-session" if empty)
//   - domain: cookie domain (empty means current host)
//   - maxAge: session cookie lifetime (e.g., 24*time.Hour)
//   - secure: if true, cookies have the Secure flag set (for HTTPS production)
//   - logger: zap logger for session error logging
//
// Returns an error if sessionKey is empty or too weak for production mode.
func NewSessionManager(sessionKey, name, domain string, maxAge time.Duration, secure bool, logger *zap.Logger) (*SessionManager, error) {
	if sessionKey == "" {
		return nil, &SessionConfigError{Message: "session key is empty; provide ≥32 random chars"}
	}

	// Check for weak/default keys
	isWeak := len(sessionKey) < 32 || isDefaultKey(sessionKey)

	if secure {
		// In production mode, require a strong key - fail startup if weak
		if isWeak {
			return nil, &SessionConfigError{
				Message: "session key is too weak for production; provide ≥32 random chars (not the default dev key)",
			}
		}
	} else if isWeak {
		// In dev mode, warn but allow weak keys
		logger.Warn("session key is weak; 32+ random chars required in production",
			zap.Int("length", len(sessionKey)),
			zap.Bool("is_default", isDefaultKey(sessionKey)))
	}

	// Set session name (use default if empty)
	if name == "" {
		name = "stratahub-session"
	}

	store := sessions.NewCookieStore([]byte(sessionKey))
	opts := &sessions.Options{
		Domain:   domain,
		Path:     "/",
		MaxAge:   int(maxAge.Seconds()),
		Secure:   secure,
		HttpOnly: true,
	}

	// SameSite=Lax is the recommended setting for first-party session cookies.
	// It allows cookies on same-site requests and top-level navigations (like
	// clicking a link from an email), while blocking cross-site POST requests.
	// Note: SameSite=None is for third-party cookies (embeds, cross-site APIs)
	// and can cause issues with browser privacy settings.
	opts.SameSite = http.SameSiteLaxMode

	store.Options = opts

	logger.Info("session manager initialized",
		zap.Bool("secure", secure),
		zap.String("name", name),
		zap.String("domain", domain))

	return &SessionManager{
		store:  store,
		logger: logger,
		name:   name,
	}, nil
}

// SessionConfigError is returned when session configuration is invalid.
type SessionConfigError struct {
	Message string
}

func (e *SessionConfigError) Error() string {
	return e.Message
}

// SessionName returns the configured session cookie name.
func (sm *SessionManager) SessionName() string {
	return sm.name
}

// Store returns the underlying session store.
// Use this when you need direct access to store options.
func (sm *SessionManager) Store() *sessions.CookieStore {
	return sm.store
}

// GetSession retrieves the session for the request.
// If the session doesn't exist or is invalid, a new empty session is returned.
func (sm *SessionManager) GetSession(r *http.Request) (*sessions.Session, error) {
	return sm.store.Get(r, sm.name)
}

// SetUserFetcher sets the UserFetcher used by LoadSessionUser to fetch fresh
// user data on each request. This must be called after database initialization.
func (sm *SessionManager) SetUserFetcher(uf UserFetcher) {
	sm.userFetcher = uf
}

/*─────────────────────────────────────────────────────────────────────────────*
| UserFetcher interface                                                       |
*─────────────────────────────────────────────────────────────────────────────*/

// UserFetcher fetches fresh user data from the database.
// Implementations should return nil if the user is not found or is disabled.
type UserFetcher interface {
	// FetchUser retrieves a user by ID. Returns nil if user not found,
	// disabled, or any other condition that should invalidate the session.
	FetchUser(ctx context.Context, userID string) *SessionUser
}

/*─────────────────────────────────────────────────────────────────────────────*
| Current-User helper                                                        |
*─────────────────────────────────────────────────────────────────────────────*/

// SessionUser represents the authenticated user in the request context.
// This data is fetched fresh from the database on each request to ensure
// role changes, disabled accounts, and profile updates take effect immediately.
type SessionUser struct {
	ID               string
	Name             string
	LoginID          string   // User's login identifier
	Role             string
	OrganizationID   string   // For leaders/members (single org)
	OrganizationName string   // For leaders/members (single org name)
	OrganizationIDs  []string // For coordinators (multiple assigned orgs)

	// Coordinator-specific permissions (only relevant when Role == "coordinator")
	CanManageMaterials bool
	CanManageResources bool

	// Multi-workspace support
	WorkspaceID  string   // User's primary workspace (empty for superadmin)
	WorkspaceIDs []string // All workspaces user has access to
	IsSuperAdmin bool     // Quick check for superadmin role

	// Session token for MongoDB session tracking
	Token string
}

// SessionToken returns the session token for this user.
func (u *SessionUser) SessionToken() string {
	return u.Token
}

// GenerateSessionToken generates a cryptographically secure session token.
// The token is 32 random bytes encoded as URL-safe base64 (43 chars).
func GenerateSessionToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.URLEncoding.EncodeToString(b), nil
}

type ctxKey string

const currentUserKey ctxKey = "currentUser"

// CurrentUser returns the user & "found?" flag from the request context.
func CurrentUser(r *http.Request) (*SessionUser, bool) {
	u, ok := r.Context().Value(currentUserKey).(*SessionUser)
	return u, ok
}

/*─────────────────────────────────────────────────────────────────────────────*
| Middleware                                                                  |
*─────────────────────────────────────────────────────────────────────────────*/

// LoadSessionUser returns middleware that injects the user into context if logged in.
// If a UserFetcher is configured, fresh user data is fetched from the database
// on each request to ensure role changes, disabled accounts, and profile updates
// take effect immediately.
func (sm *SessionManager) LoadSessionUser(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sess, err := sm.store.Get(r, sm.name)
		if err != nil {
			// Classify the session error for appropriate logging.
			errType, errCategory := classifySessionError(err)
			switch errType {
			case sessionErrExpired:
				// Normal expiration - debug level, not a concern
				sm.logger.Debug("session expired, starting fresh session",
					zap.String("category", errCategory),
					zap.String("path", r.URL.Path))
			case sessionErrTampered:
				// MAC invalid - potential tampering attempt, log at Warn with client info
				sm.logger.Warn("session MAC validation failed (possible tampering)",
					zap.String("category", errCategory),
					zap.String("path", r.URL.Path),
					zap.String("remote_addr", r.RemoteAddr),
					zap.String("user_agent", r.UserAgent()))
			case sessionErrCorrupted:
				// Decode/decrypt failed - could be key rotation or corruption
				sm.logger.Info("session decode failed, starting fresh session",
					zap.String("category", errCategory),
					zap.String("path", r.URL.Path))
			case sessionErrBackend:
				// Store backend failure - this is concerning
				sm.logger.Error("session store error, starting fresh session",
					zap.Error(err),
					zap.String("path", r.URL.Path))
			default:
				// Unknown error type
				sm.logger.Warn("session error, starting fresh session",
					zap.Error(err),
					zap.String("category", errCategory),
					zap.String("path", r.URL.Path))
			}
		}

		if isAuth, _ := sess.Values[isAuthKey].(bool); isAuth {
			userID := getString(sess, userIDKey)

			// If we have a UserFetcher, get fresh data from DB
			if sm.userFetcher != nil && userID != "" {
				u := sm.userFetcher.FetchUser(r.Context(), userID)
				if u != nil {
					// User exists and is active - inject into context
					// Also extract session token for MongoDB session tracking
					u.Token = getString(sess, sessionTokenKey)
					r = withUser(r, u)
				} else {
					// User not found, disabled, or deleted - clear session
					sm.logger.Info("session invalidated: user not found or disabled",
						zap.String("user_id", userID),
						zap.String("path", r.URL.Path))
					sess.Values[isAuthKey] = false
					delete(sess.Values, userIDKey)
					_ = sess.Save(r, w) // Best effort to clear
				}
			} else if userID != "" {
				// Fallback: no UserFetcher configured, use session data (legacy behavior)
				u := &SessionUser{
					ID:             userID,
					Name:           getString(sess, userName),
					LoginID:        getString(sess, userLoginID),
					Role:           getString(sess, userRole),
					OrganizationID: getString(sess, userOrgID),
					Token:          getString(sess, sessionTokenKey),
				}
				r = withUser(r, u)
			}
		}
		next.ServeHTTP(w, r)
	})
}

// RequireSignedIn returns middleware that ensures there is a user in context.
// If not signed in:
//   - HTMX: sends HX-Redirect to /login?return=...
//   - HTML: 303 redirect to /login?return=...
//   - API:  401 Unauthorized with a plain error body.
func (sm *SessionManager) RequireSignedIn(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, ok := CurrentUser(r); ok {
			next.ServeHTTP(w, r)
			return
		}

		ret := url.QueryEscape(currentURI(r))

		// HTMX: full-page client redirect (no partial swap)
		if r.Header.Get("HX-Request") == "true" {
			w.Header().Set("HX-Redirect", "/login?return="+ret)
			w.WriteHeader(http.StatusUnauthorized)
			return
		}

		// Browser/HTML: go to login and preserve return
		if wantsHTML(r) {
			http.Redirect(w, r, "/login?return="+ret, http.StatusSeeOther)
			return
		}

		// Non-HTML (API) callers: plain 401
		http.Error(w, "unauthorized", http.StatusUnauthorized)
	})
}

// RequireRole returns middleware that ensures there is a user with the required role.
// If not authorized, it redirects to HTML pages (or sets HX-Redirect) instead of writing a blank error.
// Note: superadmin always passes role checks - they have access to everything.
func (sm *SessionManager) RequireRole(allowed ...string) func(http.Handler) http.Handler {
	set := make(map[string]struct{}, len(allowed))
	for _, role := range allowed {
		set[normalize.Role(role)] = struct{}{}
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			u, ok := CurrentUser(r)

			// 1) Not signed in → 401 semantics
			if !ok {
				ret := url.QueryEscape(currentURI(r))

				// HTMX: tell the browser to navigate (no partial swap)
				if r.Header.Get("HX-Request") == "true" {
					w.Header().Set("HX-Redirect", "/login?return="+ret)
					w.WriteHeader(http.StatusUnauthorized)
					return
				}

				// HTML: redirect to login with return param
				if wantsHTML(r) {
					http.Redirect(w, r, "/login?return="+ret, http.StatusSeeOther)
					return
				}

				// Non-HTML (API): keep the status code
				http.Error(w, "unauthorized", http.StatusUnauthorized)
				return
			}

			// 2) Superadmin always passes - they have access to everything
			userRole := normalize.Role(u.Role)
			if userRole == "superadmin" {
				next.ServeHTTP(w, r)
				return
			}

			// 3) Signed in but wrong role → 403 semantics
			if _, has := set[userRole]; !has {
				// HTMX: redirect (so the full page swaps)
				if r.Header.Get("HX-Request") == "true" {
					dest := "/forbidden"
					w.Header().Set("HX-Redirect", dest)
					w.WriteHeader(http.StatusForbidden)
					return
				}

				// HTML: redirect to a friendly page
				if wantsHTML(r) {
					http.Redirect(w, r, "/forbidden", http.StatusSeeOther)
					return
				}

				// Non-HTML (API): keep the status code
				http.Error(w, "forbidden", http.StatusForbidden)
				return
			}

			// Authorized → carry on
			next.ServeHTTP(w, r)
		})
	}
}

/*─────────────────────────────────────────────────────────────────────────────*
| Helpers                                                                     |
*─────────────────────────────────────────────────────────────────────────────*/

func withUser(r *http.Request, u *SessionUser) *http.Request {
	return r.WithContext(context.WithValue(r.Context(), currentUserKey, u))
}

// WithTestUser injects a SessionUser into the request context for testing.
// This should only be used in tests to simulate authenticated requests.
func WithTestUser(r *http.Request, u *SessionUser) *http.Request {
	return withUser(r, u)
}

// getString safely extracts a string from a session value.
func getString(s *sessions.Session, key string) string {
	if v, ok := s.Values[key].(string); ok {
		return v
	}
	return ""
}

func wantsHTML(r *http.Request) bool {
	// Very light heuristic: treat it as HTML if it's HTMX or Accepts text/html.
	if r.Header.Get("HX-Request") == "true" {
		return true
	}
	accept := r.Header.Get("Accept")
	return strings.Contains(accept, "text/html")
}

func currentURI(r *http.Request) string {
	// Preserve path + query as a return param.
	u := *r.URL
	return u.RequestURI()
}

// isDefaultKey checks if the session key appears to be a default/placeholder value.
// This catches common patterns like "dev-only", "change-me", "placeholder", etc.
func isDefaultKey(key string) bool {
	lower := strings.ToLower(key)
	patterns := []string{
		"dev-only",
		"change-me",
		"placeholder",
		"default",
		"example",
		"insecure",
		"test-key",
		"secret123",
		"password",
	}
	for _, p := range patterns {
		if strings.Contains(lower, p) {
			return true
		}
	}
	return false
}

// classifySessionError categorizes a session/cookie error for appropriate logging.
// Returns the error type and a human-readable category string.
func classifySessionError(err error) (sessionErrorType, string) {
	if err == nil {
		return sessionErrUnknown, "none"
	}

	errStr := strings.ToLower(err.Error())

	// Check for securecookie.Error interface first
	if scErr, ok := err.(securecookie.Error); ok {
		if !scErr.IsDecode() {
			// Not a decode error - likely a backend/usage issue
			return sessionErrBackend, "backend"
		}

		// Decode errors - classify by error message patterns
		// securecookie uses specific error messages we can match
		switch {
		case strings.Contains(errStr, "expired timestamp"):
			return sessionErrExpired, "expired"
		case strings.Contains(errStr, "mac") || strings.Contains(errStr, "hash"):
			// MAC/HMAC validation failed - tampering
			return sessionErrTampered, "mac_invalid"
		case strings.Contains(errStr, "decrypt"):
			// Decryption failed - wrong key or corruption
			return sessionErrCorrupted, "decrypt_failed"
		case strings.Contains(errStr, "base64") || strings.Contains(errStr, "decode"):
			// Base64/decode issues - corruption
			return sessionErrCorrupted, "decode_failed"
		default:
			// Other decode error
			return sessionErrCorrupted, "decode_other"
		}
	}

	// Not a securecookie.Error - treat as backend error
	return sessionErrBackend, "unknown"
}
