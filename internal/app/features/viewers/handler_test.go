package viewers_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	uierrors "github.com/dalemusser/stratahub/internal/app/features/errors"
	_ "github.com/dalemusser/stratahub/internal/app/features/missionhydrosci" // registers a template func the shared layout uses
	"github.com/dalemusser/stratahub/internal/app/features/viewers"
	appresources "github.com/dalemusser/stratahub/internal/app/resources"
	"github.com/dalemusser/stratahub/internal/app/system/auth"
	"github.com/dalemusser/stratahub/internal/app/system/viewscope"
	"github.com/dalemusser/stratahub/internal/app/system/workspace"
	"github.com/dalemusser/stratahub/internal/testutil"
	"github.com/dalemusser/waffle/pantry/templates"
	"github.com/go-chi/chi/v5"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.uber.org/zap"
)

// --- template engine ---------------------------------------------------------

var bootOnce sync.Once

func bootTemplates(t *testing.T) {
	t.Helper()
	var bootErr error
	bootOnce.Do(func() {
		appresources.LoadSharedTemplates()
		eng := templates.New(false)
		if err := eng.Boot(zap.NewNop()); err != nil {
			bootErr = err
			return
		}
		templates.UseEngine(eng, zap.NewNop())
	})
	if bootErr != nil {
		t.Fatalf("template engine boot failed: %v", bootErr)
	}
}

// --- a fake viewer over in-memory rows ----------------------------------------

type fakeRow struct {
	ID   primitive.ObjectID
	When time.Time
	User primitive.ObjectID
	Org  primitive.ObjectID
	Kind string
	Text string
}

type fakeViewer struct {
	rows  []fakeRow
	roles []string
	live  int

	mu         sync.Mutex
	lastFilter bson.M
	queries    int
}

func (f *fakeViewer) Slug() string        { return "fake" }
func (f *fakeViewer) Title() string       { return "Fake Events" }
func (f *fakeViewer) Description() string { return "In-memory rows for tests." }
func (f *fakeViewer) Roles() []string     { return f.roles }
func (f *fakeViewer) Filters() []viewers.FilterSpec {
	return []viewers.FilterSpec{
		{Key: "q", Label: "Search", Type: viewers.FilterText, Placeholder: "text"},
		{Key: "kind", Label: "Kind", Type: viewers.FilterSelect, Options: []viewers.Option{{Value: "a", Label: "Kind A"}, {Value: "b", Label: "Kind B"}}},
		{Key: "when", Label: "When", Type: viewers.FilterDateRange},
		{Key: "who", Label: "User id", Type: viewers.FilterID, Placeholder: "24-char hex"},
	}
}
func (f *fakeViewer) Columns() []viewers.ColumnSpec {
	return []viewers.ColumnSpec{{Key: "when", Label: "When", Class: "whitespace-nowrap"}, {Key: "kind", Label: "Kind"}, {Key: "text", Label: "Text"}}
}
func (f *fakeViewer) LiveIntervalSeconds() int { return f.live }
func (f *fakeViewer) Summary(ctx context.Context, scope *viewscope.Scope, fl viewers.Filters) ([]viewers.Chip, error) {
	rows, err := f.visible(ctx, scope, fl)
	if err != nil {
		return nil, err
	}
	return []viewers.Chip{{Label: "Rows", Value: fmt.Sprint(len(rows))}}, nil
}

