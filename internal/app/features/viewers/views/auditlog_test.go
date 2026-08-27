package views_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/dalemusser/stratahub/internal/app/features/viewers"
	"github.com/dalemusser/stratahub/internal/app/features/viewers/views"
	"github.com/dalemusser/stratahub/internal/app/store/audit"
	"github.com/dalemusser/stratahub/internal/testutil"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

// auditWorld seeds two organizations (A in Chicago, B without a zone), an
// admin and a coordinator of A, a member in each org, and six audit events.
type auditWorld struct {
	*world
	av                   *views.AuditLog
	eLogin, eUserUpdated primitive.ObjectID // alice signs in; admin edits alice (org A)
	eGhost               primitive.ObjectID // failed login, unknown user, no org
	eOrgB, eCarolLogin   primitive.ObjectID // admin edits org B; carol (org B) signs in
	eKeyChanged          primitive.ObjectID // admin rotates the API key, 40 days ago, no org
}

func newAuditWorld(t *testing.T) *auditWorld {
	t.Helper()
	base := newWorld(t)
	ctx, cancel := testutil.TestContext()
	defer cancel()
	now := time.Now().UTC()

	w := &auditWorld{world: base, av: views.NewAuditLog(base.db)}
	c := base.db.Collection("audit_events")
	add := func(at time.Time, e audit.Event) primitive.ObjectID {
		t.Helper()
		e.ID = primitive.NewObjectID()
		e.WorkspaceID = &w.ws
		e.Timestamp = at
		if _, err := c.InsertOne(ctx, e); err != nil {
			t.Fatal(err)
		}
		return e.ID
	}
	w.eLogin = add(now.Add(-1*time.Minute), audit.Event{Category: audit.CategoryAuth, EventType: audit.EventLoginSuccess,
		UserID: &w.alice, OrganizationID: &w.orgA, IP: "10.0.0.1", UserAgent: "TestBrowser/1.0", Success: true})
	w.eUserUpdated = add(now.Add(-2*time.Minute), audit.Event{Category: audit.CategoryAdmin, EventType: audit.EventUserUpdated,
		ActorID: &w.admin, UserID: &w.alice, OrganizationID: &w.orgA, IP: "10.0.0.9", Success: true, Details: map[string]string{"changed_fields": "full_name"}})
	w.eGhost = add(now.Add(-3*time.Minute), audit.Event{Category: audit.CategoryAuth, EventType: audit.EventLoginFailedUserNotFound,
		IP: "10.0.0.2", Success: false, FailureReason: "user not found", Details: map[string]string{"attempted_login_id": "ghost@example.org"}})
	w.eOrgB = add(now.Add(-4*time.Minute), audit.Event{Category: audit.CategoryAdmin, EventType: audit.EventOrgUpdated,
		ActorID: &w.admin, OrganizationID: &w.orgB, IP: "10.0.0.9", Success: true})
	w.eCarolLogin = add(now.Add(-5*time.Minute), audit.Event{Category: audit.CategoryAuth, EventType: audit.EventLoginSuccess,
		UserID: &w.carol, OrganizationID: &w.orgB, IP: "10.0.0.3", Success: true})
	w.eKeyChanged = add(now.Add(-40*24*time.Hour), audit.Event{Category: audit.CategoryAdmin, EventType: audit.EventMemberStatusKeyChanged,
		ActorID: &w.admin, IP: "10.0.0.9", Success: true, Details: map[string]string{"action": "rotated"}})
	return w
}

func TestAuditLog_Meta(t *testing.T) {
	w := newAuditWorld(t)
	v := w.av
	if v.Slug() != "audit-log" || v.Title() != "Audit Log" || v.Description() == "" {
		t.Errorf("meta: %s %s", v.Slug(), v.Title())
	}
	if !viewers.Allowed(v, "admin") || !viewers.Allowed(v, "coordinator") || viewers.Allowed(v, "leader") || viewers.Allowed(v, "analyst") {
		t.Errorf("roles: %v", v.Roles())
	}
	if _, ok := any(v).(viewers.Liveable); ok {
		t.Error("audit log should not be live")
	}
	if _, ok := any(v).(viewers.Summarizer); !ok {
		t.Error("audit log should summarize")
	}
	specs := v.Filters()
	if len(specs) != 7 || specs[0].Key != "when" || specs[0].Default != viewers.RangeAny {
		t.Errorf("filters: %+v", specs)
	}
	var event viewers.FilterSpec
	for _, s := range specs {
		if s.Key == "event" {
			event = s
		}
	}
	seen := map[string]bool{}
	for _, o := range event.Options {
		seen[o.Value] = true
	}
	for _, want := range []string{audit.EventLoginSuccess, audit.EventMagicLinkUsed, audit.EventGroupAppEnabled, audit.EventMemberStatusKeyChanged} {
		if !seen[want] {
			t.Errorf("event options missing %s", want)
		}
	}
	if len(v.Columns()) != 8 {
		t.Errorf("columns: %d", len(v.Columns()))
	}
}

