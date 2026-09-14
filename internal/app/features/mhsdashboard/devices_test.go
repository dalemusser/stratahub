package mhsdashboard

import (
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/dalemusser/waffle/pantry/templates"
)

// TestOrderDevices: newest first, and only the oldest device carries IsLast.
func TestOrderDevices(t *testing.T) {
	old := time.Date(2026, 3, 24, 0, 0, 0, 0, time.UTC)
	newer := time.Date(2026, 4, 2, 0, 0, 0, 0, time.UTC)
	m := map[string][]DeviceInfo{
		"asher": {{DeviceType: "old", LastSeen: old}, {DeviceType: "new", LastSeen: newer}},
		"solo":  {{DeviceType: "only", LastSeen: newer}},
	}
	orderDevices(m)
	a := m["asher"]
	if a[0].DeviceType != "new" || a[1].DeviceType != "old" {
		t.Fatalf("expected newest first, got %s then %s", a[0].DeviceType, a[1].DeviceType)
	}
	if a[0].IsLast || !a[1].IsLast {
		t.Fatalf("IsLast should be only on the oldest row: got %v, %v", a[0].IsLast, a[1].IsLast)
	}
	if !m["solo"][0].IsLast {
		t.Fatalf("a single device must be its own block end")
	}
}

// TestGridDevicesBlocks renders the Devices panel for a two-device student
// and a one-device student and checks the block separators: the class that
// draws the heavier line must be on the last device row of each student and
// nowhere else, and the name cell must carry its own block-end class.
func TestGridDevicesBlocks(t *testing.T) {
	bootTemplates(t)
	full := sampleDashboard(t, false)
	old := time.Date(2026, 3, 24, 0, 0, 0, 0, time.UTC)
	newer := time.Date(2026, 4, 2, 0, 0, 0, 0, time.UTC)
	full.Members[0].Devices = []DeviceInfo{
		{DeviceType: "Chromebook", LastSeen: newer, UnitStatus: map[string]string{}},
		{DeviceType: "Chromebook", LastSeen: old, UnitStatus: map[string]string{}},
	}
	full.Members[1].Devices = []DeviceInfo{
		{DeviceType: "Chromebook", LastSeen: newer, UnitStatus: map[string]string{}},
	}
	dm := map[string][]DeviceInfo{"a": full.Members[0].Devices, "b": full.Members[1].Devices}
	orderDevices(dm)
	full.Members[0].Devices, full.Members[1].Devices = dm["a"], dm["b"]

	data := GridData{
		SelectedGroup: full.SelectedGroup, GroupName: full.GroupName, MemberCount: 2,
		LastUpdated: full.LastUpdated, UnitHeaders: full.UnitHeaders, PointHeaders: full.PointHeaders,
		Members: full.Members, SortBy: "name", SortDir: "asc",
	}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/mhsdashboard/grid", nil)
	templates.Render(rec, req, "mhsdashboard_grid", data)
	if rec.Code != 200 {
		t.Fatalf("status %d; body: %s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	panel := body[strings.Index(body, `id="mhs-tab-devices"`):]
	rows := regexp.MustCompile(`<tr class="mhs-device-row[^"]*"`).FindAllString(panel, -1)
	if len(rows) != 3 {
		t.Fatalf("expected 3 device rows, got %d: %v", len(rows), rows)
	}
	wantEnd := []bool{false, true, true}   // Alice new, Alice old (block end), Bob (block end)
	wantStart := []bool{true, false, true} // Alice new (block start), Alice old, Bob (block start)
	for i, r := range rows {
		if got := strings.Contains(r, "mhs-block-end"); got != wantEnd[i] {
			t.Errorf("row %d: block-end=%v, want %v (%s)", i, got, wantEnd[i], r)
		}
		if got := strings.Contains(r, "mhs-block-start"); got != wantStart[i] {
			t.Errorf("row %d: block-start=%v, want %v (%s)", i, got, wantStart[i], r)
		}
	}
	if strings.Count(panel, "mhs-block-end-cell") != 2 {
		t.Errorf("expected a block-end-cell on each student's name cell, got %d", strings.Count(panel, "mhs-block-end-cell"))
	}
}
