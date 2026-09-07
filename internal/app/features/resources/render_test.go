package resources

import (
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	uierrors "github.com/dalemusser/stratahub/internal/app/features/errors"
	_ "github.com/dalemusser/stratahub/internal/app/features/missionhydrosci" // registers a template func the shared layout uses
	appresources "github.com/dalemusser/stratahub/internal/app/resources"
	"github.com/dalemusser/stratahub/internal/app/system/auth"
	"github.com/dalemusser/stratahub/internal/app/system/workspace"
	"github.com/dalemusser/stratahub/internal/testutil"
	"github.com/dalemusser/waffle/pantry/templates"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.uber.org/zap"
)

var bootTemplatesOnce sync.Once

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

// TestResourceFormsRenderSurveyTracking executes the real new/edit/view
// templates through the engine and checks the Survey Tracking control
// renders with the configured options and the current selection.
func TestResourceFormsRenderSurveyTracking(t *testing.T) {
	bootTemplates(t)
	db := testutil.SetupTestDB(t)
	h := &AdminHandler{DB: db, ErrLog: uierrors.NewErrorLogger(zap.NewNop()), Log: zap.NewNop()}

	request := func(path string) *httptest.ResponseRecorder {
		req := httptest.NewRequest("GET", path, nil)
		req = workspace.WithTestWorkspace(req, primitive.NewObjectID(), "mhs", "MHS")
		req = auth.WithTestUser(req, &auth.SessionUser{ID: primitive.NewObjectID().Hex(), Name: "Admin", LoginID: "admin@example.com", Role: "admin"})
		rec := httptest.NewRecorder()
		switch {
		case strings.HasSuffix(path, "/new"):
			h.renderNewForm(rec, req, resourceFormVM{}, "")
		case strings.HasSuffix(path, "/edit"):
			h.renderEditForm(rec, req, resourceFormVM{ID: "x", ResourceTitle: "Pre link", LaunchURL: "https://surveys.example.com/pre", TrackedEntityID: "mhs"}, "")
		case strings.HasSuffix(path, "/edit-stale"):
			h.renderEditForm(rec, req, resourceFormVM{ID: "x", ResourceTitle: "Old link", LaunchURL: "https://surveys.example.com/old", TrackedEntityID: "retired"}, "")
		}
		return rec
	}

	// New form: "None" selected, all four surveys offered.
	rec := request("/resources/new")
	if rec.Code != 200 {
		t.Fatalf("new: status %d; %s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	for _, want := range []string{`name="tracked_entity_id"`, `<option value="" selected>None`, `<option value="pre" >Pre</option>`, `<option value="post" >Post</option>`} {
		if !strings.Contains(body, want) {
			t.Errorf("new form missing %q", want)
		}
	}

	// Edit form: current selection marked.
	rec = request("/resources/x/edit")
	if rec.Code != 200 {
		t.Fatalf("edit: status %d; %s", rec.Code, rec.Body.String())
	}
	body = rec.Body.String()
	if !strings.Contains(body, `<option value="mhs" selected>MHS Engagement`) {
		t.Error("edit form does not mark the current survey as selected")
	}
	if strings.Contains(body, "no longer configured") {
		t.Error("edit form shows the stale marker for a valid id")
	}

	// Edit form with a stale id: shown, selected, and flagged.
	rec = request("/resources/x/edit-stale")
	if rec.Code != 200 {
		t.Fatalf("edit-stale: status %d; %s", rec.Code, rec.Body.String())
	}
	body = rec.Body.String()
	if !strings.Contains(body, `<option value="retired" selected>retired (no longer configured)`) {
		t.Error("edit form does not surface a stale tracked id")
	}

	// View page: label rendered.
	req := httptest.NewRequest("GET", "/resources/x", nil)
	req = workspace.WithTestWorkspace(req, primitive.NewObjectID(), "mhs", "MHS")
	req = auth.WithTestUser(req, &auth.SessionUser{ID: primitive.NewObjectID().Hex(), Name: "Admin", LoginID: "admin@example.com", Role: "admin"})
	rec = httptest.NewRecorder()
	templates.Render(rec, req, "resource_view", viewData{
		ID: "x", ResourceTitle: "Pre link", LaunchURL: "https://surveys.example.com/pre", Type: "survey", Status: "active",
		URLIdentityModeLabel: urlIdentityModeLabel(""), TrackedEntityID: "pre", TrackedEntityLabel: trackedEntityLabel("pre"),
	})
	if rec.Code != 200 {
		t.Fatalf("view: status %d; %s", rec.Code, rec.Body.String())
	}
	body = rec.Body.String()
	if !strings.Contains(body, `value="Pre" readonly`) || !strings.Contains(body, "Opening this resource marks the survey") {
		t.Error("view page does not show the survey tracking label")
	}
}
