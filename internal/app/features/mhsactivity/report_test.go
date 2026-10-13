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
