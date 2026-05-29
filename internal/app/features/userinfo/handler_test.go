package userinfo_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/dalemusser/stratahub/internal/app/features/userinfo"
	"github.com/dalemusser/stratahub/internal/app/system/auth"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

func newTestHandler(t *testing.T) *userinfo.Handler {
	t.Helper()
	return userinfo.NewHandler()
}

func TestNewHandler(t *testing.T) {
	h := newTestHandler(t)
	if h == nil {
		t.Fatal("NewHandler() returned nil")
	}
}

func TestServeUserInfo_Unauthenticated(t *testing.T) {
	handler := newTestHandler(t)

	req := httptest.NewRequest("GET", "/api/userinfo", nil)
	rec := httptest.NewRecorder()

	handler.ServeUserInfo(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d", http.StatusOK, rec.Code)
	}

	// Check Content-Type
	contentType := rec.Header().Get("Content-Type")
	if contentType != "application/json" {
		t.Errorf("Content-Type: got %q, want %q", contentType, "application/json")
	}

	// Parse response JSON
	var response map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatalf("failed to parse response JSON: %v", err)
	}

	// Verify unauthenticated response
	if isAuth, ok := response["isAuthenticated"].(bool); !ok || isAuth {
		t.Errorf("isAuthenticated: got %v, want false", response["isAuthenticated"])
	}
	if name, ok := response["name"].(string); !ok || name != "" {
		t.Errorf("name: got %q, want empty string", response["name"])
	}
	if userID, ok := response["user_id"].(string); !ok || userID != "" {
		t.Errorf("user_id: got %q, want empty string", response["user_id"])
	}
	if _, exists := response["email"]; exists {
		t.Errorf("email key should not be present in response, got %v", response["email"])
	}
	if _, exists := response["login_id"]; exists {
		t.Errorf("login_id key should not be present in response, got %v", response["login_id"])
	}
}

func TestServeUserInfo_Authenticated(t *testing.T) {
	handler := newTestHandler(t)

	userID := primitive.NewObjectID()
	sessionUser := &auth.SessionUser{
		ID:      userID.Hex(),
		Name:    "Test User",
		LoginID: "test@example.com",
		Role:    "member",
	}

	req := httptest.NewRequest("GET", "/api/userinfo", nil)
	req = auth.WithTestUser(req, sessionUser)
	rec := httptest.NewRecorder()

	handler.ServeUserInfo(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d", http.StatusOK, rec.Code)
	}

	// Parse response JSON
	var response map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatalf("failed to parse response JSON: %v", err)
	}

	// Verify authenticated response
	if isAuth, ok := response["isAuthenticated"].(bool); !ok || !isAuth {
		t.Errorf("isAuthenticated: got %v, want true", response["isAuthenticated"])
	}
	if name, ok := response["name"].(string); !ok || name != "Test User" {
		t.Errorf("name: got %q, want %q", response["name"], "Test User")
	}
	if gotUserID, ok := response["user_id"].(string); !ok || gotUserID != userID.Hex() {
		t.Errorf("user_id: got %q, want %q", response["user_id"], userID.Hex())
	}
	if _, exists := response["email"]; exists {
		t.Errorf("email key should not be present in response, got %v", response["email"])
	}
	if _, exists := response["login_id"]; exists {
		t.Errorf("login_id key should not be present in response, got %v", response["login_id"])
	}
}

func TestServeUserInfo_ReturnsJSON(t *testing.T) {
	handler := newTestHandler(t)

	req := httptest.NewRequest("GET", "/api/userinfo", nil)
	rec := httptest.NewRecorder()

	handler.ServeUserInfo(rec, req)

	// Verify the response is valid JSON
	contentType := rec.Header().Get("Content-Type")
	if contentType != "application/json" {
		t.Errorf("Content-Type: got %q, want %q", contentType, "application/json")
	}

	var response map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Errorf("response body is not valid JSON: %v", err)
	}
}
