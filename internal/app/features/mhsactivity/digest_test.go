package mhsactivity

import (
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/dalemusser/stratahub/internal/domain/models"
	"github.com/dalemusser/waffle/pantry/templates"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

// A day with a school that has a problem is marked and carries the day's
// amber pills; a day with only quiet schools is not; a day with no student
// documents is "no activity" (and a Saturday is a weekend).
func TestDigestDayOf(t *testing.T) {
	orgA, orgB := primitive.NewObjectID(), primitive.NewObjectID()
	lk := Lookups{Orgs: map[primitive.ObjectID]OrgInfo{orgA: {Name: "School J", Loc: time.UTC}, orgB: {Name: "School P", Loc: time.UTC}}}
	blocked := models.MHSActivityDay{UserID: primitive.NewObjectID(), OrganizationID: orgA, Role: "member", Day: "2026-11-05", Opened: true, Launched: true,
		Launch: models.MHSActivityLaunch{OK: 1}, Log: models.MHSActivityLog{Available: true}, Blocked: true, Flags: models.MHSActivityFlags{ServicesBlocked: true}}
	played := models.MHSActivityDay{UserID: primitive.NewObjectID(), OrganizationID: orgB, Role: "member", Day: "2026-11-05", Opened: true, Played: true, Launched: true,
		Launch: models.MHSActivityLaunch{OK: 1}, Log: models.MHSActivityLog{Available: true, Events: 400}}

	d := digestDayOf("2026-11-05", []models.MHSActivityDay{blocked, played}, lk)
	if !d.Active || !d.Problem || d.Schools != 2 || d.ProblemSchools != 1 || d.Students != 2 || d.Played != 1 || d.Label != "Thu, Nov 5" || d.Href != "/mhsactivity/digest/2026-11-05" {
		t.Errorf("problem day %+v", d)
	}
	var texts []string
	for _, p := range d.Highlights {
		texts = append(texts, p.Text)
	}
	if got := strings.Join(texts, "|"); !strings.Contains(got, "1 student blocked") || !strings.Contains(got, "filter blocking the game for 1 student") {
		t.Errorf("highlights %q", got)
	}

	q := digestDayOf("2026-11-06", []models.MHSActivityDay{played}, lk)
	if !q.Active || q.Problem || len(q.Highlights) != 0 {
		t.Errorf("quiet day %+v", q)
	}
	e := digestDayOf("2026-11-07", nil, lk)
	if e.Active || e.Problem || !e.Weekend {
		t.Errorf("empty Saturday %+v", e)
	}
}

// The list's span: the last 30 days, never before the stored history starts
// or longer than the page's longest window; the window is passed to the
// page's loader as a custom range.
func TestDigestSpanAndWindow(t *testing.T) {
	h := &Handler{Config: Config{MaxDays: 31}}
	h.now = func() time.Time { return time.Date(2026, 11, 9, 20, 0, 0, 0, time.UTC) }
	if from, to := h.digestSpan("2026-11-09"); from != "2026-10-11" || to != "2026-11-09" {
		t.Errorf("span %s..%s", from, to)
	}
	h.Config.BackfillStart = "2026-10-13"
	if from, _ := h.digestSpan("2026-11-09"); from != "2026-10-13" {
		t.Errorf("span starts %s, want the backfill start", from)
	}
	h.Config.MaxDays = 7
	if from, _ := h.digestSpan("2026-11-09"); from != "2026-11-03" {
		t.Errorf("span starts %s with MaxDays 7", from)
	}
	r := withWindow(httptest.NewRequest("GET", "/mhsactivity/digest?org=x&group=y", nil), "2026-11-03", "2026-11-09")
	if q := r.URL.Query(); q.Get("range") != RangeCustom || q.Get("from") != "2026-11-03" || q.Get("to") != "2026-11-09" || q.Get("org") != "" {
		t.Errorf("window query %v", q)
	}
}

// Both digest pages render: the list with a marked day, a quiet day and an
// empty weekend; the day page with its problem school and a quiet school.
func TestDigestPagesRender(t *testing.T) {
	bootTemplates(t)
	orgA, orgB := primitive.NewObjectID(), primitive.NewObjectID()
	lk := Lookups{Orgs: map[primitive.ObjectID]OrgInfo{orgA: {Name: "School J", Loc: time.UTC}, orgB: {Name: "School P", Loc: time.UTC}}}
	blocked := models.MHSActivityDay{UserID: primitive.NewObjectID(), OrganizationID: orgA, Role: "member", Day: "2026-11-05", Opened: true, Launched: true,
		Launch: models.MHSActivityLaunch{OK: 1}, Log: models.MHSActivityLog{Available: true}, Blocked: true, Flags: models.MHSActivityFlags{ServicesBlocked: true}}
	played := models.MHSActivityDay{UserID: primitive.NewObjectID(), OrganizationID: orgB, Role: "member", Day: "2026-11-05", Opened: true, Played: true, Launched: true,
		Launch: models.MHSActivityLaunch{OK: 1}, Log: models.MHSActivityLog{Available: true, Events: 400}}
	list := digestVM{Days: []DigestDay{
		digestDayOf("2026-11-07", nil, lk),
		digestDayOf("2026-11-06", []models.MHSActivityDay{played}, lk),
		digestDayOf("2026-11-05", []models.MHSActivityDay{blocked, played}, lk),
	}}
	rec := httptest.NewRecorder()
	templates.Render(rec, httptest.NewRequest("GET", "/mhsactivity/digest", nil), "mhsactivity_digest", list)
	body := rec.Body.String()
	for _, want := range []string{"MHS Daily Digest", "Look at this", "No problems", "No activity (weekend)", `href="/mhsactivity/digest/2026-11-05"`, `data-href="/mhsactivity/digest/2026-11-05"`, "tr.mhs-digest-row", "1 student blocked"} {
		if !strings.Contains(body, want) {
			t.Errorf("list lacks %q", want)
		}
	}

	rep := BuildReport([]models.MHSActivityDay{blocked, played}, lk, Options{})
	day := digestDayVM{Day: digestDayOf("2026-11-05", []models.MHSActivityDay{blocked, played}, lk), WindowQuery: "range=custom&from=2026-11-05&to=2026-11-05"}
	day.BackURL = "/mhsactivity/digest"
	for _, s := range rep.Schools {
		if s.Problem() {
			day.Problems = append(day.Problems, s)
		} else {
			day.Quiet = append(day.Quiet, s)
		}
	}
	rec = httptest.NewRecorder()
	templates.Render(rec, httptest.NewRequest("GET", "/mhsactivity/digest/2026-11-05", nil), "mhsactivity_digest_day", day)
	body = rec.Body.String()
	for _, want := range []string{"Thu, Nov 5", "1 of 2 schools had something to look at", "To look at", "School J", "No problems", "School P", "This day in MHS Activity", "range=custom&amp;from=2026-11-05", `href="/mhsactivity/digest"`, "← Back"} {
		if !strings.Contains(body, want) {
			t.Errorf("day page lacks %q", want)
		}
	}
}
