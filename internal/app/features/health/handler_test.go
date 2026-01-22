package health_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/dalemusser/stratahub/internal/app/features/health"
	"github.com/dalemusser/stratahub/internal/testutil"
	"go.uber.org/zap"
)

func TestServe_DatabaseConnected(t *testing.T) {
	// Set up a test database to get a connected client
	db := testutil.SetupTestDB(t)
	client := db.Client()
	logger := zap.NewNop()
	handler := health.NewHandler(client, "http://localhost:8080", logger)

	req := httptest.NewRequest("GET", "/health", nil)
	rec := httptest.NewRecorder()

	handler.Serve(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d", http.StatusOK, rec.Code)
	}

	// Verify content type
	contentType := rec.Header().Get("Content-Type")
	if contentType != "application/json" {
		t.Errorf("Content-Type: got %q, want %q", contentType, "application/json")
	}

	// Verify response body
	var response struct {
		Status   string `json:"status"`
		Database string `json:"database"`
		Cert     *struct {
			DaysLeft int  `json:"days_left"`
			Valid    bool `json:"valid"`
		} `json:"cert,omitempty"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}

	if response.Status != "ok" {
		t.Errorf("status: got %q, want %q", response.Status, "ok")
	}
	if response.Database != "connected" {
		t.Errorf("database: got %q, want %q", response.Database, "connected")
	}
}
