package auth

import (
	"context"
	"net/http"
	"net/url"
	"strings"

	"github.com/gorilla/sessions"
)

/*─────────────────────────────────────────────────────────────────────────────*
| Session constants & globals                                                |
*─────────────────────────────────────────────────────────────────────────────*/

const (
	SessionName = "adroit-session"

	isAuthKey = "is_authenticated"
	userIDKey = "user_id"
	userName  = "user_name"
	userEmail = "user_email"
	userRole  = "user_role"
)

// Store is initialised once via InitSessionStore.
var Store *sessions.CookieStore

/*─────────────────────────────────────────────────────────────────────────────*
| Current-User helper                                                        |
*─────────────────────────────────────────────────────────────────────────────*/

// SessionUser is what we cache in the session & inject into r.Context().
type SessionUser struct {
	ID    string
	Name  string
	Email string
	Role  string
}

type ctxKey string

const currentUserKey ctxKey = "currentUser"

// CurrentUser returns the user & “found?” flag.
func CurrentUser(r *http.Request) (*SessionUser, bool) {
	u, ok := r.Context().Value(currentUserKey).(*SessionUser)
	return u, ok
}

// LoadSessionUser injects the user into context if they are logged in.
// If the session store has not been initialized yet, it is a no-op.
func LoadSessionUser(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// If the session store isn't configured yet, just continue.
		if Store == nil {
			next.ServeHTTP(w, r)
			return
		}

		sess, _ := Store.Get(r, SessionName)

		if isAuth, _ := sess.Values[isAuthKey].(bool); isAuth {
			u := &SessionUser{
				ID:    getString(sess, userIDKey),
				Name:  getString(sess, userName),
				Email: getString(sess, userEmail),
				Role:  getString(sess, userRole),
			}
			r = withUser(r, u)
		}
		next.ServeHTTP(w, r)
	})
}

// RequireSignedIn ensures there is a user in context (set by LoadSessionUser).
// If not signed in:
//   - HTMX: sends HX-Redirect to /login?return=...
//   - HTML: 303 redirect to /login?return=...
//   - API:  401 Unauthorized with a plain error body.
func RequireSignedIn(next http.Handler) http.Handler {
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

// RequireRole ensures there is a user with the required role in context (set by LoadSessionUser).
// If not authorized, it redirects to HTML pages (or sets HX-Redirect) instead of writing a blank error.
func RequireRole(allowed ...string) func(http.Handler) http.Handler {
	set := make(map[string]struct{}, len(allowed))
	for _, role := range allowed {
		set[strings.ToLower(strings.TrimSpace(role))] = struct{}{}
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

			// 2) Signed in but wrong role → 403 semantics
			if _, has := set[strings.ToLower(u.Role)]; !has {
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

// helpers

func withUser(r *http.Request, u *SessionUser) *http.Request {
	return r.WithContext(context.WithValue(r.Context(), currentUserKey, u))
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
