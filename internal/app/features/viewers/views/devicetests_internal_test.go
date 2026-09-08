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
		Sound: "ok", Controls: "problems", Performance: "choppy", Progress: "into-game",
		Notes:      "Globe <froze> once",
		AnsweredAt: time.Date(2026, 9, 8, 8, 26, 53, 0, time.UTC),
	}
	got := surveyHTML(q)
	for _, want := range []string{
		"Did the sound play?", "Did the picture look right?", "How far did you get?",
		"Anything else you noticed?", "Globe &lt;froze&gt; once", "Answered 2026-09-08T08:26:53Z UTC",
		qLabel("sound", "ok"), qLabel("controls", "problems"), qLabel("progress", "into-game"),
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
