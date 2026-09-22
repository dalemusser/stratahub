package missionhydrosci

import (
	"net/http/httptest"
	"regexp"
	"strings"
	"sync"
	"testing"

	appresources "github.com/dalemusser/stratahub/internal/app/resources"
	"github.com/dalemusser/stratahub/internal/app/system/viewdata"
	"github.com/dalemusser/waffle/pantry/templates"
	"go.uber.org/zap"
)

var bootCeremonyTemplatesOnce sync.Once

func bootCeremonyTemplates(t *testing.T) {
	t.Helper()
	var bootErr error
	bootCeremonyTemplatesOnce.Do(func() {
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

// TestCeremonyPageRenders executes the host page template with a full view
// model, so a wrong field name or a JS-context escaping problem fails here
// rather than on a student's screen.
func TestCeremonyPageRenders(t *testing.T) {
	bootCeremonyTemplates(t)
	data := CeremonyData{
		BaseVM:    viewdata.BaseVM{CSRFToken: "tok\"en"}.AsBare(),
		Base:      ceremonyBase("0.1.8"),
		Version:   "0.1.8",
		ScoresURL: "/missionhydrosci/api/ea-scores?user_id=665f1a2b3c4d5e6f7a8b9c0d",
		ReturnURL: "/mhsdashboard",
		ExitLabel: ceremonyExitLabel,
		Dev:       true,
		Preview:   true,
	}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", CeremonyPath, nil)
	templates.Render(rec, req, "missionhydrosci_ceremony", data)
	body := rec.Body.String()
	if rec.Code != 200 {
		t.Fatalf("status %d: %s", rec.Code, body)
	}
	// html/template pads JS-context values with spaces ("dev:  true ,"): compare on
	// collapsed whitespace.
	norm := regexp.MustCompile(`\s+`).ReplaceAllString(body, " ")
	for _, want := range []string{
		`src="/missionhydrosci/content/end/v0.1.8/lib/embed.js"`,
		`MHSCeremony.mount(document.body`,
		`scoresUrl: "/missionhydrosci/api/ea-scores?user_id=665f1a2b3c4d5e6f7a8b9c0d"`,
		`var returnUrl = "/mhsdashboard"`,
		`var preview = true`,
		`dev: true`,
		`var version = "0.1.8"`,
		`/missionhydrosci/api/ceremony/viewed`,
		`'ceremony-ok'`,
	} {
		if !strings.Contains(norm, want) {
			t.Errorf("rendered page lacks %q\n%s", want, body)
		}
	}
	// html/template escapes the token for the JS string context; the raw quote must not survive.
	if strings.Contains(body, `tok"en`) {
		t.Errorf("CSRF token not JS-escaped")
	}
	if !strings.Contains(body, `tok\"en`) && !strings.Contains(body, `tok"en`) {
		t.Errorf("escaped CSRF token missing:\n%s", body)
	}
}

// TestUnitsPageOffersCeremonyWhenComplete renders the units page in the
// complete state with and without a ceremony in the collection.
func TestUnitsPageOffersCeremonyWhenComplete(t *testing.T) {
	bootCeremonyTemplates(t)
	render := func(url string) string {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest("GET", "/missionhydrosci/units", nil)
		templates.Render(rec, req, "missionhydrosci_units", UnitsData{
			BaseVM: viewdata.BaseVM{Title: "Mission HydroSci"}, IsComplete: true, CurrentUnit: "complete", CeremonyURL: url,
		})
		if rec.Code != 200 {
			t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
		}
		return rec.Body.String()
	}
	if body := render(CeremonyPath); !strings.Contains(body, `href="/missionhydrosci/ceremony"`) || !strings.Contains(body, "Watch your ceremony") {
		t.Errorf("complete + ceremony: button missing")
	}
	if body := render(""); strings.Contains(body, "Watch your ceremony") {
		t.Errorf("complete without ceremony: button must not show")
	}
}
