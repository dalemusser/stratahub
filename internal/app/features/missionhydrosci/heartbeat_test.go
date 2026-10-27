package missionhydrosci

import (
	"encoding/json"
	"testing"
)

func TestHeartbeatKeepsTheDeviceState(t *testing.T) {
	var req deviceTestHeartbeatRequest
	body := `{"phase":"game","elapsed_ms":100230,"wasm_heap_mb":495,"fps":4.3,"visibility":"visible",
		"other_windows":2,"cpu_pressure":"serious-and-then-some-more","stalls":3,"max_stall_ms":12400,"gl_context_lost":1}`
	if err := json.Unmarshal([]byte(body), &req); err != nil {
		t.Fatal(err)
	}
	b := req.beat()
	if b.OtherWindows != 2 || b.Stalls != 3 || b.MaxStallMs != 12400 || b.GLContextLost != 1 || b.WasmHeapMB != 495 || b.FPS != 4.3 {
		t.Errorf("beat %+v", b)
	}
	if len([]rune(b.CPUPressure)) > 16 {
		t.Errorf("cpu pressure not clipped: %q", b.CPUPressure)
	}
	// An older page sends none of them.
	var old deviceTestHeartbeatRequest
	if err := json.Unmarshal([]byte(`{"phase":"game","fps":30}`), &old); err != nil {
		t.Fatal(err)
	}
	if ob := old.beat(); ob.OtherWindows != 0 || ob.CPUPressure != "" || ob.Stalls != 0 || ob.GLContextLost != 0 {
		t.Errorf("old page beat %+v", ob)
	}
}
