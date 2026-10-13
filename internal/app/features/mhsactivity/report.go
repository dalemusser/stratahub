package mhsactivity

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/dalemusser/stratahub/internal/app/features/viewers"
	"github.com/dalemusser/stratahub/internal/domain/models"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

// Churn: an organization whose students start this share of their days on a
// fresh browser profile, over at least this many student-days, is losing its
// downloads between sessions.
const (
	churnShare   = 0.8
	churnMinDays = 5
)

// Counts are the numbers every row carries.
type Counts struct {
	Students    int // distinct students in the row's scope with any activity
	Opened      int
	Played      int
	Blocked     int // students with a problem day and no play in the window
	StudentDays int

	DlComplete   int
	DlHeldDone   int
	DlHeld       int
	DlFailed     int
	DlCached     int
	DlInProgress int

	LaunchOK        int
	LaunchStalled   int
	LaunchRecovered int
	LaunchFailed    int

	CrashReal   int
	CrashNotice int

	DevicesSeen int // distinct device ids
	DevicesNew  int // device rows created in the window (fresh browser profiles)

	BgfetchRefused int // students whose browser refused the background download
}

// Problem reports whether the counts show something to act on.
func (c Counts) Problem() bool {
	return c.Blocked > 0 || c.DlHeld > 0 || c.DlFailed > 0 || c.LaunchFailed > 0
}

// Downloads is the number of download episodes that moved data.
func (c Counts) Downloads() int {
	return c.DlComplete + c.DlHeldDone + c.DlHeld + c.DlFailed + c.DlInProgress
}

// Pill is a short status marker. The text carries the meaning; the class
// only colors it (amber for problems, gray otherwise).
type Pill struct {
	Text  string
	Class string
	Title string
}

// SchoolRow is one organization.
type SchoolRow struct {
	OrgID string
	Name  string
	TZ    string
	Counts
	Churn         bool
	Platforms     string
	LastActivity  string
	lastAt        time.Time
	Pills         []Pill
	Href          string // this page, narrowed to the school
	DashboardHref string
}

// GroupRow is one group of the selected school.
type GroupRow struct {
	GroupID string
	Name    string
	Counts
	Pills         []Pill
	DashboardHref string
}

// UnitCell is one unit in a student's row.
type UnitCell struct {
	Unit    string // unit1
	Label   string // U1
	State   string
	Text    string
	Class   string
	Title   string
	Devices int
}

// StudentRow is one student (or staff member) over the window.
type StudentRow struct {
	UserID          string
	Name            string
	Groups          string
	Staff           bool
	Opened          bool
	Played          bool
	Blocked         bool
	Days            int
	Units           []UnitCell
	Launch          models.MHSActivityLaunch
	CrashReal       int
	CrashNotice     int
	Devices         int
	NewDevices      int
	Platforms       string
	CurrentUnit     string
	LastSeen        string
	lastAt          time.Time
	Pills           []Pill
	DashboardHref   string
	DeviceTestsHref string
	Partial         bool
}

// DayRow is one organization on one day.
type DayRow struct {
	Day     string
	OrgName string
	Counts
}

// SchoolView is the selected school: its groups and students.
type SchoolView struct {
	OrgID string
	Name  string
	TZ    string
	Counts
	Groups   []GroupRow
	Students []StudentRow
	Staff    []StudentRow
}

// Report is the page's content.
type Report struct {
	Summary             Counts
	SchoolsActive       int
	SchoolsWithProblems int
	StaffLaunches       int
	Schools             []SchoolRow
	School              *SchoolView
	Days                []DayRow
}

// Options shape the report.
type Options struct {
	SelectedOrg        primitive.ObjectID
	SelectedGroup      primitive.ObjectID
	ShowStaff          bool
	MultiDay           bool
	Now                time.Time
	CanOpenDeviceTests bool // link each student to their load records in the Device Tests viewer
}

// DeviceTestsHref links to a member's load records in the Device Tests viewer.
func DeviceTestsHref(userID primitive.ObjectID) string {
	return "/views/device-tests?kind=" + models.MHSDeviceTestKindMember + "&when=" + viewers.RangeMonth + "&user=" + userID.Hex()
}

