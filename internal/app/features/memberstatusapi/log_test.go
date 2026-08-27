package memberstatusapi_test

import (
	"net/http"
	"testing"

	"github.com/dalemusser/stratahub/internal/app/store/memberstatuslog"
	"github.com/dalemusser/stratahub/internal/domain/models"
	"github.com/dalemusser/stratahub/internal/testutil"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

// logEntry fetches the log entry named by a response's event_id.
func (e *env) logEntry(t *testing.T, res result) models.MemberStatusLogEntry {
	t.Helper()
	hex, _ := res.body["event_id"].(string)
	if hex == "" {
		t.Fatalf("response has no event_id: %s", res.raw)
	}
	id, err := primitive.ObjectIDFromHex(hex)
	if err != nil {
		t.Fatalf("event_id %q is not an ObjectID", hex)
	}
	ctx, cancel := testutil.TestContext()
	defer cancel()
	entry, err := memberstatuslog.New(e.db).Get(ctx, e.wsID, id)
	if err != nil {
		t.Fatalf("log entry %s not found: %v", hex, err)
	}
	return entry
}

func (e *env) logCount(t *testing.T) int64 {
	t.Helper()
	ctx, cancel := testutil.TestContext()
	defer cancel()
	n, err := memberstatuslog.New(e.db).Count(ctx, memberstatuslog.ListQuery{WorkspaceID: e.wsID})
	if err != nil {
		t.Fatal(err)
	}
	return n
}

func TestLog_AcceptedEventIsLoggedWithEventID(t *testing.T) {
	e := newEnv(t)
	uid := e.member.ID.Hex()

	body := event(goodKey, uid, "mhs engagement", "Completed")
	body["occurred_at"] = "2026-08-25T14:03:11Z"
	res := e.post(e.router, "/api/member-status", body, nil)
	e.expect(res, http.StatusOK, "")

	entry := e.logEntry(t, res)
	if entry.Source != models.MemberStatusSourceAPI || entry.RemoteIP != "203.0.113.5" || !entry.Accepted() || entry.Outcome.HTTPStatus != 200 {
		t.Errorf("entry envelope: %+v", entry)
	}
	// The request is stored exactly as sent…
	if entry.Request.UserID != uid || entry.Request.Entity != "mhs engagement" || entry.Request.State != "Completed" ||
		entry.Request.StateNorm != "completed" || entry.Request.OccurredAt != "2026-08-25T14:03:11Z" {
		t.Errorf("request as sent: %+v", entry.Request)
	}
	// …and what StrataHub made of it.
	r := entry.Resolved
	if r.UserID == nil || *r.UserID != e.member.ID || r.OrganizationID == nil || *r.OrganizationID != e.orgID ||
		r.EntityKey != "mhs" || r.EntityTitle != "MHS Engagement" || !r.KnownEntity || r.StateApplied != "completed" || r.ResultingState != "completed" {
		t.Errorf("resolved: %+v", r)
	}
}

func TestLog_RejectedRequestsAreLoggedWithTheirError(t *testing.T) {
	e := newEnv(t)
	uid := e.member.ID.Hex()

	cases := []struct {
		name string
		body map[string]any
		code int
		err  string
	}{
		{"unknown user", event(goodKey, primitive.NewObjectID().Hex(), "Pre", "started"), 404, "unknown_user"},
		{"bad user id", event(goodKey, "nope", "Pre", "started"), 400, "invalid_user_id"},
		{"bad state", event(goodKey, uid, "Pre", "finished"), 400, "invalid_state"},
		{"unknown entity (accepted, flagged)", event(goodKey, uid, "Mid Survey", "started"), 200, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			res := e.post(e.router, "/api/member-status", tc.body, nil)
			if res.code != tc.code || res.errCode() != tc.err {
				t.Fatalf("got %d/%q, want %d/%q (%s)", res.code, res.errCode(), tc.code, tc.err, res.raw)
			}
			entry := e.logEntry(t, res)
			if entry.Outcome.HTTPStatus != tc.code || entry.Outcome.Error != tc.err {
				t.Errorf("outcome: %+v", entry.Outcome)
			}
			if tc.err != "" && entry.Outcome.Message == "" {
				t.Error("rejected entry should carry the message")
			}
		})
	}

	// The bad-user-id entry keeps the raw string; the unknown-user entry has
	// no resolved user; the unrecognized-name entry is stored under a name: key.
	ctx, cancel := testutil.TestContext()
	defer cancel()
	store := memberstatuslog.New(e.db)
	if es, _, _ := store.List(ctx, memberstatuslog.ListQuery{WorkspaceID: e.wsID, RawUserID: "nope"}); len(es) != 1 || es[0].Resolved.UserID != nil {
		t.Errorf("raw user id entry: %+v", es)
	}
	if es, _, _ := store.List(ctx, memberstatuslog.ListQuery{WorkspaceID: e.wsID, Outcome: "unknown_user"}); len(es) != 1 || es[0].Resolved.UserID != nil {
		t.Errorf("unknown user entry: %+v", es)
	}
	if es, _, _ := store.List(ctx, memberstatuslog.ListQuery{WorkspaceID: e.wsID, EntityKey: memberstatuslog.EntityUnrecognized}); len(es) != 1 || es[0].Resolved.KnownEntity || es[0].Request.Entity != "Mid Survey" {
		t.Errorf("unrecognized entity entry: %+v", es)
	}
	if n := e.logCount(t); n != 4 {
		t.Errorf("log entries: %d, want 4", n)
	}
}

func TestLog_UnauthenticatedAndPingAreNotLogged(t *testing.T) {
	e := newEnv(t)
	e.post(e.router, "/api/member-status", event("ms_wrong_key_0123456789", e.member.ID.Hex(), "Pre", "started"), nil)
	e.post(e.router, "/api/member-status", `{"key": `, nil)
	e.post(e.router, "/api/member-status/ping", map[string]any{"key": goodKey}, nil)
	if n := e.logCount(t); n != 0 {
		t.Errorf("unauthenticated requests and pings must not be logged; found %d entries", n)
	}
	// No event_id on unauthenticated errors.
	res := e.post(e.router, "/api/member-status", event("ms_wrong_key_0123456789", e.member.ID.Hex(), "Pre", "started"), nil)
	if _, has := res.body["event_id"]; has {
		t.Error("unauthenticated error carried an event_id")
	}
}
