package views

import (
	"strings"
	"testing"
	"time"

	"github.com/dalemusser/stratahub/internal/domain/models"
)

func TestSurveyHTML(t *testing.T) {
	if got := surveyHTML(nil); !strings.Contains(got, "Survey answers") || !strings.Contains(got, "Not answered") {
		t.Fatalf("nil questionnaire: %s", got)
	}
	q := &models.MHSDeviceTestQuestionnaire{
		Sound: "ok", Controls: "problems", Performance: "choppy", Progress: "met-anderson",
		Notes:      "Globe <froze> once",
		AnsweredAt: time.Date(2026, 9, 8, 8, 26, 53, 0, time.UTC),
	}
	got := surveyHTML(q)
	for _, want := range []string{
		"Did the sound play?", "Did the picture look right?", "Identify the farthest point in the game that you reached?",
		"Anything else you noticed?", "Globe &lt;froze&gt; once", "Answered 2026-09-08T08:26:53Z UTC",
		qLabel("sound", "ok"), qLabel("controls", "problems"), "Met Anderson and her hoverboard",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in %s", want, got)
		}
	}
	if strings.Contains(got, "<froze>") {
		t.Errorf("notes not escaped: %s", got)
	}
	if !strings.Contains(got, "<dd class=\"text-gray-800 dark:text-gray-100\">—</dd>") {
		t.Errorf("unanswered picture question should show a dash: %s", got)
	}
}

func TestReportsHTML(t *testing.T) {
	if got := reportsHTML(nil); !strings.Contains(got, "Tester reports (0)") || !strings.Contains(got, "None sent") {
		t.Fatalf("no reports: %s", got)
	}
	got := reportsHTML([]models.MHSDeviceTestReport{
		{At: time.Date(2026, 9, 8, 8, 30, 0, 0, time.UTC), Note: "Sound cut out <twice>"},
	})
	for _, want := range []string{"Tester reports (1)", "2026-09-08T08:30:00Z UTC", "Sound cut out &lt;twice&gt;"} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in %s", want, got)
		}
	}
}
