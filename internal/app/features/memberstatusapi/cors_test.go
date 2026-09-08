package memberstatusapi_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/dalemusser/stratahub/internal/app/features/memberstatusapi"
	settingsstore "github.com/dalemusser/stratahub/internal/app/store/settings"
	"github.com/dalemusser/stratahub/internal/app/system/workspace"
	"github.com/dalemusser/stratahub/internal/testutil"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/cors"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

const (
	allowedOrigin = "https://surveys.example.com"
	otherOrigin   = "https://elsewhere.example.com"
	globalOrigin  = "https://mhs.example.com" // an origin the global CORS config (game clients) would allow
)

// setOrigins stores the workspace's allowed browser origins the way the
// settings page does: overlaying the current document so the key survives.
func (e *env) setOrigins(origins ...string) {
	e.t.Helper()
	ctx, cancel := testutil.TestContext()
	defer cancel()
	st := settingsstore.New(e.db)
	s, err := st.Get(ctx, e.wsID)
	if err != nil {
		e.t.Fatalf("settings get: %v", err)
	}
	s.MemberStatusAPIAllowedOrigins = origins
	if err := st.Save(ctx, e.wsID, s); err != nil {
		e.t.Fatalf("settings save: %v", err)
	}
}

// corsRouter builds the chain the way bootstrap installs it: workspace
// context → the API's CORS middleware → optionally a global CORS handler
// shaped like production's (credentials, other origins) → the API routes,
// plus an unrelated route to check pass-through. withAPI=false leaves the
// API's middleware out, which is the chain production had before this
// feature (the control case).
func (e *env) corsRouter(wsID *primitive.ObjectID, withAPI, withGlobal bool) http.Handler {
	r := chi.NewRouter()
	r.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			if wsID != nil {
				req = workspace.WithTestWorkspace(req, *wsID, "mhs", "MHS")
			} else {
				req = workspace.WithTestApex(req)
			}
			next.ServeHTTP(w, req)
		})
	})
	if withAPI {
		r.Use(e.h.CORS())
	}
	if withGlobal {
		r.Use(cors.Handler(cors.Options{
			AllowedOrigins:   []string{globalOrigin},
			AllowedMethods:   []string{"GET", "POST", "OPTIONS"},
			AllowCredentials: true,
			MaxAge:           3600,
		}))
	}
	memberstatusapi.MountRoutes(r, e.h)
	r.Post("/other", func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusOK) })
	return r
}

// preflight sends the OPTIONS request a browser sends before a cross-origin
// JSON POST from origin.
func preflight(router http.Handler, path, origin, requestHeaders string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodOptions, path, nil)
	req.Header.Set("Origin", origin)
	req.Header.Set("Access-Control-Request-Method", "POST")
	if requestHeaders != "" {
		req.Header.Set("Access-Control-Request-Headers", requestHeaders)
	}
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	return rec
}

func allowOrigin(rec *httptest.ResponseRecorder) string {
	return rec.Header().Get("Access-Control-Allow-Origin")
}

func TestCORS_PreflightAllowedOrigin(t *testing.T) {
	e := newEnv(t)
	e.setOrigins(allowedOrigin)
	router := e.corsRouter(&e.wsID, true, false)

	for _, path := range []string{"/api/member-status", "/api/member-status/ping"} {
		rec := preflight(router, path, allowedOrigin, "content-type")
		if rec.Code != http.StatusOK {
			t.Errorf("%s: preflight status %d, want 200", path, rec.Code)
		}
		if got := allowOrigin(rec); got != allowedOrigin {
			t.Errorf("%s: Allow-Origin %q, want %q", path, got, allowedOrigin)
		}
		if m := rec.Header().Get("Access-Control-Allow-Methods"); !strings.Contains(m, "POST") {
			t.Errorf("%s: Allow-Methods %q, want POST", path, m)
		}
		if h := rec.Header().Get("Access-Control-Allow-Headers"); !strings.Contains(strings.ToLower(h), "content-type") {
			t.Errorf("%s: Allow-Headers %q, want Content-Type", path, h)
		}
		if a := rec.Header().Get("Access-Control-Max-Age"); a != "3600" {
			t.Errorf("%s: Max-Age %q, want 3600", path, a)
		}
		if c := rec.Header().Get("Access-Control-Allow-Credentials"); c != "" {
			t.Errorf("%s: Allow-Credentials %q must never be sent", path, c)
		}
		if v := rec.Header().Values("Vary"); !containsFold(v, "Origin") {
			t.Errorf("%s: Vary %v, want Origin", path, v)
		}
	}

	// The Bearer alternative is reachable from a browser too.
	rec := preflight(router, "/api/member-status/ping", allowedOrigin, "content-type, authorization")
	if allowOrigin(rec) != allowedOrigin {
		t.Errorf("authorization header: Allow-Origin %q, want %q", allowOrigin(rec), allowedOrigin)
	}

	// Any other request header is refused (no allow headers at all).
	rec = preflight(router, "/api/member-status", allowedOrigin, "content-type, x-custom")
	if allowOrigin(rec) != "" {
		t.Errorf("unknown request header: Allow-Origin %q, want none", allowOrigin(rec))
	}

	// The browser lowercases nothing it does not have to, but a mixed-case
	// origin still matches.
	rec = preflight(router, "/api/member-status", "https://Surveys.Example.com", "content-type")
	if allowOrigin(rec) != "https://Surveys.Example.com" {
		t.Errorf("mixed-case origin: Allow-Origin %q, want the origin echoed", allowOrigin(rec))
	}
}

