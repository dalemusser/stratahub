package mhsdashboard

import (
	"strings"
	"testing"
	"time"

	uierrors "github.com/dalemusser/stratahub/internal/app/features/errors"
	"github.com/dalemusser/stratahub/internal/app/store/memberstatus"
	"github.com/dalemusser/stratahub/internal/app/system/memberstatuscfg"
	"github.com/dalemusser/stratahub/internal/domain/models"
	"github.com/dalemusser/stratahub/internal/testutil"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.uber.org/zap"
)

var testItem = memberstatuscfg.Item{
	ID: "pre", Title: "Pre-Survey", ShortName: "Pre", APINames: []string{"Pre"}, Description: "Before the game.",
}

func ts(y int, mo time.Month, d, h, m, s int) *time.Time {
	t := time.Date(y, mo, d, h, m, s, 0, time.UTC)
	return &t
}

func TestBuildSurveyCell_NotStarted(t *testing.T) {
	cell := buildSurveyCell(testItem, nil, nil, "Alice")
	if cell.State != models.MemberStatusNotStarted || cell.Label != "Not started" || cell.Glyph != "○" || cell.CellClass != "mhs-survey-none" {
		t.Errorf("not started cell: %+v", cell)
	}
	if cell.ShortDate != "" || cell.StateAt != "" || cell.OpenedAt != "" || cell.StartedAt != "" || cell.CompletedAt != "" {
		t.Errorf("not started cell must have no timestamps: %+v", cell)
	}
	if cell.Tooltip != "Pre-Survey: not started" || cell.StudentName != "Alice" || cell.EntityID != "pre" || cell.Title != "Pre-Survey" {
		t.Errorf("cell identity: %+v", cell)
	}
}

func TestBuildSurveyCell_StatesAndTimezone(t *testing.T) {
	chicago, err := time.LoadLocation("America/Chicago")
	if err != nil {
		t.Skip("tzdata unavailable")
	}
	// Aug 25 2026 18:41 UTC = 1:41 PM CDT
	doc := &models.MemberStatus{
		State:       models.MemberStatusCompleted,
		OpenedAt:    ts(2026, 8, 25, 18, 39, 0),
		StartedAt:   ts(2026, 8, 25, 18, 41, 0),
		CompletedAt: ts(2026, 8, 25, 19, 3, 0),
		Source:      models.MemberStatusSourceAPI,
	}
	cell := buildSurveyCell(testItem, doc, chicago, "Alice")

	if cell.State != models.MemberStatusCompleted || cell.Label != "Completed" || cell.Glyph != "✓" || cell.CellClass != "mhs-survey-completed" {
		t.Errorf("completed cell: %+v", cell)
	}
	if cell.ShortDate != "Aug 25" {
		t.Errorf("ShortDate: %q", cell.ShortDate)
	}
	if cell.StateAt != "Aug 25, 2026 2:03 PM CDT" {
		t.Errorf("StateAt: %q", cell.StateAt)
	}
	if cell.OpenedAt != "Aug 25, 2026 1:39 PM CDT" || cell.StartedAt != "Aug 25, 2026 1:41 PM CDT" || cell.CompletedAt != "Aug 25, 2026 2:03 PM CDT" {
		t.Errorf("timestamps: opened=%q started=%q completed=%q", cell.OpenedAt, cell.StartedAt, cell.CompletedAt)
	}
	wantTip := "Pre-Survey: Opened Aug 25, 2026 1:39 PM CDT · Started Aug 25, 2026 1:41 PM CDT · Completed Aug 25, 2026 2:03 PM CDT"
	if cell.Tooltip != wantTip {
		t.Errorf("Tooltip:\n got %q\nwant %q", cell.Tooltip, wantTip)
	}
	if cell.Source != "api" {
		t.Errorf("Source: %q", cell.Source)
	}

	// Started only (no opened, no completed), UTC when loc is nil.
	doc = &models.MemberStatus{State: models.MemberStatusStarted, StartedAt: ts(2026, 9, 1, 8, 0, 0)}
	cell = buildSurveyCell(testItem, doc, nil, "Bob")
	if cell.Label != "Started" || cell.Glyph != "◐" || cell.CellClass != "mhs-survey-started" || cell.ShortDate != "Sep 1" {
		t.Errorf("started cell: %+v", cell)
	}
	if cell.OpenedAt != "" || cell.CompletedAt != "" || cell.Tooltip != "Pre-Survey: Started Sep 1, 2026 8:00 AM UTC" {
		t.Errorf("started cell details: %+v", cell)
	}

	// Opened only (launched in StrataHub, provider hasn't reported).
	doc = &models.MemberStatus{State: models.MemberStatusOpened, OpenedAt: ts(2026, 9, 2, 8, 0, 0), Source: models.MemberStatusSourceLaunch}
	cell = buildSurveyCell(testItem, doc, nil, "Cy")
	if cell.Label != "Opened" || cell.Glyph != "◔" || cell.CellClass != "mhs-survey-opened" || cell.Source != "launch" {
		t.Errorf("opened cell: %+v", cell)
	}

	// A document with a state the dashboard doesn't know renders as not started.
	doc = &models.MemberStatus{State: "weird"}
	if cell := buildSurveyCell(testItem, doc, nil, "Dee"); cell.State != models.MemberStatusNotStarted {
		t.Errorf("unknown state should degrade to not started: %+v", cell)
	}
}

