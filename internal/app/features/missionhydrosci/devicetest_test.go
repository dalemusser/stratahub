package missionhydrosci

import (
	"testing"
	"time"

	"github.com/dalemusser/stratahub/internal/domain/models"
)

func TestStageFromStep(t *testing.T) {
	cases := []struct{ step, state, want string }{
		{"download", "running", models.MHSDeviceTestStageDownloading},
		{"download", "warn", models.MHSDeviceTestStageDownloading},
		{"download", "ok", models.MHSDeviceTestStageDownloaded},
		{"download", "fail", ""},
		{"verify", "ok", models.MHSDeviceTestStageDownloaded},
		{"launch", "running", models.MHSDeviceTestStageLaunching},
		{"launch", "fail", models.MHSDeviceTestStageLaunching},
		{"game", "running", models.MHSDeviceTestStageGameplay},
		{"game", "ok", models.MHSDeviceTestStageGameplay},
		{"game", "fail", ""},
		{"sw", "ok", ""},
	}
	for _, c := range cases {
		if got := stageFromStep(c.step, c.state); got != c.want {
			t.Errorf("stageFromStep(%q,%q)=%q want %q", c.step, c.state, got, c.want)
		}
	}
}

func TestBoundDiagnostics(t *testing.T) {
	in := map[string]interface{}{}
	for i := 0; i < deviceTestMaxDiagKeys+10; i++ {
		in[string(rune('a'+i%26))+string(rune('a'+i/26))] = i
	}
	long := make([]byte, deviceTestMaxDiagValue+50)
	for i := range long {
		long[i] = 'x'
	}
	in["long"] = string(long)
	in["nested"] = map[string]interface{}{"k": "v"}
	in["flag"] = true
	out := boundDiagnostics(in)
	if len(out) > deviceTestMaxDiagKeys {
		t.Fatalf("keys not capped: %d", len(out))
	}
	if v, ok := out["long"].(string); ok && len([]rune(v)) > deviceTestMaxDiagValue {
		t.Fatalf("long value not clipped: %d", len(v))
	}
	if v, ok := out["nested"].(string); ok && v != `{"k":"v"}` {
		t.Fatalf("nested not re-encoded: %q", v)
	}
	if v, ok := out["flag"].(bool); ok && !v {
		t.Fatalf("bool lost")
	}
}

func TestDeviceTestShortID(t *testing.T) {
	if got := deviceTestShortID("ffffffff0123456789abcdef"); got != "ABCDEF" {
		t.Fatalf("got %q", got)
	}
}

func TestDeriveFromSteps(t *testing.T) {
	now := time.Now().UTC()
	req := deviceTestStepsRequest{}
	add := func(step, state, msg string) {
		req.Entries = append(req.Entries, struct {
			T      int64             `json:"t"`
			At     string            `json:"at"`
			Step   string            `json:"step"`
			State  string            `json:"state"`
			Msg    string            `json:"msg"`
			Detail map[string]string `json:"detail"`
		}{T: 1, At: now.Format(time.RFC3339Nano), Step: step, State: state, Msg: msg})
	}
	add("sw", "ok", "registered")
	add("download", "running", "10%")
	add("download", "ok", "complete")
	add("launch", "fail", "loader failed")
	steps, d := deriveFromSteps(req, models.MHSDeviceTestStageRun, now)
	if len(steps) != 4 {
		t.Fatalf("steps=%d", len(steps))
	}
	if d.reached != models.MHSDeviceTestStageLaunching || d.stage != models.MHSDeviceTestStageFailed || d.failedStep != "launch" {
		t.Fatalf("after failure: %+v", d)
	}
	// A later success moves the run on again and records gameplay.
	req2 := deviceTestStepsRequest{}
	req = req2
	add("launch", "ok", "unity started")
	add("game", "running", "running")
	_, d2 := deriveFromStepsWith(req, d.reached, true, d.failedStep, d.failedReason, false, now)
	if d2.stage != models.MHSDeviceTestStageGameplay || d2.reached != models.MHSDeviceTestStageGameplay || d2.gameplayAt == nil {
		t.Fatalf("after recovery: %+v", d2)
	}
	if d2.failedStep != "launch" {
		t.Fatalf("last problem should be kept: %+v", d2)
	}
}

func TestDropStoredSteps(t *testing.T) {
	at := time.Date(2026, 9, 8, 7, 44, 9, 882000000, time.UTC)
	mk := func(tm int64, step, msg string, when time.Time) models.MHSDeviceTestStep {
		return models.MHSDeviceTestStep{T: tm, At: when, Step: step, State: "running", Msg: msg}
	}
	stored := []models.MHSDeviceTestStep{
		mk(0, "page", "Opened the devicetest-play page", at),
		mk(1, "launch", "Loading the game loader", at.Add(time.Millisecond)),
	}
	batch := []models.MHSDeviceTestStep{
		mk(1, "launch", "Loading the game loader", at.Add(time.Millisecond)), // resent
		mk(1, "launch", "Loading the game loader", at.Add(2*time.Second)),    // same text, later: a real repeat
		mk(2400, "game", "Game is rendering", at.Add(2400*time.Millisecond)),
	}
	got := dropStoredSteps(batch, stored)
	if len(got) != 2 || got[0].At != at.Add(2*time.Second) || got[1].Step != "game" {
		t.Fatalf("dropStoredSteps = %+v, want the later repeat and the game entry", got)
	}
	if out := dropStoredSteps(batch, nil); len(out) != len(batch) {
		t.Fatalf("no stored steps: got %d entries, want %d", len(out), len(batch))
	}
	if out := dropStoredSteps(nil, stored); len(out) != 0 {
		t.Fatalf("empty batch: got %d entries", len(out))
	}
}
