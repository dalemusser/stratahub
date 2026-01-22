package auth_test

// Terminology: User Identifiers
//   - UserID / userID / user_id: The MongoDB ObjectID (_id) that uniquely identifies a user record
//   - LoginID / loginID / login_id: The human-readable string users type to log in

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/dalemusser/stratahub/internal/app/system/auth"
	"go.uber.org/zap"
)

func newTestSessionManager(t *testing.T) *auth.SessionManager {
	t.Helper()
	logger := zap.NewNop()
	sm, err := auth.NewSessionManager(
		"test-session-key-must-be-32-chars-long",
		"test-session",
		"",
		24*time.Hour,
		false,
		logger,
	)
	if err != nil {
		t.Fatalf("failed to create session manager: %v", err)
	}
	return sm
}

func TestRequireSignedIn_NoUser_RedirectsToLogin(t *testing.T) {
	sm := newTestSessionManager(t)

	handler := sm.RequireSignedIn(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("protected content"))
	}))

	req := httptest.NewRequest("GET", "/protected", nil)
	req.Header.Set("Accept", "text/html")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusSeeOther {
		t.Errorf("expected status %d, got %d", http.StatusSeeOther, rec.Code)
	}

	location := rec.Header().Get("Location")
	if !strings.HasPrefix(location, "/login") {
		t.Errorf("expected redirect to /login, got %q", location)
	}
}

func TestRequireSignedIn_NoUser_API_Returns401(t *testing.T) {
	sm := newTestSessionManager(t)

	handler := sm.RequireSignedIn(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("GET", "/api/data", nil)
	req.Header.Set("Accept", "application/json")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("expected status %d, got %d", http.StatusUnauthorized, rec.Code)
	}
}

func TestRequireSignedIn_NoUser_HTMX_ReturnsHXRedirect(t *testing.T) {
	sm := newTestSessionManager(t)

	handler := sm.RequireSignedIn(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("GET", "/protected", nil)
	req.Header.Set("HX-Request", "true")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("expected status %d, got %d", http.StatusUnauthorized, rec.Code)
	}

	hxRedirect := rec.Header().Get("HX-Redirect")
	if !strings.HasPrefix(hxRedirect, "/login") {
		t.Errorf("expected HX-Redirect to /login, got %q", hxRedirect)
	}
}

func TestRequireRole_NoUser_RedirectsToLogin(t *testing.T) {
	sm := newTestSessionManager(t)

	handler := sm.RequireRole("admin")(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("GET", "/admin", nil)
	req.Header.Set("Accept", "text/html")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusSeeOther {
		t.Errorf("expected status %d, got %d", http.StatusSeeOther, rec.Code)
	}

	location := rec.Header().Get("Location")
	if !strings.HasPrefix(location, "/login") {
		t.Errorf("expected redirect to /login, got %q", location)
	}
}

func TestRequireRole_WrongRole_RedirectsToForbidden(t *testing.T) {
	sm := newTestSessionManager(t)

	handler := sm.RequireRole("admin")(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	// Create a request with a member user in context
	req := httptest.NewRequest("GET", "/admin", nil)
	req.Header.Set("Accept", "text/html")

	// Inject a user with "member" role into context
	req = withTestUser(req, "member")

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusSeeOther {
		t.Errorf("expected status %d, got %d", http.StatusSeeOther, rec.Code)
	}

	location := rec.Header().Get("Location")
	if location != "/forbidden" {
		t.Errorf("expected redirect to /forbidden, got %q", location)
	}
}

func TestRequireRole_WrongRole_API_Returns403(t *testing.T) {
	sm := newTestSessionManager(t)

	handler := sm.RequireRole("admin")(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("GET", "/api/admin", nil)
	req.Header.Set("Accept", "application/json")
	req = withTestUser(req, "member")

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Errorf("expected status %d, got %d", http.StatusForbidden, rec.Code)
	}
}

func TestRequireRole_CorrectRole_Proceeds(t *testing.T) {
	sm := newTestSessionManager(t)

	called := false
	handler := sm.RequireRole("admin")(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("GET", "/admin", nil)
	req = withTestUser(req, "admin")

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if !called {
		t.Error("expected handler to be called")
	}
	if rec.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d", http.StatusOK, rec.Code)
	}
}

func TestRequireRole_MultipleRoles(t *testing.T) {
	sm := newTestSessionManager(t)

	handler := sm.RequireRole("admin", "analyst")(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	tests := []struct {
		role     string
		expected int
	}{
		{"admin", http.StatusOK},
		{"analyst", http.StatusOK},
		{"member", http.StatusSeeOther}, // redirect to forbidden
		{"leader", http.StatusSeeOther}, // redirect to forbidden
	}

	for _, tc := range tests {
		t.Run(tc.role, func(t *testing.T) {
			req := httptest.NewRequest("GET", "/reports", nil)
			req.Header.Set("Accept", "text/html")
			req = withTestUser(req, tc.role)

			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)

			if rec.Code != tc.expected {
				t.Errorf("role %q: expected status %d, got %d", tc.role, tc.expected, rec.Code)
			}
		})
	}
}

func TestRequireRole_CaseInsensitive(t *testing.T) {
	sm := newTestSessionManager(t)

	handler := sm.RequireRole("admin")(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	// Test with uppercase role
	req := httptest.NewRequest("GET", "/admin", nil)
	req = withTestUser(req, "ADMIN")

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected status %d for uppercase role, got %d", http.StatusOK, rec.Code)
	}
}

func TestCurrentUser_NoUser(t *testing.T) {
	req := httptest.NewRequest("GET", "/", nil)

	user, ok := auth.CurrentUser(req)

	if ok {
		t.Error("expected ok to be false when no user in context")
	}
	if user != nil {
		t.Error("expected user to be nil when no user in context")
	}
}

func TestCurrentUser_WithUser(t *testing.T) {
	req := httptest.NewRequest("GET", "/", nil)
	req = withTestUser(req, "admin")

	user, ok := auth.CurrentUser(req)

	if !ok {
		t.Error("expected ok to be true when user in context")
	}
	if user == nil {
		t.Fatal("expected user to not be nil")
	}
	if user.Role != "admin" {
		t.Errorf("expected role 'admin', got %q", user.Role)
	}
}

// withTestUser injects a SessionUser into the request context for testing.
// This simulates what LoadSessionUser middleware does.
func withTestUser(r *http.Request, role string) *http.Request {
	user := &auth.SessionUser{
		ID:      "507f1f77bcf86cd799439011", // Valid ObjectID hex
		Name:    "Test User",
		LoginID: "test@example.com",
		Role:    role,
	}
	return auth.WithTestUser(r, user)
}
