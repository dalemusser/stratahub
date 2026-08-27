package views_test

import (
	"context"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/dalemusser/stratahub/internal/app/features/viewers"
	"github.com/dalemusser/stratahub/internal/app/features/viewers/views"
	"github.com/dalemusser/stratahub/internal/app/store/memberstatuslog"
	"github.com/dalemusser/stratahub/internal/app/system/auth"
	"github.com/dalemusser/stratahub/internal/app/system/viewscope"
	"github.com/dalemusser/stratahub/internal/app/system/workspace"
	"github.com/dalemusser/stratahub/internal/domain/models"
	"github.com/dalemusser/stratahub/internal/testutil"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
)

type world struct {
	db                 *mongo.Database
	v                  *views.SurveyEvents
	ws, orgA, orgB     primitive.ObjectID
	groupA             primitive.ObjectID
	alice, bob, carol  primitive.ObjectID // alice, bob in org A (group A); carol in org B
	leader, coord      primitive.ObjectID
	admin              primitive.ObjectID
	eAccepted, eLaunch primitive.ObjectID
	eUnknown, eUnrecog primitive.ObjectID
	eCarol, eOld       primitive.ObjectID
}

func newWorld(t *testing.T) *world {
	t.Helper()
	db := testutil.SetupTestDB(t)
	ctx, cancel := testutil.TestContext()
	defer cancel()
	now := time.Now().UTC()
	oid := primitive.NewObjectID

	w := &world{db: db, ws: oid(), orgA: oid(), orgB: oid(), groupA: oid(),
		alice: oid(), bob: oid(), carol: oid(), leader: oid(), coord: oid(), admin: oid()}

	must := func(_ any, err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	must(db.Collection("organizations").InsertMany(ctx, []any{
		bson.M{"_id": w.orgA, "workspace_id": w.ws, "name": "Alpha", "name_ci": "alpha", "status": "active", "time_zone": "America/Chicago", "created_at": now},
		bson.M{"_id": w.orgB, "workspace_id": w.ws, "name": "Beta", "name_ci": "beta", "status": "active", "created_at": now},
	}))
	must(db.Collection("groups").InsertOne(ctx, bson.M{"_id": w.groupA, "workspace_id": w.ws, "organization_id": w.orgA, "name": "A1", "name_ci": "a1", "status": "active", "created_at": now}))
	user := func(id, org primitive.ObjectID, role, name string) bson.M {
		login := strings.ToLower(strings.ReplaceAll(name, " ", ".")) + "@example.org"
		return bson.M{"_id": id, "workspace_id": w.ws, "organization_id": org, "role": role, "status": "active", "full_name": name, "full_name_ci": strings.ToLower(name),
			"login_id": login, "login_id_ci": login, "auth_method": "trust", "created_at": now, "updated_at": now}
	}
	must(db.Collection("users").InsertMany(ctx, []any{
		user(w.alice, w.orgA, "member", "Alice Cole"), user(w.bob, w.orgA, "member", "Bob Ng"), user(w.carol, w.orgB, "member", "Carol Diaz"),
		user(w.leader, w.orgA, "leader", "Lee Leader"), user(w.coord, w.orgA, "coordinator", "Cory Coord"), user(w.admin, w.orgA, "admin", "Ada Admin"),
	}))
	membership := func(uid primitive.ObjectID, role string) bson.M {
		return bson.M{"_id": oid(), "workspace_id": w.ws, "group_id": w.groupA, "org_id": w.orgA, "user_id": uid, "role": role, "created_at": now}
	}
	must(db.Collection("group_memberships").InsertMany(ctx, []any{membership(w.alice, "member"), membership(w.bob, "member"), membership(w.leader, "leader")}))
	must(db.Collection("coordinator_assignments").InsertOne(ctx, bson.M{"_id": oid(), "user_id": w.coord, "organization_id": w.orgA, "created_at": now}))

	log := memberstatuslog.New(db)
	add := func(at time.Time, e models.MemberStatusLogEntry) primitive.ObjectID {
		t.Helper()
		e.WorkspaceID = w.ws
		e.ReceivedAt = at
		saved, err := log.Append(ctx, e)
		if err != nil {
			t.Fatal(err)
		}
		return saved.ID
	}
	// Alice: accepted "MHS Engagement" completed via API, 1 minute ago.
	w.eAccepted = add(now.Add(-1*time.Minute), models.MemberStatusLogEntry{
		Source: models.MemberStatusSourceAPI, RemoteIP: "203.0.113.5",
		Request:  models.MemberStatusLogRequest{UserID: w.alice.Hex(), Entity: "mhs engagement", State: "Completed", OccurredAt: "2026-08-25T14:03:11Z"},
		Resolved: models.MemberStatusLogResolved{UserID: &w.alice, OrganizationID: &w.orgA, EntityKey: "mhs", EntityTitle: "MHS Engagement", KnownEntity: true, StateApplied: "completed", ResultingState: "completed"},
		Outcome:  models.MemberStatusLogOutcome{HTTPStatus: 200},
	})
	// Bob: launch of the Pre survey, 2 minutes ago.
	w.eLaunch = add(now.Add(-2*time.Minute), models.MemberStatusLogEntry{
		Source:   models.MemberStatusSourceLaunch,
		Request:  models.MemberStatusLogRequest{UserID: w.bob.Hex(), Entity: "Pre-Survey", State: "opened", ResourceID: oid().Hex()},
		Resolved: models.MemberStatusLogResolved{UserID: &w.bob, OrganizationID: &w.orgA, EntityKey: "pre", EntityTitle: "Pre-Survey", KnownEntity: true, StateApplied: "opened", ResultingState: "opened"},
		Outcome:  models.MemberStatusLogOutcome{HTTPStatus: 200},
	})
	// Unknown user (rejected), 3 minutes ago.
	w.eUnknown = add(now.Add(-3*time.Minute), models.MemberStatusLogEntry{
		Source: models.MemberStatusSourceAPI, RemoteIP: "203.0.113.5",
		Request: models.MemberStatusLogRequest{UserID: "0123456789abcdef01234567", Entity: "Pre", State: "started"},
		Outcome: models.MemberStatusLogOutcome{HTTPStatus: 404, Error: "unknown_user", Message: "No active member with that user_id in this workspace."},
	})
	// Alice: unrecognized survey name (accepted, flagged), 4 minutes ago.
	w.eUnrecog = add(now.Add(-4*time.Minute), models.MemberStatusLogEntry{
		Source:   models.MemberStatusSourceAPI,
		Request:  models.MemberStatusLogRequest{UserID: w.alice.Hex(), Entity: "Mid Survey", State: "started"},
		Resolved: models.MemberStatusLogResolved{UserID: &w.alice, OrganizationID: &w.orgA, EntityKey: "name:mid survey", EntityTitle: "Mid Survey", KnownEntity: false, StateApplied: "started", ResultingState: "started"},
		Outcome:  models.MemberStatusLogOutcome{HTTPStatus: 200},
	})
	// Carol (org B): accepted, 5 minutes ago.
	w.eCarol = add(now.Add(-5*time.Minute), models.MemberStatusLogEntry{
		Source:   models.MemberStatusSourceAPI,
		Request:  models.MemberStatusLogRequest{UserID: w.carol.Hex(), Entity: "Post", State: "completed"},
		Resolved: models.MemberStatusLogResolved{UserID: &w.carol, OrganizationID: &w.orgB, EntityKey: "post", EntityTitle: "Post-Survey", KnownEntity: true, StateApplied: "completed", ResultingState: "completed"},
		Outcome:  models.MemberStatusLogOutcome{HTTPStatus: 200},
	})
	// Alice: old accepted event outside the default 7-day window.
	w.eOld = add(now.Add(-30*24*time.Hour), models.MemberStatusLogEntry{
		Source:   models.MemberStatusSourceAPI,
		Request:  models.MemberStatusLogRequest{UserID: w.alice.Hex(), Entity: "Pre", State: "started"},
		Resolved: models.MemberStatusLogResolved{UserID: &w.alice, OrganizationID: &w.orgA, EntityKey: "pre", EntityTitle: "Pre-Survey", KnownEntity: true, StateApplied: "started", ResultingState: "started"},
		Outcome:  models.MemberStatusLogOutcome{HTTPStatus: 200},
	})

	v, err := views.NewSurveyEvents(db)
	if err != nil {
		t.Fatal(err)
	}
	w.v = v
	return w
}

func (w *world) scope(t *testing.T, userID primitive.ObjectID, role string) *viewscope.Scope {
	t.Helper()
	r := httptest.NewRequest("GET", "/views/survey-events", nil)
	r = workspace.WithTestWorkspace(r, w.ws, "mhs", "MHS")
	r = auth.WithTestUser(r, &auth.SessionUser{ID: userID.Hex(), Name: role, LoginID: role + "@example.org", Role: role})
	s, err := viewscope.Resolve(context.Background(), w.db, r, viewscope.Options{})
	if err != nil {
		t.Fatalf("scope(%s): %v", role, err)
	}
	return s
}

func filters(v viewers.Viewer, kv ...string) viewers.Filters {
	q := url.Values{}
	for i := 0; i+1 < len(kv); i += 2 {
		q.Set(kv[i], kv[i+1])
	}
	return viewers.ParseFilters(v.Filters(), q)
}

func ids(rows []viewers.Row) []string {
	out := make([]string, len(rows))
	for i, r := range rows {
		out[i] = r.ID
	}
	return out
}

func expectIDs(t *testing.T, label string, rows []viewers.Row, want ...primitive.ObjectID) {
	t.Helper()
	got := ids(rows)
	w := make([]string, len(want))
	for i, id := range want {
		w[i] = id.Hex()
	}
	if strings.Join(got, ",") != strings.Join(w, ",") {
		t.Errorf("%s: got %v, want %v", label, got, w)
	}
}

func TestSurveyEvents_Meta(t *testing.T) {
	w := newWorld(t)
	if w.v.Slug() != "survey-events" || w.v.Title() != "Survey Events" || w.v.LiveIntervalSeconds() != 10 {
		t.Errorf("meta: %s %s %d", w.v.Slug(), w.v.Title(), w.v.LiveIntervalSeconds())
	}
	if len(w.v.Roles()) != 4 || !viewers.Allowed(w.v, "analyst") || viewers.Allowed(w.v, "member") {
		t.Errorf("roles: %v", w.v.Roles())
	}
	specs := w.v.Filters()
	if len(specs) != 7 || specs[0].Key != "when" || specs[0].Default != viewers.RangeWeek {
		t.Errorf("filters: %+v", specs)
	}
	// Survey options: the four configured plus "unrecognized".
	var entity viewers.FilterSpec
	for _, s := range specs {
		if s.Key == "entity" {
			entity = s
		}
	}
	if len(entity.Options) != 5 || entity.Options[0].Value != "pre" || entity.Options[4].Value != memberstatuslog.EntityUnrecognized {
		t.Errorf("survey options: %+v", entity.Options)
	}
	if len(w.v.Columns()) != 6 {
		t.Errorf("columns: %d", len(w.v.Columns()))
	}
}

func TestSurveyEvents_QueryRowsAndScope(t *testing.T) {
	w := newWorld(t)
	ctx := context.Background()

	// Admin, default filters (last 7 days): everything but the old event, newest first.
	page, err := w.v.Query(ctx, w.scope(t, w.admin, "admin"), filters(w.v), nil, 50)
	if err != nil {
		t.Fatal(err)
	}
	expectIDs(t, "admin default", page.Rows, w.eAccepted, w.eLaunch, w.eUnknown, w.eUnrecog, w.eCarol)
	if page.Next != nil {
		t.Error("no more pages expected")
	}

	// Cell content of the accepted row.
	r := page.Rows[0]
	if len(r.Cells) != 6 {
		t.Fatalf("cells: %d", len(r.Cells))
	}
	if !strings.Contains(r.Cells[0].Text, "CDT") && !strings.Contains(r.Cells[0].Text, "CST") {
		t.Errorf("received should be in the org's zone (Chicago): %q", r.Cells[0].Text)
	}
	if r.Cells[1].Text != "Provider" || r.Cells[2].Text != "Alice Cole" || !strings.Contains(r.Cells[2].Href, "student="+w.alice.Hex()) ||
		r.Cells[3].Text != "MHS Engagement" || r.Cells[4].Text != "Completed" || r.Cells[5].Text != "Accepted · Completed" || r.Cells[5].Class != viewers.PillGreen {
		t.Errorf("accepted row cells: %+v", r.Cells)
	}
	// Launch row, unknown-user row, unrecognized-name row.
	if l := page.Rows[1]; l.Cells[1].Text != "Launch" || l.Cells[2].Text != "Bob Ng" || l.Cells[4].Text != "Opened" || l.Cells[5].Class != viewers.PillBlue {
		t.Errorf("launch row: %+v", l.Cells)
	}
	if u := page.Rows[2]; u.Cells[2].Text != "0123456789abcdef01234567" || u.Cells[2].Href != "" || !strings.HasPrefix(u.Cells[5].Text, "Rejected · unknown_user") || u.Cells[5].Class != viewers.PillRed {
		t.Errorf("unknown-user row: %+v", u.Cells)
	}
	if x := page.Rows[3]; !strings.HasPrefix(x.Cells[3].Text, "Unrecognized: Mid Survey") || x.Cells[3].Class != viewers.PillAmber {
		t.Errorf("unrecognized row: %+v", x.Cells)
	}

	// "Any time" brings the old event back.
	page, _ = w.v.Query(ctx, w.scope(t, w.admin, "admin"), filters(w.v, "when", ""), nil, 50)
	expectIDs(t, "any time", page.Rows, w.eAccepted, w.eLaunch, w.eUnknown, w.eUnrecog, w.eCarol, w.eOld)

	// Leader of group A: Alice and Bob only; the unknown-user row (no member) is hidden.
	page, _ = w.v.Query(ctx, w.scope(t, w.leader, "leader"), filters(w.v), nil, 50)
	expectIDs(t, "leader", page.Rows, w.eAccepted, w.eLaunch, w.eUnrecog)

	// Coordinator of org A: same reach by organization.
	page, _ = w.v.Query(ctx, w.scope(t, w.coord, "coordinator"), filters(w.v), nil, 50)
	expectIDs(t, "coordinator", page.Rows, w.eAccepted, w.eLaunch, w.eUnrecog)
}

func TestSurveyEvents_Filters(t *testing.T) {
	w := newWorld(t)
	ctx := context.Background()
	admin := w.scope(t, w.admin, "admin")
	q := func(kv ...string) []viewers.Row {
		t.Helper()
		page, err := w.v.Query(ctx, admin, filters(w.v, kv...), nil, 50)
		if err != nil {
			t.Fatal(err)
		}
		return page.Rows
	}

	expectIDs(t, "source launch", q("source", "launch"), w.eLaunch)
	expectIDs(t, "survey pre", q("entity", "pre"), w.eLaunch)
	expectIDs(t, "unrecognized", q("entity", memberstatuslog.EntityUnrecognized), w.eUnrecog)
	expectIDs(t, "state completed", q("state", "completed"), w.eAccepted, w.eCarol)
	expectIDs(t, "rejected", q("outcome", memberstatuslog.OutcomeRejected), w.eUnknown)
	expectIDs(t, "unknown_user", q("outcome", "unknown_user"), w.eUnknown)
	expectIDs(t, "accepted", q("outcome", memberstatuslog.OutcomeAccepted), w.eAccepted, w.eLaunch, w.eUnrecog, w.eCarol)
	expectIDs(t, "student by name prefix", q("student", "ali"), w.eAccepted, w.eUnrecog)
	expectIDs(t, "student by name, case-insensitive", q("student", "BOB"), w.eLaunch)
	expectIDs(t, "student by hex (resolved)", q("student", w.carol.Hex()), w.eCarol)
	expectIDs(t, "student by hex (unresolved)", q("student", "0123456789abcdef01234567"), w.eUnknown)
	expectIDs(t, "student no match", q("student", "zzz"))
	expectIDs(t, "event id", q("event", w.eLaunch.Hex(), "when", ""), w.eLaunch)
	expectIDs(t, "event id bogus", q("event", "not-hex"))
	expectIDs(t, "combined", q("student", "ali", "state", "started"), w.eUnrecog)
}

func TestSurveyEvents_PagingDetailSummary(t *testing.T) {
	w := newWorld(t)
	ctx := context.Background()
	admin := w.scope(t, w.admin, "admin")

	// Two per page, walked with cursors.
	p1, _ := w.v.Query(ctx, admin, filters(w.v), nil, 2)
	expectIDs(t, "p1", p1.Rows, w.eAccepted, w.eLaunch)
	if p1.Next == nil {
		t.Fatal("p1 should have a next cursor")
	}
	p2, _ := w.v.Query(ctx, admin, filters(w.v), p1.Next, 2)
	expectIDs(t, "p2", p2.Rows, w.eUnknown, w.eUnrecog)
	p3, _ := w.v.Query(ctx, admin, filters(w.v), p2.Next, 2)
	expectIDs(t, "p3", p3.Rows, w.eCarol)
	if p3.Next != nil {
		t.Error("p3 should be the last page")
	}

	// Detail: fields and raw JSON; scope applies.
	d, err := w.v.Detail(ctx, admin, w.eAccepted.Hex())
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(d.Title, w.eAccepted.Hex()) || d.Subtitle != "Accepted" || !strings.Contains(d.RawJSON, `"entity_key": "mhs"`) {
		t.Errorf("detail: %+v", d)
	}
	joined := ""
	for _, f := range d.Fields {
		joined += f.Label + "=" + f.Value + ";"
	}
	for _, want := range []string{"Sent entity=mhs engagement", "Sent state=Completed", "Alice Cole", "Survey=MHS Engagement (recognized, key mhs)", "Student's status after=Completed", "Outcome=200 accepted"} {
		if !strings.Contains(joined, want) {
			t.Errorf("detail fields missing %q in %s", want, joined)
		}
	}
	d, _ = w.v.Detail(ctx, admin, w.eUnknown.Hex())
	if d == nil || d.Subtitle != "Rejected: unknown_user" {
		t.Errorf("rejected detail: %+v", d)
	}
	if _, err := w.v.Detail(ctx, w.scope(t, w.leader, "leader"), w.eCarol.Hex()); err != viewers.ErrNotFound {
		t.Errorf("leader detail of another org's event: %v, want ErrNotFound", err)
	}
	if _, err := w.v.Detail(ctx, admin, "nope"); err != viewers.ErrNotFound {
		t.Errorf("bad id: %v", err)
	}

	// Summary chips.
	chips, err := w.v.Summary(ctx, admin, filters(w.v))
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]string{}
	for _, c := range chips {
		got[c.Label] = c.Value
	}
	if got["Events"] != "5" || got["Accepted"] != "4" || got["Rejected"] != "1" || got["Last received"] != "1 min ago" {
		t.Errorf("chips: %v", got)
	}
	chips, _ = w.v.Summary(ctx, admin, filters(w.v, "student", "zzz"))
	if len(chips) != 1 || chips[0].Value != "0" {
		t.Errorf("no-match summary: %+v", chips)
	}
}
