package auth

import (
	"context"
	"net/http"

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

func withUser(r *http.Request, u *SessionUser) *http.Request {
	return r.WithContext(context.WithValue(r.Context(), currentUserKey, u))
}

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

// getString safely extracts a string from a session value.
func getString(s *sessions.Session, key string) string {
	if v, ok := s.Values[key].(string); ok {
		return v
	}
	return ""
}