// visible applies scope and filters, newest first.
func (f *fakeViewer) visible(ctx context.Context, scope *viewscope.Scope, fl viewers.Filters) ([]fakeRow, error) {
	filter, err := scope.Filter(ctx, "org_id", "user_id")
	if err != nil {
		return nil, err
	}
	f.mu.Lock()
	f.lastFilter = filter
	f.queries++
	f.mu.Unlock()

	allowed := func(r fakeRow) bool {
		if filter == nil {
			return true
		}
		if m, ok := filter["org_id"].(bson.M); ok {
			return containsID(m["$in"].([]primitive.ObjectID), r.Org)
		}
		if m, ok := filter["user_id"].(bson.M); ok {
			return containsID(m["$in"].([]primitive.ObjectID), r.User)
		}
		return false
	}
	from, to, ranged := fl.DateRange("when", time.Now().UTC())
	who, hasWho := fl.ID("who")

	var out []fakeRow
	for _, r := range f.rows {
		if !allowed(r) {
			continue
		}
		if q := fl.Get("q"); q != "" && !strings.Contains(strings.ToLower(r.Text), strings.ToLower(q)) {
			continue
		}
		if k := fl.Get("kind"); k != "" && r.Kind != k {
			continue
		}
		if ranged && ((!from.IsZero() && r.When.Before(from)) || (!to.IsZero() && r.When.After(to))) {
			continue
		}
		if hasWho && r.User != who {
			continue
		}
		out = append(out, r)
	}
	sort.Slice(out, func(i, j int) bool {
		if !out[i].When.Equal(out[j].When) {
			return out[i].When.After(out[j].When)
		}
		return out[i].ID.Hex() > out[j].ID.Hex()
	})
	return out, nil
}

func (f *fakeViewer) Query(ctx context.Context, scope *viewscope.Scope, fl viewers.Filters, after *viewers.Cursor, limit int) (viewers.Page, error) {
	rows, err := f.visible(ctx, scope, fl)
	if err != nil {
		return viewers.Page{}, err
	}
	if after != nil {
		key, err := viewers.ParseTimeKey(after.Key)
		if err != nil {
			return viewers.Page{}, err
		}
		var rest []fakeRow
		for _, r := range rows {
			if r.When.Before(key) || (r.When.Equal(key) && r.ID.Hex() < after.ID.Hex()) {
				rest = append(rest, r)
			}
		}
		rows = rest
	}
	page := viewers.Page{}
	total := int64(len(rows))
	page.Total = &total
	for i, r := range rows {
		if i == limit {
			last := rows[i-1]
			page.Next = &viewers.Cursor{Key: viewers.TimeKey(last.When), ID: last.ID}
			break
		}
		page.Rows = append(page.Rows, viewers.Row{ID: r.ID.Hex(), Cells: []viewers.Cell{
			{Text: r.When.UTC().Format("Jan 2 15:04")},
			{Text: r.Kind, Class: viewers.PillBlue},
			{Text: r.Text},
		}})
	}
	return page, nil
}

func (f *fakeViewer) Detail(ctx context.Context, scope *viewscope.Scope, id string) (*viewers.Detail, error) {
	rows, err := f.visible(ctx, scope, viewers.ParseFilters(f.Filters(), nil))
	if err != nil {
		return nil, err
	}
	for _, r := range rows {
		if r.ID.Hex() == id {
			raw, _ := json.Marshal(map[string]string{"id": id, "text": r.Text})
			return &viewers.Detail{Title: r.Text, Subtitle: "detail of " + id, Fields: []viewers.Field{{Label: "Kind", Value: r.Kind}}, RawJSON: string(raw)}, nil
		}
	}
	return nil, viewers.ErrNotFound
}

func containsID(ids []primitive.ObjectID, id primitive.ObjectID) bool {
	for _, x := range ids {
		if x == id {
			return true
		}
	}
	return false
}

// --- world: workspace with two orgs, a group, members, a leader, a coordinator --

type world struct {
	db                               *mongo.Database
	ws, orgA, orgB, groupA           primitive.ObjectID
	m1, m2, mB, leader, coord, admin primitive.ObjectID
	analyst, member                  primitive.ObjectID
	rows                             []fakeRow
	router                           http.Handler
	fake                             *fakeViewer
	handler                          *viewers.Handler
}

