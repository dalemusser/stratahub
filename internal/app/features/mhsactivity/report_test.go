package mhsactivity

import (
	"strings"
	"testing"
	"time"

	"github.com/dalemusser/stratahub/internal/domain/models"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

func TestBuildReport(t *testing.T) {
	chicago, _ := time.LoadLocation("America/Chicago")
	orgA, orgB := primitive.NewObjectID(), primitive.NewObjectID()
	g1 := primitive.NewObjectID()
	u1, u2, u3, teacher := primitive.NewObjectID(), primitive.NewObjectID(), primitive.NewObjectID(), primitive.NewObjectID()
	lk := Lookups{
		Orgs:   map[primitive.ObjectID]OrgInfo{orgA: {Name: "School A", TZ: "America/Chicago", Loc: chicago}, orgB: {Name: "School B", TZ: "America/Chicago", Loc: chicago}},
		Groups: map[primitive.ObjectID]GroupInfo{g1: {Name: "Period 5", OrgID: orgA}},
		Users:  map[primitive.ObjectID]string{u1: "Ada", u2: "Ben", teacher: "Tess"},
	}
	at := time.Date(2026, 10, 12, 17, 30, 0, 0, time.UTC)
	held := models.MHSActivityUnit{Unit: "unit1", DeviceID: "d1", State: models.MHSActivityUnitHeld, TotalMB: 213, MaxMB: 201, HeldAtMB: 201, LastAt: at}
	done := models.MHSActivityUnit{Unit: "unit1", DeviceID: "d2", State: models.MHSActivityUnitComplete, Complete: true, CompleteMB: 213, CompleteSeconds: 22, LastAt: at.Add(time.Hour)}
	docs := []models.MHSActivityDay{
		{UserID: u1, OrganizationID: orgA, GroupIDs: []primitive.ObjectID{g1}, Role: "member", Day: "2026-10-12", Opened: true, Blocked: true, Units: []models.MHSActivityUnit{held},
			Devices: []models.MHSActivityDevice{{DeviceID: "d1", Platform: "ChromeOS", New: true}}, Flags: models.MHSActivityFlags{Held: true, BgfetchRefused: true}, LastAt: &at},
		{UserID: u2, OrganizationID: orgA, GroupIDs: []primitive.ObjectID{g1}, Role: "member", Day: "2026-10-12", Opened: true, Played: true, Launched: true, Units: []models.MHSActivityUnit{done},
			Launch: models.MHSActivityLaunch{OK: 2, Stalled: 1, StalledRecovered: 1, UnitySecondsMax: 42.1}, Crashes: models.MHSActivityCrashes{Notice: 3},
			Devices: []models.MHSActivityDevice{{DeviceID: "d2", Platform: "ChromeOS", New: true}}, CurrentUnit: "unit2"},
		{UserID: u3, OrganizationID: orgA, Role: "member", Day: "2026-10-11", Opened: true, Played: true, Devices: []models.MHSActivityDevice{{DeviceID: "d3", Platform: "Windows"}}},
		{UserID: teacher, OrganizationID: orgA, Role: "staff", Day: "2026-10-12", Opened: true, Launched: true, Launch: models.MHSActivityLaunch{OK: 1}},
		{UserID: primitive.NewObjectID(), OrganizationID: orgB, Role: "member", Day: "2026-10-12", Opened: true, Played: true, Units: []models.MHSActivityUnit{{Unit: "unit3", State: models.MHSActivityUnitCached}}},
	}

	rep := BuildReport(docs, lk, Options{MultiDay: true})
	if rep.Summary.Students != 4 || rep.Summary.Opened != 4 || rep.Summary.Played != 3 || rep.Summary.Blocked != 1 {
		t.Errorf("summary students %+v", rep.Summary)
	}
	if rep.Summary.DlComplete != 1 || rep.Summary.DlHeld != 1 || rep.Summary.DlCached != 1 || rep.Summary.LaunchOK != 2 || rep.Summary.LaunchRecovered != 1 || rep.Summary.CrashNotice != 3 || rep.Summary.DevicesNew != 2 || rep.Summary.DevicesSeen != 3 {
		t.Errorf("summary counts %+v", rep.Summary)
	}
	if rep.StaffLaunches != 1 || rep.SchoolsActive != 2 || rep.SchoolsWithProblems != 1 {
		t.Errorf("schools %+v", rep)
	}
	if len(rep.Schools) != 2 || rep.Schools[0].Name != "School A" || !rep.Schools[0].Problem() || rep.Schools[1].Problem() {
		t.Errorf("school order %+v", rep.Schools)
	}
	a := rep.Schools[0]
	if a.Opened != 3 || a.Blocked != 1 || a.StudentDays != 3 || a.Platforms != "ChromeOS 2, Windows 1" || a.LastActivity == "" || a.Churn {
		t.Errorf("school A %+v", a)
	}
	pills := pillTexts(a.Pills)
	for _, want := range []string{"1 student blocked", "1 download held", "background fetch refused", "3 browser notices"} {
		if !strings.Contains(pills, want) {
			t.Errorf("school A pills %q lack %q", pills, want)
		}
	}
	if !strings.Contains(pills, "amber") {
		t.Errorf("blocked school has no amber pill: %+v", a.Pills)
	}
	if len(rep.Days) != 3 || rep.Days[0].Day != "2026-10-12" || rep.Days[2].Day != "2026-10-11" {
		t.Errorf("days %+v", rep.Days)
	}

	// The school view.
	rep = BuildReport(docs, lk, Options{SelectedOrg: orgA, ShowStaff: true})
	if rep.School == nil || rep.School.Name != "School A" || len(rep.School.Groups) != 2 || len(rep.School.Students) != 3 || len(rep.School.Staff) != 1 {
		t.Fatalf("school view %+v", rep.School)
	}
	if rep.School.Groups[0].Name != "(no group)" || rep.School.Groups[1].Name != "Period 5" || rep.School.Groups[1].Opened != 2 || rep.School.Groups[1].DashboardHref != "/mhsdashboard?group="+g1.Hex() {
		t.Errorf("groups %+v", rep.School.Groups)
	}
	// Blocked students first, then by name.
	st := rep.School.Students
	if st[0].Name != "Ada" || !st[0].Blocked || st[1].Name != "Ben" || st[2].Name != "Member …"+u3.Hex()[18:] {
		t.Errorf("student order %v %v %v", st[0].Name, st[1].Name, st[2].Name)
	}
	if len(st[0].Units) != 1 || st[0].Units[0].Text != "held at 201/213 MB" || st[0].Units[0].Label != "U1" || !strings.Contains(st[0].Units[0].Class, "amber") {
		t.Errorf("Ada units %+v", st[0].Units)
	}
	if st[1].Units[0].Text != "done 213 MB in 0:22" || st[1].CurrentUnit != "U2" || st[1].Launch.OK != 2 || st[1].Groups != "Period 5" {
		t.Errorf("Ben row %+v", st[1])
	}
	if p := pillTexts(st[1].Pills); !strings.Contains(p, "1 slow start (recovered)") || !strings.Contains(p, "3 notices") || strings.Contains(p, "amber") {
		t.Errorf("Ben pills %q", p)
	}
	if !rep.School.Staff[0].Staff || rep.School.Staff[0].Name != "Tess" {
		t.Errorf("staff %+v", rep.School.Staff)
	}

	// A group selection narrows to its members.
	rep = BuildReport(docs, lk, Options{SelectedOrg: orgA, SelectedGroup: g1})
	if len(rep.School.Students) != 2 || len(rep.School.Groups) != 1 {
		t.Errorf("group view %+v", rep.School)
	}
}

func TestStudentStatusText(t *testing.T) {
	chicago, _ := time.LoadLocation("America/Chicago")
	org, u1, u2 := primitive.NewObjectID(), primitive.NewObjectID(), primitive.NewObjectID()
	lk := Lookups{Orgs: map[primitive.ObjectID]OrgInfo{org: {Name: "School A", Loc: chicago}}, Users: map[primitive.ObjectID]string{u1: "Ada", u2: "Bo"}}
	f1, l1 := time.Date(2026, 10, 12, 17, 4, 0, 0, time.UTC), time.Date(2026, 10, 12, 17, 53, 0, 0, time.UTC)
	f2 := time.Date(2026, 10, 13, 5, 39, 30, 0, time.UTC)
	docs := []models.MHSActivityDay{
		{UserID: u1, OrganizationID: org, Role: "member", Day: "2026-10-12", Opened: true, Played: true, Log: models.MHSActivityLog{Available: true, Events: 312, First: &f1, Last: &l1}},
		{UserID: u2, OrganizationID: org, Role: "member", Day: "2026-10-13", Log: models.MHSActivityLog{Available: true, Events: 1, First: &f2, Last: &f2}, Flags: models.MHSActivityFlags{StrayEvents: true}},
	}
	rep := BuildReport(docs, lk, Options{SelectedOrg: org, MultiDay: true})
	if rep.Summary.Opened != 1 || rep.Summary.Played != 1 || rep.Summary.Students != 2 {
		t.Errorf("summary %+v", rep.Summary)
	}
	st := rep.School.Students
	if len(st) != 2 {
		t.Fatalf("students %d", len(st))
	}
	ada, bo := st[0], st[1]
	if ada.Name != "Ada" || !ada.Played || ada.Events != 312 || ada.PlaySpan != "12:04–12:53 PM" || ada.Stray {
		t.Errorf("Ada %+v", ada)
	}
	if bo.Name != "Bo" || bo.Played || bo.Opened || !bo.Stray || bo.Events != 1 || bo.PlaySpan != "12:39–12:39 AM" || !strings.Contains(pillTexts(bo.Pills), "earlier tab") {
		t.Errorf("Bo %+v pills %s", bo, pillTexts(bo.Pills))
	}
	if got := spanText(time.Date(2026, 10, 12, 16, 50, 0, 0, time.UTC), l1, chicago); got != "11:50 AM–12:53 PM" {
		t.Errorf("span across noon %q", got)
	}
}

func TestChurnFlag(t *testing.T) {
	org := primitive.NewObjectID()
	var docs []models.MHSActivityDay
	for i := 0; i < 6; i++ {
		docs = append(docs, models.MHSActivityDay{UserID: primitive.NewObjectID(), OrganizationID: org, Role: "member", Day: "2026-10-12", Opened: true, Played: true,
			Devices: []models.MHSActivityDevice{{DeviceID: primitive.NewObjectID().Hex(), New: i < 5}}})
	}
	rep := BuildReport(docs, Lookups{}, Options{})
	if len(rep.Schools) != 1 || !rep.Schools[0].Churn || !strings.Contains(pillTexts(rep.Schools[0].Pills), "new browser profile every day") {
		t.Errorf("churn %+v", rep.Schools)
	}
	docs[0].Devices[0].New = false
	docs[1].Devices[0].New = false
	if rep := BuildReport(docs, Lookups{}, Options{}); rep.Schools[0].Churn {
		t.Error("3 of 6 new is not churn")
	}
}

func TestUnitText(t *testing.T) {
	cases := []struct {
		u    models.MHSActivityUnit
		want string
	}{
		{models.MHSActivityUnit{State: models.MHSActivityUnitHeldDone, HeldAtMB: 201, TotalMB: 213, HoldSeconds: 162}, "held at 201/213 MB, done after 2.7 min"},
		{models.MHSActivityUnit{State: models.MHSActivityUnitHeldDone, HeldAtMB: 201, TotalMB: 213, HoldSeconds: 45}, "held at 201/213 MB, done after 45 s"},
		{models.MHSActivityUnit{State: models.MHSActivityUnitFailed, LastErrorClass: "network"}, "failed (network)"},
		{models.MHSActivityUnit{State: models.MHSActivityUnitCached}, "on device"},
		{models.MHSActivityUnit{State: models.MHSActivityUnitInProgress, MaxMB: 26, TotalMB: 213}, "12% (26/213 MB)"},
		{models.MHSActivityUnit{State: models.MHSActivityUnitInProgress}, "started"},
		{models.MHSActivityUnit{State: models.MHSActivityUnitComplete, CompleteMB: 156}, "done 156 MB"},
	}
	for _, c := range cases {
		if got := unitText(c.u); got != c.want {
			t.Errorf("unitText = %q, want %q", got, c.want)
		}
	}
}

func pillTexts(ps []Pill) string {
	var b strings.Builder
	for _, p := range ps {
		b.WriteString(p.Text)
		if strings.Contains(p.Class, "amber") {
			b.WriteString("[amber]")
		}
		b.WriteString("; ")
	}
	return b.String()
}

// Issue 3's signature: the game ran for most of a school's students and none
// of their activity arrived (amber at the school, gray per student); one
// such student is a gray note; a page that found the services blocked makes
// the school amber by itself; a day computed without the log service counts
// for neither.
func TestReportActivityNotArriving(t *testing.T) {
	orgJ, orgK, orgL := primitive.NewObjectID(), primitive.NewObjectID(), primitive.NewObjectID()
	lk := Lookups{Orgs: map[primitive.ObjectID]OrgInfo{orgJ: {Name: "School J", Loc: time.UTC}, orgK: {Name: "School K", Loc: time.UTC}, orgL: {Name: "School L", Loc: time.UTC}}}
	day := func(org primitive.ObjectID, played, available, blocked bool) models.MHSActivityDay {
		return models.MHSActivityDay{UserID: primitive.NewObjectID(), OrganizationID: org, Role: "member", Day: "2026-11-05",
			Opened: true, Launched: true, Played: played, Launch: models.MHSActivityLaunch{OK: 1},
			Log: models.MHSActivityLog{Available: available}, Flags: models.MHSActivityFlags{ServicesBlocked: blocked}}
	}
	var docs []models.MHSActivityDay
	for i := 0; i < 11; i++ { // J: 11 of 13 launched with nothing arriving
		docs = append(docs, day(orgJ, false, true, false))
	}
	docs = append(docs, day(orgJ, true, true, false), day(orgJ, true, true, false))
	for i := 0; i < 5; i++ { // K: one of six, and one day without the log service
		docs = append(docs, day(orgK, true, true, false))
	}
	docs = append(docs, day(orgK, false, true, false), day(orgK, false, false, false))
	docs = append(docs, day(orgL, false, true, true)) // L: one page found the services blocked

	rep := BuildReport(docs, lk, Options{SelectedOrg: orgJ})
	by := map[string]SchoolRow{}
	for _, s := range rep.Schools {
		by[s.Name] = s
	}
	j, k, l := by["School J"], by["School K"], by["School L"]
	if j.Launched != 13 || j.NoActivity != 11 || !j.ActivityNotArriving() || !j.Problem() {
		t.Errorf("School J %+v", j.Counts)
	}
	if p := pillTexts(j.Pills); !strings.Contains(p, "no game activity reaching the server from 11 students") || !strings.Contains(p, "amber") {
		t.Errorf("School J pills %q", p)
	}
	if k.NoActivity != 1 || k.ActivityNotArriving() || k.Problem() {
		t.Errorf("School K %+v", k.Counts)
	}
	if p := pillTexts(k.Pills); !strings.Contains(p, "1 student launched, no activity arrived") || strings.Contains(p, "amber") {
		t.Errorf("School K pills %q", p)
	}
	if l.ServicesBlocked != 1 || !l.ActivityNotArriving() {
		t.Errorf("School L %+v", l.Counts)
	}
	if p := pillTexts(l.Pills); !strings.Contains(p, "filter blocking the game for 1 student (1 with no activity arriving)") {
		t.Errorf("School L pills %q", p)
	}
	n := 0
	for _, st := range rep.School.Students {
		if st.NoActivity {
			n++
			if p := pillTexts(st.Pills); !strings.Contains(p, "no activity arrived") {
				t.Errorf("student pills %q", p)
			}
		}
	}
	if n != 11 {
		t.Errorf("students with no activity arrived: %d, want 11", n)
	}
}

// School C's pattern (Issue 2): the same unit held at the tail on several of
// a student's download days with no completion since is "held at the tail
// since" its first day; a completion in between restarts the count; one held
// day is not enough; another student's held day does not join it.
func TestReportStuckAtTheTail(t *testing.T) {
	org := primitive.NewObjectID()
	lk := Lookups{Orgs: map[primitive.ObjectID]OrgInfo{org: {Name: "School C", Loc: time.UTC}}}
	held := func(unit string) models.MHSActivityUnit {
		return models.MHSActivityUnit{Unit: unit, Version: "2.8.9", State: models.MHSActivityUnitHeld, TotalMB: 156, HeldAtMB: 144}
	}
	done := func(unit string) models.MHSActivityUnit {
		return models.MHSActivityUnit{Unit: unit, Version: "2.8.9", State: models.MHSActivityUnitComplete}
	}
	stuck, recovered, once := primitive.NewObjectID(), primitive.NewObjectID(), primitive.NewObjectID()
	day := func(u primitive.ObjectID, d string, units ...models.MHSActivityUnit) models.MHSActivityDay {
		return models.MHSActivityDay{UserID: u, OrganizationID: org, Role: "member", Day: d, Opened: true, Units: units}
	}
	docs := []models.MHSActivityDay{
		day(stuck, "2026-10-22", held("unit3")), day(stuck, "2026-10-23"), day(stuck, "2026-10-26", held("unit3")), day(stuck, "2026-10-27", held("unit3")),
		day(recovered, "2026-10-20", held("unit1")), day(recovered, "2026-10-21", done("unit1")), day(recovered, "2026-10-22", held("unit1")),
		day(once, "2026-10-27", held("unit3")),
	}
	rep := BuildReport(docs, lk, Options{SelectedOrg: org})
	if got := rep.Schools[0].StuckTail; got != 1 {
		t.Errorf("StuckTail = %d, want 1", got)
	}
	if p := pillTexts(rep.Schools[0].Pills); !strings.Contains(p, "1 student held on 2+ days") {
		t.Errorf("school pills %q", p)
	}
	for _, st := range rep.School.Students {
		switch st.UserID {
		case stuck.Hex():
			if st.StuckTailSince != "2026-10-22" || st.StuckTailUnit != "U3" {
				t.Errorf("stuck: since %q unit %q", st.StuckTailSince, st.StuckTailUnit)
			}
			if p := pillTexts(st.Pills); !strings.Contains(p, "U3 held at the tail since Oct 22") {
				t.Errorf("stuck pills %q", p)
			}
		case recovered.Hex(), once.Hex():
			if st.StuckTailSince != "" {
				t.Errorf("%s: since %q, want none", st.Name, st.StuckTailSince)
			}
		}
	}
}

// Issue 1 on the page: two discards in a day are amber for the student and
// the school; one is gray; a second game window is amber.
func TestReportDiscardsAndSecondWindow(t *testing.T) {
	org := primitive.NewObjectID()
	lk := Lookups{Orgs: map[primitive.ObjectID]OrgInfo{org: {Name: "School F", Loc: time.UTC}}}
	twice, once := primitive.NewObjectID(), primitive.NewObjectID()
	docs := []models.MHSActivityDay{
		{UserID: twice, OrganizationID: org, Role: "member", Day: "2026-10-27", Opened: true, Launched: true,
			Launch: models.MHSActivityLaunch{OK: 8, Discarded: 6, PageFrozen: 3}, Flags: models.MHSActivityFlags{ChromeClosed: true, SecondGameWindow: true}},
		{UserID: once, OrganizationID: org, Role: "member", Day: "2026-10-27", Opened: true, Launched: true,
			Launch: models.MHSActivityLaunch{OK: 2, Discarded: 1}},
	}
	rep := BuildReport(docs, lk, Options{SelectedOrg: org})
	s := rep.Schools[0]
	if s.LaunchDiscarded != 7 || s.Discarded != 2 || s.ChromeClosed != 1 || s.SecondWindow != 1 || !s.Problem() {
		t.Errorf("school %+v", s.Counts)
	}
	p := pillTexts(s.Pills)
	for _, want := range []string{"1 student had the game closed by Chrome", "1 student with 2 game windows open"} {
		if !strings.Contains(p, want) {
			t.Errorf("school pills %q lack %q", p, want)
		}
	}
	for _, st := range rep.School.Students {
		p := pillTexts(st.Pills)
		switch st.UserID {
		case twice.Hex():
			if !strings.Contains(p, "Chrome closed the game 6 times") || !strings.Contains(p, "2 game windows open") {
				t.Errorf("twice: %q", p)
			}
		case once.Hex():
			if !strings.Contains(p, "Chrome closed the game 1 time") {
				t.Errorf("once: %q", p)
			}
		}
	}
}
