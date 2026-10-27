package mhsactivity

import (
	"testing"
	"time"

	"github.com/dalemusser/stratahub/internal/domain/models"
	"github.com/dalemusser/stratahub/internal/testutil"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.uber.org/zap"
)

// A school day in Chicago: 2026-10-12 12:xx local = 17:xx UTC.
func utc(h, m int) time.Time { return time.Date(2026, 10, 12, h, m, 0, 0, time.UTC) }

func TestComputeDayEndToEnd(t *testing.T) {
	db := testutil.SetupTestDB(t)
	ctx, cancel := testutil.TestContext()
	defer cancel()
	f := testutil.NewFixtures(t, db)
	ws := primitive.NewObjectID()

	org := f.CreateOrganizationWithDetails(ctx, "School A", "Park Hills", "MO", "America/Chicago")
	otherOrg := f.CreateOrganizationWithDetails(ctx, "School B", "Austin", "TX", "America/Chicago")
	for _, o := range []primitive.ObjectID{org.ID, otherOrg.ID} {
		if _, err := db.Collection("organizations").UpdateByID(ctx, o, bson.M{"$set": bson.M{"workspace_id": ws}}); err != nil {
			t.Fatal(err)
		}
	}
	student := f.CreateUserInWorkspace(ctx, "Ada Student", "ada@example.com", "member", ws, &org.ID)
	quiet := f.CreateUserInWorkspace(ctx, "Quiet Student", "quiet@example.com", "member", ws, &org.ID)
	teacher := f.CreateUserInWorkspace(ctx, "Tess Teacher", "tess@example.com", "leader", ws, &org.ID)
	elsewhere := f.CreateUserInWorkspace(ctx, "Bo Elsewhere", "bo@example.com", "member", ws, &otherOrg.ID)
	_ = quiet
	group := f.CreateGroup(ctx, "Period 5", org.ID)
	for _, m := range []struct {
		user primitive.ObjectID
		role string
	}{{student.ID, "member"}, {teacher.ID, "leader"}} {
		mem := f.CreateGroupMembership(ctx, m.user, group.ID, org.ID, m.role)
		if _, err := db.Collection("group_memberships").UpdateByID(ctx, mem.ID, bson.M{"$set": bson.M{"workspace_id": ws}}); err != nil {
			t.Fatal(err)
		}
	}

	sid := student.ID
	tid := teacher.ID
	eid := elsewhere.ID
	// A completed download with a summary already on the record.
	heldAt := utc(17, 5)
	doneAt := utc(17, 20)
	withSummary := models.MHSDeviceTest{ID: primitive.NewObjectID(), WorkspaceID: ws, Kind: models.MHSDeviceTestKindMember, UserID: &sid, OrganizationID: &org.ID,
		UnitID: "unit1", UnitVersion: "2.8.1", DeviceID: "dev-1", DeviceType: "Chromebook", StartedAt: doneAt, ExpiresAt: doneAt.Add(6 * time.Hour),
		UserAgent:   "Mozilla/5.0 (X11; CrOS x86_64 16805.31.0) Chrome/152.0.0.0 Safari/537.36",
		Diagnostics: map[string]interface{}{"outcome": "download-complete"},
		Summary: &models.MHSMemberSummary{V: models.MHSMemberSummaryVersion, Outcome: "download-complete", Units: []models.MHSStepUnitSummary{{
			Unit: "unit1", Version: "2.8.1", FirstAt: utc(17, 4), LastAt: doneAt, Requested: true, TotalMB: 213, MaxMB: 213, Drops: 4, Stalls: 1,
			Complete: true, CompleteAt: &doneAt, CompleteMB: 213, CompleteSeconds: 960, Held: true, HeldAtMB: 201, HeldAt: &heldAt,
		}}},
	}
	// A launch record with steps only (stored before summaries existed).
	launched := models.MHSDeviceTest{ID: primitive.NewObjectID(), WorkspaceID: ws, Kind: models.MHSDeviceTestKindMember, UserID: &sid, OrganizationID: &org.ID,
		UnitID: "unit1", UnitVersion: "2.8.1", DeviceID: "dev-1", DeviceType: "Chromebook", StartedAt: utc(17, 22), ExpiresAt: utc(23, 22),
		Diagnostics: map[string]interface{}{"outcome": "launch-ok"}, HeartbeatCount: 20,
		Steps: []models.MHSDeviceTestStep{
			{At: utc(17, 21), Step: "launch", State: "ok", Msg: "Game loader loaded (from the downloaded files)"},
			{At: utc(17, 21).Add(30 * time.Second), Step: "launch", State: "ok", Msg: "Unity started in 28.0 s"},
			{At: utc(17, 22), Step: "game", State: "ok", Msg: "Game is rendering"},
			{At: utc(17, 40), Step: "game", State: "fail", Msg: "Crash (unhandled_rejection, game): A user gesture is required to request Pointer Lock."},
		},
	}
	// The teacher's own launch, and a student of another school.
	teacherRec := models.MHSDeviceTest{ID: primitive.NewObjectID(), WorkspaceID: ws, Kind: models.MHSDeviceTestKindMember, UserID: &tid, OrganizationID: &org.ID,
		UnitID: "unit1", DeviceID: "dev-t", StartedAt: utc(13, 0), ExpiresAt: utc(19, 0), Diagnostics: map[string]interface{}{"outcome": "launch-ok"},
		Steps: []models.MHSDeviceTestStep{{At: utc(13, 0), Step: "game", State: "ok", Msg: "Game is rendering"}}}
	otherRec := models.MHSDeviceTest{ID: primitive.NewObjectID(), WorkspaceID: ws, Kind: models.MHSDeviceTestKindMember, UserID: &eid, OrganizationID: &otherOrg.ID,
		UnitID: "unit1", DeviceID: "dev-e", StartedAt: utc(15, 0), ExpiresAt: utc(21, 0), Diagnostics: map[string]interface{}{"outcome": "download-failed"},
		Steps: []models.MHSDeviceTestStep{{At: utc(15, 0), Step: "download", State: "fail", Msg: "Unit 1: Water Cycle: Download failed [network:timeout] — x"}}}
	// Yesterday's record of the same student: outside the day.
	old := withSummary
	old.ID = primitive.NewObjectID()
	old.StartedAt = utc(17, 0).AddDate(0, 0, -1)
	old.Summary = nil
	old.Steps = []models.MHSDeviceTestStep{{At: old.StartedAt, Step: "game", State: "ok", Msg: "Game is rendering"}}
	for _, rec := range []models.MHSDeviceTest{withSummary, launched, teacherRec, otherRec, old} {
		if _, err := db.Collection("mhs_device_tests").InsertOne(ctx, rec); err != nil {
			t.Fatal(err)
		}
	}
	// A device row created today (fresh profile) and a page view.
	if _, err := db.Collection("mhs_device_status").InsertOne(ctx, models.MHSDeviceStatus{WorkspaceID: ws, UserID: sid, DeviceID: "dev-1", DeviceType: "Chromebook",
		DeviceDetails: map[string]string{"user_agent": "Mozilla/5.0 (X11; CrOS x86_64 16805.31.0) Chrome/152.0.0.0 Safari/537.36"},
		UnitStatus:    map[string]string{"unit1": "cached"}, StorageQuota: 11 * 1024 * 1024 * 1024, CreatedAt: utc(17, 2), LastSeen: utc(17, 22), UpdatedAt: utc(17, 22)}); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Collection("activity_events").InsertOne(ctx, bson.M{"user_id": sid, "session_id": primitive.NewObjectID(), "timestamp": utc(17, 1), "event_type": "page_view", "page_path": UnitsPagePath}); err != nil {
		t.Fatal(err)
	}

	zone := Zone{TZ: "America/Chicago", Loc: chicago, OrgIDs: []primitive.ObjectID{org.ID}}
	docs, err := ComputeDay(ctx, Deps{DB: db, Log: zap.NewNop()}, ws, "2026-10-12", zone)
	if err != nil {
		t.Fatal(err)
	}
	if len(docs) != 2 {
		t.Fatalf("want the student and the teacher, got %d docs: %+v", len(docs), docs)
	}
	var s, st *models.MHSActivityDay
	for i := range docs {
		switch docs[i].UserID {
		case sid:
			s = &docs[i]
		case tid:
			st = &docs[i]
		}
	}
	if s == nil || st == nil {
		t.Fatalf("docs %+v", docs)
	}
	if !s.Opened || !s.Launched || s.Records != 2 || s.PageViews != 1 || s.Role != models.MHSActivityRoleMember || len(s.GroupIDs) != 1 || s.GroupIDs[0] != group.ID {
		t.Errorf("student doc %+v", s)
	}
	if len(s.Units) != 1 || s.Units[0].State != models.MHSActivityUnitHeldDone || s.Units[0].HoldSeconds != 15*60 || s.Units[0].Drops != 4 {
		t.Errorf("units %+v", s.Units)
	}
	if s.Launch.OK != 1 || s.Launch.LoaderCache != 1 || s.Launch.UnitySecondsMax != 28 || s.Crashes.Notice != 1 || s.Crashes.Real != 0 {
		t.Errorf("launch %+v crashes %+v", s.Launch, s.Crashes)
	}
	if len(s.Devices) != 1 || !s.Devices[0].New || s.Devices[0].StorageQuotaMB != 11*1024 || s.Devices[0].Records != 2 {
		t.Errorf("devices %+v", s.Devices)
	}
	// No log service and no grader: partial, and played comes from the heartbeats.
	if !s.Partial || len(s.Missing) != 2 || !s.Played || s.Log.Source != "launch" || s.CurrentUnit != "" {
		t.Errorf("partial handling %+v", s)
	}
	if s.Blocked {
		t.Error("a student who played is not blocked")
	}
	if st.Role != models.MHSActivityRoleStaff || st.Launch.OK != 1 || len(st.GroupIDs) != 1 {
		t.Errorf("teacher doc %+v", st)
	}
	if s.Day != "2026-10-12" || s.TZ != "America/Chicago" || s.Final || s.Version != models.MHSActivityVersion {
		t.Errorf("identity %+v", s)
	}

	// The other school's day, computed on its own.
	other := Zone{TZ: "America/Chicago", Loc: chicago, OrgIDs: []primitive.ObjectID{otherOrg.ID}}
	docs, err = ComputeDay(ctx, Deps{DB: db}, ws, "2026-10-12", other)
	if err != nil || len(docs) != 1 || docs[0].UserID != eid || !docs[0].Blocked || docs[0].Units[0].State != models.MHSActivityUnitFailed {
		t.Errorf("other school: %v %+v", err, docs)
	}

	// Yesterday for the first school: only the old launch.
	docs, err = ComputeDay(ctx, Deps{DB: db}, ws, "2026-10-11", zone)
	if err != nil || len(docs) != 1 || docs[0].Launch.OK != 1 || docs[0].Records != 1 {
		t.Errorf("yesterday: %v %+v", err, docs)
	}

	// Zones: both schools share Chicago.
	zones, err := ZonesFor(ctx, db, ws, nil)
	if err != nil || len(zones) != 1 || zones[0].TZ != "America/Chicago" || len(zones[0].OrgIDs) != 2 {
		t.Errorf("zones %v %+v", err, zones)
	}
	zones, _ = ZonesFor(ctx, db, ws, []primitive.ObjectID{otherOrg.ID})
	if len(zones) != 1 || len(zones[0].OrgIDs) != 1 {
		t.Errorf("zones for one org %+v", zones)
	}
}

