package login_test

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	uierrors "github.com/dalemusser/stratahub/internal/app/features/errors"
	"github.com/dalemusser/stratahub/internal/app/features/login"
	"github.com/dalemusser/stratahub/internal/app/system/auth"
	"github.com/dalemusser/stratahub/internal/testutil"
	"go.uber.org/zap"
)

func newTestHandler(t *testing.T) (*login.Handler, *testutil.Fixtures) {
	t.Helper()
	db := testutil.SetupTestDB(t)
	logger := zap.NewNop()
	errLog := uierrors.NewErrorLogger(logger)

	// Create a session manager for testing (dev mode, weak key allowed)
	sessionMgr, err := auth.NewSessionManager("test-session-key-for-testing-only", "test-session", "", false, logger)
	if err != nil {
		t.Fatalf("NewSessionManager failed: %v", err)
	}

	handler := login.NewHandler(db, sessionMgr, errLog, logger)
	fixtures := testutil.NewFixtures(t, db)
	return handler, fixtures
}

func TestHandleLoginPost_Success(t *testing.T) {
	handler, fixtures := newTestHandler(t)
	ctx, cancel := testutil.TestContext()
	defer cancel()

	// Create a user to log in as
	fixtures.CreateAdmin(ctx, "Test Admin", "admin@example.com")

	form := url.Values{
		"email": {"admin@example.com"},
	}

	req := httptest.NewRequest("POST", "/login", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	rec := httptest.NewRecorder()
	handler.HandleLoginPost(rec, req)

	// Should redirect to dashboard
	if rec.Code != http.StatusSeeOther {
		t.Errorf("expected status %d, got %d", http.StatusSeeOther, rec.Code)
	}

	location := rec.Header().Get("Location")
	if location != "/dashboard" {
		t.Errorf("Location: got %q, want %q", location, "/dashboard")
	}

	// Should have set a session cookie
	cookies := rec.Result().Cookies()
	found := false
	for _, c := range cookies {
		if c.Name == "test-session" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected session cookie to be set")
	}
}

func TestHandleLoginPost_WithReturnURL(t *testing.T) {
	handler, fixtures := newTestHandler(t)
	ctx, cancel := testutil.TestContext()
	defer cancel()

	fixtures.CreateAdmin(ctx, "Test Admin", "admin@example.com")

	form := url.Values{
		"email":  {"admin@example.com"},
		"return": {"/organizations"},
	}

	req := httptest.NewRequest("POST", "/login", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	rec := httptest.NewRecorder()
	handler.HandleLoginPost(rec, req)

	if rec.Code != http.StatusSeeOther {
		t.Errorf("expected status %d, got %d", http.StatusSeeOther, rec.Code)
	}

	location := rec.Header().Get("Location")
	if location != "/organizations" {
		t.Errorf("Location: got %q, want %q", location, "/organizations")
	}
}

func TestHandleLoginPost_NonexistentEmail(t *testing.T) {
	handler, _ := newTestHandler(t)

	form := url.Values{
		"email": {"nobody@example.com"},
	}

	req := httptest.NewRequest("POST", "/login", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	rec := httptest.NewRecorder()

	// Handler will try to render a template which will panic without initialized templates
	func() {
		defer func() { recover() }()
		handler.HandleLoginPost(rec, req)
	}()

	// No session cookie should be set on failed login
	cookies := rec.Result().Cookies()
	for _, c := range cookies {
		if c.Name == "test-session" && c.Value != "" {
			t.Error("session cookie should not be set for nonexistent user")
		}
	}
}

func TestHandleLoginPost_EmptyEmail(t *testing.T) {
	handler, _ := newTestHandler(t)

	form := url.Values{
		"email": {""},
	}

	req := httptest.NewRequest("POST", "/login", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	rec := httptest.NewRecorder()

	// Handler will try to render a template
	func() {
		defer func() { recover() }()
		handler.HandleLoginPost(rec, req)
	}()

	// No session cookie should be set
	cookies := rec.Result().Cookies()
	for _, c := range cookies {
		if c.Name == "test-session" && c.Value != "" {
			t.Error("session cookie should not be set for empty email")
		}
	}
}

func TestHandleLoginPost_DisabledUser(t *testing.T) {
	handler, fixtures := newTestHandler(t)
	ctx, cancel := testutil.TestContext()
	defer cancel()

	// Create a disabled user
	fixtures.CreateDisabledUser(ctx, "Disabled User", "disabled@example.com")

	form := url.Values{
		"email": {"disabled@example.com"},
	}

	req := httptest.NewRequest("POST", "/login", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	rec := httptest.NewRecorder()

	// Handler will try to render a template with error
	func() {
		defer func() { recover() }()
		handler.HandleLoginPost(rec, req)
	}()

	// No session cookie should be set for disabled user
	cookies := rec.Result().Cookies()
	for _, c := range cookies {
		if c.Name == "test-session" && c.Value != "" {
			t.Error("session cookie should not be set for disabled user")
		}
	}
}

func TestHandleLoginPost_CaseInsensitiveEmail(t *testing.T) {
	handler, fixtures := newTestHandler(t)
	ctx, cancel := testutil.TestContext()
	defer cancel()

	// Email stored as lowercase
	fixtures.CreateAdmin(ctx, "Test Admin", "admin@example.com")

	form := url.Values{
		"email": {"ADMIN@EXAMPLE.COM"}, // uppercase
	}

	req := httptest.NewRequest("POST", "/login", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	rec := httptest.NewRecorder()
	handler.HandleLoginPost(rec, req)

	// Should succeed with case-insensitive match
	if rec.Code != http.StatusSeeOther {
		t.Errorf("expected status %d, got %d (case-insensitive email should work)", http.StatusSeeOther, rec.Code)
	}
}

func TestHandleLoginPost_EmailWithWhitespace(t *testing.T) {
	handler, fixtures := newTestHandler(t)
	ctx, cancel := testutil.TestContext()
	defer cancel()

	fixtures.CreateAdmin(ctx, "Test Admin", "admin@example.com")

	form := url.Values{
		"email": {"  admin@example.com  "}, // whitespace around email
	}

	req := httptest.NewRequest("POST", "/login", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	rec := httptest.NewRecorder()
	handler.HandleLoginPost(rec, req)

	// Should succeed after trimming whitespace
	if rec.Code != http.StatusSeeOther {
		t.Errorf("expected status %d, got %d (whitespace should be trimmed)", http.StatusSeeOther, rec.Code)
	}
}
