package logout_test

// Terminology: User Identifiers
//   - UserID / userID / user_id: The MongoDB ObjectID (_id) that uniquely identifies a user record
//   - LoginID / loginID / login_id: The human-readable string users type to log in

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/dalemusser/stratahub/internal/app/features/logout"
	"github.com/dalemusser/stratahub/internal/app/system/auth"
	"go.uber.org/zap"
)

func newTestHandler(t *testing.T) *logout.Handler {
	t.Helper()
	logger := zap.NewNop()

	// Create a session manager for testing
	sessionMgr, err := auth.NewSessionManager("test-session-key-for-testing-only", "test-session", "", 24*time.Hour, false, logger)
	if err != nil {
		t.Fatalf("NewSessionManager failed: %v", err)
	}

	// Pass nil for audit logger and sessions store in tests (handler has nil checks)
	return logout.NewHandler(sessionMgr, nil, nil, logger)
}

func TestServeLogout_RedirectsToHome(t *testing.T) {
	handler := newTestHandler(t)

	req := httptest.NewRequest("GET", "/logout", nil)
	rec := httptest.NewRecorder()

	handler.ServeLogout(rec, req)

	if rec.Code != http.StatusSeeOther {
		t.Errorf("expected status %d, got %d", http.StatusSeeOther, rec.Code)
	}

	location := rec.Header().Get("Location")
	if location != "/" {
		t.Errorf("Location: got %q, want %q", location, "/")
	}
}

func TestServeLogout_ClearsSessionCookie(t *testing.T) {
	handler := newTestHandler(t)

	req := httptest.NewRequest("GET", "/logout", nil)
	rec := httptest.NewRecorder()

	handler.ServeLogout(rec, req)

	// Check that the session cookie is being deleted (MaxAge = -1)
	cookies := rec.Result().Cookies()
	found := false
	for _, c := range cookies {
		if c.Name == "test-session" {
			found = true
			if c.MaxAge != -1 {
				t.Errorf("cookie MaxAge: got %d, want -1 (delete)", c.MaxAge)
			}
			break
		}
	}
	if !found {
		t.Error("expected session cookie to be set for deletion")
	}
}

func TestServeLogout_HTMX_ReturnsHXRedirect(t *testing.T) {
	handler := newTestHandler(t)

	req := httptest.NewRequest("GET", "/logout", nil)
	req.Header.Set("HX-Request", "true")
	rec := httptest.NewRecorder()

	handler.ServeLogout(rec, req)

	// HTMX should get HX-Redirect header
	hxRedirect := rec.Header().Get("HX-Redirect")
	if hxRedirect != "/" {
		t.Errorf("HX-Redirect: got %q, want %q", hxRedirect, "/")
	}

	// Status should be 200 for HTMX
	if rec.Code != http.StatusOK {
		t.Errorf("expected status %d for HTMX, got %d", http.StatusOK, rec.Code)
	}
}

func TestServeLogout_WithExistingSession(t *testing.T) {
	logger := zap.NewNop()
	sessionMgr, err := auth.NewSessionManager("test-session-key-for-testing-only", "test-session", "", 24*time.Hour, false, logger)
	if err != nil {
		t.Fatalf("NewSessionManager failed: %v", err)
	}
	handler := logout.NewHandler(sessionMgr, nil, nil, logger)

	// First, simulate having a session by making a request and setting session values
	req1 := httptest.NewRequest("GET", "/setup", nil)
	rec1 := httptest.NewRecorder()

	session, _ := sessionMgr.GetSession(req1)
	session.Values["is_authenticated"] = true
	session.Values["user_id"] = "test-user-id"
	_ = session.Save(req1, rec1)

	// Get the cookie from the first response
	cookies := rec1.Result().Cookies()

	// Now make the logout request with the session cookie
	req2 := httptest.NewRequest("GET", "/logout", nil)
	for _, c := range cookies {
		req2.AddCookie(c)
	}
	rec2 := httptest.NewRecorder()

	handler.ServeLogout(rec2, req2)

	if rec2.Code != http.StatusSeeOther {
		t.Errorf("expected status %d, got %d", http.StatusSeeOther, rec2.Code)
	}

	// Verify the session cookie is being deleted
	logoutCookies := rec2.Result().Cookies()
	for _, c := range logoutCookies {
		if c.Name == "test-session" {
			if c.MaxAge != -1 {
				t.Errorf("cookie MaxAge after logout: got %d, want -1", c.MaxAge)
			}
		}
	}
}
