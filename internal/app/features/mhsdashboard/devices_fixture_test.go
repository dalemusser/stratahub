package mhsdashboard

import (
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/dalemusser/waffle/pantry/templates"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

// TestWriteDevicesFixture renders the real dashboard page with fictional
// students covering every Devices-tab state and writes the HTML to the
// path in MHS_DEVICES_FIXTURE_OUT. It is skipped otherwise. The teacher
// guide's figures are screenshots of this page (see
// docs/mission-hydrosci-teacher-guide/devices-tab-notes.md).
func TestWriteDevicesFixture(t *testing.T) {
	out := os.Getenv("MHS_DEVICES_FIXTURE_OUT")
	if out == "" {
		t.Skip("set MHS_DEVICES_FIXTURE_OUT to write the fixture page")
	}
	bootTemplates(t)
	data := sampleDashboard(t, false)
	data.GroupName = "Period 3"
	data.MemberCount = 8

	now := time.Now()
	ago := func(days int) time.Time { return now.Add(-time.Duration(days) * 24 * time.Hour) }
	dev := func(kind string, seen time.Time, pct int, used, total string, units map[string]string) DeviceInfo {
		return DeviceInfo{
			DeviceType: kind, UnitStatus: units, LastSeen: seen, IsStale: now.Sub(seen) > staleDeviceThreshold,
			StoragePct: pct, StorageUsed: used, StorageTotal: total, StorageQuota: 1, StorageUsage: 1,
		}
	}
	details := map[string]string{"uad_platform": "Chrome OS", "uad_platform_version": "16093.57.0", "uad_brands": "Google Chrome 128", "device_memory_gb": "4", "cpu_cores": "4", "connection_type": "wifi", "screen_width": "1366", "screen_height": "768"}
	type student struct {
		name, summary string
		progress      map[string]string
		devices       []DeviceInfo
	}
	students := []student{
		{"Avery Kim", "Completed Unit 1, Unit 2 · Current Unit 3",
			map[string]string{"unit1": "completed", "unit2": "completed", "unit3": "current"},
			[]DeviceInfo{dev("Chromebook", ago(0), 12, "1.3 GB", "10.7 GB", map[string]string{"unit3": "cached", "unit4": "downloading"})}},
		{"Jordan Patel", "Completed Unit 1 · Current Unit 2",
			map[string]string{"unit1": "completed", "unit2": "current"},
			[]DeviceInfo{
				dev("Chromebook", ago(1), 6, "729 MB", "10.7 GB", map[string]string{"unit5": "cached"}),
				dev("Chromebook", ago(12), 2, "388 MB", "14.6 GB", map[string]string{"unit1": "cached", "unit2": "cached"}),
			}},
		{"Riley Nguyen", "Current Unit 1",
			map[string]string{"unit1": "current"},
			[]DeviceInfo{func() DeviceInfo {
				d := dev("iPad", ago(0), 4, "410 MB", "9.8 GB", map[string]string{"unit1": "downloading"})
				d.PWAInstalled = true
				return d
			}()}},
		{"Sam Okafor", "Completed Unit 1, Unit 2 · Current Unit 3",
			map[string]string{"unit1": "completed", "unit2": "completed", "unit3": "current"},
			[]DeviceInfo{dev("Chromebook", ago(1), 93, "2.1 GB", "2.3 GB", map[string]string{"unit3": "error"})}},
		{"Morgan Diaz", "Completed all units",
			map[string]string{"unit1": "completed", "unit2": "completed", "unit3": "completed", "unit4": "completed", "unit5": "completed"},
			[]DeviceInfo{dev("Chromebook", ago(2), 8, "860 MB", "10.7 GB", map[string]string{"unit5": "cached"})}},
		{"Casey Brooks", "Not started", map[string]string{}, nil},
		{"Taylor Reed", "Completed Unit 1 · Current Unit 3",
			map[string]string{"unit1": "completed", "unit3": "current"},
			[]DeviceInfo{dev("Chromebook", ago(0), 76, "3.4 GB", "4.5 GB", map[string]string{"unit3": "cached", "unit4": "cached"})}},
		{"Jamie Fox", "Completed Unit 1 · Current Unit 2",
			map[string]string{"unit1": "completed", "unit2": "current"},
			[]DeviceInfo{dev("Windows", ago(20), 5, "540 MB", "10.7 GB", map[string]string{"unit2": "partial"})}},
	}
	data.Members = nil
	for i, s := range students {
		for _, u := range []string{"unit1", "unit2", "unit3", "unit4", "unit5"} {
			if _, ok := s.progress[u]; !ok {
				s.progress[u] = "future"
			}
		}
		devs := s.devices
		if len(devs) > 0 {
			devs[0].DeviceDetails = details
			dm := map[string][]DeviceInfo{"x": devs}
			orderDevices(dm)
			devs = dm["x"]
		}
		data.Members = append(data.Members, MemberRow{
			ID: primitive.NewObjectID().Hex(), Name: s.name, IsEven: i%2 == 0,
			Cells: make([]CellData, len(data.Members[:0])), UnitProgress: s.progress, ProgressSummary: s.summary, Devices: devs,
		})
	}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/mhsdashboard", nil)
	templates.Render(rec, req, "mhsdashboard_view", data)
	if rec.Code != 200 {
		t.Fatalf("status %d; body: %s", rec.Code, rec.Body.String())
	}
	if err := os.WriteFile(out, rec.Body.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
}
