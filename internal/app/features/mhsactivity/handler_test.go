package mhsactivity

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	_ "github.com/dalemusser/stratahub/internal/app/features/missionhydrosci" // registers a template func the shared layout uses
	appresources "github.com/dalemusser/stratahub/internal/app/resources"
	"github.com/dalemusser/stratahub/internal/app/store/mhsactivitydays"
	"github.com/dalemusser/stratahub/internal/app/system/auth"
	"github.com/dalemusser/stratahub/internal/app/system/workspace"
	"github.com/dalemusser/stratahub/internal/domain/models"
	"github.com/dalemusser/stratahub/internal/testutil"
	"github.com/dalemusser/waffle/pantry/templates"
	"github.com/go-chi/chi/v5"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.uber.org/zap"
)

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

type fixture struct {
	db      *mongo.Database
	ws      primitive.ObjectID
	orgA    primitive.ObjectID
	orgB    primitive.ObjectID
	student models.User
	h       *Handler
}

// setup seeds one school with a student who downloaded unit 1 (held, then
// done) and launched on 2026-10-12, and an empty second school.
func setup(t *testing.T) fixture {
	t.Helper()
	bootTemplates(t)
	db := testutil.SetupTestDB(t)
	ctx, cancel := testutil.TestContext()
	defer cancel()
	f := testutil.NewFixtures(t, db)
	ws := primitive.NewObjectID()
	orgA := f.CreateOrganizationWithDetails(ctx, "School A", "Park Hills", "MO", "America/Chicago")
	orgB := f.CreateOrganizationWithDetails(ctx, "School B", "Austin", "TX", "America/Chicago")
	for _, o := range []primitive.ObjectID{orgA.ID, orgB.ID} {
		if _, err := db.Collection("organizations").UpdateByID(ctx, o, bson.M{"$set": bson.M{"workspace_id": ws}}); err != nil {
			t.Fatal(err)
		}
	}
	student := f.CreateUserInWorkspace(ctx, "Ada Student", "ada@example.com", "member", ws, &orgA.ID)
	group := f.CreateGroup(ctx, "Period 5", orgA.ID)
	mem := f.CreateGroupMembership(ctx, student.ID, group.ID, orgA.ID, "member")
	if _, err := db.Collection("group_memberships").UpdateByID(ctx, mem.ID, bson.M{"$set": bson.M{"workspace_id": ws}}); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Collection("groups").UpdateByID(ctx, group.ID, bson.M{"$set": bson.M{"workspace_id": ws}}); err != nil {
		t.Fatal(err)
	}
	sid := student.ID
	at := time.Date(2026, 10, 12, 17, 20, 0, 0, time.UTC)
	heldAt := at.Add(-15 * time.Minute)
	doneAt := at.Add(-time.Minute)
	rec := models.MHSDeviceTest{ID: primitive.NewObjectID(), WorkspaceID: ws, Kind: models.MHSDeviceTestKindMember, UserID: &sid, OrganizationID: &orgA.ID,
		UnitID: "unit1", DeviceID: "dev-1", DeviceType: "Chromebook", StartedAt: at, ExpiresAt: at.Add(6 * time.Hour),
		UserAgent: "Mozilla/5.0 (X11; CrOS x86_64 16805.31.0) Chrome/152.0.0.0 Safari/537.36", Diagnostics: map[string]interface{}{"outcome": "launch-ok"},
		Summary: &models.MHSMemberSummary{V: models.MHSMemberSummaryVersion, Outcome: "launch-ok",
			Units:  []models.MHSStepUnitSummary{{Unit: "unit1", FirstAt: at.Add(-16 * time.Minute), LastAt: at.Add(-time.Minute), Requested: true, TotalMB: 213, MaxMB: 213, Complete: true, CompleteAt: &doneAt, CompleteMB: 213, CompleteSeconds: 900, Held: true, HeldAtMB: 201, HeldAt: &heldAt}},
			Launch: []models.MHSStepLaunchEvent{{At: at, Kind: models.MHSLaunchEventRendering}}}}
	if _, err := db.Collection("mhs_device_tests").InsertOne(ctx, rec); err != nil {
		t.Fatal(err)
	}
	h := NewHandler(db, nil, nil, nil, zap.NewNop(), Config{Enabled: true, CloseAfter: 6 * time.Hour, CacheTTL: time.Minute, MaxLiveDays: 7, MaxDays: 31})
	h.setNow(func() time.Time { return time.Date(2026, 10, 12, 20, 0, 0, 0, time.UTC) }) // 3 pm in Chicago on the 12th
	return fixture{db: db, ws: ws, orgA: orgA.ID, orgB: orgB.ID, student: student, h: h}
}