// BuildReport aggregates day documents into the page's rows. It is pure: it
// reads nothing and depends only on its inputs, so it can be tested without
// a database.
func BuildReport(docs []models.MHSActivityDay, lk Lookups, opts Options) Report {
	var students, staff []models.MHSActivityDay
	for _, d := range docs {
		if d.Role == models.MHSActivityRoleStaff {
			staff = append(staff, d)
		} else {
			students = append(students, d)
		}
	}
	rep := Report{Summary: aggregate(students)}
	for _, s := range staff {
		rep.StaffLaunches += s.Launch.OK
	}

	byOrg := map[primitive.ObjectID][]models.MHSActivityDay{}
	for _, d := range students {
		byOrg[d.OrganizationID] = append(byOrg[d.OrganizationID], d)
	}
	for org, ds := range byOrg {
		row := SchoolRow{OrgID: org.Hex(), Name: lk.OrgName(org), TZ: lk.OrgLoc(org).String(), Counts: aggregate(ds)}
		row.Churn = row.StudentDays >= churnMinDays && float64(row.DevicesNew) >= churnShare*float64(row.StudentDays)
		row.Platforms = platformsOf(ds)
		row.lastAt = lastOf(ds)
		if !row.lastAt.IsZero() {
			row.LastActivity = formatAt(row.lastAt, lk.OrgLoc(org))
		}
		row.Pills = schoolPills(row.Counts, row.Churn)
		row.Href = "/mhsactivity?org=" + org.Hex()
		if g := firstGroupOf(lk, org); !g.IsZero() {
			row.DashboardHref = "/mhsdashboard?group=" + g.Hex()
		}
		rep.Schools = append(rep.Schools, row)
	}
	sort.Slice(rep.Schools, func(i, j int) bool {
		if rep.Schools[i].Problem() != rep.Schools[j].Problem() {
			return rep.Schools[i].Problem()
		}
		return rep.Schools[i].Name < rep.Schools[j].Name
	})
	rep.SchoolsActive = len(rep.Schools)
	for _, s := range rep.Schools {
		if s.Problem() {
			rep.SchoolsWithProblems++
		}
	}

	if opts.MultiDay {
		type key struct {
			day string
			org primitive.ObjectID
		}
		byDay := map[key][]models.MHSActivityDay{}
		for _, d := range students {
			k := key{d.Day, d.OrganizationID}
			byDay[k] = append(byDay[k], d)
		}
		for k, ds := range byDay {
			rep.Days = append(rep.Days, DayRow{Day: k.day, OrgName: lk.OrgName(k.org), Counts: aggregate(ds)})
		}
		sort.Slice(rep.Days, func(i, j int) bool {
			if rep.Days[i].Day != rep.Days[j].Day {
				return rep.Days[i].Day > rep.Days[j].Day
			}
			return rep.Days[i].OrgName < rep.Days[j].OrgName
		})
	}

	if !opts.SelectedOrg.IsZero() {
		rep.School = buildSchool(opts.SelectedOrg, byOrg[opts.SelectedOrg], staff, lk, opts)
	}
	return rep
}

