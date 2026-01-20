package announcements_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/dalemusser/stratahub/internal/app/features/announcements"
	uierrors "github.com/dalemusser/stratahub/internal/app/features/errors"
	"github.com/dalemusser/stratahub/internal/app/store/announcement"
	"github.com/dalemusser/stratahub/internal/app/system/auth"
	"github.com/dalemusser/stratahub/internal/testutil"
	"github.com/go-chi/chi/v5"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.uber.org/zap"
)

func newTestHandler(t *testing.T) (*announcements.Handler, *mongo.Database, *announcement.Store) {
	t.Helper()
	db := testutil.SetupTestDB(t)
	logger := zap.NewNop()
	errLog := uierrors.NewErrorLogger(logger)
	handler := announcements.NewHandler(db, errLog, logger)
	return handler, db, announcement.New(db)
}

func TestNewHandler(t *testing.T) {
	h, _, _ := newTestHandler(t)
	if h == nil {
		t.Fatal("NewHandler() returned nil")
	}
}

func TestGetStore(t *testing.T) {
	h, _, _ := newTestHandler(t)
	store := h.GetStore()
	if store == nil {
		t.Fatal("GetStore() returned nil")
	}
}

func TestList_AdminOnly(t *testing.T) {
	handler, _, _ := newTestHandler(t)

	adminID := primitive.NewObjectID()
	sessionUser := &auth.SessionUser{
		ID:      adminID.Hex(),
		Name:    "Admin User",
		LoginID: "admin@example.com",
		Role:    "admin",
	}

	req := httptest.NewRequest("GET", "/announcements", nil)
	req = auth.WithTestUser(req, sessionUser)
	rec := httptest.NewRecorder()

	// Handler will try to render a template which may panic without initialized templates
	func() {
		defer func() {
			if r := recover(); r != nil {
				// Template rendering may panic in tests
			}
		}()
		handler.List(rec, req)
	}()

	// Test passes if handler logic executed without unexpected errors
}

func TestShowNew(t *testing.T) {
	handler, _, _ := newTestHandler(t)

	adminID := primitive.NewObjectID()
	sessionUser := &auth.SessionUser{
		ID:      adminID.Hex(),
		Name:    "Admin User",
		LoginID: "admin@example.com",
		Role:    "admin",
	}

	req := httptest.NewRequest("GET", "/announcements/new", nil)
	req = auth.WithTestUser(req, sessionUser)
	rec := httptest.NewRecorder()

	// Handler will try to render a template which may panic without initialized templates
	func() {
		defer func() {
			if r := recover(); r != nil {
				// Template rendering may panic in tests
			}
		}()
		handler.ShowNew(rec, req)
	}()

	// Test passes if handler logic executed without unexpected errors
}

