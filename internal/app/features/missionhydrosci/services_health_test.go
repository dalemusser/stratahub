package missionhydrosci

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

// The server's view of the game services: one service up, one down, each
// probe answered once a minute at most (the play gate asks on every launch).
func TestServicesHealth(t *testing.T) {
	var calls int32
	h := &Handler{Probes: []ManifestProbe{
		{Name: "log", URL: "https://log.example.org/health", Post: "https://log.example.org/api/log/probe"},
		{Name: "save", URL: "https://save.example.org/health", Post: "https://save.example.org/api/state/probe"},
	}}
	h.svcHealth.probeFn = func(_ context.Context, url string) bool {
		atomic.AddInt32(&calls, 1)
		return strings.Contains(url, "log.")
	}
	for i := 0; i < 3; i++ {
		rec := httptest.NewRecorder()
		h.ServeServicesHealth(rec, httptest.NewRequest(http.MethodGet, "/missionhydrosci/api/services-health", nil))
		var st ServicesHealthState
		if err := json.NewDecoder(rec.Body).Decode(&st); err != nil {
			t.Fatal(err)
		}
		if st.Services["log"] != "ok" || st.Services["save"] != "down" {
			t.Fatalf("state %+v", st.Services)
		}
	}
	if calls != 2 {
		t.Errorf("probes sent: %d, want 2 (one per service, then cached)", calls)
	}
}

// postProbe accepts only the service's own {"ok":true}.
func TestPostProbe(t *testing.T) {
	for _, tc := range []struct {
		status int
		body   string
		want   bool
	}{
		{200, `{"ok":true,"service":"stratalog"}`, true},
		{200, `<html>Blocked by policy</html>`, false},
		{401, `{"error":"unauthorized"}`, false},
	} {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method != http.MethodPost || r.Header.Get("Authorization") == "" || r.Header.Get("Content-Type") != "application/json" {
				t.Errorf("probe not in the game's form: %s %v", r.Method, r.Header)
			}
			w.WriteHeader(tc.status)
			_, _ = w.Write([]byte(tc.body))
		}))
		if got := postProbe(context.Background(), srv.Client(), srv.URL+"/api/log/probe"); got != tc.want {
			t.Errorf("%d %q: got %v", tc.status, tc.body, got)
		}
		srv.Close()
	}
}