func (fx fixture) get(t *testing.T, target string, user testutil.TestUser) *httptest.ResponseRecorder {
	t.Helper()
	return fx.getAs(t, target, &auth.SessionUser{ID: user.ID, Name: user.Name, LoginID: user.Email, Role: user.Role, OrganizationID: user.OrganizationID})
}

func (fx fixture) getAs(t *testing.T, target string, su *auth.SessionUser) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, target, nil)
	req.Host = "mhs.example.com"
	req = auth.WithTestUser(req, su)
	req = workspace.WithTestWorkspace(req, fx.ws, "mhs", "MHS")
	rec := httptest.NewRecorder()
	fx.h.ServePage(rec, req)
	return rec
}

func TestAdminSeesTheOverviewAndTheSchool(t *testing.T) {
	fx := setup(t)
	rec := fx.get(t, "/mhsactivity", testutil.AdminUser())
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d: %s", rec.Code, rec.Body.String()[:min(600, rec.Body.Len())])
	}
	body := rec.Body.String()
	for _, want := range []string{"MHS Activity", "School A", "1 hold released", "name=\"range\"", "computed", "the game log service"} {
		if !strings.Contains(body, want) {
			t.Errorf("overview lacks %q", want)
		}
	}
	if strings.Contains(body, "School B</a>") {
		t.Error("a school without activity is listed")
	}
	// The download forms are siblings of the filter form, never inside it
	// (nested forms fold their inputs into the outer one).
	filterEnd := strings.Index(body, "</form>")
	if csvForm := strings.Index(body, `action="/mhsactivity/schools.csv"`); filterEnd < 0 || csvForm < filterEnd {
		t.Error("the download forms sit inside the filter form")
	}
	if !strings.Contains(body, ">School B</option>") {
		t.Error("the school filter should still offer every school")
	}
	// The school view lists the student with the unit cell and the group.
	rec = fx.get(t, "/mhsactivity?org="+fx.orgA.Hex(), testutil.AdminUser())
	if rec.Code != http.StatusOK {
		t.Fatalf("school status %d", rec.Code)
	}
	body = rec.Body.String()
	for _, want := range []string{"Ada Student", "Period 5", "held at 201/213 MB, done after 14.0 min", "← All schools", "/mhsdashboard?group="} {
		if !strings.Contains(body, want) {
			t.Errorf("school view lacks %q", want)
		}
	}
	// Yesterday has nothing.
	rec = fx.get(t, "/mhsactivity?range=yesterday", testutil.AnalystUser())
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "No Mission HydroSci activity") {
		t.Errorf("yesterday: %d", rec.Code)
	}
}

func TestRoleGates(t *testing.T) {
	fx := setup(t)
	for _, tc := range []struct {
		name string
		user testutil.TestUser
		want int
	}{
		{"analyst", testutil.AnalystUser(), http.StatusOK},
		{"leader", testutil.LeaderUser(fx.orgA), http.StatusForbidden},
		{"member", testutil.MemberUser(fx.orgA), http.StatusForbidden},
		{"coordinator without schools", testutil.TestUser{ID: primitive.NewObjectID().Hex(), Name: "Co", Email: "co@example.com", Role: "coordinator"}, http.StatusForbidden},
	} {
		rec := fx.get(t, "/mhsactivity", tc.user)
		if rec.Code != tc.want {
			t.Errorf("%s: status %d, want %d", tc.name, rec.Code, tc.want)
		}
	}
	// A custom range past the live limit is refused.
	rec := fx.get(t, "/mhsactivity?range=custom&from=2026-09-01&to=2026-10-12", testutil.AdminUser())
	if rec.Code != http.StatusBadRequest {
		t.Errorf("long range: status %d", rec.Code)
	}
}

