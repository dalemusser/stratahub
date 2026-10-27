package models

import "testing"

// Run shapes from the production data of 2026-10-26 (heartbeats every 30 s).
func TestMHSRunFroze(t *testing.T) {
	hb := func(elapsedS int64, wasm int, fps float64) MHSDeviceTestHeartbeat {
		return MHSDeviceTestHeartbeat{ElapsedMs: elapsedS * 1000, WasmHeapMB: wasm, FPS: fps}
	}
	tests := []struct {
		name string
		hbs  []MHSDeviceTestHeartbeat
		want bool
	}{
		{"froze at the opening and ended", []MHSDeviceTestHeartbeat{hb(10, 96, 43), hb(40, 115, 58), hb(70, 495, 47), hb(100, 495, 4.3), hb(108, 495, 1.6)}, true},
		{"froze, last beat hidden at 0.1 fps", []MHSDeviceTestHeartbeat{hb(10, 96, 37), hb(100, 495, 50), hb(134, 495, 36), hb(144, 495, 0.1)}, true},
		{"slow but steady for 11 minutes", []MHSDeviceTestHeartbeat{hb(7, 96, 37), hb(67, 495, 12), hb(577, 495, 5.3), hb(676, 495, 25)}, false},
		{"very slow for 29 minutes", []MHSDeviceTestHeartbeat{hb(10, 96, 9), hb(70, 495, 4), hb(1750, 495, 6)}, false},
		{"short run that never froze", []MHSDeviceTestHeartbeat{hb(10, 96, 45), hb(40, 495, 57)}, false},
		{"low frame rate before the scene loaded", []MHSDeviceTestHeartbeat{hb(10, 96, 2), hb(40, 115, 50)}, false},
		{"no heartbeats", nil, false},
	}
	for _, tc := range tests {
		if got := MHSRunFroze(tc.hbs); got != tc.want {
			t.Errorf("%s: MHSRunFroze = %v, want %v", tc.name, got, tc.want)
		}
	}
}