func TestCORS_PreflightRefused(t *testing.T) {
	e := newEnv(t)
	router := e.corsRouter(&e.wsID, true, false)

	// No origins configured: nothing is allowed.
	rec := preflight(router, "/api/member-status", allowedOrigin, "content-type")
	if rec.Code != http.StatusOK || allowOrigin(rec) != "" {
		t.Errorf("unconfigured: status %d Allow-Origin %q; want 200 and none", rec.Code, allowOrigin(rec))
	}

	e.setOrigins(allowedOrigin)
	for _, origin := range []string{
		otherOrigin,
		"https://example.com",
		"https://surveys.example.com.evil.io",
		"http://surveys.example.com",
		"https://surveys.example.com:8443",
		"null",
	} {
		rec := preflight(router, "/api/member-status", origin, "content-type")
		if rec.Code != http.StatusOK {
			t.Errorf("%s: status %d, want 200 (the browser reads the headers, not the status)", origin, rec.Code)
		}
		if got := allowOrigin(rec); got != "" {
			t.Errorf("%s: Allow-Origin %q, want none", origin, got)
		}
	}

	// A preflight that reached a host with no workspace is refused too.
	rec = preflight(e.corsRouter(nil, true, false), "/api/member-status", allowedOrigin, "content-type")
	if allowOrigin(rec) != "" {
		t.Errorf("apex: Allow-Origin %q, want none", allowOrigin(rec))
	}
}

func TestCORS_ActualRequests(t *testing.T) {
	e := newEnv(t)
	e.setOrigins(allowedOrigin)
	router := e.corsRouter(&e.wsID, true, false)
	from := func(origin string) map[string]string { return map[string]string{"Origin": origin} }

	// Accepted call: the page can read the response.
	res := e.postRec(router, "/api/member-status/ping", map[string]any{"key": goodKey}, from(allowedOrigin))
	if res.Code != http.StatusOK || allowOrigin(res) != allowedOrigin {
		t.Errorf("ping: status %d Allow-Origin %q; want 200 and %q", res.Code, allowOrigin(res), allowedOrigin)
	}
	if c := res.Header().Get("Access-Control-Allow-Credentials"); c != "" {
		t.Errorf("Allow-Credentials %q must never be sent", c)
	}

	// Rejected calls carry the header too, so the page can read the error body.
	res = e.postRec(router, "/api/member-status/ping", map[string]any{"key": "ms_wrong_key_0123456789"}, from(allowedOrigin))
	if res.Code != http.StatusUnauthorized || allowOrigin(res) != allowedOrigin {
		t.Errorf("wrong key: status %d Allow-Origin %q; want 401 and %q", res.Code, allowOrigin(res), allowedOrigin)
	}
	res = e.postRec(router, "/api/member-status", event(goodKey, primitive.NewObjectID().Hex(), "Pre", "started"), from(allowedOrigin))
	if res.Code != http.StatusNotFound || allowOrigin(res) != allowedOrigin {
		t.Errorf("unknown user: status %d Allow-Origin %q; want 404 and %q", res.Code, allowOrigin(res), allowedOrigin)
	}

	// No Origin header (a server-to-server caller): served as before, no CORS headers.
	res = e.postRec(router, "/api/member-status/ping", map[string]any{"key": goodKey}, nil)
	if res.Code != http.StatusOK || allowOrigin(res) != "" {
		t.Errorf("no origin: status %d Allow-Origin %q; want 200 and none", res.Code, allowOrigin(res))
	}

	// An unlisted origin: the request is still processed (CORS is a browser
	// rule; the key is the authentication), but the browser gets no
	// allow-origin header and will not let the page read the response.
	res = e.postRec(router, "/api/member-status/ping", map[string]any{"key": goodKey}, from(otherOrigin))
	if res.Code != http.StatusOK || allowOrigin(res) != "" {
		t.Errorf("unlisted origin: status %d Allow-Origin %q; want 200 and none", res.Code, allowOrigin(res))
	}
}