func TestCoordinatorScope(t *testing.T) {
	fx := setup(t)
	ctx, cancel := testutil.TestContext()
	defer cancel()
	coID := primitive.NewObjectID()
	// Assigned to School B only: the session carries the assignment for the
	// policy, the collection carries it for the scope.
	co := &auth.SessionUser{ID: coID.Hex(), Name: "Co", LoginID: "co@example.com", Role: "coordinator", OrganizationIDs: []string{fx.orgB.Hex()}}
	if _, err := fx.db.Collection("coordinator_assignments").InsertOne(ctx, bson.M{"user_id": coID, "organization_id": fx.orgB, "created_at": time.Now()}); err != nil {
		t.Fatal(err)
	}
	rec := fx.getAs(t, "/mhsactivity", co)
	if rec.Code != http.StatusOK {
		t.Fatalf("coordinator status %d: %s", rec.Code, rec.Body.String()[:min(400, rec.Body.Len())])
	}
	if body := rec.Body.String(); strings.Contains(body, "Ada Student") || strings.Contains(body, ">School A<") {
		t.Error("coordinator sees a school outside their assignment")
	}
	rec = fx.getAs(t, "/mhsactivity?org="+fx.orgA.Hex(), co)
	if rec.Code != http.StatusForbidden {
		t.Errorf("out-of-scope school: status %d", rec.Code)
	}
}

// TestStoredDayReplacesLiveComputation stores a different picture of the
// 12th than the raw records give and asks for it after it has closed: the
// page must show the stored one and say so.
func TestStoredDayReplacesLiveComputation(t *testing.T) {
	fx := setup(t)
	ctx, cancel := testutil.TestContext()
	defer cancel()
	store := mhsactivitydays.New(fx.db)
	other := primitive.NewObjectID()
	docs := []models.MHSActivityDay{{WorkspaceID: fx.ws, UserID: other, OrganizationID: fx.orgB, Role: models.MHSActivityRoleMember, Day: "2026-10-12", TZ: "America/Chicago", Opened: true, Played: true, Version: models.MHSActivityVersion, ComputedAt: time.Now()}}
	if err := store.ReplaceDay(ctx, fx.ws, "2026-10-12", []primitive.ObjectID{fx.orgA, fx.orgB}, docs); err != nil {
		t.Fatal(err)
	}
	for _, org := range []primitive.ObjectID{fx.orgA, fx.orgB} {
		if err := store.SetMark(ctx, models.MHSActivityMark{WorkspaceID: fx.ws, OrganizationID: org, Day: "2026-10-12", TZ: "America/Chicago", Final: true, Version: models.MHSActivityVersion, ComputedAt: time.Date(2026, 10, 13, 11, 12, 0, 0, time.UTC)}); err != nil {
			t.Fatal(err)
		}
	}
	// Before the day closes (3 pm Chicago on the 12th) the live picture wins.
	rec := fx.get(t, "/mhsactivity", testutil.AdminUser())
	if body := rec.Body.String(); !strings.Contains(body, "School A</a>") || strings.Contains(body, "School B</a>") || !strings.Contains(body, "Live: Oct 12, 2026, computed") {
		t.Errorf("open day did not compute live")
	}
	// After it closes (noon UTC on the 13th), the stored picture wins.
	fx.h.setNow(func() time.Time { return time.Date(2026, 10, 13, 12, 0, 0, 0, time.UTC) })
	rec = fx.get(t, "/mhsactivity?range=yesterday", testutil.AdminUser())
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "School B</a>") || strings.Contains(body, "School A</a>") || !strings.Contains(body, "Stored: Oct 12, 2026 (closed days, stored once; last stored Oct 13 11:12 UTC)") {
		t.Errorf("closed day did not come from the store: %s", body[strings.Index(body, "Data:"):min(len(body), strings.Index(body, "Data:")+300)])
	}
	// A window needing more live days than allowed is refused.
	fx.h.Config.MaxLiveDays = 1
	rec = fx.get(t, "/mhsactivity?range=7d", testutil.AdminUser())
	if rec.Code != http.StatusBadRequest {
		t.Errorf("live cap: status %d", rec.Code)
	}
}

