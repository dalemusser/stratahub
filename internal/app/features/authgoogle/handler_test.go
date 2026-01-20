package authgoogle_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/dalemusser/stratahub/internal/app/features/authgoogle"
	uierrors "github.com/dalemusser/stratahub/internal/app/features/errors"
	"github.com/dalemusser/stratahub/internal/app/store/oauthstate"
	"github.com/dalemusser/stratahub/internal/app/store/sessions"
	"github.com/dalemusser/stratahub/internal/app/store/workspaces"
	"github.com/dalemusser/stratahub/internal/app/system/auth"
	"github.com/dalemusser/stratahub/internal/app/system/auditlog"
	"github.com/dalemusser/stratahub/internal/testutil"
	"go.uber.org/zap"
)

func newTestHandler(t *testing.T) *authgoogle.Handler {
	t.Helper()
	db := testutil.SetupTestDB(t)
	logger := zap.NewNop()
	errLog := uierrors.NewErrorLogger(logger)

	sessionMgr, err := auth.NewSessionManager("test-session-key-for-testing-only", "test-session", "", false, logger)
	if err != nil {
		t.Fatalf("NewSessionManager failed: %v", err)
	}

	sessStore := sessions.New(db)
	stateStore := oauthstate.New(db)
	wsStore := workspaces.New(db)
	audit := auditlog.NewNopLogger()

	return authgoogle.NewHandler(
		db,
		sessionMgr,
		errLog,
		audit,
		sessStore,
		stateStore,
		wsStore,
		"test-client-id",
		"test-client-secret",
		"http://localhost:8080",
		false, // multiWorkspace
		"",    // primaryDomain
		logger,
	)
}

func TestNewHandler(t *testing.T) {
	h := newTestHandler(t)
	if h == nil {
		t.Fatal("NewHandler() returned nil")
	}
}

func TestIsConfigured_Configured(t *testing.T) {
	h := newTestHandler(t)
	if !h.IsConfigured() {
		t.Error("IsConfigured() should return true with client ID and secret")
	}
}

func TestIsConfigured_NotConfigured(t *testing.T) {
	db := testutil.SetupTestDB(t)
	logger := zap.NewNop()
	errLog := uierrors.NewErrorLogger(logger)

	sessionMgr, err := auth.NewSessionManager("test-session-key-for-testing-only", "test-session", "", false, logger)
	if err != nil {
		t.Fatalf("NewSessionManager failed: %v", err)
	}

	sessStore := sessions.New(db)
	stateStore := oauthstate.New(db)
	wsStore := workspaces.New(db)
	audit := auditlog.NewNopLogger()

	h := authgoogle.NewHandler(
		db,
		sessionMgr,
		errLog,
		audit,
		sessStore,
		stateStore,
		wsStore,
		"",                      // empty client ID
		"",                      // empty client secret
		"http://localhost:8080",
		false,
		"",
		logger,
	)

	if h.IsConfigured() {
		t.Error("IsConfigured() should return false without client ID and secret")
	}
}

func TestServeLogin_NotConfigured(t *testing.T) {
	db := testutil.SetupTestDB(t)
	logger := zap.NewNop()
	errLog := uierrors.NewErrorLogger(logger)

	sessionMgr, err := auth.NewSessionManager("test-session-key-for-testing-only", "test-session", "", false, logger)
	if err != nil {
		t.Fatalf("NewSessionManager failed: %v", err)
	}

	sessStore := sessions.New(db)
	stateStore := oauthstate.New(db)
	wsStore := workspaces.New(db)
	audit := auditlog.NewNopLogger()

	h := authgoogle.NewHandler(
		db,
		sessionMgr,
		errLog,
		audit,
		sessStore,
		stateStore,
		wsStore,
		"", // empty client ID
		"", // empty client secret
		"http://localhost:8080",
		false,
		"",
		logger,
	)

	req := httptest.NewRequest("GET", "/auth/google", nil)
	rec := httptest.NewRecorder()

	h.ServeLogin(rec, req)

	// Should redirect to login with error
	if rec.Code != http.StatusSeeOther {
		t.Errorf("expected status %d, got %d", http.StatusSeeOther, rec.Code)
	}

	location := rec.Header().Get("Location")
	if !strings.Contains(location, "google_not_configured") {
		t.Errorf("Location = %q, want to contain 'google_not_configured'", location)
	}
}

func TestServeLogin_RedirectsToGoogle(t *testing.T) {
	handler := newTestHandler(t)

	req := httptest.NewRequest("GET", "/auth/google", nil)
	rec := httptest.NewRecorder()

	handler.ServeLogin(rec, req)

	// Should redirect to Google
	if rec.Code != http.StatusTemporaryRedirect {
		t.Errorf("expected status %d, got %d", http.StatusTemporaryRedirect, rec.Code)
	}

	location := rec.Header().Get("Location")
	if !strings.Contains(location, "accounts.google.com") {
		t.Errorf("Location = %q, want to contain 'accounts.google.com'", location)
	}
}

func TestServeCallback_MissingState(t *testing.T) {
	handler := newTestHandler(t)

	req := httptest.NewRequest("GET", "/auth/google/callback?code=test-code", nil)
	rec := httptest.NewRecorder()

	handler.ServeCallback(rec, req)

	// Should redirect with invalid_state error
	if rec.Code != http.StatusSeeOther {
		t.Errorf("expected status %d, got %d", http.StatusSeeOther, rec.Code)
	}

	location := rec.Header().Get("Location")
	if !strings.Contains(location, "invalid_state") {
		t.Errorf("Location = %q, want to contain 'invalid_state'", location)
	}
}

func TestServeCallback_GoogleError(t *testing.T) {
	handler := newTestHandler(t)

	req := httptest.NewRequest("GET", "/auth/google/callback?error=access_denied", nil)
	rec := httptest.NewRecorder()

	handler.ServeCallback(rec, req)

	// Should redirect with google_denied error
	if rec.Code != http.StatusSeeOther {
		t.Errorf("expected status %d, got %d", http.StatusSeeOther, rec.Code)
	}

	location := rec.Header().Get("Location")
	if !strings.Contains(location, "google_denied") {
		t.Errorf("Location = %q, want to contain 'google_denied'", location)
	}
}

func TestServeCallback_InvalidState(t *testing.T) {
	handler := newTestHandler(t)

	req := httptest.NewRequest("GET", "/auth/google/callback?state=invalid-state&code=test-code", nil)
	rec := httptest.NewRecorder()

	handler.ServeCallback(rec, req)

	// Should redirect with invalid_state error
	if rec.Code != http.StatusSeeOther {
		t.Errorf("expected status %d, got %d", http.StatusSeeOther, rec.Code)
	}

	location := rec.Header().Get("Location")
	if !strings.Contains(location, "invalid_state") {
		t.Errorf("Location = %q, want to contain 'invalid_state'", location)
	}
}

func TestRoutes(t *testing.T) {
	handler := newTestHandler(t)

	router := authgoogle.Routes(handler)
	if router == nil {
		t.Fatal("Routes() returned nil")
	}
}