func newWorld(t *testing.T, roles []string, live int) *world {
	t.Helper()
	bootTemplates(t)
	db := testutil.SetupTestDB(t)
	ctx, cancel := testutil.TestContext()
	defer cancel()
	now := time.Now().UTC()
	oid := primitive.NewObjectID

	w := &world{db: db, ws: oid(), orgA: oid(), orgB: oid(), groupA: oid(),
		m1: oid(), m2: oid(), mB: oid(), leader: oid(), coord: oid(), admin: oid(), analyst: oid(), member: oid()}

	must := func(_ interface{}, err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	must(db.Collection("organizations").InsertMany(ctx, []interface{}{
		bson.M{"_id": w.orgA, "workspace_id": w.ws, "name": "Alpha", "name_ci": "alpha", "status": "active", "created_at": now},
		bson.M{"_id": w.orgB, "workspace_id": w.ws, "name": "Beta", "name_ci": "beta", "status": "active", "created_at": now},
	}))
	must(db.Collection("groups").InsertOne(ctx, bson.M{"_id": w.groupA, "workspace_id": w.ws, "organization_id": w.orgA, "name": "A1", "name_ci": "a1", "status": "active", "created_at": now}))
	user := func(id, org primitive.ObjectID, role, name string) bson.M {
		login := name + "@example.org"
		return bson.M{"_id": id, "workspace_id": w.ws, "organization_id": org, "role": role, "status": "active", "full_name": name, "full_name_ci": name,
			"login_id": login, "login_id_ci": login, "auth_method": "trust", "created_at": now, "updated_at": now}
	}
	must(db.Collection("users").InsertMany(ctx, []interface{}{
		user(w.m1, w.orgA, "member", "m1"), user(w.m2, w.orgA, "member", "m2"), user(w.mB, w.orgB, "member", "mb"),
		user(w.leader, w.orgA, "leader", "leader"), user(w.coord, w.orgA, "coordinator", "coord"),
		user(w.admin, w.orgA, "admin", "admin"), user(w.analyst, w.orgA, "analyst", "analyst"), user(w.member, w.orgA, "member", "member"),
	}))
	membership := func(uid primitive.ObjectID, role string) bson.M {
		return bson.M{"_id": oid(), "workspace_id": w.ws, "group_id": w.groupA, "org_id": w.orgA, "user_id": uid, "role": role, "created_at": now}
	}
	must(db.Collection("group_memberships").InsertMany(ctx, []interface{}{membership(w.m1, "member"), membership(w.m2, "member"), membership(w.leader, "leader")}))
	must(db.Collection("coordinator_assignments").InsertOne(ctx, bson.M{"_id": oid(), "user_id": w.coord, "organization_id": w.orgA, "created_at": now}))

	// Five rows: three recent in org A (two for m1, one for m2), one recent in org B, one old in org A.
	w.rows = []fakeRow{
		{ID: oid(), When: now.Add(-1 * time.Minute), User: w.m1, Org: w.orgA, Kind: "a", Text: "m1 newest"},
		{ID: oid(), When: now.Add(-2 * time.Minute), User: w.m2, Org: w.orgA, Kind: "b", Text: "m2 recent"},
		{ID: oid(), When: now.Add(-3 * time.Minute), User: w.mB, Org: w.orgB, Kind: "a", Text: "mb recent"},
		{ID: oid(), When: now.Add(-4 * time.Minute), User: w.m1, Org: w.orgA, Kind: "a", Text: "m1 second"},
		{ID: oid(), When: now.Add(-40 * 24 * time.Hour), User: w.m1, Org: w.orgA, Kind: "b", Text: "m1 ancient"},
	}
	w.fake = &fakeViewer{rows: w.rows, roles: roles, live: live}

	reg := viewers.NewRegistry()
	reg.Register(w.fake)
	w.handler = viewers.NewHandler(db, reg, uierrors.NewErrorLogger(zap.NewNop()), zap.NewNop())

	sm, err := auth.NewSessionManager("test-session-key-must-be-32-chars-long", "test-session", "", time.Hour, false, zap.NewNop())
	if err != nil {
		t.Fatal(err)
	}
	r := chi.NewRouter()
	r.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
			next.ServeHTTP(rw, workspace.WithTestWorkspace(req, w.ws, "mhs", "MHS"))
		})
	})
	r.Mount("/views", viewers.Routes(w.handler, sm))
	w.router = r
	return w
}

func (w *world) get(t *testing.T, path string, userID primitive.ObjectID, role string, htmx bool) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest("GET", path, nil)
	if role != "" {
		req = auth.WithTestUser(req, &auth.SessionUser{ID: userID.Hex(), Name: role, LoginID: role + "@example.org", Role: role})
	}
	if htmx {
		req.Header.Set("HX-Request", "true")
	} else {
		req.Header.Set("Accept", "text/html") // a browser navigation
	}
	rec := httptest.NewRecorder()
	w.router.ServeHTTP(rec, req)
	return rec
}