func TestAdminPageAndBackfill(t *testing.T) {
	fx := setup(t)
	// With a (empty) log service and grader the computation is complete, so
	// the backfill's result is final and the page serves it from the store.
	fx.h = NewHandler(fx.db, fx.db, fx.db, nil, zap.NewNop(), fx.h.Config)
	fx.h.setNow(func() time.Time { return time.Date(2026, 10, 12, 20, 0, 0, 0, time.UTC) })
	get := func(target string, user testutil.TestUser, method string) *httptest.ResponseRecorder {
		req := testutil.NewAuthenticatedRequest(method, target, user)
		req.Host = "mhs.example.com"
		req = workspace.WithTestWorkspace(req, fx.ws, "mhs", "MHS")
		rec := httptest.NewRecorder()
		switch {
		case target == "/mhsactivity/admin":
			fx.h.ServeAdmin(rec, req)
		case strings.HasPrefix(target, "/mhsactivity/admin/backfill"):
			fx.h.HandleBackfill(rec, req)
		case target == "/mhsactivity/admin/status":
			fx.h.ServeAdminStatus(rec, req)
		}
		return rec
	}
	rec := get("/mhsactivity/admin", testutil.AdminUser(), http.MethodGet)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "No backfill has run") {
		t.Fatalf("admin page: %d", rec.Code)
	}
	if rec := get("/mhsactivity/admin", testutil.AnalystUser(), http.MethodGet); rec.Code != http.StatusForbidden {
		t.Errorf("analyst on the admin page: %d", rec.Code)
	}
	// Start a backfill of the 12th (closed by the 13th noon) and wait for it.
	fx.h.setNow(func() time.Time { return time.Date(2026, 10, 13, 12, 0, 0, 0, time.UTC) })
	rec = get("/mhsactivity/admin/backfill?from=2026-10-12&to=2026-10-12&stamp=1", testutil.AdminUser(), http.MethodPost)
	if rec.Code != http.StatusOK {
		t.Fatalf("start: %d %s", rec.Code, rec.Body.String()[:min(300, rec.Body.Len())])
	}
	deadline := time.Now().Add(30 * time.Second)
	for fx.h.Backfill.Status().Running && time.Now().Before(deadline) {
		time.Sleep(50 * time.Millisecond)
	}
	rec = get("/mhsactivity/admin/status", testutil.AdminUser(), http.MethodGet)
	body := rec.Body.String()
	if !strings.Contains(body, "Finished") || !strings.Contains(body, "2026-10-12 to 2026-10-12") || !strings.Contains(body, "1 student-days stored") {
		t.Errorf("status after backfill: %s", body)
	}
	// The stored day now serves the page.
	rec = fx.get(t, "/mhsactivity?range=yesterday", testutil.AdminUser())
	if !strings.Contains(rec.Body.String(), "Stored: Oct 12, 2026") || !strings.Contains(rec.Body.String(), "Ada Student") && !strings.Contains(rec.Body.String(), "School A</a>") {
		t.Errorf("page after backfill does not use the store")
	}
}