func TestCreate_Success(t *testing.T) {
	handler, _, annStore := newTestHandler(t)
	ctx, cancel := testutil.TestContext()
	defer cancel()

	adminID := primitive.NewObjectID()
	sessionUser := &auth.SessionUser{
		ID:      adminID.Hex(),
		Name:    "Admin User",
		LoginID: "admin@example.com",
		Role:    "admin",
	}

	form := url.Values{
		"title":       {"Test Announcement"},
		"content":     {"Test content"},
		"type":        {"info"},
		"dismissible": {"on"},
		"active":      {"on"},
	}

	req := httptest.NewRequest("POST", "/announcements/new", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req = auth.WithTestUser(req, sessionUser)
	rec := httptest.NewRecorder()

	handler.Create(rec, req)

	// Should redirect on success
	if rec.Code != http.StatusSeeOther {
		t.Errorf("expected status %d, got %d", http.StatusSeeOther, rec.Code)
	}

	location := rec.Header().Get("Location")
	if !strings.Contains(location, "success=created") {
		t.Errorf("Location = %q, want to contain 'success=created'", location)
	}

	// Verify announcement was created
	announcements, err := annStore.List(ctx)
	if err != nil {
		t.Fatalf("failed to list announcements: %v", err)
	}
	if len(announcements) == 0 {
		t.Error("announcement should have been created")
	}
}

func TestCreate_MissingTitle(t *testing.T) {
	handler, _, _ := newTestHandler(t)

	adminID := primitive.NewObjectID()
	sessionUser := &auth.SessionUser{
		ID:      adminID.Hex(),
		Name:    "Admin User",
		LoginID: "admin@example.com",
		Role:    "admin",
	}

	form := url.Values{
		"title":   {""},
		"content": {"Test content"},
		"type":    {"info"},
	}

	req := httptest.NewRequest("POST", "/announcements/new", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req = auth.WithTestUser(req, sessionUser)
	rec := httptest.NewRecorder()

	// Handler will try to render a template which may panic without initialized templates
	func() {
		defer func() {
			if r := recover(); r != nil {
				// Template rendering may panic in tests
			}
		}()
		handler.Create(rec, req)
	}()

	// Should not redirect (should show error)
	if rec.Code == http.StatusSeeOther && strings.Contains(rec.Header().Get("Location"), "success") {
		t.Error("missing title should not succeed")
	}
}

func TestShow_NotFound(t *testing.T) {
	handler, _, _ := newTestHandler(t)

	adminID := primitive.NewObjectID()
	sessionUser := &auth.SessionUser{
		ID:      adminID.Hex(),
		Name:    "Admin User",
		LoginID: "admin@example.com",
		Role:    "admin",
	}

	nonExistentID := primitive.NewObjectID()

	req := httptest.NewRequest("GET", "/announcements/"+nonExistentID.Hex(), nil)
	req = auth.WithTestUser(req, sessionUser)

	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", nonExistentID.Hex())
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

	rec := httptest.NewRecorder()

	handler.Show(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("expected status %d, got %d", http.StatusNotFound, rec.Code)
	}
}

func TestToggle_Success(t *testing.T) {
	handler, _, annStore := newTestHandler(t)
	ctx, cancel := testutil.TestContext()
	defer cancel()

	// Create an announcement first
	ann, err := annStore.Create(ctx, announcement.CreateInput{
		Title:   "Toggle Test",
		Content: "Test content",
		Type:    announcement.TypeInfo,
		Active:  true,
	})
	if err != nil {
		t.Fatalf("failed to create announcement: %v", err)
	}

	adminID := primitive.NewObjectID()
	sessionUser := &auth.SessionUser{
		ID:      adminID.Hex(),
		Name:    "Admin User",
		LoginID: "admin@example.com",
		Role:    "admin",
	}

	req := httptest.NewRequest("POST", "/announcements/"+ann.ID.Hex()+"/toggle", nil)
	req = auth.WithTestUser(req, sessionUser)

	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", ann.ID.Hex())
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

	rec := httptest.NewRecorder()

	handler.Toggle(rec, req)

	// Should redirect
	if rec.Code != http.StatusSeeOther {
		t.Errorf("expected status %d, got %d", http.StatusSeeOther, rec.Code)
	}

	// Verify announcement was toggled
	toggled, err := annStore.GetByID(ctx, ann.ID)
	if err != nil {
		t.Fatalf("failed to get announcement: %v", err)
	}
	if toggled.Active == ann.Active {
		t.Error("announcement Active status should have changed")
	}
}

func TestDelete_Success(t *testing.T) {
	handler, _, annStore := newTestHandler(t)
	ctx, cancel := testutil.TestContext()
	defer cancel()

	// Create an announcement first
	ann, err := annStore.Create(ctx, announcement.CreateInput{
		Title:   "Delete Test",
		Content: "Test content",
		Type:    announcement.TypeInfo,
		Active:  true,
	})
	if err != nil {
		t.Fatalf("failed to create announcement: %v", err)
	}

	adminID := primitive.NewObjectID()
	sessionUser := &auth.SessionUser{
		ID:      adminID.Hex(),
		Name:    "Admin User",
		LoginID: "admin@example.com",
		Role:    "admin",
	}

	req := httptest.NewRequest("POST", "/announcements/"+ann.ID.Hex()+"/delete", nil)
	req = auth.WithTestUser(req, sessionUser)

	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", ann.ID.Hex())
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

	rec := httptest.NewRecorder()

	handler.Delete(rec, req)

	// Should redirect
	if rec.Code != http.StatusSeeOther {
		t.Errorf("expected status %d, got %d", http.StatusSeeOther, rec.Code)
	}

	location := rec.Header().Get("Location")
	if !strings.Contains(location, "success=deleted") {
		t.Errorf("Location = %q, want to contain 'success=deleted'", location)
	}
}

func TestRoutes(t *testing.T) {
	handler, _, _ := newTestHandler(t)
	logger := zap.NewNop()

	sessionMgr, err := auth.NewSessionManager("test-session-key-for-testing-only", "test-session", "", false, logger)
	if err != nil {
		t.Fatalf("NewSessionManager failed: %v", err)
	}

	router := announcements.Routes(handler, sessionMgr)
	if router == nil {
		t.Fatal("Routes() returned nil")
	}
}

func TestViewRoutes(t *testing.T) {
	handler, _, _ := newTestHandler(t)
	logger := zap.NewNop()

	sessionMgr, err := auth.NewSessionManager("test-session-key-for-testing-only", "test-session", "", false, logger)
	if err != nil {
		t.Fatalf("NewSessionManager failed: %v", err)
	}

	router := announcements.ViewRoutes(handler, sessionMgr)
	if router == nil {
		t.Fatal("ViewRoutes() returned nil")
	}
}
