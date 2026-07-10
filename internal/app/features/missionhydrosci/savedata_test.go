// internal/app/features/missionhydrosci/savedata_test.go
package missionhydrosci

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"go.uber.org/zap"
)

// newTestHandler builds a Handler with just the fields callStratasaveDelete
// needs (Log + Services), avoiding the full DB/session wiring.
func newTestHandler(saveAuth string) *Handler {
	return &Handler{
		Log:      zap.NewNop(),
		Services: GameServices{SaveAuth: saveAuth},
	}
}

func TestCallStratasaveDelete_SendsExpectedRequest(t *testing.T) {
	var gotAuth, gotContentType, gotBody string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("method = %s, want POST", r.Method)
		}
		gotAuth = r.Header.Get("Authorization")
		gotContentType = r.Header.Get("Content-Type")
		b, _ := io.ReadAll(r.Body)
		gotBody = string(b)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"user_id": "x", "game": "mhs", "deleted": 3})
	}))
	defer srv.Close()

	h := newTestHandler("Bearer test-key")
	deleted, err := h.callStratasaveDelete(context.Background(), srv.URL, "69b4449ec6006ac370dad9df")
	if err != nil {
		t.Fatalf("callStratasaveDelete() error = %v", err)
	}
	if deleted != 3 {
		t.Errorf("deleted = %d, want 3", deleted)
	}
	if gotAuth != "Bearer test-key" {
		t.Errorf("Authorization = %q, want %q", gotAuth, "Bearer test-key")
	}
	if gotContentType != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", gotContentType)
	}

	// Body must carry the caller's user_id and the fixed MHS game id.
	var body map[string]string
	if err := json.Unmarshal([]byte(gotBody), &body); err != nil {
		t.Fatalf("request body not JSON: %v (%s)", err, gotBody)
	}
	if body["user_id"] != "69b4449ec6006ac370dad9df" {
		t.Errorf("body user_id = %q, want the caller's id", body["user_id"])
	}
	if body["game"] != mhsGameID {
		t.Errorf("body game = %q, want %q", body["game"], mhsGameID)
	}
}

func TestCallStratasaveDelete_NoAuthHeaderWhenUnset(t *testing.T) {
	var hadAuthHeader bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, hadAuthHeader = r.Header["Authorization"]
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"deleted": 0})
	}))
	defer srv.Close()

	h := newTestHandler("")
	if _, err := h.callStratasaveDelete(context.Background(), srv.URL, "69b4449ec6006ac370dad9df"); err != nil {
		t.Fatalf("callStratasaveDelete() error = %v", err)
	}
	if hadAuthHeader {
		t.Error("Authorization header should be absent when SaveAuth is empty")
	}
}

func TestCallStratasaveDelete_UpstreamErrorSurfaces(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "'user_id' must be a 24-character lowercase hex string"})
	}))
	defer srv.Close()

	h := newTestHandler("Bearer k")
	_, err := h.callStratasaveDelete(context.Background(), srv.URL, "bad")
	if err == nil {
		t.Fatal("expected error for non-2xx upstream response, got nil")
	}
}
