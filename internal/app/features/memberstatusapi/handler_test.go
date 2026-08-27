package memberstatusapi_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/dalemusser/stratahub/internal/app/features/memberstatusapi"
	"github.com/dalemusser/stratahub/internal/app/store/memberstatus"
	settingsstore "github.com/dalemusser/stratahub/internal/app/store/settings"
	"github.com/dalemusser/stratahub/internal/app/system/ratelimit"
	"github.com/dalemusser/stratahub/internal/app/system/workspace"
	"github.com/dalemusser/stratahub/internal/domain/models"
	"github.com/dalemusser/stratahub/internal/testutil"
	"github.com/go-chi/chi/v5"
	"github.com/gorilla/csrf"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.uber.org/zap"
)

const goodKey = "ms_test_shared_key_0123456789abcdef"

// env is one test's world: a DB, a workspace with a configured key, and an
// active member in it.
type env struct {
	t       *testing.T
	db      *mongo.Database
	h       *memberstatusapi.Handler
	wsID    primitive.ObjectID
	orgID   primitive.ObjectID
	member  models.User
	router  http.Handler // workspace context injected, API mounted
	apexRtr http.Handler // no workspace context (apex host)
}

func newEnv(t *testing.T) *env {
	t.Helper()
	db := testutil.SetupTestDB(t)
	ctx, cancel := testutil.TestContext()
	defer cancel()

	h, err := memberstatusapi.NewHandler(db, zap.NewNop())
	if err != nil {
		t.Fatalf("NewHandler: %v", err)
	}

	fx := testutil.NewFixtures(t, db)
	ws := fx.CreateWorkspace(ctx, "MHS", "mhs")
	org := fx.CreateOrganizationInWorkspace(ctx, "Hillsdale Middle School", ws.ID)
	member := fx.CreateUserInWorkspace(ctx, "Alice Cole", "acole@example.org", "member", ws.ID, &org.ID)
	if err := settingsstore.New(db).SetMemberStatusAPIKey(ctx, ws.ID, goodKey); err != nil {
		t.Fatalf("set key: %v", err)
	}

	e := &env{t: t, db: db, h: h, wsID: ws.ID, orgID: org.ID, member: member}
	e.router = e.buildRouter(&ws.ID, nil)
	e.apexRtr = e.buildRouter(nil, nil)
	return e
}

// buildRouter mounts the API behind a middleware that injects workspace
// context (as the real workspace middleware would), plus any extra root
// middleware given (used for the CSRF test).
func (e *env) buildRouter(wsID *primitive.ObjectID, extra []func(http.Handler) http.Handler) http.Handler {
	r := chi.NewRouter()
	r.Use(extra...)
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
	memberstatusapi.MountRoutes(r, e.h)
	return r
}

type result struct {
	code int
	body map[string]any
	raw  string
}

func (e *env) post(router http.Handler, path string, body any, headers map[string]string) result {
	e.t.Helper()
	var buf bytes.Buffer
	switch b := body.(type) {
	case string:
		buf.WriteString(b)
	default:
		if err := json.NewEncoder(&buf).Encode(body); err != nil {
			e.t.Fatalf("encode: %v", err)
		}
	}
	req := httptest.NewRequest("POST", path, &buf)
	req.Header.Set("Content-Type", "application/json")
	req.RemoteAddr = "203.0.113.5:44321"
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	res := result{code: rec.Code, raw: rec.Body.String()}
	if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
		e.t.Errorf("%s: Content-Type %q, want application/json", path, ct)
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &res.body)
	return res
}

func (r result) errCode() string {
	s, _ := r.body["error"].(string)
	return s
}

func event(key, userID, entity, state string) map[string]any {
	return map[string]any{"key": key, "user_id": userID, "entity": entity, "state": state}
}

func (e *env) expect(res result, code int, errCode string) {
	e.t.Helper()
	if res.code != code {
		e.t.Fatalf("status: got %d, want %d (body: %s)", res.code, code, res.raw)
	}
	if errCode != "" && res.errCode() != errCode {
		e.t.Errorf("error code: got %q, want %q (body: %s)", res.errCode(), errCode, res.raw)
	}
}

// --- authentication -------------------------------------------------------

func TestAuth_WorkspaceRequired(t *testing.T) {
	e := newEnv(t)
	res := e.post(e.apexRtr, "/api/member-status/ping", map[string]any{"key": goodKey}, nil)
	e.expect(res, http.StatusBadRequest, "workspace_required")
}

