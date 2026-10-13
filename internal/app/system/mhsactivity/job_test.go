package mhsactivity

import (
	"testing"
	"time"

	"github.com/dalemusser/stratahub/internal/app/store/mhsactivitydays"
	workspacestore "github.com/dalemusser/stratahub/internal/app/store/workspaces"
	"github.com/dalemusser/stratahub/internal/domain/models"
	"github.com/dalemusser/stratahub/internal/testutil"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.uber.org/zap"
)

// TestRollupJobStoresClosedDays seeds one school with a launch on the 12th
// and runs the job as if it were the 13th: the 12th is closed and gets
// stored (partial: no log service), the 13th is not touched, a second run
// retries the partial day, and force recomputes a final one.
func TestRollupJobStoresClosedDays(t *testing.T) {
	db := testutil.SetupTestDB(t)
	ctx, cancel := testutil.TestContext()
	defer cancel()
	f := testutil.NewFixtures(t, db)
	ws := f.CreateWorkspace(ctx, "MHS", "mhs")
	org := f.CreateOrganizationWithDetails(ctx, "School A", "Park Hills", "MO", "America/Chicago")
	if _, err := db.Collection("organizations").UpdateByID(ctx, org.ID, bson.M{"$set": bson.M{"workspace_id": ws.ID}}); err != nil {
		t.Fatal(err)
	}
	student := f.CreateUserInWorkspace(ctx, "Ada", "ada@example.com", "member", ws.ID, &org.ID)
	sid := student.ID
	at := time.Date(2026, 10, 12, 17, 20, 0, 0, time.UTC)
	rec := models.MHSDeviceTest{ID: primitive.NewObjectID(), WorkspaceID: ws.ID, Kind: models.MHSDeviceTestKindMember, UserID: &sid, OrganizationID: &org.ID,
		UnitID: "unit1", DeviceID: "dev-1", StartedAt: at, ExpiresAt: at.Add(6 * time.Hour), Diagnostics: map[string]interface{}{"outcome": "launch-ok"}, HeartbeatCount: 10,
		Summary: &models.MHSMemberSummary{V: models.MHSMemberSummaryVersion, Outcome: "launch-ok", Launch: []models.MHSStepLaunchEvent{{At: at, Kind: models.MHSLaunchEventRendering}}}}
	if _, err := db.Collection("mhs_device_tests").InsertOne(ctx, rec); err != nil {
		t.Fatal(err)
	}
	store := mhsactivitydays.New(db)
	deps := Deps{DB: db, Log: zap.NewNop()} // no log service: partial
	// 2026-10-13 14:00 UTC: the 12th closed at 11:00 UTC (05:00 + 6 h); the 13th is open.
	now := time.Date(2026, 10, 13, 14, 0, 0, 0, time.UTC)
	cfg := JobConfig{CatchUpDays: 3, Now: func() time.Time { return now }}
	if err := RunRollup(ctx, deps, store, workspacestore.New(db), cfg); err != nil {
		t.Fatalf("run: %v", err)
	}
	days, _ := store.ListDays(ctx, mhsactivitydays.DaysQuery{WorkspaceID: ws.ID})
	if len(days) != 1 || days[0].Day != "2026-10-12" || !days[0].Final || !days[0].Launched || !days[0].Partial {
		t.Fatalf("stored days %+v", days)
	}
	marks, _ := store.GetMarks(ctx, ws.ID, "2026-10-10", "2026-10-13", nil)
	if len(marks) != 3 { // the 10th, 11th and 12th; not the 13th
		t.Fatalf("marks %d", len(marks))
	}
	m12 := marks[mhsactivitydays.MarkKey(org.ID, "2026-10-12")]
	if m12.Final || !m12.Partial || m12.Attempts != 1 || m12.Students != 1 {
		t.Errorf("mark for the 12th %+v", m12)
	}
	// A day with no activity at all has nothing to be partial about: final.
	m11 := marks[mhsactivitydays.MarkKey(org.ID, "2026-10-11")]
	if !m11.Final || m11.Students != 0 {
		t.Errorf("mark for the 11th %+v", m11)
	}
	// A second run retries the partial day (attempts 2), still not final.
	if err := RunRollup(ctx, deps, store, workspacestore.New(db), cfg); err != nil {
		t.Fatal(err)
	}
	marks, _ = store.GetMarks(ctx, ws.ID, "2026-10-12", "2026-10-12", nil)
	if m := marks[mhsactivitydays.MarkKey(org.ID, "2026-10-12")]; m.Final || m.Attempts != 2 {
		t.Errorf("after retry %+v", m)
	}
	// Mark it final by hand; a run skips it, force recomputes it (attempts back to 1).
	m12 = marks[mhsactivitydays.MarkKey(org.ID, "2026-10-12")]
	m12.Final = true
	if err := store.SetMark(ctx, m12); err != nil {
		t.Fatal(err)
	}
	res, err := RollupDay(ctx, deps, store, ws.ID, "2026-10-12", Zone{TZ: "America/Chicago", Loc: chicago, OrgIDs: []primitive.ObjectID{org.ID}}, false)
	if err != nil || !res.Skipped {
		t.Errorf("final day not skipped: %v %+v", err, res)
	}
	res, err = RollupDay(ctx, deps, store, ws.ID, "2026-10-12", Zone{TZ: "America/Chicago", Loc: chicago, OrgIDs: []primitive.ObjectID{org.ID}}, true)
	if err != nil || res.Skipped || res.Docs != 1 {
		t.Errorf("force: %v %+v", err, res)
	}
	marks, _ = store.GetMarks(ctx, ws.ID, "2026-10-12", "2026-10-12", nil)
	if m := marks[mhsactivitydays.MarkKey(org.ID, "2026-10-12")]; m.Attempts != 1 {
		t.Errorf("force resets attempts: %+v", m)
	}
	// An open day is never closed for the job.
	if Closed("2026-10-13", chicago, 6*time.Hour, now) {
		t.Error("the 13th should be open")
	}
}