func rowIDs(body string, rows []fakeRow) []string {
	var out []string
	for _, r := range rows {
		if strings.Contains(body, `id="viewer-row-`+r.ID.Hex()+`"`) {
			out = append(out, r.Text)
		}
	}
	return out
}

func expectRows(t *testing.T, label, body string, rows []fakeRow, want ...string) {
	t.Helper()
	got := rowIDs(body, rows)
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("%s: rows %v, want %v", label, got, want)
	}
}

// --- tests -------------------------------------------------------------------

func TestIndexAndPage(t *testing.T) {
	w := newWorld(t, []string{"admin", "analyst", "coordinator", "leader"}, 10)

	// Index lists the viewer for an allowed role, not for a member.
	rec := w.get(t, "/views", w.admin, "admin", false)
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), `href="/views/fake"`) || !strings.Contains(rec.Body.String(), "Fake Events") {
		t.Errorf("index (admin): %d %q", rec.Code, rec.Body.String()[:200])
	}
	rec = w.get(t, "/views", w.member, "member", false)
	if rec.Code != 200 || strings.Contains(rec.Body.String(), `href="/views/fake"`) || !strings.Contains(rec.Body.String(), "No data views") {
		t.Errorf("index (member): %d", rec.Code)
	}

	// Page: filters, org/group selectors, chips, live toggle, inline rows, export link.
	rec = w.get(t, "/views/fake?kind=a", w.admin, "admin", false)
	if rec.Code != 200 {
		t.Fatalf("page: %d %s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	for _, want := range []string{
		`id="viewer-filters"`, `name="org"`, `name="group"`, `name="q"`, `name="kind"`, `name="when"`, `name="who"`,
		`<option value="a" selected>Kind A`, `id="viewer-live"`, `every 10s`, "Rows</span>", // chips from Summarizer
		`href="/views/fake/export.csv?kind=a"`, `id="viewer-table"`, `id="viewer-rows"`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("page missing %q", want)
		}
	}
	// Three matching rows fit on one page: rendered inline, no load-more.
	expectRows(t, "page kind=a", body, w.rows, "m1 newest", "mb recent", "m1 second")
	if strings.Contains(body, `id="viewer-load-more"`) {
		t.Error("page should not offer load-more when all rows fit")
	}

	// Wrong role and unknown slug.
	if rec := w.get(t, "/views/fake", w.member, "member", false); rec.Code != http.StatusForbidden {
		t.Errorf("member page: %d, want 403", rec.Code)
	}
	if rec := w.get(t, "/views/nope", w.admin, "admin", false); rec.Code != http.StatusNotFound {
		t.Errorf("unknown slug: %d, want 404", rec.Code)
	}
	if rec := w.get(t, "/views/fake", primitive.NilObjectID, "", false); rec.Code != http.StatusSeeOther {
		t.Errorf("anonymous: %d, want 303 to login", rec.Code)
	}
}

