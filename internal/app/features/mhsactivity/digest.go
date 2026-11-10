package mhsactivity

import (
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"

	uierrors "github.com/dalemusser/stratahub/internal/app/features/errors"
	"github.com/dalemusser/stratahub/internal/app/features/viewers"
	activity "github.com/dalemusser/stratahub/internal/app/system/mhsactivity"
	"github.com/dalemusser/stratahub/internal/app/system/viewdata"
	"github.com/dalemusser/stratahub/internal/app/system/workspace"
	"github.com/dalemusser/stratahub/internal/domain/models"
	"github.com/dalemusser/waffle/pantry/templates"
	"github.com/go-chi/chi/v5"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

// The daily digest (docs/mission-hydrosci/mhs-issues-remaining-plan-110926.md,
// I-2): nobody opens MHS Activity during the school day, so problems were
// found by the teachers' emails first. The digest lists the recent days,
// marks the ones with something to look at, and shows each day's problems
// school by school, linking into MHS Activity. It reads the same days as the
// page (stored, or live for open days) within the viewer's scope.

// digestDays is how many days the list covers (bounded by the page's
// longest window and by the start of the stored history).
const digestDays = 30

// DigestDay is one day in the list.
type DigestDay struct {
	Day            string
	Label          string // "Mon, Nov 9"
	Weekend        bool
	Active         bool // any student activity
	Problem        bool // a school had something to look at
	Schools        int
	ProblemSchools int
	Students       int
	Played         int
	Highlights     []Pill // the day's problem pills, summed over its schools
	Href           string
}

type digestVM struct {
	viewdata.BaseVM
	Days      []DigestDay
	From, To  string
	Freshness []string
}

type digestDayVM struct {
	viewdata.BaseVM
	Day         DigestDay
	Problems    []SchoolRow // schools with something to look at
	Quiet       []SchoolRow // schools with activity and nothing to look at
	WindowQuery string      // the day as MHS Activity's window
	Freshness   []string
	Missing     []string
}

// withWindow returns a copy of the request whose query asks for the given
// days (the page's custom window); other parameters are dropped.
func withWindow(r *http.Request, from, to string) *http.Request {
	r2 := r.Clone(r.Context())
	u := *r.URL
	q := url.Values{}
	q.Set("range", RangeCustom)
	q.Set("from", from)
	q.Set("to", to)
	u.RawQuery = q.Encode()
	r2.URL = &u
	return r2
}

// digestToday is the newest day the list shows: the latest local today
// among the workspace's school time zones (a UTC day would show tomorrow
// in the American evening); UTC when the zones cannot be read.
func (h *Handler) digestToday(r *http.Request) string {
	now := h.now()
	today := ""
	if zones, err := activity.ZonesFor(r.Context(), h.DB, workspace.IDFromRequest(r), nil); err == nil {
		for _, z := range zones {
			if d := activity.LocalDay(now, z.Loc); d > today {
				today = d
			}
		}
	}
	if today == "" {
		today = now.UTC().Format(activity.DayFormat)
	}
	return today
}

// digestSpan is the list's window ending on today: the last digestDays
// days, not before the stored history starts and not longer than the
// page's longest window.
func (h *Handler) digestSpan(today string) (string, string) {
	n := digestDays
	if h.Config.MaxDays > 0 && h.Config.MaxDays < n {
		n = h.Config.MaxDays
	}
	from, _ := activity.AddDays(today, -(n - 1))
	if h.Config.BackfillStart != "" && h.Config.BackfillStart > from && h.Config.BackfillStart <= today {
		from = h.Config.BackfillStart
	}
	return from, today
}

// ServeDigest lists the recent days.
func (h *Handler) ServeDigest(w http.ResponseWriter, r *http.Request) {
	from, to := h.digestSpan(h.digestToday(r))
	vm, ok := h.load(w, withWindow(r, from, to), primitive.NilObjectID)
	if !ok {
		return
	}
	days, _ := activity.DaysBetween(from, to)
	out := digestVM{BaseVM: viewdata.NewBaseVM(r, h.DB, "MHS Daily Digest", "/mhsactivity"), From: from, To: to, Freshness: vm.Freshness}
	byDay := map[string][]models.MHSActivityDay{}
	for _, d := range vm.docs {
		if d.Role != models.MHSActivityRoleStaff {
			byDay[d.Day] = append(byDay[d.Day], d)
		}
	}
	for i := len(days) - 1; i >= 0; i-- {
		out.Days = append(out.Days, digestDayOf(days[i], byDay[days[i]], vm.lk))
	}
	templates.Render(w, r, "mhsactivity_digest", out)
}

// ServeDigestDay shows one day's digest.
func (h *Handler) ServeDigestDay(w http.ResponseWriter, r *http.Request) {
	day := chi.URLParam(r, "day")
	if _, err := time.Parse(activity.DayFormat, day); err != nil {
		uierrors.RenderNotFound(w, r, "No such day.", "/mhsactivity/digest")
		return
	}
	vm, ok := h.load(w, withWindow(r, day, day), primitive.NilObjectID)
	if !ok {
		return
	}
	var students []models.MHSActivityDay
	for _, d := range vm.docs {
		if d.Role != models.MHSActivityRoleStaff {
			students = append(students, d)
		}
	}
	rep := BuildReport(students, vm.lk, Options{Now: h.now()})
	out := digestDayVM{
		BaseVM:      viewdata.NewBaseVM(r, h.DB, "MHS Daily Digest: "+prettyDay(day), "/mhsactivity/digest"),
		Day:         digestDayOf(day, students, vm.lk),
		WindowQuery: vm.WindowQuery,
		Freshness:   vm.Freshness,
		Missing:     vm.Missing,
	}
	for _, s := range rep.Schools {
		if s.Problem() {
			out.Problems = append(out.Problems, s)
		} else {
			out.Quiet = append(out.Quiet, s)
		}
	}
	templates.Render(w, r, "mhsactivity_digest_day", out)
}

// digestDayOf sums one day's student documents (all of the viewer's schools).
func digestDayOf(day string, ds []models.MHSActivityDay, lk Lookups) DigestDay {
	d := DigestDay{Day: day, Label: day, Href: "/mhsactivity/digest/" + day}
	if t, err := time.Parse(activity.DayFormat, day); err == nil {
		d.Label = t.Format("Mon, Jan 2")
		d.Weekend = t.Weekday() == time.Saturday || t.Weekday() == time.Sunday
	}
	if len(ds) == 0 {
		return d
	}
	rep := BuildReport(ds, lk, Options{})
	d.Active = rep.Summary.Opened > 0 || rep.Summary.Played > 0
	d.Schools = rep.SchoolsActive
	d.ProblemSchools = rep.SchoolsWithProblems
	d.Problem = rep.SchoolsWithProblems > 0
	d.Students = rep.Summary.Students
	d.Played = rep.Summary.Played
	// Highlights: the amber pills of the day as a whole (the same wording as
	// a school row, counted over every school of the day).
	for _, p := range schoolPills(rep.Summary, false) {
		if p.Class == viewers.PillAmber {
			d.Highlights = append(d.Highlights, p)
		}
	}
	sort.SliceStable(d.Highlights, func(i, j int) bool {
		return strings.Contains(d.Highlights[i].Text, "blocked") && !strings.Contains(d.Highlights[j].Text, "blocked")
	})
	return d
}