func buildSchool(org primitive.ObjectID, ds []models.MHSActivityDay, staff []models.MHSActivityDay, lk Lookups, opts Options) *SchoolView {
	loc := lk.OrgLoc(org)
	v := &SchoolView{OrgID: org.Hex(), Name: lk.OrgName(org), TZ: loc.String()}
	if !opts.SelectedGroup.IsZero() {
		kept := ds[:0:0]
		for _, d := range ds {
			if hasGroup(d, opts.SelectedGroup) {
				kept = append(kept, d)
			}
		}
		ds = kept
	}
	v.Counts = aggregate(ds)

	// Groups: every group the school's active students belong to; students
	// in no group under "(no group)".
	byGroup := map[primitive.ObjectID][]models.MHSActivityDay{}
	var noGroup []models.MHSActivityDay
	for _, d := range ds {
		if len(d.GroupIDs) == 0 {
			noGroup = append(noGroup, d)
			continue
		}
		for _, g := range d.GroupIDs {
			if !opts.SelectedGroup.IsZero() && g != opts.SelectedGroup {
				continue
			}
			byGroup[g] = append(byGroup[g], d)
		}
	}
	for g, gds := range byGroup {
		row := GroupRow{GroupID: g.Hex(), Name: lk.GroupName(g), Counts: aggregate(gds), DashboardHref: "/mhsdashboard?group=" + g.Hex()}
		row.Pills = schoolPills(row.Counts, false)
		v.Groups = append(v.Groups, row)
	}
	if len(noGroup) > 0 {
		row := GroupRow{Name: "(no group)", Counts: aggregate(noGroup)}
		row.Pills = schoolPills(row.Counts, false)
		v.Groups = append(v.Groups, row)
	}
	sort.Slice(v.Groups, func(i, j int) bool { return v.Groups[i].Name < v.Groups[j].Name })

	v.Students = studentRows(ds, lk, loc, false)
	if opts.CanOpenDeviceTests {
		for i := range v.Students {
			id, _ := primitive.ObjectIDFromHex(v.Students[i].UserID)
			v.Students[i].DeviceTestsHref = DeviceTestsHref(id)
		}
	}
	if opts.ShowStaff {
		var sds []models.MHSActivityDay
		for _, d := range staff {
			if d.OrganizationID == org {
				sds = append(sds, d)
			}
		}
		v.Staff = studentRows(sds, lk, loc, true)
	}
	return v
}

func hasGroup(d models.MHSActivityDay, g primitive.ObjectID) bool {
	for _, id := range d.GroupIDs {
		if id == g {
			return true
		}
	}
	return false
}

// aggregate sums a set of day documents into counts. Students are distinct
// users; downloads, launches and crashes add up; devices are distinct ids.
func aggregate(ds []models.MHSActivityDay) Counts {
	var c Counts
	users := map[primitive.ObjectID]*struct{ opened, played, blocked, bgfetch bool }{}
	devices := map[string]bool{}
	for _, d := range ds {
		u := users[d.UserID]
		if u == nil {
			u = &struct{ opened, played, blocked, bgfetch bool }{}
			users[d.UserID] = u
		}
		u.opened = u.opened || d.Opened
		u.played = u.played || d.Played
		u.blocked = u.blocked || d.Blocked
		u.bgfetch = u.bgfetch || d.Flags.BgfetchRefused
		c.StudentDays++
		for _, un := range d.Units {
			switch un.State {
			case models.MHSActivityUnitComplete:
				c.DlComplete++
			case models.MHSActivityUnitHeldDone:
				c.DlHeldDone++
			case models.MHSActivityUnitHeld:
				c.DlHeld++
			case models.MHSActivityUnitFailed:
				c.DlFailed++
			case models.MHSActivityUnitCached:
				c.DlCached++
			default:
				c.DlInProgress++
			}
		}
		c.LaunchOK += d.Launch.OK
		c.LaunchStalled += d.Launch.Stalled
		c.LaunchRecovered += d.Launch.StalledRecovered
		c.LaunchFailed += d.Launch.Failed
		c.CrashReal += d.Crashes.Real
		c.CrashNotice += d.Crashes.Notice
		for _, dev := range d.Devices {
			devices[dev.DeviceID] = true
			if dev.New {
				c.DevicesNew++
			}
		}
	}
	c.Students = len(users)
	for _, u := range users {
		if u.opened {
			c.Opened++
		}
		if u.played {
			c.Played++
		}
		// Blocked over the window: a day with a problem and no play at all
		// in the window (a student who got in the next day is not blocked).
		if u.blocked && !u.played {
			c.Blocked++
		}
		if u.bgfetch {
			c.BgfetchRefused++
		}
	}
	c.DevicesSeen = len(devices)
	return c
}

