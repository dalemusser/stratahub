package mhsdashboard

import (
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	_ "github.com/dalemusser/stratahub/internal/app/features/missionhydrosci" // registers a template func the shared layout uses
	appresources "github.com/dalemusser/stratahub/internal/app/resources"
	"github.com/dalemusser/stratahub/internal/app/system/auth"
	"github.com/dalemusser/stratahub/internal/app/system/memberstatuscfg"
	"github.com/dalemusser/stratahub/internal/app/system/viewdata"
	"github.com/dalemusser/stratahub/internal/app/system/workspace"
	"github.com/dalemusser/stratahub/internal/domain/models"
	"github.com/dalemusser/stratahub/internal/testutil"
	"github.com/dalemusser/waffle/pantry/templates"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.uber.org/zap"
)

var bootTemplatesOnce sync.Once

// bootTemplates boots the template engine once with the shared layout and
// this feature's templates so template errors fail here, not in production.
func bootTemplates(t *testing.T) {
	t.Helper()
	var bootErr error
	bootTemplatesOnce.Do(func() {
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

// sampleDashboard builds a small dashboard view model with two students and
// the embedded survey configuration, using real cell formatting.
func sampleDashboard(t *testing.T, withSurveys bool) DashboardData {
	t.Helper()
	h := &Handler{}
	if withSurveys {
		cfg, err := memberstatuscfg.Load()
		if err != nil {
			t.Fatal(err)
		}
		h.SurveyConfig = cfg
	}

	cfg, err := LoadProgressConfig()
	if err != nil {
		t.Fatalf("progress config: %v", err)
	}
	unitHeaders, pointHeaders := h.buildHeaders(cfg)

	makeRow := func(name string, even bool) MemberRow {
		row := MemberRow{
			ID:           primitive.NewObjectID().Hex(),
			Name:         name,
			IsEven:       even,
			Cells:        make([]CellData, cfg.TotalProgressPoints()),
			UnitProgress: map[string]string{},
		}
		if withSurveys {
			for _, item := range h.SurveyConfig.Items {
				var doc *models.MemberStatus
				if item.ID == "pre" {
					doc = &models.MemberStatus{State: models.MemberStatusCompleted, StartedAt: ts(2026, 8, 25, 18, 41, 0), CompletedAt: ts(2026, 8, 25, 19, 3, 0), Source: models.MemberStatusSourceAPI}
				}
				row.Surveys = append(row.Surveys, buildSurveyCell(item, doc, nil, name))
			}
		}
		return row
	}

	return DashboardData{
		Groups:         []GroupOption{{ID: primitive.NewObjectID().Hex(), Name: "Period 3", Selected: true}},
		SelectedGroup:  "x",
		GroupName:      "Period 3",
		MemberCount:    2,
		LastUpdated:    "Aug 26, 2026 9:00 AM",
		TimezoneAbbr:   "UTC",
		UnitHeaders:    unitHeaders,
		PointHeaders:   pointHeaders,
		Members:        []MemberRow{makeRow("Alice Cole", true), makeRow("Bob Ng", false)},
		SurveyTabTitle: h.surveyTabTitle(),
		SurveyHeaders:  h.surveyHeaders(),
		SortBy:         "name",
		SortDir:        "asc",
	}
}

// TestDashboardRendersSurveysTab executes the real dashboard page template
// (layout + view + grid) with survey data and checks the Surveys tab, legend,
// panel, and cells are present — and absent when no surveys are configured.
func TestDashboardRendersSurveysTab(t *testing.T) {
	bootTemplates(t)
	db := testutil.SetupTestDB(t)

	newReq := func() *httptest.ResponseRecorder {
		req := httptest.NewRequest("GET", "/mhsdashboard", nil)
		req.Host = "mhs.example.com"
		req = workspace.WithTestWorkspace(req, primitive.NewObjectID(), "mhs", "MHS")
		req = auth.WithTestUser(req, &auth.SessionUser{ID: primitive.NewObjectID().Hex(), Name: "Lee Leader", LoginID: "lee@example.com", Role: "leader"})
		rec := httptest.NewRecorder()
		data := sampleDashboard(t, true)
		data.BaseVM = viewdata.NewBaseVM(req, db, "MHS Dashboard", "/dashboard")
		templates.Render(rec, req, "mhsdashboard_view", data)
		return rec
	}

	rec := newReq()
	if rec.Code != 200 {
		t.Fatalf("status %d; body: %s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	for _, want := range []string{
		`id="mhs-tab-btn-surveys"`, "switchTab('surveys')", // tab
		`id="mhs-legend-surveys"`, "Click a cell for dates and details", // legend
		`id="mhs-tab-surveys"`, `title="Survey taken before students begin Mission HydroSci."`, // panel + column tooltip
		"Alice Cole", "Bob Ng",
		`class="mhs-survey-cell mhs-survey-completed"`, `data-completed-at="Aug 25, 2026 7:03 PM UTC"`, "&middot; Aug 25", // completed cell
		`class="mhs-survey-cell mhs-survey-none"`, "Not started", // not-started cells
		`id="mhs-survey-modal"`, "openSurveyModal(surveyCell.dataset)", // modal + click wiring
		"surveys: document.getElementById('mhs-tab-surveys')", // switchTab knows the tab
	} {
		if !strings.Contains(body, want) {
			t.Errorf("rendered dashboard missing %q", want)
		}
	}
	// The tab button carries the configured title (rendered with surrounding whitespace).
	if i := strings.Index(body, `id="mhs-tab-btn-surveys"`); i < 0 {
		t.Error("tab button missing")
	} else if end := strings.Index(body[i:], "</button>"); end < 0 || !strings.Contains(body[i:i+end], "Surveys") {
		t.Errorf("tab button does not carry the configured title: %q", body[i:i+min(end, 200)])
	}
	// Four header columns, one per configured survey.
	for _, title := range []string{"Pre-Survey", "MHS Engagement", "EWS Engagement", "Post-Survey"} {
		if !strings.Contains(body, ">"+title+"</th>") {
			t.Errorf("missing survey column header %q", title)
		}
	}

	// Without a survey configuration: no tab, legend, or panel.
	req := httptest.NewRequest("GET", "/mhsdashboard", nil)
	req = workspace.WithTestWorkspace(req, primitive.NewObjectID(), "mhs", "MHS")
	req = auth.WithTestUser(req, &auth.SessionUser{ID: primitive.NewObjectID().Hex(), Name: "Lee", LoginID: "lee@example.com", Role: "leader"})
	rec = httptest.NewRecorder()
	data := sampleDashboard(t, false)
	data.BaseVM = viewdata.NewBaseVM(req, db, "MHS Dashboard", "/dashboard")
	templates.Render(rec, req, "mhsdashboard_view", data)
	if rec.Code != 200 {
		t.Fatalf("status %d (no surveys); body: %s", rec.Code, rec.Body.String())
	}
	body = rec.Body.String()
	for _, absent := range []string{`id="mhs-tab-btn-surveys"`, `id="mhs-legend-surveys"`, `id="mhs-tab-surveys"`} {
		if strings.Contains(body, absent) {
			t.Errorf("dashboard without surveys still contains %q", absent)
		}
	}
}

// TestGridRendersSurveysPanel executes the HTMX grid partial alone, as the
// 30-second refresh does, and checks the Surveys panel survives a refresh.
func TestGridRendersSurveysPanel(t *testing.T) {
	bootTemplates(t)
	full := sampleDashboard(t, true)
	data := GridData{
		SelectedGroup:  full.SelectedGroup,
		GroupName:      full.GroupName,
		MemberCount:    full.MemberCount,
		LastUpdated:    full.LastUpdated,
		UnitHeaders:    full.UnitHeaders,
		PointHeaders:   full.PointHeaders,
		Members:        full.Members,
		SurveyTabTitle: full.SurveyTabTitle,
		SurveyHeaders:  full.SurveyHeaders,
		SortBy:         "name",
		SortDir:        "asc",
	}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/mhsdashboard/grid", nil)
	templates.Render(rec, req, "mhsdashboard_grid", data)
	if rec.Code != 200 {
		t.Fatalf("status %d; body: %s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	if !strings.Contains(body, `id="mhs-tab-surveys"`) || !strings.Contains(body, "mhs-survey-completed") {
		t.Errorf("grid partial missing the surveys panel")
	}
}