func TestAuditLog_RowsAndScope(t *testing.T) {
	w := newAuditWorld(t)
	ctx := context.Background()

	// Admin, default filters (any time): every event, newest first.
	page, err := w.av.Query(ctx, w.scope(t, w.admin, "admin"), filters(w.av), nil, 50)
	if err != nil {
		t.Fatal(err)
	}
	expectIDs(t, "admin default", page.Rows, w.eLogin, w.eUserUpdated, w.eGhost, w.eOrgB, w.eCarolLogin, w.eKeyChanged)
	if page.Next != nil {
		t.Error("no more pages expected")
	}

	// Login (auth): the user is the actor; no target; org time zone; OK pill.
	r := page.Rows[0]
	if len(r.Cells) != 8 {
		t.Fatalf("cells: %d", len(r.Cells))
	}
	if !strings.Contains(r.Cells[0].Text, "CDT") && !strings.Contains(r.Cells[0].Text, "CST") {
		t.Errorf("time should be in the org's zone (Chicago): %q", r.Cells[0].Text)
	}
	if r.Cells[1].Text != "Authentication" || r.Cells[1].Class != viewers.PillBlue || r.Cells[2].Text != "Login success" ||
		r.Cells[3].Text != "Alice Cole" || r.Cells[4].Text != "" || r.Cells[5].Text != "Alpha" ||
		r.Cells[6].Text != "OK" || r.Cells[6].Class != viewers.PillGreen || r.Cells[7].Text != "10.0.0.1" {
		t.Errorf("login row: %+v", r.Cells)
	}
	// Admin action: actor and target both named.
	if u := page.Rows[1]; u.Cells[1].Text != "Administration" || u.Cells[1].Class != viewers.PillIndigo || u.Cells[2].Text != "User updated" ||
		u.Cells[3].Text != "Ada Admin" || u.Cells[4].Text != "Alice Cole" {
		t.Errorf("user-updated row: %+v", u.Cells)
	}
	// Failed login for an unknown user: attempted login id, muted; Failed pill with the reason as tooltip; UTC.
	if g := page.Rows[2]; g.Cells[3].Text != "ghost@example.org" || g.Cells[3].Class != viewers.TextMuted || g.Cells[4].Text != "" ||
		g.Cells[6].Text != "Failed" || g.Cells[6].Class != viewers.PillRed || g.Cells[6].Title != "user not found" || !strings.HasSuffix(g.Cells[0].Text, "UTC") {
		t.Errorf("ghost row: %+v", g.Cells)
	}

	// Coordinator of org A sees org A's events only: no org-less, no org B.
	page, err = w.av.Query(ctx, w.scope(t, w.coord, "coordinator"), filters(w.av), nil, 50)
	if err != nil {
		t.Fatal(err)
	}
	expectIDs(t, "coordinator", page.Rows, w.eLogin, w.eUserUpdated)
}

func TestAuditLog_Filters(t *testing.T) {
	w := newAuditWorld(t)
	ctx := context.Background()
	admin := w.scope(t, w.admin, "admin")
	q := func(kv ...string) []viewers.Row {
		t.Helper()
		page, err := w.av.Query(ctx, admin, filters(w.av, kv...), nil, 50)
		if err != nil {
			t.Fatal(err)
		}
		return page.Rows
	}

	expectIDs(t, "last 7 days", q("when", viewers.RangeWeek), w.eLogin, w.eUserUpdated, w.eGhost, w.eOrgB, w.eCarolLogin)
	expectIDs(t, "category auth", q("category", audit.CategoryAuth), w.eLogin, w.eGhost, w.eCarolLogin)
	expectIDs(t, "category admin", q("category", audit.CategoryAdmin), w.eUserUpdated, w.eOrgB, w.eKeyChanged)
	expectIDs(t, "event login_success", q("event", audit.EventLoginSuccess), w.eLogin, w.eCarolLogin)
	expectIDs(t, "result failure", q("result", "failure"), w.eGhost)
	expectIDs(t, "result success", q("result", "success"), w.eLogin, w.eUserUpdated, w.eOrgB, w.eCarolLogin, w.eKeyChanged)
	expectIDs(t, "person by name prefix (target)", q("person", "ali"), w.eLogin, w.eUserUpdated)
	expectIDs(t, "person by name prefix (actor)", q("person", "ADA"), w.eUserUpdated, w.eOrgB, w.eKeyChanged)
	expectIDs(t, "person by hex", q("person", w.carol.Hex()), w.eCarolLogin)
	expectIDs(t, "person no match", q("person", "zzz"))
	expectIDs(t, "ip", q("ip", "10.0.0.9"), w.eUserUpdated, w.eOrgB, w.eKeyChanged)
	expectIDs(t, "ip no match", q("ip", "10.0.0.99"))
	expectIDs(t, "event id", q("id", w.eOrgB.Hex()), w.eOrgB)
	expectIDs(t, "event id bogus", q("id", "not-hex"))
	expectIDs(t, "combined", q("person", "ada", "category", audit.CategoryAdmin, "when", viewers.RangeWeek), w.eUserUpdated, w.eOrgB)
}

