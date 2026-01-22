package heartbeat_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/dalemusser/stratahub/internal/app/features/heartbeat"
	activitystore "github.com/dalemusser/stratahub/internal/app/store/activity"
	"github.com/dalemusser/stratahub/internal/app/store/sessions"
	"github.com/dalemusser/stratahub/internal/app/system/auth"
	"github.com/dalemusser/stratahub/internal/testutil"
	"go.uber.org/zap"
)

func newTestHandler(t *testing.T) *heartbeat.Handler {
	t.Helper()
	db := testutil.SetupTestDB(t)
	logger := zap.NewNop()

	sessionsStore := sessions.New(db)
	activityStore := activitystore.New(db)

	sessionMgr, err := auth.NewSessionManager("test-session-key-for-testing-only", "test-session", "", 24*time.Hour, false, logger)
	if err != nil {
		t.Fatalf("NewSessionManager failed: %v", err)
	}

	return heartbeat.NewHandler(sessionsStore, activityStore, sessionMgr, logger)
}

func TestNewHandler(t *testing.T) {
	h := newTestHandler(t)
	if h == nil {
		t.Fatal("NewHandler() returned nil")
	}
}

func TestServeHeartbeat_Unauthenticated(t *testing.T) {
	handler := newTestHandler(t)

	req := httptest.NewRequest("POST", "/api/heartbeat", nil)
	rec := httptest.NewRecorder()

	handler.ServeHeartbeat(rec, req)

	// Unauthenticated heartbeats should return OK (silent fail)
	if rec.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d", http.StatusOK, rec.Code)
	}
}

func TestServeHeartbeat_WithBody(t *testing.T) {
	handler := newTestHandler(t)

	body := `{"page":"/dashboard","had_user_activity":true}`
	req := httptest.NewRequest("POST", "/api/heartbeat", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	handler.ServeHeartbeat(rec, req)

	// Should return OK
	if rec.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d", http.StatusOK, rec.Code)
	}
}

func TestSetIdleLogoutConfig(t *testing.T) {
	handler := newTestHandler(t)

	handler.SetIdleLogoutConfig(true, 30*time.Minute, 5*time.Minute)

	if !handler.IdleLogoutEnabled {
		t.Error("IdleLogoutEnabled should be true")
	}
	if handler.IdleLogoutTimeout != 30*time.Minute {
		t.Errorf("IdleLogoutTimeout = %v, want %v", handler.IdleLogoutTimeout, 30*time.Minute)
	}
	if handler.IdleLogoutWarning != 5*time.Minute {
		t.Errorf("IdleLogoutWarning = %v, want %v", handler.IdleLogoutWarning, 5*time.Minute)
	}
}

func TestServeHeartbeat_InvalidJSON(t *testing.T) {
	handler := newTestHandler(t)

	body := `{invalid json}`
	req := httptest.NewRequest("POST", "/api/heartbeat", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	handler.ServeHeartbeat(rec, req)

	// Should still return OK (graceful handling of bad JSON)
	if rec.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d", http.StatusOK, rec.Code)
	}
}

func TestServeHeartbeat_EmptyBody(t *testing.T) {
	handler := newTestHandler(t)

	req := httptest.NewRequest("POST", "/api/heartbeat", nil)
	rec := httptest.NewRecorder()

	handler.ServeHeartbeat(rec, req)

	// Should return OK
	if rec.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d", http.StatusOK, rec.Code)
	}
}

func TestRoutes(t *testing.T) {
	handler := newTestHandler(t)
	logger := zap.NewNop()

	sessionMgr, err := auth.NewSessionManager("test-session-key-for-testing-only", "test-session", "", 24*time.Hour, false, logger)
	if err != nil {
		t.Fatalf("NewSessionManager failed: %v", err)
	}

	router := heartbeat.Routes(handler, sessionMgr)
	if router == nil {
		t.Fatal("Routes() returned nil")
	}
}

func TestHeartbeatResponse_JSONStructure(t *testing.T) {
	// Test that the heartbeat response structure is valid
	resp := struct {
		IdleWarning      bool `json:"idle_warning,omitempty"`
		SecondsRemaining int  `json:"seconds_remaining,omitempty"`
	}{
		IdleWarning:      true,
		SecondsRemaining: 300,
	}

	data, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("failed to marshal heartbeat response: %v", err)
	}

	// Verify JSON contains expected fields
	if !strings.Contains(string(data), "idle_warning") {
		t.Error("expected idle_warning in JSON")
	}
	if !strings.Contains(string(data), "seconds_remaining") {
		t.Error("expected seconds_remaining in JSON")
	}
}