func TestAuth_NotConfigured(t *testing.T) {
	e := newEnv(t)
	ctx, cancel := testutil.TestContext()
	defer cancel()
	if err := settingsstore.New(e.db).SetMemberStatusAPIKey(ctx, e.wsID, ""); err != nil {
		t.Fatal(err)
	}
	res := e.post(e.router, "/api/member-status/ping", map[string]any{"key": goodKey}, nil)
	e.expect(res, http.StatusUnauthorized, "not_configured")
}

func TestAuth_MissingAndWrongKey(t *testing.T) {
	e := newEnv(t)
	e.expect(e.post(e.router, "/api/member-status/ping", map[string]any{}, nil), http.StatusUnauthorized, "unauthorized")
	e.expect(e.post(e.router, "/api/member-status/ping", map[string]any{"key": "ms_wrong_key_0123456789"}, nil), http.StatusUnauthorized, "unauthorized")
	// Wrong key on the status route must fail before any validation of the event.
	res := e.post(e.router, "/api/member-status", event("nope", "not-a-hex-id", "", ""), nil)
	e.expect(res, http.StatusUnauthorized, "unauthorized")
}

func TestAuth_BadJSON(t *testing.T) {
	e := newEnv(t)
	e.expect(e.post(e.router, "/api/member-status", `{"key": `, nil), http.StatusBadRequest, "bad_json")
	e.expect(e.post(e.router, "/api/member-status", `[1,2,3]`, nil), http.StatusBadRequest, "bad_json")
}

func TestAuth_BearerHeaderAlternative(t *testing.T) {
	e := newEnv(t)
	res := e.post(e.router, "/api/member-status/ping", map[string]any{}, map[string]string{"Authorization": "Bearer " + goodKey})
	e.expect(res, http.StatusOK, "")
}

func TestAuth_FailureThrottle(t *testing.T) {
	e := newEnv(t)
	e.h.Limiter = ratelimit.New(3, time.Minute)

	for i := 0; i < 3; i++ {
		e.expect(e.post(e.router, "/api/member-status/ping", map[string]any{"key": "ms_wrong_key_0123456789"}, nil), http.StatusUnauthorized, "unauthorized")
	}
	// Fourth attempt from the same IP is throttled — even with the right key.
	e.expect(e.post(e.router, "/api/member-status/ping", map[string]any{"key": goodKey}, nil), http.StatusTooManyRequests, "rate_limited")

	// A different IP is unaffected.
	req := httptest.NewRequest("POST", "/api/member-status/ping", strings.NewReader(`{"key":"`+goodKey+`"}`))
	req.RemoteAddr = "198.51.100.7:1234"
	rec := httptest.NewRecorder()
	e.router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Errorf("other IP: got %d, want 200", rec.Code)
	}
}

func TestAuth_SuccessResetsThrottle(t *testing.T) {
	e := newEnv(t)
	e.h.Limiter = ratelimit.New(3, time.Minute)
	for i := 0; i < 2; i++ {
		e.post(e.router, "/api/member-status/ping", map[string]any{"key": "ms_wrong_key_0123456789"}, nil)
	}
	e.expect(e.post(e.router, "/api/member-status/ping", map[string]any{"key": goodKey}, nil), http.StatusOK, "")
	// Counter was reset by the success: two more failures are still allowed.
	for i := 0; i < 2; i++ {
		e.expect(e.post(e.router, "/api/member-status/ping", map[string]any{"key": "ms_wrong_key_0123456789"}, nil), http.StatusUnauthorized, "unauthorized")
	}
}

// --- ping ----------------------------------------------------------------

func TestPing(t *testing.T) {
	e := newEnv(t)
	res := e.post(e.router, "/api/member-status/ping", map[string]any{"key": goodKey}, nil)
	e.expect(res, http.StatusOK, "")
	if res.body["ok"] != true || res.body["workspace"] != "mhs" {
		t.Errorf("ping body: %s", res.raw)
	}
	entities, _ := res.body["entities"].([]any)
	want := []string{"Pre", "MHS Engagement", "EWS Engagement", "Post"}
	if len(entities) != len(want) {
		t.Fatalf("entities: %v", entities)
	}
	for i, w := range want {
		if entities[i] != w {
			t.Errorf("entities[%d]: got %v, want %q", i, entities[i], w)
		}
	}
}

// --- status: validation ---------------------------------------------------

