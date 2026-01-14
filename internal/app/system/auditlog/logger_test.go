package auditlog_test

import (
	"net/http/httptest"
	"testing"

	"github.com/dalemusser/stratahub/internal/app/store/audit"
	"github.com/dalemusser/stratahub/internal/app/system/auditlog"
	"github.com/dalemusser/stratahub/internal/testutil"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.uber.org/zap"
)

func TestLogger_NilLogger(t *testing.T) {
	// nil logger should be a no-op (not panic)
	var logger *auditlog.Logger
	ctx, cancel := testutil.TestContext()
	defer cancel()
	req := httptest.NewRequest("GET", "/", nil)

	// These should all be no-ops, not panic
	logger.Log(ctx, audit.Event{EventType: "test"})
	logger.LoginSuccess(ctx, req, primitive.NewObjectID(), nil, "password", "test")
	logger.Logout(ctx, req, primitive.NewObjectID().Hex(), "")
}

func TestLogger_Log_ConfigOff(t *testing.T) {
	db := testutil.SetupTestDB(t)
	store := audit.New(db)
	zapLog := zap.NewNop()
	ctx, cancel := testutil.TestContext()
	defer cancel()

	logger := auditlog.New(store, zapLog, auditlog.Config{
		Auth:  "off",
		Admin: "off",
	})

	// Log an auth event
	logger.Log(ctx, audit.Event{
		Category:  audit.CategoryAuth,
		EventType: audit.EventLoginSuccess,
		Success:   true,
	})

	// Verify nothing was logged to DB
	events, err := store.GetByUser(ctx, primitive.NewObjectID(), 10)
	if err != nil {
		t.Fatalf("GetByUser failed: %v", err)
	}
	if len(events) != 0 {
		t.Error("expected no events when config is 'off'")
	}
}

func TestLogger_Log_ConfigDB(t *testing.T) {
	db := testutil.SetupTestDB(t)
	store := audit.New(db)
	zapLog := zap.NewNop()
	ctx, cancel := testutil.TestContext()
	defer cancel()

	userID := primitive.NewObjectID()
	logger := auditlog.New(store, zapLog, auditlog.Config{
		Auth:  "db",
		Admin: "db",
	})

	// Log an auth event
	logger.Log(ctx, audit.Event{
		Category:  audit.CategoryAuth,
		EventType: audit.EventLoginSuccess,
		UserID:    &userID,
		Success:   true,
	})

	// Verify event was logged to DB
	events, err := store.GetByUser(ctx, userID, 10)
	if err != nil {
		t.Fatalf("GetByUser failed: %v", err)
	}
	if len(events) != 1 {
		t.Errorf("expected 1 event, got %d", len(events))
	}
}

func TestLogger_Log_ConfigAll(t *testing.T) {
	db := testutil.SetupTestDB(t)
	store := audit.New(db)
	zapLog := zap.NewNop()
	ctx, cancel := testutil.TestContext()
	defer cancel()

	userID := primitive.NewObjectID()
	logger := auditlog.New(store, zapLog, auditlog.Config{
		Auth:  "all",
		Admin: "all",
	})

	// Log an auth event
	logger.Log(ctx, audit.Event{
		Category:  audit.CategoryAuth,
		EventType: audit.EventLoginSuccess,
		UserID:    &userID,
		Success:   true,
	})

	// Verify event was logged to DB
	events, err := store.GetByUser(ctx, userID, 10)
	if err != nil {
		t.Fatalf("GetByUser failed: %v", err)
	}
	if len(events) != 1 {
		t.Errorf("expected 1 event, got %d", len(events))
	}
}