func TestTable_FiltersScopeAndURLState(t *testing.T) {
	w := newWorld(t, []string{"admin", "analyst", "coordinator", "leader"}, 0)

	// Admin, no filters: everything, newest first, address bar synced.
	rec := w.get(t, "/views/fake/table", w.admin, "admin", true)
	if rec.Code != 200 {
		t.Fatalf("table: %d %s", rec.Code, rec.Body.String())
	}
	expectRows(t, "admin all", rec.Body.String(), w.rows, "m1 newest", "m2 recent", "mb recent", "m1 second", "m1 ancient")
	if got := rec.Header().Get("HX-Replace-Url"); got != "/views/fake" {
		t.Errorf("HX-Replace-Url: %q", got)
	}
	if w.fake.lastFilter != nil {
		t.Errorf("admin scope filter should be nil, got %v", w.fake.lastFilter)
	}

	// Filters: text, kind, date preset, exact id — and the URL carries them.
	rec = w.get(t, "/views/fake/table?q=M1&kind=a&when=7d", w.admin, "admin", true)
	expectRows(t, "q+kind+when", rec.Body.String(), w.rows, "m1 newest", "m1 second")
	if got := rec.Header().Get("HX-Replace-Url"); got != "/views/fake?kind=a&q=M1&when=7d" {
		t.Errorf("HX-Replace-Url with filters: %q", got)
	}
	rec = w.get(t, "/views/fake/table?who="+w.m2.Hex(), w.admin, "admin", true)
	expectRows(t, "who", rec.Body.String(), w.rows, "m2 recent")

	// Org selection (admin) → org filter; group selection → member filter.
	rec = w.get(t, "/views/fake/table?org="+w.orgB.Hex(), w.admin, "admin", true)
	expectRows(t, "org B", rec.Body.String(), w.rows, "mb recent")
	rec = w.get(t, "/views/fake/table?group="+w.groupA.Hex(), w.admin, "admin", true)
	expectRows(t, "group A1", rec.Body.String(), w.rows, "m1 newest", "m2 recent", "m1 second", "m1 ancient")

	// Coordinator: org A only; org B is out of scope.
	rec = w.get(t, "/views/fake/table", w.coord, "coordinator", true)
	expectRows(t, "coordinator", rec.Body.String(), w.rows, "m1 newest", "m2 recent", "m1 second", "m1 ancient")
	if _, ok := w.fake.lastFilter["org_id"]; !ok {
		t.Errorf("coordinator scope should filter by org: %v", w.fake.lastFilter)
	}
	if rec := w.get(t, "/views/fake/table?org="+w.orgB.Hex(), w.coord, "coordinator", true); rec.Code != http.StatusForbidden {
		t.Errorf("coordinator out-of-scope org: %d, want 403", rec.Code)
	}

	// Leader: members of their group only, filtered by user id (never org).
	rec = w.get(t, "/views/fake/table", w.leader, "leader", true)
	expectRows(t, "leader", rec.Body.String(), w.rows, "m1 newest", "m2 recent", "m1 second", "m1 ancient")
	if _, ok := w.fake.lastFilter["user_id"]; !ok {
		t.Errorf("leader scope should filter by user: %v", w.fake.lastFilter)
	}

	// Analyst: whole workspace. Member: forbidden. Bad org id: 400.
	rec = w.get(t, "/views/fake/table", w.analyst, "analyst", true)
	expectRows(t, "analyst", rec.Body.String(), w.rows, "m1 newest", "m2 recent", "mb recent", "m1 second", "m1 ancient")
	if rec := w.get(t, "/views/fake/table", w.member, "member", true); rec.Code != http.StatusForbidden {
		t.Errorf("member table: %d, want 403", rec.Code)
	}
	if rec := w.get(t, "/views/fake/table?org=zzz", w.admin, "admin", true); rec.Code != http.StatusBadRequest {
		t.Errorf("bad org: %d, want 400", rec.Code)
	}
}

func TestTable_LoadMorePaging(t *testing.T) {
	w := newWorld(t, []string{"admin"}, 0)
	w.handler.PageSize = 2

	rec := w.get(t, "/views/fake/table?kind=a", w.admin, "admin", true)
	body := rec.Body.String()
	expectRows(t, "page 1", body, w.rows, "m1 newest", "mb recent")
	if !strings.Contains(body, `id="viewer-load-more"`) || !strings.Contains(body, "after=") {
		t.Fatalf("page 1 should offer load-more with a cursor")
	}
	// Pull the load-more URL out of the markup and follow it.
	i := strings.Index(body, `hx-get="/views/fake/table?`)
	url := body[i+len(`hx-get="`):]
	url = url[:strings.IndexByte(url, '"')]
	url = strings.ReplaceAll(url, "&amp;", "&")
	if !strings.Contains(url, "kind=a") {
		t.Errorf("load-more URL must keep the filters: %s", url)
	}

	rec = w.get(t, url, w.admin, "admin", true)
	if rec.Code != 200 {
		t.Fatalf("page 2: %d %s", rec.Code, rec.Body.String())
	}
	body2 := rec.Body.String()
	expectRows(t, "page 2", body2, w.rows, "m1 second")
	if strings.Contains(body2, "<table") || strings.Contains(body2, `id="viewer-load-more"`) {
		t.Error("continuation should be rows only, with no further load-more")
	}
	if rec.Header().Get("HX-Replace-Url") != "" {
		t.Error("a continuation must not rewrite the address bar")
	}

	if rec := w.get(t, "/views/fake/table?after=%21%21", w.admin, "admin", true); rec.Code != http.StatusBadRequest {
		t.Errorf("bad cursor: %d, want 400", rec.Code)
	}
}

