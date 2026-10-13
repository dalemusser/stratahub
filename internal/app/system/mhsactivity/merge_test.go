package mhsactivity

import (
	"testing"
	"time"

	"github.com/dalemusser/stratahub/internal/app/store/activity"
	"github.com/dalemusser/stratahub/internal/app/store/logdata"
	"github.com/dalemusser/stratahub/internal/domain/models"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

var chicago = func() *time.Location { l, _ := time.LoadLocation("America/Chicago"); return l }()

// ct is a Central-time instant on 2026-10-08.
func ct(h, m, s int) time.Time { return time.Date(2026, 10, 8, h, m, s, 0, chicago) }

func tp(t time.Time) *time.Time { return &t }

func testBuilder(t *testing.T) (*builder, *person, *person) {
	t.Helper()
	ws, org := primitive.NewObjectID(), primitive.NewObjectID()
	group := primitive.NewObjectID()
	student := &person{ID: primitive.NewObjectID(), Org: org, Role: models.MHSActivityRoleMember, Groups: []primitive.ObjectID{group}}
	teacher := &person{ID: primitive.NewObjectID(), Org: org, Role: models.MHSActivityRoleStaff, Groups: []primitive.ObjectID{group}}
	people := map[primitive.ObjectID]*person{student.ID: student, teacher.ID: teacher}
	zone := Zone{TZ: "America/Chicago", Loc: chicago, OrgIDs: []primitive.ObjectID{org}}
	start, end, _ := DayBounds("2026-10-08", chicago)
	return newBuilder(ws, "2026-10-08", zone, start, end, people), student, teacher
}

func rec(user *person, dev, unit, outcome string, at time.Time) models.MHSDeviceTest {
	id := user.ID
	return models.MHSDeviceTest{ID: primitive.NewObjectID(), UserID: &id, OrganizationID: &user.Org, UnitID: unit, DeviceID: dev, DeviceType: "Chromebook",
		UserAgent: "Mozilla/5.0 (X11; CrOS x86_64 16805.31.0) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/152.0.0.0 Safari/537.36",
		StartedAt: at.UTC(), Diagnostics: map[string]interface{}{"outcome": outcome}}
}

func TestMergeHeldThenReleasedAcrossRecords(t *testing.T) {
	b, student, _ := testBuilder(t)
	held := models.MHSStepUnitSummary{Unit: "unit1", Version: "2.8.1", FirstAt: ct(12, 4, 0), LastAt: ct(12, 20, 0), Requested: true, TotalMB: 213, MaxMB: 201, Progress: 5, Drops: 6, Stalls: 1, Fails: 1, LastErrorClass: "stalled", Held: true, HeldAtMB: 201, HeldAt: tp(ct(12, 5, 0))}
	// The failed record, then a later record from the same tab that repeats
	// the hold and adds the completion.
	b.addRecord(rec(student, "dev-a", "unit1", "download-failed", ct(12, 20, 0)), &models.MHSMemberSummary{V: 1, Units: []models.MHSStepUnitSummary{held}})
	done := held
	done.LastAt = ct(12, 25, 0)
	done.Complete = true
	done.CompleteAt = tp(ct(12, 25, 0))
	done.CompleteMB = 213
	done.CompleteSeconds = 1260
	done.Drops = 7
	b.addRecord(rec(student, "dev-a", "unit1", "download-complete", ct(12, 25, 0)), &models.MHSMemberSummary{V: 1, Units: []models.MHSStepUnitSummary{done}})
	docs := b.finish(time.Now())
	if len(docs) != 1 {
		t.Fatalf("docs %d", len(docs))
	}
	d := docs[0]
	if len(d.Units) != 1 {
		t.Fatalf("units %+v", d.Units)
	}
	u := d.Units[0]
	if u.State != models.MHSActivityUnitHeldDone || u.HoldSeconds != 20*60 || u.Drops != 7 || u.Fails != 1 || u.Records != 2 || u.HeldAtMB != 201 || u.TotalMB != 213 {
		t.Errorf("merged unit %+v", u)
	}
	if d.Records != 2 || !d.Opened || d.Blocked || !d.Flags.Held || !d.Flags.DownloadFailed {
		t.Errorf("doc %+v", d)
	}
	if len(d.Devices) != 1 || d.Devices[0].Platform != "ChromeOS" || d.Devices[0].Browser != "Chrome 152" || d.Devices[0].Records != 2 {
		t.Errorf("devices %+v", d.Devices)
	}
	if d.Role != models.MHSActivityRoleMember || len(d.GroupIDs) != 1 {
		t.Errorf("identity %+v", d)
	}
}

func TestMergeBlockedWhenHeldAndNeverPlayed(t *testing.T) {
	b, student, _ := testBuilder(t)
	held := models.MHSStepUnitSummary{Unit: "unit1", FirstAt: ct(12, 4, 0), LastAt: ct(12, 40, 0), Requested: true, TotalMB: 213, MaxMB: 201, Held: true, HeldAtMB: 201, HeldAt: tp(ct(12, 5, 0))}
	b.addRecord(rec(student, "dev-a", "unit1", "download-failed", ct(12, 40, 0)), &models.MHSMemberSummary{V: 1, Units: []models.MHSStepUnitSummary{held}})
	b.logAvailable = true
	d := b.finish(time.Now())[0]
	if !d.Blocked || d.Played || d.Units[0].State != models.MHSActivityUnitHeld {
		t.Errorf("%+v", d)
	}
	// The same student with log events is not blocked.
	b2, student2, _ := testBuilder(t)
	b2.addRecord(rec(student2, "dev-a", "unit1", "download-failed", ct(12, 40, 0)), &models.MHSMemberSummary{V: 1, Units: []models.MHSStepUnitSummary{held}})
	b2.logAvailable = true
	b2.addLog(student2.ID, logdata.UserActivity{Events: 40, First: ct(12, 50, 0), Last: ct(13, 10, 0)})
	d2 := b2.finish(time.Now())[0]
	if d2.Blocked || !d2.Played || d2.Log.Events != 40 || d2.Log.Source != "logdata" {
		t.Errorf("%+v", d2)
	}
}

func TestMergeLaunchEventsDedupeAndStallRecovery(t *testing.T) {
	b, student, _ := testBuilder(t)
	launch := []models.MHSStepLaunchEvent{
		{At: ct(12, 9, 59), Kind: models.MHSLaunchEventLoaderOK, Source: "cache"},
		{At: ct(12, 10, 35), Kind: models.MHSLaunchEventWatchdogWarn, Seconds: 33, Phase: "almost ready"},
	}
	stalled := &models.MHSMemberSummary{V: 1, Launch: launch}
	b.addRecord(rec(student, "dev-a", "unit1", "launch-stalled", ct(12, 10, 36)), stalled)
	// The launch-ok record repeats the two entries (the warning's time moved
	// by a second by the page's repeat collapsing) and adds the start.
	ok := &models.MHSMemberSummary{V: 1, Launch: append([]models.MHSStepLaunchEvent{
		launch[0],
		{At: ct(12, 10, 36), Kind: models.MHSLaunchEventWatchdogWarn, Seconds: 34, Phase: "almost ready"},
	}, models.MHSStepLaunchEvent{At: ct(12, 10, 41), Kind: models.MHSLaunchEventUnityOK, Seconds: 42.1},
		models.MHSStepLaunchEvent{At: ct(12, 10, 42), Kind: models.MHSLaunchEventRendering}),
		Crashes: []models.MHSStepCrash{
			{At: ct(12, 30, 0), Type: "unhandled_rejection", Phase: "game", Notice: true, Message: "A user gesture is required to request Pointer Lock."},
			{At: ct(12, 30, 0), Type: "unhandled_rejection", Phase: "game", Notice: true, Message: "A user gesture is required to request Pointer Lock."},
		}}
	okRec := rec(student, "dev-a", "unit1", "launch-ok", ct(12, 10, 42))
	okRec.HeartbeatCount = 12
	b.addRecord(okRec, ok)
	// A crash record repeating the notice plus a real crash.
	b.addRecord(rec(student, "dev-a", "unit1", "crash", ct(12, 55, 0)), &models.MHSMemberSummary{V: 1, Launch: ok.Launch, Crashes: append(ok.Crashes,
		models.MHSStepCrash{At: ct(12, 55, 0), Type: "runtime_error", Phase: "running", Message: "RuntimeError: memory access out of bounds"})})
	d := b.finish(time.Now())[0]
	l := d.Launch
	if l.OK != 1 || l.Stalled != 1 || l.StalledRecovered != 1 || l.Failed != 0 || l.LoaderCache != 1 || l.UnitySecondsMax != 42.1 {
		t.Errorf("launch %+v", l)
	}
	if d.Crashes.Notice != 1 || d.Crashes.Real != 1 || len(d.Crashes.Types) != 1 || d.Crashes.Types[0] != "runtime_error" {
		t.Errorf("crashes %+v", d.Crashes)
	}
	if !d.Launched || d.Records != 3 {
		t.Errorf("doc %+v", d)
	}
	// Without the game log, heartbeats stand in for played.
	if !d.Played || d.Log.Source != "launch" {
		t.Errorf("played fallback: played=%v source=%q", d.Played, d.Log.Source)
	}
}

func TestMergeAttributesEventsToTheirOwnDay(t *testing.T) {
	b, student, _ := testBuilder(t)
	// Stored at 00:10 on the 9th (within close-after of the 8th) with one
	// unit from the 8th and a launch from the 9th.
	late := time.Date(2026, 10, 9, 0, 10, 0, 0, chicago)
	sum := &models.MHSMemberSummary{V: 1,
		Units:  []models.MHSStepUnitSummary{{Unit: "unit2", FirstAt: ct(23, 40, 0), LastAt: ct(23, 55, 0), Requested: true, TotalMB: 156, MaxMB: 156, Complete: true, CompleteAt: tp(ct(23, 55, 0)), CompleteMB: 156}},
		Launch: []models.MHSStepLaunchEvent{{At: late, Kind: models.MHSLaunchEventRendering}},
	}
	b.addRecord(rec(student, "dev-a", "unit2", "launch-ok", late), sum)
	docs := b.finish(time.Now())
	if len(docs) != 1 {
		t.Fatalf("docs %d", len(docs))
	}
	d := docs[0]
	if d.Records != 0 || len(d.Units) != 1 || d.Units[0].State != models.MHSActivityUnitComplete || d.Launch.OK != 0 {
		t.Errorf("%+v", d)
	}
	if !d.Opened || d.FirstAt == nil || !d.FirstAt.Equal(ct(23, 40, 0)) {
		t.Errorf("opened/first %+v %v", d.Opened, d.FirstAt)
	}
}

func TestMergeDevicesPageViewsAndStaff(t *testing.T) {
	b, student, teacher := testBuilder(t)
	b.addDevice(models.MHSDeviceStatus{UserID: student.ID, DeviceID: "dev-new", DeviceType: "Chromebook", CreatedAt: ct(12, 1, 0).UTC(), LastSeen: ct(12, 30, 0).UTC(),
		DeviceDetails: map[string]string{"user_agent": "Mozilla/5.0 (X11; CrOS x86_64 16805.31.0) Chrome/150.0.0.0"}, StoragePersisted: boolp(false), StorageQuota: 2 * 1024 * 1024 * 1024})
	// An old row seen another day: ignored.
	b.addDevice(models.MHSDeviceStatus{UserID: student.ID, DeviceID: "dev-old", CreatedAt: time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC), LastSeen: time.Date(2026, 9, 2, 0, 0, 0, 0, time.UTC)})
	b.addPageViews(student.ID, activity.PageViewSummary{Count: 2, First: ct(12, 0, 30).UTC(), Last: ct(12, 1, 0).UTC()})
	b.addRecord(rec(teacher, "dev-t", "unit1", "launch-ok", ct(8, 0, 0)), &models.MHSMemberSummary{V: 1, Launch: []models.MHSStepLaunchEvent{{At: ct(8, 0, 0), Kind: models.MHSLaunchEventRendering}}})
	docs := b.finish(time.Now())
	if len(docs) != 2 {
		t.Fatalf("docs %d", len(docs))
	}
	var s, st *models.MHSActivityDay
	for i := range docs {
		if docs[i].UserID == student.ID {
			s = &docs[i]
		} else {
			st = &docs[i]
		}
	}
	if s == nil || st == nil {
		t.Fatal("missing docs")
	}
	if !s.Opened || s.PageViews != 2 || len(s.Devices) != 1 || !s.Devices[0].New || s.Devices[0].Platform != "ChromeOS" || s.Devices[0].StorageQuotaMB != 2048 || !s.Flags.NewDevice || !s.Flags.StorageNotPersisted {
		t.Errorf("student %+v devices %+v", s, s.Devices)
	}
	if s.FirstAt == nil || !s.FirstAt.Equal(ct(12, 0, 30).UTC()) {
		t.Errorf("first %v", s.FirstAt)
	}
	if st.Role != models.MHSActivityRoleStaff || st.Launch.OK != 1 || !st.Launched {
		t.Errorf("staff %+v", st)
	}
}