// studentRows folds each user's days into one row.
func studentRows(ds []models.MHSActivityDay, lk Lookups, loc *time.Location, staff bool) []StudentRow {
	byUser := map[primitive.ObjectID][]models.MHSActivityDay{}
	for _, d := range ds {
		byUser[d.UserID] = append(byUser[d.UserID], d)
	}
	rows := make([]StudentRow, 0, len(byUser))
	for id, uds := range byUser {
		sort.Slice(uds, func(i, j int) bool { return uds[i].Day < uds[j].Day })
		row := StudentRow{UserID: id.Hex(), Name: lk.UserName(id), Staff: staff, Days: len(uds)}
		groupNames := map[string]bool{}
		devices := map[string]bool{}
		var firstGroup primitive.ObjectID
		for _, d := range uds {
			row.Opened = row.Opened || d.Opened
			row.Played = row.Played || d.Played
			row.Blocked = row.Blocked || d.Blocked
			row.Partial = row.Partial || d.Partial
			row.Launch.OK += d.Launch.OK
			row.Launch.Stalled += d.Launch.Stalled
			row.Launch.StalledRecovered += d.Launch.StalledRecovered
			row.Launch.Failed += d.Launch.Failed
			row.Launch.LoaderCache += d.Launch.LoaderCache
			row.Launch.LoaderNetwork += d.Launch.LoaderNetwork
			if d.Launch.UnitySecondsLast > 0 {
				row.Launch.UnitySecondsLast = d.Launch.UnitySecondsLast
			}
			if d.Launch.UnitySecondsMax > row.Launch.UnitySecondsMax {
				row.Launch.UnitySecondsMax = d.Launch.UnitySecondsMax
			}
			row.CrashReal += d.Crashes.Real
			row.CrashNotice += d.Crashes.Notice
			for _, dev := range d.Devices {
				devices[dev.DeviceID] = true
				if dev.New {
					row.NewDevices++
				}
			}
			for _, g := range d.GroupIDs {
				groupNames[lk.GroupName(g)] = true
				if firstGroup.IsZero() {
					firstGroup = g
				}
			}
			if d.CurrentUnit != "" {
				row.CurrentUnit = unitLabel(d.CurrentUnit)
			}
			if d.LastAt != nil && d.LastAt.After(row.lastAt) {
				row.lastAt = *d.LastAt
			}
		}
		row.Blocked = row.Blocked && !row.Played // see aggregate: never got in during the window
		row.Devices = len(devices)
		row.Platforms = platformsOf(uds)
		names := make([]string, 0, len(groupNames))
		for n := range groupNames {
			names = append(names, n)
		}
		sort.Strings(names)
		row.Groups = strings.Join(names, ", ")
		if !firstGroup.IsZero() {
			row.DashboardHref = "/mhsdashboard?group=" + firstGroup.Hex()
		}
		if !row.lastAt.IsZero() {
			row.LastSeen = formatAt(row.lastAt, loc)
		}
		row.Units = unitCells(uds)
		row.Pills = studentPills(row)
		rows = append(rows, row)
	}
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].Blocked != rows[j].Blocked {
			return rows[i].Blocked
		}
		return rows[i].Name < rows[j].Name
	})
	return rows
}

// unitCells merges a student's episodes per unit; the latest episode gives
// the state, the title lists them all.
func unitCells(uds []models.MHSActivityDay) []UnitCell {
	type ep struct {
		u   models.MHSActivityUnit
		day string
	}
	byUnit := map[string][]ep{}
	for _, d := range uds {
		for _, u := range d.Units {
			byUnit[u.Unit] = append(byUnit[u.Unit], ep{u, d.Day})
		}
	}
	units := make([]string, 0, len(byUnit))
	for u := range byUnit {
		units = append(units, u)
	}
	sort.Strings(units)
	cells := make([]UnitCell, 0, len(units))
	for _, unit := range units {
		eps := byUnit[unit]
		sort.Slice(eps, func(i, j int) bool { return eps[i].u.LastAt.Before(eps[j].u.LastAt) })
		latest := eps[len(eps)-1].u
		devices := map[string]bool{}
		var titles []string
		for _, e := range eps {
			devices[e.u.DeviceID] = true
			titles = append(titles, e.day+" · "+unitText(e.u)+" · device …"+tail(e.u.DeviceID))
		}
		cell := UnitCell{Unit: unit, Label: unitLabel(unit), State: latest.State, Text: unitText(latest), Class: unitClass(latest.State), Devices: len(devices)}
		if len(eps) > 1 {
			cell.Title = strings.Join(titles, "\n")
		}
		cells = append(cells, cell)
	}
	return cells
}

