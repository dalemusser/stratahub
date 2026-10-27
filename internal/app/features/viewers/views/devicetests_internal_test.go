package views

import (
	"context"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/dalemusser/stratahub/internal/app/features/viewers"
	"github.com/dalemusser/stratahub/internal/app/system/viewscope"
	"github.com/dalemusser/stratahub/internal/domain/models"
	"go.mongodb.org/mongo-driver/bson/primitive"
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

// Every code the form accepts must have a label, or the viewer would show
// the raw code.
func TestQuestionnaireLabelsCoverOptions(t *testing.T) {
	for question, codes := range models.MHSDeviceTestQuestionnaireOptions {
		for _, code := range codes {
			if got := qLabel(question, code); got == "" || got == code {
				t.Errorf("%s/%s: no label (got %q)", question, code, got)
			}
		}
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

// The Member filter (MHS Activity links here) narrows the list to one
// member's load records; a malformed id matches nothing.
func TestDeviceTestsMemberFilter(t *testing.T) {
	v := &DeviceTests{}
	scope := &viewscope.Scope{WorkspaceID: primitive.NewObjectID(), Role: "admin", All: true}
	id := primitive.NewObjectID()
	q, err := v.listQuery(context.Background(), scope, viewers.ParseFilters(v.Filters(), url.Values{"user": {id.Hex()}, "kind": {models.MHSDeviceTestKindMember}}))
	if err != nil || q == nil || q.UserID == nil || *q.UserID != id || q.Kind != models.MHSDeviceTestKindMember {
		t.Fatalf("query %+v err %v", q, err)
	}
	q, err = v.listQuery(context.Background(), scope, viewers.ParseFilters(v.Filters(), url.Values{"user": {"not-an-id"}}))
	if err != nil || q != nil {
		t.Errorf("malformed member id should match nothing: %+v %v", q, err)
	}
}

func TestHeartbeatDeviceState(t *testing.T) {
	cases := []struct {
		hb   models.MHSDeviceTestHeartbeat
		want string
	}{
		{models.MHSDeviceTestHeartbeat{}, ""},
		{models.MHSDeviceTestHeartbeat{CPUPressure: "fair"}, ""},
		{models.MHSDeviceTestHeartbeat{OtherWindows: 1}, "1 other game window"},
		{models.MHSDeviceTestHeartbeat{OtherWindows: 2, CPUPressure: "serious", Stalls: 3, MaxStallMs: 12400, GLContextLost: 1},
			"2 other game windows · CPU serious · 3 stalls, longest 12 s · graphics lost 1×"},
		{models.MHSDeviceTestHeartbeat{Stalls: 1, MaxStallMs: 3600}, "1 stall, longest 4 s"},
	}
	for _, c := range cases {
		if got := heartbeatDeviceState(c.hb); got != c.want {
			t.Errorf("heartbeatDeviceState(%+v) = %q, want %q", c.hb, got, c.want)
		}
	}
}