func TestLogger_LoginSuccess(t *testing.T) {
	db := testutil.SetupTestDB(t)
	store := audit.New(db)
	zapLog := zap.NewNop()
	ctx, cancel := testutil.TestContext()
	defer cancel()

	userID := primitive.NewObjectID()
	orgID := primitive.NewObjectID()
	logger := auditlog.New(store, zapLog, auditlog.Config{
		Auth: "db",
	})

	req := httptest.NewRequest("GET", "/", nil)
	req.RemoteAddr = "192.168.1.1:12345"
	req.Header.Set("User-Agent", "TestBrowser/1.0")

	logger.LoginSuccess(ctx, req, userID, &orgID, "password", "testuser")

	// Verify event was logged
	events, err := store.GetByUser(ctx, userID, 10)
	if err != nil {
		t.Fatalf("GetByUser failed: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}

	event := events[0]
	if event.EventType != audit.EventLoginSuccess {
		t.Errorf("EventType: got %q, want %q", event.EventType, audit.EventLoginSuccess)
	}
	if !event.Success {
		t.Error("expected Success to be true")
	}
}

func TestLogger_LoginFailedUserNotFound(t *testing.T) {
	db := testutil.SetupTestDB(t)
	store := audit.New(db)
	zapLog := zap.NewNop()
	ctx, cancel := testutil.TestContext()
	defer cancel()

	logger := auditlog.New(store, zapLog, auditlog.Config{
		Auth: "db",
	})

	req := httptest.NewRequest("GET", "/", nil)
	logger.LoginFailedUserNotFound(ctx, req, "unknown@example.com")

	// Verify event was logged - use GetRecent since no user ID
	events, err := store.GetRecent(ctx, 10)
	if err != nil {
		t.Fatalf("GetRecent failed: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}

	event := events[0]
	if event.EventType != audit.EventLoginFailedUserNotFound {
		t.Errorf("EventType: got %q, want %q", event.EventType, audit.EventLoginFailedUserNotFound)
	}
	if event.Success {
		t.Error("expected Success to be false")
	}
	if event.FailureReason != "user not found" {
		t.Errorf("FailureReason: got %q, want %q", event.FailureReason, "user not found")
	}
}

func TestLogger_Logout(t *testing.T) {
	db := testutil.SetupTestDB(t)
	store := audit.New(db)
	zapLog := zap.NewNop()
	ctx, cancel := testutil.TestContext()
	defer cancel()

	userID := primitive.NewObjectID()
	orgID := primitive.NewObjectID()
	logger := auditlog.New(store, zapLog, auditlog.Config{
		Auth: "db",
	})

	req := httptest.NewRequest("GET", "/", nil)
	logger.Logout(ctx, req, userID.Hex(), orgID.Hex())

	// Verify event was logged
	events, err := store.GetByUser(ctx, userID, 10)
	if err != nil {
		t.Fatalf("GetByUser failed: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}

	if events[0].EventType != audit.EventLogout {
		t.Errorf("EventType: got %q, want %q", events[0].EventType, audit.EventLogout)
	}
}

func TestLogger_Logout_InvalidIDs(t *testing.T) {
	db := testutil.SetupTestDB(t)
	store := audit.New(db)
	zapLog := zap.NewNop()
	ctx, cancel := testutil.TestContext()
	defer cancel()

	logger := auditlog.New(store, zapLog, auditlog.Config{
		Auth: "db",
	})

	req := httptest.NewRequest("GET", "/", nil)
	// Should not panic with invalid hex IDs
	logger.Logout(ctx, req, "invalid-hex", "also-invalid")
}

func TestLogger_UserCreated(t *testing.T) {
	db := testutil.SetupTestDB(t)
	store := audit.New(db)
	zapLog := zap.NewNop()
	ctx, cancel := testutil.TestContext()
	defer cancel()

	actorID := primitive.NewObjectID()
	targetUserID := primitive.NewObjectID()
	orgID := primitive.NewObjectID()
	logger := auditlog.New(store, zapLog, auditlog.Config{
		Admin: "db",
	})

	req := httptest.NewRequest("GET", "/", nil)
	logger.UserCreated(ctx, req, actorID, targetUserID, &orgID, "admin", "member", "password")

	// Verify event was logged
	events, err := store.GetByUser(ctx, targetUserID, 10)
	if err != nil {
		t.Fatalf("GetByUser failed: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}

	event := events[0]
	if event.EventType != audit.EventUserCreated {
		t.Errorf("EventType: got %q, want %q", event.EventType, audit.EventUserCreated)
	}
	if event.ActorID == nil || *event.ActorID != actorID {
		t.Error("expected ActorID to be set")
	}
}

func TestLogger_AuthCategoryFilteredByConfig(t *testing.T) {
	db := testutil.SetupTestDB(t)
	store := audit.New(db)
	zapLog := zap.NewNop()
	ctx, cancel := testutil.TestContext()
	defer cancel()

	userID := primitive.NewObjectID()
	// Auth = off, Admin = db
	logger := auditlog.New(store, zapLog, auditlog.Config{
		Auth:  "off",
		Admin: "db",
	})

	req := httptest.NewRequest("GET", "/", nil)

	// Auth event should be skipped
	logger.LoginSuccess(ctx, req, userID, nil, "password", "test")

	// Admin event should be logged
	targetUser := primitive.NewObjectID()
	logger.UserCreated(ctx, req, userID, targetUser, nil, "admin", "member", "password")

	// Verify only admin event was logged
	authEvents, _ := store.GetByUser(ctx, userID, 10)
	if len(authEvents) != 0 {
		t.Error("expected no auth events when auth config is 'off'")
	}

	adminEvents, _ := store.GetByUser(ctx, targetUser, 10)
	if len(adminEvents) != 1 {
		t.Errorf("expected 1 admin event, got %d", len(adminEvents))
	}
}

func TestGetClientIP_XForwardedFor(t *testing.T) {
	db := testutil.SetupTestDB(t)
	store := audit.New(db)
	zapLog := zap.NewNop()
	ctx, cancel := testutil.TestContext()
	defer cancel()

	userID := primitive.NewObjectID()
	logger := auditlog.New(store, zapLog, auditlog.Config{
		Auth: "db",
	})

	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("X-Forwarded-For", "203.0.113.195")
	req.Header.Set("X-Real-IP", "192.168.1.1")
	req.RemoteAddr = "127.0.0.1:12345"

	logger.LoginSuccess(ctx, req, userID, nil, "password", "test")

	events, _ := store.GetByUser(ctx, userID, 10)
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}

	// X-Forwarded-For should take precedence
	if events[0].IP != "203.0.113.195" {
		t.Errorf("IP: got %q, want %q", events[0].IP, "203.0.113.195")
	}
}

func TestGetClientIP_XRealIP(t *testing.T) {
	db := testutil.SetupTestDB(t)
	store := audit.New(db)
	zapLog := zap.NewNop()
	ctx, cancel := testutil.TestContext()
	defer cancel()

	userID := primitive.NewObjectID()
	logger := auditlog.New(store, zapLog, auditlog.Config{
		Auth: "db",
	})

	req := httptest.NewRequest("GET", "/", nil)
	// No X-Forwarded-For
	req.Header.Set("X-Real-IP", "192.168.1.100")
	req.RemoteAddr = "127.0.0.1:12345"

	logger.LoginSuccess(ctx, req, userID, nil, "password", "test")

	events, _ := store.GetByUser(ctx, userID, 10)
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}

	// X-Real-IP should be used when no X-Forwarded-For
	if events[0].IP != "192.168.1.100" {
		t.Errorf("IP: got %q, want %q", events[0].IP, "192.168.1.100")
	}
}

func TestGetClientIP_RemoteAddr(t *testing.T) {
	db := testutil.SetupTestDB(t)
	store := audit.New(db)
	zapLog := zap.NewNop()
	ctx, cancel := testutil.TestContext()
	defer cancel()

	userID := primitive.NewObjectID()
	logger := auditlog.New(store, zapLog, auditlog.Config{
		Auth: "db",
	})

	req := httptest.NewRequest("GET", "/", nil)
	// No proxy headers
	req.RemoteAddr = "10.0.0.5:12345"

	logger.LoginSuccess(ctx, req, userID, nil, "password", "test")

	events, _ := store.GetByUser(ctx, userID, 10)
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}

	// Should fall back to RemoteAddr (port stripped)
	if events[0].IP != "10.0.0.5" {
		t.Errorf("IP: got %q, want %q", events[0].IP, "10.0.0.5")
	}
}

func TestConfig_Defaults(t *testing.T) {
	// Test that default config values work
	config := auditlog.Config{}
	if config.Auth != "" {
		t.Errorf("expected empty default Auth, got %q", config.Auth)
	}
	if config.Admin != "" {
		t.Errorf("expected empty default Admin, got %q", config.Admin)
	}
}