func TestCSVExports(t *testing.T) {
	fx := setup(t)
	for _, tc := range []struct {
		target, filename, header, row string
	}{
		{"/mhsactivity/schools.csv", "mhs-activity-schools-today.csv", "window,organization,time_zone,students", "Today,School A,America/Chicago,1,1,0,0,1,0,1,0,0,0,0,1,0,0,0,0,0,1,0,no,0,ChromeOS 1,"},
		{"/mhsactivity/students.csv?org=" + fx.orgA.Hex(), "mhs-activity-students-today.csv", "day,time_zone,organization,groups,user_id,name,role", "2026-10-12,America/Chicago,School A,Period 5," + fx.student.ID.Hex() + ",Ada Student,member,yes,no,no,unit1,held_done,dev-1,213,213,0,0,0,0,,213,900,201,840,no,1,0,0,0,0,0,1,0,ChromeOS,,"},
	} {
		req := testutil.NewAuthenticatedRequest(http.MethodGet, tc.target, testutil.AdminUser())
		req.Host = "mhs.example.com"
		req = workspace.WithTestWorkspace(req, fx.ws, "mhs", "MHS")
		rec := httptest.NewRecorder()
		if strings.HasPrefix(tc.target, "/mhsactivity/schools") {
			fx.h.ServeSchoolsCSV(rec, req)
		} else {
			fx.h.ServeStudentsCSV(rec, req)
		}
		if rec.Code != http.StatusOK || !strings.Contains(rec.Header().Get("Content-Type"), "text/csv") || !strings.Contains(rec.Header().Get("Content-Disposition"), tc.filename) {
			t.Fatalf("%s: status %d, type %q, disposition %q", tc.target, rec.Code, rec.Header().Get("Content-Type"), rec.Header().Get("Content-Disposition"))
		}
		body := rec.Body.String()
		if !strings.HasPrefix(body, "\xEF\xBB\xBF"+tc.header) {
			t.Errorf("%s: header %q", tc.target, body[:min(80, len(body))])
		}
		if !strings.Contains(body, tc.row) {
			t.Errorf("%s: row not found in:\n%s", tc.target, body)
		}
	}
	// A leader gets no CSV.
	req := testutil.NewAuthenticatedRequest(http.MethodGet, "/mhsactivity/schools.csv", testutil.LeaderUser(fx.orgA))
	req.Host = "mhs.example.com"
	req = workspace.WithTestWorkspace(req, fx.ws, "mhs", "MHS")
	rec := httptest.NewRecorder()
	fx.h.ServeSchoolsCSV(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Errorf("leader csv: status %d", rec.Code)
	}
	// Admins see the record links in the school view; coordinators do not.
	rec = fx.get(t, "/mhsactivity?org="+fx.orgA.Hex(), testutil.AdminUser())
	if !strings.Contains(rec.Body.String(), "/views/device-tests?kind=member&amp;when=30d&amp;user="+fx.student.ID.Hex()) {
		t.Error("admin school view lacks the record link")
	}
}