func TestStatus_Validation(t *testing.T) {
	e := newEnv(t)
	uid := e.member.ID.Hex()

	cases := []struct {
		name string
		body map[string]any
		code int
		err  string
	}{
		{"bad user id", event(goodKey, "zzz", "Pre", "started"), 400, "invalid_user_id"},
		{"empty entity", event(goodKey, uid, "   ", "started"), 400, "invalid_entity"},
		{"long entity", event(goodKey, uid, strings.Repeat("x", 101), "started"), 400, "invalid_entity"},
		{"bad state", event(goodKey, uid, "Pre", "finished"), 400, "invalid_state"},
		{"opened not accepted from provider", event(goodKey, uid, "Pre", "opened"), 400, "invalid_state"},
		{"bad occurred_at", func() map[string]any {
			b := event(goodKey, uid, "Pre", "started")
			b["occurred_at"] = "yesterday"
			return b
		}(), 400, "invalid_occurred_at"},
		{"unknown user", event(goodKey, primitive.NewObjectID().Hex(), "Pre", "started"), 404, "unknown_user"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			res := e.post(e.router, "/api/member-status", tc.body, nil)
			if res.code != tc.code || res.errCode() != tc.err {
				t.Errorf("got %d/%q, want %d/%q (body: %s)", res.code, res.errCode(), tc.code, tc.err, res.raw)
			}
		})
	}

	// Nothing was recorded for any of those.
	ctx, cancel := testutil.TestContext()
	defer cancel()
	if docs, _ := memberstatus.New(e.db).ListForUser(ctx, e.wsID, e.member.ID); len(docs) != 0 {
		t.Errorf("rejected requests recorded %d documents", len(docs))
	}
}

func TestStatus_UserMustBeActiveMemberOfThisWorkspace(t *testing.T) {
	e := newEnv(t)
	ctx, cancel := testutil.TestContext()
	defer cancel()
	fx := testutil.NewFixtures(t, e.db)

	otherWS := fx.CreateWorkspace(ctx, "Other", "other")
	elsewhere := fx.CreateUserInWorkspace(ctx, "Bob Other", "bob@example.org", "member", otherWS.ID, nil)
	leader := fx.CreateUserInWorkspace(ctx, "Lee Leader", "lee@example.org", "leader", e.wsID, nil)
	disabled := fx.CreateUserInWorkspace(ctx, "Dee Disabled", "dee@example.org", "member", e.wsID, nil)
	if _, err := e.db.Collection("users").UpdateByID(ctx, disabled.ID, bson.M{"$set": bson.M{"status": "disabled"}}); err != nil {
		t.Fatal(err)
	}

	for name, id := range map[string]primitive.ObjectID{
		"member of another workspace": elsewhere.ID,
		"leader, not a member":        leader.ID,
		"disabled member":             disabled.ID,
	} {
		res := e.post(e.router, "/api/member-status", event(goodKey, id.Hex(), "Pre", "started"), nil)
		if res.code != http.StatusNotFound || res.errCode() != "unknown_user" {
			t.Errorf("%s: got %d/%q, want 404/unknown_user", name, res.code, res.errCode())
		}
	}
}

// --- status: happy paths --------------------------------------------------

func TestStatus_StartedThenCompleted(t *testing.T) {
	e := newEnv(t)
	uid := e.member.ID.Hex()

	res := e.post(e.router, "/api/member-status", event(goodKey, uid, "MHS Engagement", "started"), nil)
	e.expect(res, http.StatusOK, "")
	if res.body["ok"] != true || res.body["state"] != "started" || res.body["known_entity"] != true ||
		res.body["entity_key"] != "mhs" || res.body["entity"] != "MHS Engagement" || res.body["user_id"] != uid {
		t.Errorf("started body: %s", res.raw)
	}
	if res.body["started_at"] == nil || res.body["completed_at"] != nil {
		t.Errorf("started timestamps: %s", res.raw)
	}
	startedAt := res.body["started_at"]

	body := event(goodKey, uid, "mhs engagement", "COMPLETED") // case-insensitive name and state
	body["occurred_at"] = "2026-08-25T14:03:11Z"
	res = e.post(e.router, "/api/member-status", body, nil)
	e.expect(res, http.StatusOK, "")
	if res.body["state"] != "completed" || res.body["completed_at"] == nil || res.body["started_at"] != startedAt {
		t.Errorf("completed body: %s", res.raw)
	}

	// Repeating "started" after "completed" is accepted and changes nothing.
	res = e.post(e.router, "/api/member-status", event(goodKey, uid, "MHS Engagement", "started"), nil)
	e.expect(res, http.StatusOK, "")
	if res.body["state"] != "completed" || res.body["started_at"] != startedAt {
		t.Errorf("late started body: %s", res.raw)
	}

	// One document, source api, history of three, provider timestamp kept.
	ctx, cancel := testutil.TestContext()
	defer cancel()
	docs, err := memberstatus.New(e.db).ListForUser(ctx, e.wsID, e.member.ID)
	if err != nil || len(docs) != 1 {
		t.Fatalf("docs: %d, %v", len(docs), err)
	}
	d := docs[0]
	if d.EntityKey != "mhs" || d.Entity != "MHS Engagement" || d.Source != models.MemberStatusSourceAPI || len(d.History) != 3 {
		t.Errorf("doc: %+v", d)
	}
	if d.History[1].OccurredAt == nil || !d.History[1].OccurredAt.Equal(time.Date(2026, 8, 25, 14, 3, 11, 0, time.UTC)) {
		t.Errorf("occurred_at not kept: %+v", d.History[1])
	}
	if d.History[0].RemoteIP != "203.0.113.5" {
		t.Errorf("remote ip: %q", d.History[0].RemoteIP)
	}
}