func TestBackfillRunsARangeAndStamps(t *testing.T) {
	db := testutil.SetupTestDB(t)
	ctx, cancel := testutil.TestContext()
	defer cancel()
	f := testutil.NewFixtures(t, db)
	ws := f.CreateWorkspace(ctx, "MHS", "mhs")
	org := f.CreateOrganizationWithDetails(ctx, "School A", "Park Hills", "MO", "America/Chicago")
	if _, err := db.Collection("organizations").UpdateByID(ctx, org.ID, bson.M{"$set": bson.M{"workspace_id": ws.ID}}); err != nil {
		t.Fatal(err)
	}
	student := f.CreateUserInWorkspace(ctx, "Ada", "ada@example.com", "member", ws.ID, &org.ID)
	sid := student.ID
	at := time.Date(2026, 10, 8, 17, 20, 0, 0, time.UTC)
	rec := models.MHSDeviceTest{ID: primitive.NewObjectID(), WorkspaceID: ws.ID, Kind: models.MHSDeviceTestKindMember, UserID: &sid, OrganizationID: &org.ID,
		UnitID: "unit1", DeviceID: "dev-1", StartedAt: at, ExpiresAt: at.Add(6 * time.Hour), Diagnostics: map[string]interface{}{"outcome": "launch-ok"},
		Steps: []models.MHSDeviceTestStep{{At: at, Step: "game", State: "ok", Msg: "Game is rendering"}}}
	if _, err := db.Collection("mhs_device_tests").InsertOne(ctx, rec); err != nil {
		t.Fatal(err)
	}
	store := mhsactivitydays.New(db)
	b := NewBackfiller(Deps{DB: db, Log: zap.NewNop()}, store, mhsdevicetestsStore(db))
	b.Now = func() time.Time { return time.Date(2026, 10, 13, 12, 0, 0, 0, time.UTC) }
	if err := b.Start(BackfillRequest{WorkspaceID: ws.ID, From: "2026-10-07", To: "2026-10-09", Stamp: true}); err != nil {
		t.Fatal(err)
	}
	if err := b.Start(BackfillRequest{WorkspaceID: ws.ID, From: "2026-10-07", To: "2026-10-09"}); err != ErrBackfillRunning && b.Status().Running {
		t.Errorf("second start while running: %v", err)
	}
	deadline := time.Now().Add(30 * time.Second)
	for b.Status().Running && time.Now().Before(deadline) {
		time.Sleep(50 * time.Millisecond)
	}
	st := b.Status()
	if st.Running || st.Total != 3 || st.Done != 3 || st.Written != 1 || st.Stamped != 1 || len(st.Errors) != 0 {
		t.Fatalf("status %+v", st)
	}
	days, _ := store.ListDays(ctx, mhsactivitydays.DaysQuery{WorkspaceID: ws.ID})
	if len(days) != 1 || days[0].Day != "2026-10-08" || !days[0].Launched {
		t.Errorf("stored %+v", days)
	}
	var stamped models.MHSDeviceTest
	if err := db.Collection("mhs_device_tests").FindOne(ctx, bson.M{"_id": rec.ID}).Decode(&stamped); err != nil || stamped.Summary == nil || len(stamped.Summary.Launch) != 1 {
		t.Errorf("summary not stamped: %v %+v", err, stamped.Summary)
	}
	if _, err := DaysBetween("2026-10-09", "2026-10-07"); err == nil {
		t.Error("reversed range accepted")
	}
	if err := b.Start(BackfillRequest{WorkspaceID: ws.ID, From: "2026-10-09", To: "2026-10-07"}); err == nil {
		t.Error("reversed backfill accepted")
	}
}