func TestStudentDayTimeline(t *testing.T) {
	fx := setup(t)
	ctx, cancel := testutil.TestContext()
	defer cancel()
	// A second record from the same tab repeats the first's entries (and one
	// of them moved by the page's repeat collapsing) and adds a launch.
	sid := fx.student.ID
	at := time.Date(2026, 10, 12, 17, 20, 0, 0, time.UTC)
	first := []models.MHSDeviceTestStep{
		{At: at.Add(-10 * time.Minute), Step: "page", State: "info", Msg: "Opened the units page"},
		{At: at.Add(-9 * time.Minute), Step: "download", State: "running", Msg: "Requesting Unit 1: Water Cycle v2.8.1 (213 MB)", Detail: map[string]string{"unit": "unit1", "total_mb": "213"}},
	}
	rec1 := models.MHSDeviceTest{ID: primitive.NewObjectID(), WorkspaceID: fx.ws, Kind: models.MHSDeviceTestKindMember, UserID: &sid, OrganizationID: &fx.orgA,
		UnitID: "unit1", DeviceID: "dev-1", StartedAt: at.Add(-8 * time.Minute), ExpiresAt: at.Add(6 * time.Hour), Diagnostics: map[string]interface{}{"outcome": "download-complete"}, Steps: append(first, models.MHSDeviceTestStep{At: at.Add(-8 * time.Minute), Step: "download", State: "ok", Msg: "Unit 1: Water Cycle: download complete — 213 MB in 0:22 (9.7 MB/s)"})}
	rec2 := models.MHSDeviceTest{ID: primitive.NewObjectID(), WorkspaceID: fx.ws, Kind: models.MHSDeviceTestKindMember, UserID: &sid, OrganizationID: &fx.orgA,
		UnitID: "unit1", DeviceID: "dev-1", StartedAt: at, ExpiresAt: at.Add(6 * time.Hour), Diagnostics: map[string]interface{}{"outcome": "launch-ok"}, Steps: append(append([]models.MHSDeviceTestStep{}, rec1.Steps...), models.MHSDeviceTestStep{At: at, Step: "game", State: "ok", Msg: "Game is rendering"})}
	for _, rec := range []models.MHSDeviceTest{rec1, rec2} {
		if _, err := fx.db.Collection("mhs_device_tests").InsertOne(ctx, rec); err != nil {
			t.Fatal(err)
		}
	}
	req := testutil.NewAuthenticatedRequest(http.MethodGet, "/mhsactivity/student/"+sid.Hex()+"/day/2026-10-12", testutil.AdminUser())
	req.Host = "mhs.example.com"
	req = workspace.WithTestWorkspace(req, fx.ws, "mhs", "MHS")
	req = withRouteParams(req, map[string]string{"userID": sid.Hex(), "day": "2026-10-12"})
	rec := httptest.NewRecorder()
	fx.h.ServeStudentDay(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d: %s", rec.Code, rec.Body.String()[:min(400, rec.Body.Len())])
	}
	body := rec.Body.String()
	// The fixture's own launch-ok record (setup) has no steps; these two add
	// four distinct entries, each once, in time order, with the detail shown.
	for _, want := range []string{"Ada Student", "3 load records", "Opened the units page", "Requesting Unit 1", "download complete", "Game is rendering", "total_mb=213", "12:10:00 PM", "…dev-1"} {
		if !strings.Contains(body, want) {
			t.Errorf("timeline lacks %q", want)
		}
	}
	if n := strings.Count(body, "Requesting Unit 1"); n != 1 {
		t.Errorf("the repeated entry appears %d times", n)
	}
	if strings.Index(body, "Opened the units page") > strings.Index(body, "Game is rendering") {
		t.Error("entries out of time order")
	}
	// A coordinator for another school may not see it.
	co := &auth.SessionUser{ID: primitive.NewObjectID().Hex(), Name: "Co", LoginID: "co@example.com", Role: "coordinator", OrganizationIDs: []string{fx.orgB.Hex()}}
	coID, _ := primitive.ObjectIDFromHex(co.ID)
	if _, err := fx.db.Collection("coordinator_assignments").InsertOne(ctx, bson.M{"user_id": coID, "organization_id": fx.orgB, "created_at": time.Now()}); err != nil {
		t.Fatal(err)
	}
	req = httptest.NewRequest(http.MethodGet, "/mhsactivity/student/"+sid.Hex()+"/day/2026-10-12", nil)
	req.Host = "mhs.example.com"
	req = auth.WithTestUser(req, co)
	req = workspace.WithTestWorkspace(req, fx.ws, "mhs", "MHS")
	req = withRouteParams(req, map[string]string{"userID": sid.Hex(), "day": "2026-10-12"})
	rec = httptest.NewRecorder()
	fx.h.ServeStudentDay(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Errorf("coordinator outside scope: status %d", rec.Code)
	}
	// The school view links to it.
	rec = fx.get(t, "/mhsactivity?org="+fx.orgA.Hex(), testutil.AdminUser())
	if !strings.Contains(rec.Body.String(), "/mhsactivity/student/"+sid.Hex()+"/day/2026-10-12") {
		t.Error("school view lacks the timeline link")
	}
}

// withRouteParams sets several chi URL parameters on one request.
func withRouteParams(req *http.Request, params map[string]string) *http.Request {
	rctx := chi.NewRouteContext()
	for k, v := range params {
		rctx.URLParams.Add(k, v)
	}
	return req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
}
