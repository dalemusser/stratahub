package missionhydrosci

import (
	"testing"

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
