package mhssteplog

import (
	"testing"
	"time"

	"github.com/dalemusser/stratahub/internal/domain/models"
)

// The play page's device-state lines (Issue 1 of
// docs/mission-hydrosci/mhs-issues-102826.md) are read as events and kept in
// the summary with their own times.
func TestDeviceStateLines(t *testing.T) {
	at := time.Date(2026, 10, 27, 15, 0, 0, 0, time.UTC)
	steps := []models.MHSDeviceTestStep{
		{At: at, Step: "page", State: "warn", Msg: "The browser had discarded this page earlier (usually when the device is short of memory) and reloaded it", Detail: map[string]string{"unit": "unit3"}},
		{At: at.Add(time.Minute), Step: "game", State: "warn", Msg: "Another game window is open on this device (1 besides this one)", Detail: map[string]string{"unit": "unit3", "other_windows": "1"}},
		{At: at.Add(2 * time.Minute), Step: "game", State: "warn", Msg: "The page was frozen for 22 s", Detail: map[string]string{"unit": "unit3", "stall_s": "22"}},
		{At: at.Add(3 * time.Minute), Step: "page", State: "info", Msg: "Opened the play page for unit3 v2.8.9"},
	}
	ev, ok := Parse(steps[1])
	if !ok || ev.Kind != KindOtherWindow || ev.Count != 1 {
		t.Errorf("other window: %+v %v", ev, ok)
	}
	ev, ok = Parse(steps[2])
	if !ok || ev.Kind != KindPageFrozen || ev.Seconds != 22 {
		t.Errorf("page frozen: %+v %v", ev, ok)
	}
	if ev, ok := Parse(steps[3]); !ok || ev.Kind != KindPage {
		t.Errorf("an ordinary page line still parses as a page: %+v", ev)
	}
	s := Summarize(steps, "launch-ok", Context{UnitID: "unit3"})
	if s.V != 2 || len(s.Device) != 3 {
		t.Fatalf("summary %+v", s)
	}
	want := []models.MHSStepDeviceEvent{
		{At: at, Kind: models.MHSDeviceEventDiscarded, Unit: "unit3"},
		{At: at.Add(time.Minute), Kind: models.MHSDeviceEventOtherWindows, Unit: "unit3", N: 1},
		{At: at.Add(2 * time.Minute), Kind: models.MHSDeviceEventPageFrozen, Unit: "unit3", N: 22},
	}
	for i := range want {
		if s.Device[i] != want[i] {
			t.Errorf("device[%d] = %+v, want %+v", i, s.Device[i], want[i])
		}
	}
}