func unitText(u models.MHSActivityUnit) string {
	switch u.State {
	case models.MHSActivityUnitComplete:
		s := "done"
		if u.CompleteMB > 0 {
			s += fmt.Sprintf(" %d MB", u.CompleteMB)
		}
		if u.CompleteSeconds > 0 {
			s += " in " + mmss(u.CompleteSeconds)
		}
		return s
	case models.MHSActivityUnitHeldDone:
		s := fmt.Sprintf("held at %d/%d MB", u.HeldAtMB, u.TotalMB)
		if u.HoldSeconds > 0 {
			s += ", done after " + humanDur(u.HoldSeconds)
		} else {
			s += ", then done"
		}
		return s
	case models.MHSActivityUnitHeld:
		return fmt.Sprintf("held at %d/%d MB", u.HeldAtMB, u.TotalMB)
	case models.MHSActivityUnitFailed:
		if u.LastErrorClass != "" {
			return "failed (" + u.LastErrorClass + ")"
		}
		return "failed"
	case models.MHSActivityUnitCached:
		return "on device"
	}
	if u.TotalMB > 0 {
		return fmt.Sprintf("%d%% (%d/%d MB)", u.MaxMB*100/u.TotalMB, u.MaxMB, u.TotalMB)
	}
	return "started"
}

func unitClass(state string) string {
	switch state {
	case models.MHSActivityUnitHeld, models.MHSActivityUnitFailed:
		return viewers.PillAmber
	case models.MHSActivityUnitComplete:
		return viewers.PillGreen
	case models.MHSActivityUnitInProgress:
		return viewers.TextMuted
	}
	return viewers.PillGray
}

func unitLabel(unit string) string {
	if strings.HasPrefix(unit, "unit") && len(unit) > 4 {
		return "U" + unit[4:]
	}
	if unit == "end" {
		return "End"
	}
	return unit
}

func schoolPills(c Counts, churn bool) []Pill {
	var p []Pill
	if c.Blocked > 0 {
		p = append(p, Pill{Text: plural(c.Blocked, "student", "students") + " blocked", Class: viewers.PillAmber, Title: "Opened the page, never played, and had a download or launch problem"})
	}
	if c.DlHeld > 0 {
		p = append(p, Pill{Text: plural(c.DlHeld, "download held", "downloads held"), Class: viewers.PillAmber, Title: "Stopped a few MB before the end and never finished (a scanning web filter holding the file)"})
	}
	if c.DlFailed > 0 {
		p = append(p, Pill{Text: plural(c.DlFailed, "download failed", "downloads failed"), Class: viewers.PillAmber})
	}
	if c.LaunchFailed > 0 {
		p = append(p, Pill{Text: plural(c.LaunchFailed, "launch failed", "launches failed"), Class: viewers.PillAmber})
	}
	if c.CrashReal > 0 {
		p = append(p, Pill{Text: plural(c.CrashReal, "crash", "crashes"), Class: viewers.PillAmber})
	}
	if churn {
		p = append(p, Pill{Text: "new browser profile every day", Class: viewers.PillAmber, Title: "Most student-days used a device whose browser data was new that day: downloads do not survive between sessions"})
	}
	if c.DlHeldDone > 0 {
		p = append(p, Pill{Text: plural(c.DlHeldDone, "hold released", "holds released"), Class: viewers.PillGray, Title: "Held near the end, then finished"})
	}
	if c.BgfetchRefused > 0 {
		p = append(p, Pill{Text: fmt.Sprintf("background fetch refused (%d)", c.BgfetchRefused), Class: viewers.PillGray, Title: "Students whose browser refused the background download; the direct download took over (Chrome 149+ does this everywhere)"})
	}
	if c.CrashNotice > 0 {
		p = append(p, Pill{Text: plural(c.CrashNotice, "browser notice", "browser notices"), Class: viewers.PillGray, Title: "Pointer-lock, fullscreen or audio refusals the game survives; not crashes"})
	}
	return p
}