func TestFindSurveyDoc_FallsBackToUnknownNameKeys(t *testing.T) {
	item := memberstatuscfg.Item{ID: "mhs", Title: "MHS Engagement", APINames: []string{"MHS Engagement", "MHS Survey"}}

	if findSurveyDoc(nil, item) != nil {
		t.Error("nil docs should yield nil")
	}
	byID := map[string]models.MemberStatus{"mhs": {State: models.MemberStatusStarted}}
	if d := findSurveyDoc(byID, item); d == nil || d.State != models.MemberStatusStarted {
		t.Error("lookup by id failed")
	}
	// Recorded before the alias was configured: stored under the unknown-name key.
	legacy := map[string]models.MemberStatus{memberstatuscfg.UnknownKey("mhs survey"): {State: models.MemberStatusCompleted}}
	if d := findSurveyDoc(legacy, item); d == nil || d.State != models.MemberStatusCompleted {
		t.Error("fallback to unknown-name key failed")
	}
	// The canonical key wins when both exist.
	both := map[string]models.MemberStatus{
		"mhs": {State: models.MemberStatusStarted},
		memberstatuscfg.UnknownKey("MHS Engagement"): {State: models.MemberStatusCompleted},
	}
	if d := findSurveyDoc(both, item); d == nil || d.State != models.MemberStatusStarted {
		t.Error("canonical key should take precedence")
	}
	if findSurveyDoc(map[string]models.MemberStatus{"post": {}}, item) != nil {
		t.Error("unrelated key matched")
	}
}

func TestSurveyHeadersAndTitle(t *testing.T) {
	h := &Handler{}
	if h.surveyHeaders() != nil || h.surveyTabTitle() != "Surveys" {
		t.Errorf("nil config: headers=%v title=%q", h.surveyHeaders(), h.surveyTabTitle())
	}

	cfg, err := memberstatuscfg.Parse([]byte(`{"tab_title":"Questionnaires","items":[
		{"id":"a","title":"Alpha","short_name":"A","description":"first"},
		{"id":"b","title":"Beta","short_name":"B"}]}`))
	if err != nil {
		t.Fatal(err)
	}
	h.SurveyConfig = cfg
	if h.surveyTabTitle() != "Questionnaires" {
		t.Errorf("title: %q", h.surveyTabTitle())
	}
	headers := h.surveyHeaders()
	if len(headers) != 2 || headers[0].ID != "a" || headers[0].Title != "Alpha" || headers[0].ShortName != "A" || headers[0].Description != "first" || headers[1].ID != "b" {
		t.Errorf("headers: %+v", headers)
	}

	// The embedded configuration yields the four survey columns in order.
	embedded, err := memberstatuscfg.Load()
	if err != nil {
		t.Fatalf("embedded survey config did not load: %v", err)
	}
	real := &Handler{SurveyConfig: embedded}
	var ids []string
	for _, hd := range real.surveyHeaders() {
		ids = append(ids, hd.ID)
	}
	if strings.Join(ids, ",") != "pre,mhs,ews,post" {
		t.Errorf("embedded headers: %v", ids)
	}
}

func TestLoadSurveyCells(t *testing.T) {
	db := testutil.SetupTestDB(t)
	ctx, cancel := testutil.TestContext()
	defer cancel()

	h := NewHandler(db, nil, nil, uierrors.NewErrorLogger(zap.NewNop()), zap.NewNop())
	wsID := primitive.NewObjectID()
	alice := models.User{ID: primitive.NewObjectID(), FullName: "Alice"}
	bob := models.User{ID: primitive.NewObjectID(), FullName: "Bob"}
	store := memberstatus.New(db)

	// Alice: completed "pre" (recorded via the API under the canonical key),
	// started "post" recorded under the unknown-name key of its title.
	if _, err := store.Record(ctx, memberstatus.RecordInput{WorkspaceID: wsID, UserID: alice.ID, EntityKey: "pre", Entity: "Pre-Survey", State: models.MemberStatusCompleted}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Record(ctx, memberstatus.RecordInput{WorkspaceID: wsID, UserID: alice.ID, EntityKey: memberstatuscfg.UnknownKey("Post"), Entity: "Post", State: models.MemberStatusStarted}); err != nil {
		t.Fatal(err)
	}
	// Same user, other workspace: must not leak in.
	if _, err := store.Record(ctx, memberstatus.RecordInput{WorkspaceID: primitive.NewObjectID(), UserID: bob.ID, EntityKey: "mhs", Entity: "MHS", State: models.MemberStatusCompleted}); err != nil {
		t.Fatal(err)
	}

	cells := h.loadSurveyCells(ctx, wsID, []models.User{alice, bob}, time.UTC)
	if len(cells) != 2 {
		t.Fatalf("members with cells: %d", len(cells))
	}

	a := cells[alice.ID.Hex()]
	if len(a) != 4 {
		t.Fatalf("alice cells: %d", len(a))
	}
	states := make([]string, 0, 4)
	for _, c := range a {
		states = append(states, c.EntityID+"="+c.State)
	}
	if got := strings.Join(states, ","); got != "pre=completed,mhs=not_started,ews=not_started,post=started" {
		t.Errorf("alice states: %s", got)
	}
	if a[0].StudentName != "Alice" || a[0].ShortDate == "" || a[0].CompletedAt == "" {
		t.Errorf("alice pre cell: %+v", a[0])
	}

	b := cells[bob.ID.Hex()]
	if len(b) != 4 {
		t.Fatalf("bob cells: %d", len(b))
	}
	for _, c := range b {
		if c.State != models.MemberStatusNotStarted {
			t.Errorf("bob %s: %q (other-workspace data leaked)", c.EntityID, c.State)
		}
	}

	// No members / no config → nil, never an error.
	if h.loadSurveyCells(ctx, wsID, nil, time.UTC) != nil {
		t.Error("no members should yield nil")
	}
	h.SurveyConfig = nil
	if h.loadSurveyCells(ctx, wsID, []models.User{alice}, time.UTC) != nil {
		t.Error("no config should yield nil")
	}
}