func TestStatus_UnknownEntityIsStoredAndFlagged(t *testing.T) {
	e := newEnv(t)
	uid := e.member.ID.Hex()

	res := e.post(e.router, "/api/member-status", event(goodKey, uid, "Mid Survey", "completed"), nil)
	e.expect(res, http.StatusOK, "")
	if res.body["known_entity"] != false || res.body["entity_key"] != "name:mid survey" || res.body["entity"] != "Mid Survey" {
		t.Errorf("unknown entity body: %s", res.raw)
	}

	ctx, cancel := testutil.TestContext()
	defer cancel()
	doc, err := memberstatus.New(e.db).Get(ctx, e.wsID, e.member.ID, "name:mid survey")
	if err != nil {
		t.Fatalf("stored doc: %v", err)
	}
	if doc.State != models.MemberStatusCompleted || doc.Entity != "Mid Survey" {
		t.Errorf("doc: %+v", doc)
	}
}

func TestStatus_SeparateDocumentsPerEntity(t *testing.T) {
	e := newEnv(t)
	uid := e.member.ID.Hex()
	for _, name := range []string{"Pre", "MHS Engagement", "EWS Engagement", "Post"} {
		e.expect(e.post(e.router, "/api/member-status", event(goodKey, uid, name, "completed"), nil), http.StatusOK, "")
	}
	ctx, cancel := testutil.TestContext()
	defer cancel()
	docs, _ := memberstatus.New(e.db).ListForUser(ctx, e.wsID, e.member.ID)
	keys := make([]string, 0, len(docs))
	for _, d := range docs {
		keys = append(keys, d.EntityKey)
	}
	if strings.Join(keys, ",") != "ews,mhs,post,pre" {
		t.Errorf("entity keys: %v", keys)
	}
}

// --- wiring ---------------------------------------------------------------

func TestMethodNotAllowed(t *testing.T) {
	e := newEnv(t)
	req := httptest.NewRequest("GET", "/api/member-status", nil)
	rec := httptest.NewRecorder()
	e.router.ServeHTTP(rec, req)
	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("GET: got %d, want 405", rec.Code)
	}
}

// TestCSRFExempt proves the exemption works the way it is installed in
// bootstrap/routes.go: before gorilla/csrf on the root router. Without it a
// cookie-less POST would be rejected with 403.
func TestCSRFExempt(t *testing.T) {
	e := newEnv(t)
	protect := csrf.Protect([]byte("0123456789abcdef0123456789abcdef"), csrf.Secure(false))

	withExempt := e.buildRouter(&e.wsID, []func(http.Handler) http.Handler{memberstatusapi.CSRFExempt, protect})
	res := e.post(withExempt, "/api/member-status/ping", map[string]any{"key": goodKey}, nil)
	if res.code != http.StatusOK {
		t.Errorf("with exemption: got %d, want 200 (body: %s)", res.code, res.raw)
	}

	// Control: the same request with CSRF protection but no exemption is refused.
	withoutExempt := e.buildRouter(&e.wsID, []func(http.Handler) http.Handler{protect})
	req := httptest.NewRequest("POST", "/api/member-status/ping", strings.NewReader(`{"key":"`+goodKey+`"}`))
	rec := httptest.NewRecorder()
	withoutExempt.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Errorf("without exemption: got %d, want 403", rec.Code)
	}

	// And the exemption is path-scoped: another path is still protected.
	other := chi.NewRouter()
	other.Use(memberstatusapi.CSRFExempt, protect)
	other.Post("/other", func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(200) })
	req = httptest.NewRequest("POST", "/other", nil)
	rec = httptest.NewRecorder()
	other.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Errorf("other path: got %d, want 403", rec.Code)
	}
}
