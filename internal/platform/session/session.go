// internal/platform/session/session.go
package session

import (
	"context"
	"net/http"

	"github.com/gorilla/sessions"
)

const (
	sessionName = "adroit-session"

	isAuthKey    = "is_authenticated"
	userIDKey    = "user_id"
	userNameKey  = "user_name"
	userEmailKey = "user_email"
	userRoleKey  = "user_role"
)

type Manager struct {
	store *sessions.CookieStore
}

// New initialises a Gorilla CookieStore with your secret keys.
func New(hashKey, blockKey []byte) *Manager {
	cs := sessions.NewCookieStore(hashKey, blockKey)
	cs.Options = &sessions.Options{
		Path:     "/",
		Secure:   true,
		HttpOnly: true,
		SameSite: http.SameSiteNoneMode,
	}
	return &Manager{store: cs}
}

/*────────────────────── Small helper getters ──────────────────────*/

func (m *Manager) load(r *http.Request) (*sessions.Session, error) {
	return m.store.Get(r, sessionName)
}

func (m *Manager) IsAuth(r *http.Request) bool {
	s, _ := m.load(r)
	v, _ := s.Values[isAuthKey].(bool)
	return v
}

func (m *Manager) Role(r *http.Request) string {
	s, _ := m.load(r)
	if v, ok := s.Values[userRoleKey].(string); ok {
		return v
	}
	return ""
}

func (m *Manager) UserName(r *http.Request) string {
	s, _ := m.load(r)
	if v, ok := s.Values[userNameKey].(string); ok {
		return v
	}
	return ""
}

/*────────────────────── Middleware helpers ────────────────────────*/

type ctxKey string

const currentUserKey ctxKey = "currentUser"

type SessionUser struct {
	ID   string
	Name string
	Role string
}

func (m *Manager) withUser(r *http.Request, u *SessionUser) *http.Request {
	return r.WithContext(context.WithValue(r.Context(), currentUserKey, u))
}

// RequireAuth ensures the user is logged in.
func (m *Manager) RequireAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !m.IsAuth(r) {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		u := &SessionUser{
			Name: m.UserName(r),
			Role: m.Role(r),
		}
		next.ServeHTTP(w, m.withUser(r, u))
	})
}

// RequireRole("admin","leader") guards routes by role.
func (m *Manager) RequireRole(allowed ...string) func(http.Handler) http.Handler {
	set := make(map[string]struct{}, len(allowed))
	for _, role := range allowed {
		set[role] = struct{}{}
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			role := m.Role(r)
			if _, ok := set[role]; !ok {
				http.Error(w, "forbidden", http.StatusForbidden)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// convenience method to store the claims you need

func (m *Manager) Login(
	w http.ResponseWriter, r *http.Request,
	id any, name, email, role string, orgID any, orgName string,
) {
	s, _ := m.store.Get(r, sessionName)
	s.Values[isAuthKey] = true
	s.Values[userIDKey] = id
	s.Values[userNameKey] = name
	s.Values[userEmailKey] = email
	s.Values[userRoleKey] = role
	s.Values["organization_id"] = orgID
	s.Values["organization_name"] = orgName
	_ = s.Save(r, w)
}