func studentPills(r StudentRow) []Pill {
	var p []Pill
	if r.Blocked {
		p = append(p, Pill{Text: "blocked", Class: viewers.PillAmber})
	}
	if r.Launch.Failed > 0 {
		p = append(p, Pill{Text: plural(r.Launch.Failed, "launch failed", "launches failed"), Class: viewers.PillAmber})
	}
	if r.CrashReal > 0 {
		p = append(p, Pill{Text: plural(r.CrashReal, "crash", "crashes"), Class: viewers.PillAmber})
	}
	if r.Launch.Stalled > 0 {
		t := plural(r.Launch.Stalled, "slow start", "slow starts")
		if r.Launch.StalledRecovered == r.Launch.Stalled {
			t += " (recovered)"
		}
		p = append(p, Pill{Text: t, Class: viewers.PillGray, Title: "No loader progress for 30 s; the game went on to start"})
	}
	if r.NewDevices > 0 {
		p = append(p, Pill{Text: plural(r.NewDevices, "new device", "new devices"), Class: viewers.PillGray, Title: "A browser profile first seen that day"})
	}
	if r.CrashNotice > 0 {
		p = append(p, Pill{Text: plural(r.CrashNotice, "notice", "notices"), Class: viewers.TextMuted, Title: "Browser notices the game survives"})
	}
	if r.Partial {
		p = append(p, Pill{Text: "partial", Class: viewers.TextMuted, Title: "A data source was unavailable when this was computed"})
	}
	return p
}

func platformsOf(ds []models.MHSActivityDay) string {
	counts := map[string]int{}
	seen := map[string]bool{}
	for _, d := range ds {
		for _, dev := range d.Devices {
			if seen[dev.DeviceID] || dev.Platform == "" {
				continue
			}
			seen[dev.DeviceID] = true
			counts[dev.Platform]++
		}
	}
	type kv struct {
		k string
		v int
	}
	var list []kv
	for k, v := range counts {
		list = append(list, kv{k, v})
	}
	sort.Slice(list, func(i, j int) bool {
		if list[i].v != list[j].v {
			return list[i].v > list[j].v
		}
		return list[i].k < list[j].k
	})
	parts := make([]string, 0, len(list))
	for _, e := range list {
		parts = append(parts, fmt.Sprintf("%s %d", e.k, e.v))
	}
	return strings.Join(parts, ", ")
}

func lastOf(ds []models.MHSActivityDay) time.Time {
	var last time.Time
	for _, d := range ds {
		if d.LastAt != nil && d.LastAt.After(last) {
			last = *d.LastAt
		}
	}
	return last
}

func firstGroupOf(lk Lookups, org primitive.ObjectID) primitive.ObjectID {
	var best primitive.ObjectID
	var bestName string
	for id, g := range lk.Groups {
		if g.OrgID != org {
			continue
		}
		if best.IsZero() || g.Name < bestName {
			best, bestName = id, g.Name
		}
	}
	return best
}

func formatAt(t time.Time, loc *time.Location) string {
	return t.In(loc).Format("Jan 2, 3:04 PM MST")
}

func mmss(sec int) string {
	return fmt.Sprintf("%d:%02d", sec/60, sec%60)
}

func humanDur(sec int) string {
	if sec < 90 {
		return fmt.Sprintf("%d s", sec)
	}
	return fmt.Sprintf("%.1f min", float64(sec)/60)
}

func plural(n int, one, many string) string {
	if n == 1 {
		return fmt.Sprintf("%d %s", n, one)
	}
	return fmt.Sprintf("%d %s", n, many)
}