func TestDetail(t *testing.T) {
	w := newWorld(t, []string{"admin", "leader"}, 0)
	mb := w.rows[2] // org B row

	rec := w.get(t, "/views/fake/rows/"+w.rows[0].ID.Hex(), w.admin, "admin", true)
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), "m1 newest") || !strings.Contains(rec.Body.String(), `class="viewer-detail`) || !strings.Contains(rec.Body.String(), "Stored record") {
		t.Errorf("detail: %d %s", rec.Code, rec.Body.String())
	}
	if rec := w.get(t, "/views/fake/rows/"+primitive.NewObjectID().Hex(), w.admin, "admin", true); rec.Code != http.StatusNotFound {
		t.Errorf("unknown row: %d, want 404", rec.Code)
	}
	// A leader cannot fetch a row outside their scope by guessing its id.
	if rec := w.get(t, "/views/fake/rows/"+mb.ID.Hex(), w.leader, "leader", true); rec.Code != http.StatusNotFound {
		t.Errorf("leader out-of-scope detail: %d, want 404", rec.Code)
	}
}

func TestExport_GenericCSV(t *testing.T) {
	w := newWorld(t, []string{"admin", "leader"}, 0)

	rec := w.get(t, "/views/fake/export.csv?kind=b", w.admin, "admin", false)
	if rec.Code != 200 {
		t.Fatalf("export: %d %s", rec.Code, rec.Body.String())
	}
	if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/csv") {
		t.Errorf("Content-Type: %q", ct)
	}
	if cd := rec.Header().Get("Content-Disposition"); !strings.Contains(cd, `filename="fake-`) {
		t.Errorf("Content-Disposition: %q", cd)
	}
	lines := strings.Split(strings.TrimSpace(rec.Body.String()), "\n")
	if lines[0] != "When,Kind,Text" || len(lines) != 3 || !strings.HasSuffix(lines[1], "b,m2 recent") || !strings.HasSuffix(lines[2], "b,m1 ancient") {
		t.Errorf("csv: %q", lines)
	}

	// Scope applies to export too.
	rec = w.get(t, "/views/fake/export.csv", w.leader, "leader", false)
	if strings.Contains(rec.Body.String(), "mb recent") {
		t.Error("leader export leaked another org's row")
	}
	if rec := w.get(t, "/views/fake/export.csv", w.member, "member", false); rec.Code != http.StatusForbidden {
		t.Errorf("member export: %d, want 403", rec.Code)
	}
}

func TestExport_PagesThroughAllRows(t *testing.T) {
	w := newWorld(t, []string{"admin"}, 0)
	// Force tiny query pages via the viewer: the generic exporter asks for 500
	// at a time, so make sure it follows cursors by counting queries on a
	// large synthetic set.
	var rows []fakeRow
	now := time.Now().UTC()
	for i := 0; i < 1200; i++ {
		rows = append(rows, fakeRow{ID: primitive.NewObjectID(), When: now.Add(-time.Duration(i) * time.Second), User: w.m1, Org: w.orgA, Kind: "a", Text: fmt.Sprintf("row %d", i)})
	}
	w.fake.rows = rows

	rec := w.get(t, "/views/fake/export.csv", w.admin, "admin", false)
	lines := strings.Split(strings.TrimSpace(rec.Body.String()), "\n")
	if len(lines) != 1201 {
		t.Errorf("expected 1200 data rows + header, got %d lines", len(lines))
	}
	if w.fake.queries < 3 {
		t.Errorf("exporter should have paged (queries=%d)", w.fake.queries)
	}
}