func TestComputeDayCountsFrozenRuns(t *testing.T) {
	db := testutil.SetupTestDB(t)
	ctx, cancel := testutil.TestContext()
	defer cancel()
	f := testutil.NewFixtures(t, db)
	ws := primitive.NewObjectID()
	org := f.CreateOrganizationWithDetails(ctx, "School F", "Anytown", "MO", "America/Chicago")
	if _, err := db.Collection("organizations").UpdateByID(ctx, org.ID, bson.M{"$set": bson.M{"workspace_id": ws}}); err != nil {
		t.Fatal(err)
	}
	student := f.CreateUserInWorkspace(ctx, "Fran Student", "fran@example.com", "member", ws, &org.ID)
	sid := student.ID
	hb := func(at time.Time, elapsedS int64, wasm int, fps float64) models.MHSDeviceTestHeartbeat {
		return models.MHSDeviceTestHeartbeat{At: at, ElapsedMs: elapsedS * 1000, Phase: "game", JSHeapMB: 300, WasmHeapMB: wasm, FPS: fps, Visibility: "visible"}
	}
	var recs []interface{}
	for i := 0; i < 3; i++ {
		start := utc(19, 47+i*4)
		recs = append(recs, models.MHSDeviceTest{ID: primitive.NewObjectID(), WorkspaceID: ws, Kind: models.MHSDeviceTestKindMember, UserID: &sid, OrganizationID: &org.ID,
			UnitID: "unit2", DeviceID: "dev-arm", StartedAt: start, ExpiresAt: start.Add(6 * time.Hour), Diagnostics: map[string]interface{}{"outcome": "launch-ok"},
			HeartbeatCount: 4, Heartbeats: []models.MHSDeviceTestHeartbeat{hb(start, 10, 96, 43), hb(start.Add(30*time.Second), 40, 115, 58), hb(start.Add(time.Minute), 70, 495, 47), hb(start.Add(90*time.Second), 100, 495, 1.6)},
			Steps: []models.MHSDeviceTestStep{{At: start, Step: "game", State: "ok", Msg: "Game is rendering", Detail: map[string]string{"unit": "unit2"}}}})
	}
	// A slow but steady run with a browser notice written the new way.
	slow := utc(20, 5)
	recs = append(recs, models.MHSDeviceTest{ID: primitive.NewObjectID(), WorkspaceID: ws, Kind: models.MHSDeviceTestKindMember, UserID: &sid, OrganizationID: &org.ID,
		UnitID: "unit2", DeviceID: "dev-arm", StartedAt: slow, ExpiresAt: slow.Add(6 * time.Hour), Diagnostics: map[string]interface{}{"outcome": "launch-ok"},
		HeartbeatCount: 3, Heartbeats: []models.MHSDeviceTestHeartbeat{hb(slow, 10, 96, 30), hb(slow.Add(time.Minute), 70, 495, 12), hb(slow.Add(10*time.Minute), 600, 495, 13)},
		Steps: []models.MHSDeviceTestStep{
			{At: slow, Step: "game", State: "ok", Msg: "Game is rendering", Detail: map[string]string{"unit": "unit2"}},
			{At: slow.Add(2 * time.Minute), Step: "game", State: "warn", Msg: "Browser notice (unhandled_rejection, running): Permissions check failed", Detail: map[string]string{"type": "unhandled_rejection", "phase": "running", "unit": "unit2"}},
		}})
	if _, err := db.Collection("mhs_device_tests").InsertMany(ctx, recs); err != nil {
		t.Fatal(err)
	}
	zone := Zone{TZ: "America/Chicago", Loc: chicago, OrgIDs: []primitive.ObjectID{org.ID}}
	docs, err := ComputeDay(ctx, Deps{DB: db, Log: zap.NewNop()}, ws, "2026-10-12", zone)
	if err != nil {
		t.Fatal(err)
	}
	if len(docs) != 1 {
		t.Fatalf("docs %+v", docs)
	}
	d := docs[0]
	if d.Launch.OK != 4 || d.Launch.Froze != 3 || !d.Flags.FreezeLoop || !d.HasProblem() {
		t.Errorf("launch %+v flags %+v", d.Launch, d.Flags)
	}
	if d.Crashes.Notice != 1 || d.Crashes.Real != 0 {
		t.Errorf("crashes %+v", d.Crashes)
	}
}