func TestAuditLog_PagingDetailSummary(t *testing.T) {
	w := newAuditWorld(t)
	ctx := context.Background()
	admin := w.scope(t, w.admin, "admin")

	p1, _ := w.av.Query(ctx, admin, filters(w.av), nil, 4)
	expectIDs(t, "p1", p1.Rows, w.eLogin, w.eUserUpdated, w.eGhost, w.eOrgB)
	if p1.Next == nil {
		t.Fatal("p1 should have a next cursor")
	}
	p2, _ := w.av.Query(ctx, admin, filters(w.av), p1.Next, 4)
	expectIDs(t, "p2", p2.Rows, w.eCarolLogin, w.eKeyChanged)
	if p2.Next != nil {
		t.Error("p2 should be the last page")
	}

	// Detail of the failed login: fields, details map, raw JSON.
	d, err := w.av.Detail(ctx, admin, w.eGhost.Hex())
	if err != nil {
		t.Fatal(err)
	}
	if d.Title != "Login failed user not found" || d.Subtitle != "Authentication · Failed — user not found" || !strings.Contains(d.RawJSON, `"attempted_login_id": "ghost@example.org"`) {
		t.Errorf("detail: %+v", d)
	}
	joined := ""
	for _, f := range d.Fields {
		joined += f.Label + "=" + f.Value + ";"
	}
	for _, want := range []string{"Event id=" + w.eGhost.Hex(), "Category=Authentication", "Actor=ghost@example.org", "IP address=10.0.0.2", "Result=Failed — user not found", "Attempted login id=ghost@example.org"} {
		if !strings.Contains(joined, want) {
			t.Errorf("detail fields missing %q in %s", want, joined)
		}
	}
	if strings.Contains(joined, "Target=") || strings.Contains(joined, "Organization=") || strings.Contains(joined, "User agent=") {
		t.Errorf("detail has fields it should omit: %s", joined)
	}
	d, _ = w.av.Detail(ctx, admin, w.eUserUpdated.Hex())
	joined = ""
	for _, f := range d.Fields {
		joined += f.Label + "=" + f.Value + ";"
	}
	for _, want := range []string{"Actor=Ada Admin  " + w.admin.Hex(), "Target=Alice Cole  " + w.alice.Hex(), "Organization=Alpha  " + w.orgA.Hex(), "Result=Succeeded", "Changed fields=full_name"} {
		if !strings.Contains(joined, want) {
			t.Errorf("user-updated detail missing %q in %s", want, joined)
		}
	}
	if _, err := w.av.Detail(ctx, w.scope(t, w.coord, "coordinator"), w.eCarolLogin.Hex()); err != viewers.ErrNotFound {
		t.Errorf("coordinator detail of another org's event: %v, want ErrNotFound", err)
	}
	if _, err := w.av.Detail(ctx, admin, "nope"); err != viewers.ErrNotFound {
		t.Errorf("bad id: %v", err)
	}

	// Summary chips.
	chip := func(f viewers.Filters) map[string]viewers.Chip {
		t.Helper()
		chips, err := w.av.Summary(ctx, admin, f)
		if err != nil {
			t.Fatal(err)
		}
		m := map[string]viewers.Chip{}
		for _, c := range chips {
			m[c.Label] = c
		}
		return m
	}
	got := chip(filters(w.av))
	if got["Events"].Value != "6" || got["Failed"].Value != "1" || got["Failed"].Class != viewers.TextRed {
		t.Errorf("chips: %v", got)
	}
	got = chip(filters(w.av, "result", "success"))
	if got["Events"].Value != "5" || got["Failed"].Value != "0" || got["Failed"].Class != "" {
		t.Errorf("success-only chips: %v", got)
	}
	got = chip(filters(w.av, "person", "zzz"))
	if len(got) != 1 || got["Events"].Value != "0" {
		t.Errorf("no-match chips: %v", got)
	}
}