func TestPlatformAndBrowser(t *testing.T) {
	cases := map[string][2]string{
		"Mozilla/5.0 (X11; CrOS x86_64 14541.0.0) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/152.0.0.0 Safari/537.36":                    {"ChromeOS", "Chrome 152"},
		"Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/154.0.0.0 Safari/537.36 Edg/154.0.0.0":     {"Windows", "Edge 154"},
		"Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/18.1 Safari/605.1.15":             {"macOS", "Safari 18"},
		"Mozilla/5.0 (iPad; CPU OS 18_0 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) CriOS/150.0.0.0 Mobile/15E148 Safari/604.1": {"iPadOS", "Chrome 150"},
		"": {"", ""},
	}
	for ua, want := range cases {
		if got := PlatformOf(ua); got != want[0] {
			t.Errorf("PlatformOf(%q) = %q, want %q", ua, got, want[0])
		}
		if got := BrowserOf(ua); got != want[1] {
			t.Errorf("BrowserOf(%q) = %q, want %q", ua, got, want[1])
		}
	}
}

func boolp(b bool) *bool { return &b }

func TestMergeStrayEventsAreNotPlayed(t *testing.T) {
	b, student, _ := testBuilder(t)
	b.logAvailable = true
	b.addLog(student.ID, logdata.UserActivity{Events: 1, First: ct(0, 39, 30), Last: ct(0, 39, 30)})
	docs := b.finish(time.Now())
	if len(docs) != 1 {
		t.Fatalf("docs %d", len(docs))
	}
	d := docs[0]
	if d.Played || d.Opened || !d.Flags.StrayEvents || d.Log.Events != 1 || d.Blocked {
		t.Errorf("stray day %+v", d)
	}
	// With a record that day the events are the student's own.
	b2, s2, _ := testBuilder(t)
	b2.logAvailable = true
	b2.addRecord(rec(s2, "dev-a", "unit1", "launch-ok", ct(12, 0, 0)), &models.MHSMemberSummary{V: 1, Launch: []models.MHSStepLaunchEvent{{At: ct(12, 0, 0), Kind: models.MHSLaunchEventRendering}}})
	b2.addLog(s2.ID, logdata.UserActivity{Events: 3, First: ct(12, 1, 0), Last: ct(12, 2, 0)})
	d2 := b2.finish(time.Now())[0]
	if d2.Flags.StrayEvents || !d2.Opened || d2.Played {
		t.Errorf("few events after a launch %+v", d2)
	}
	// Enough events: played.
	b3, s3, _ := testBuilder(t)
	b3.logAvailable = true
	b3.addLog(s3.ID, logdata.UserActivity{Events: models.MHSActivityMinPlayedEvents, First: ct(12, 1, 0), Last: ct(12, 40, 0)})
	if d3 := b3.finish(time.Now())[0]; !d3.Played || !d3.Opened || d3.Flags.StrayEvents {
		t.Errorf("played %+v", d3)
	}
}