func TestCORS_PassesOtherPathsThrough(t *testing.T) {
	e := newEnv(t)
	e.setOrigins(allowedOrigin)
	router := e.corsRouter(&e.wsID, true, false)

	// A preflight for another route is not answered here: it falls through to
	// the router, which has no OPTIONS handler for it.
	rec := preflight(router, "/other", allowedOrigin, "content-type")
	if rec.Code == http.StatusOK || allowOrigin(rec) != "" {
		t.Errorf("/other preflight: status %d Allow-Origin %q; want not-200 and none", rec.Code, allowOrigin(rec))
	}
	// An actual request to another route gets no CORS headers.
	req := httptest.NewRequest(http.MethodPost, "/other", nil)
	req.Header.Set("Origin", allowedOrigin)
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || allowOrigin(rec) != "" {
		t.Errorf("/other post: status %d Allow-Origin %q; want 200 and none", rec.Code, allowOrigin(rec))
	}
	// And a look-alike path is not the API.
	rec = preflight(router, "/api/member-statuses", allowedOrigin, "content-type")
	if allowOrigin(rec) != "" {
		t.Errorf("look-alike path: Allow-Origin %q, want none", allowOrigin(rec))
	}
}

// TestCORS_AheadOfGlobalCORS proves the placement bootstrap relies on: with
// a production-shaped global CORS handler (credentials, other origins) after
// the API's middleware, the provider's origin is answered by the API's
// policy — without credentials — and the global list's own origins do not
// get browser access to the API. The control shows why the order matters:
// the global handler alone answers the preflight with no allow-origin.
func TestCORS_AheadOfGlobalCORS(t *testing.T) {
	e := newEnv(t)
	e.setOrigins(allowedOrigin)

	t.Run("control: global CORS only", func(t *testing.T) {
		router := e.corsRouter(&e.wsID, false, true)
		rec := preflight(router, "/api/member-status", allowedOrigin, "content-type")
		if rec.Code != http.StatusOK || allowOrigin(rec) != "" {
			t.Errorf("status %d Allow-Origin %q; want 200 and none (the global handler swallows the preflight)", rec.Code, allowOrigin(rec))
		}
	})

	router := e.corsRouter(&e.wsID, true, true)

	rec := preflight(router, "/api/member-status", allowedOrigin, "content-type")
	if allowOrigin(rec) != allowedOrigin {
		t.Errorf("preflight: Allow-Origin %q, want %q", allowOrigin(rec), allowedOrigin)
	}
	if c := rec.Header().Get("Access-Control-Allow-Credentials"); c != "" {
		t.Errorf("preflight: Allow-Credentials %q leaked from the global policy", c)
	}

	res := e.postRec(router, "/api/member-status/ping", map[string]any{"key": goodKey}, map[string]string{"Origin": allowedOrigin})
	if res.Code != http.StatusOK || allowOrigin(res) != allowedOrigin {
		t.Errorf("actual: status %d Allow-Origin %q; want 200 and %q", res.Code, allowOrigin(res), allowedOrigin)
	}
	if c := res.Header().Get("Access-Control-Allow-Credentials"); c != "" {
		t.Errorf("actual: Allow-Credentials %q leaked from the global policy", c)
	}

	// The global list's origin is not a workspace-listed origin for the API.
	rec = preflight(router, "/api/member-status", globalOrigin, "content-type")
	if allowOrigin(rec) != "" {
		t.Errorf("global-list origin: Allow-Origin %q, want none", allowOrigin(rec))
	}
	// …while the global handler still serves its own routes as before.
	rec = preflight(router, "/other", globalOrigin, "content-type")
	if allowOrigin(rec) != globalOrigin || rec.Header().Get("Access-Control-Allow-Credentials") != "true" {
		t.Errorf("/other from the global-list origin: Allow-Origin %q Credentials %q; want the global policy",
			allowOrigin(rec), rec.Header().Get("Access-Control-Allow-Credentials"))
	}
}

// postRec is post without the JSON-body assertions, returning the recorder
// so headers can be inspected.
func (e *env) postRec(router http.Handler, path string, body any, headers map[string]string) *httptest.ResponseRecorder {
	e.t.Helper()
	req := httptest.NewRequest(http.MethodPost, path, encodeBody(e.t, body))
	req.Header.Set("Content-Type", "application/json")
	req.RemoteAddr = "203.0.113.5:44321"
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	return rec
}

func containsFold(values []string, want string) bool {
	for _, v := range values {
		for _, part := range strings.Split(v, ",") {
			if strings.EqualFold(strings.TrimSpace(part), want) {
				return true
			}
		}
	}
	return false
}

// encodeBody turns a string or a JSON-encodable value into a request body.
func encodeBody(t *testing.T, body any) *bytes.Buffer {
	t.Helper()
	var buf bytes.Buffer
	switch b := body.(type) {
	case nil:
	case string:
		buf.WriteString(b)
	default:
		if err := json.NewEncoder(&buf).Encode(body); err != nil {
			t.Fatalf("encode: %v", err)
		}
	}
	return &buf
}
