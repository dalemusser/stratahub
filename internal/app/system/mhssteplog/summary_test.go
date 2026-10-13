package mhssteplog

import (
	"testing"
	"time"

	"github.com/dalemusser/stratahub/internal/domain/models"
)

var t0 = time.Date(2026, 10, 8, 17, 30, 0, 0, time.UTC)

func at(sec int) time.Time { return t0.Add(time.Duration(sec) * time.Second) }

func s(sec int, stepID, state, msg string) models.MHSDeviceTestStep {
	return models.MHSDeviceTestStep{At: at(sec), Step: stepID, State: state, Msg: msg}
}

// The Oct 2026 hold: progress stops 12 MB short, the connection drops and
// resumes at that point again and again, nothing completes.
func heldSteps() []models.MHSDeviceTestStep {
	return []models.MHSDeviceTestStep{
		s(0, "page", "info", "Opened the units page"),
		s(1, "sw", "ok", "Service worker active (v1.0.16)"),
		s(2, "storage", "ok", "Storage: 620 MB used of 2048 MB (30%) · persisted: no"),
		s(3, "method", "warn", "The browser refused the background download (policy) — using the direct download; keep this tab open"),
		s(4, "download", "running", "Requesting Unit 1: Water Cycle v2.8.1 (213 MB)"),
		s(9, "download", "running", "Unit 1: Water Cycle: 10% · 21 of 213 MB · 4.2 MB/s · 0:45 left"),
		s(30, "download", "running", "Unit 1: Water Cycle: 94% · 201 of 213 MB · 9.3 MB/s · 0:01 left"),
		s(60, "download", "info", "Unit 1: Water Cycle: picked up at 201 MB after a dropped connection"),
		s(90, "download", "info", "Unit 1: Water Cycle: picked up at 201 MB after a dropped connection (2 drops so far)"),
		s(120, "download", "info", "Unit 1: Water Cycle: picked up at 201 MB after a dropped connection (3 drops so far)"),
		s(150, "download", "warn", "Unit 1: Water Cycle: no data received for 30 s — restarting the download"),
		s(180, "download", "fail", "Unit 1: Water Cycle: 6 dropped connections resumed, then: no data [stalled] — the connection keeps closing at the same point"),
		s(181, "download", "warn", "Download interrupted. Retrying in 5 s (attempt 1)"),
	}
}

func TestSummarizeHeld(t *testing.T) {
	sum := Summarize(heldSteps(), "download-failed", Context{UnitID: "unit1", DeviceID: "dev-1", SWVersion: "1.0.16", DownloadMode: "direct"})
	if sum.V != models.MHSMemberSummaryVersion || sum.Outcome != "download-failed" || sum.Steps != 13 {
		t.Fatalf("header: %+v", sum)
	}
	if !sum.FirstAt.Equal(at(0)) || !sum.LastAt.Equal(at(181)) {
		t.Errorf("span %v..%v", sum.FirstAt, sum.LastAt)
	}
	if sum.Storage == nil || sum.Storage.QuotaMB != 2048 || sum.Storage.Persisted == nil || *sum.Storage.Persisted {
		t.Errorf("storage %+v", sum.Storage)
	}
	if len(sum.Units) != 1 {
		t.Fatalf("units %+v", sum.Units)
	}
	u := sum.Units[0]
	if u.Unit != "unit1" || u.Title != "Unit 1: Water Cycle" || u.Version != "2.8.1" || !u.Requested {
		t.Errorf("identity %+v", u)
	}
	if u.TotalMB != 213 || u.MaxMB != 201 || u.Progress != 2 || u.Drops != 3 || u.Stalls != 1 || u.Retries != 1 || u.Fails != 1 {
		t.Errorf("counts %+v", u)
	}
	if u.LastErrorClass != "stalled" || u.LastErrorText != "6 dropped connections resumed, then: no data" {
		t.Errorf("error %q %q", u.LastErrorClass, u.LastErrorText)
	}
	if !u.Held || u.HeldAtMB != 201 || u.HeldAt == nil || !u.HeldAt.Equal(at(30)) || u.Complete || u.HoldSeconds != 0 {
		t.Errorf("hold %+v", u)
	}
	if !u.BgfetchRefused {
		t.Errorf("bgfetch refused not seen")
	}
	if !u.FirstAt.Equal(at(3)) || !u.LastAt.Equal(at(181)) {
		t.Errorf("unit span %v..%v", u.FirstAt, u.LastAt)
	}
}

func TestSummarizeHeldThenReleased(t *testing.T) {
	steps := heldSteps()[:11] // through the stall, no fail
	steps = append(steps,
		s(192, "download", "ok", "Unit 1: Water Cycle: download complete — 213 MB in 3:08 (1.1 MB/s)"),
		s(193, "verify", "ok", "All 5 files present; largest file size verified"),
	)
	sum := Summarize(steps, "download-complete", Context{UnitID: "unit1"})
	u := sum.Units[0]
	if !u.Complete || u.CompleteAt == nil || !u.CompleteAt.Equal(at(192)) || u.CompleteMB != 213 || u.CompleteSeconds != 188 || u.CompleteMBps != 1.1 {
		t.Errorf("complete %+v", u)
	}
	if !u.Held || u.HeldAtMB != 201 || u.HoldSeconds != 162 {
		t.Errorf("held→done: %+v", u)
	}
}

func TestSummarizeCleanDownloadIsNotHeld(t *testing.T) {
	steps := []models.MHSDeviceTestStep{
		s(0, "download", "running", "Requesting Unit 1: Water Cycle v2.8.1 (213 MB)"),
		s(5, "download", "running", "Unit 1: Water Cycle: 48% · 102 of 213 MB · 9.3 MB/s · 0:12 left"),
		s(10, "download", "running", "Unit 1: Water Cycle: 95% · 203 of 213 MB · 9.3 MB/s · 0:01 left"),
		s(12, "download", "ok", "Unit 1: Water Cycle: download complete — 213 MB in 0:22 (9.7 MB/s)"),
	}
	u := Summarize(steps, "download-complete", Context{}).Units[0]
	if u.Held || !u.Complete || u.MaxMB != 203 || u.Drops != 0 {
		t.Errorf("clean download read as %+v", u)
	}
	// One resumed connection near the end that completes at once is not a hold either.
	steps = append(steps[:3], s(11, "download", "info", "Unit 1: Water Cycle: picked up at 203 MB after a dropped connection"), steps[3])
	if u := Summarize(steps, "download-complete", Context{}).Units[0]; u.Held || u.Drops != 1 {
		t.Errorf("single drop read as %+v", u)
	}
}

func TestSummarizeStopFarFromEndIsNotHeld(t *testing.T) {
	steps := []models.MHSDeviceTestStep{
		s(0, "download", "running", "Requesting Unit 2: Water Quality v2.8.6 (156 MB)"),
		s(5, "download", "running", "Unit 2: Water Quality: 70% · 110 of 156 MB · 0.3 MB/s · 2:33 left"),
		s(40, "download", "info", "Unit 2: Water Quality: picked up at 110 MB after a dropped connection"),
		s(70, "download", "info", "Unit 2: Water Quality: picked up at 110 MB after a dropped connection (2 drops so far)"),
		s(100, "download", "fail", "Unit 2: Water Quality: Download failed [network:ECONNRESET] — TypeError: Failed to fetch"),
	}
	u := Summarize(steps, "download-failed", Context{}).Units[0]
	if u.Held || u.Fails != 1 || u.LastErrorClass != "network" || u.MaxMB != 110 {
		t.Errorf("far stop read as %+v", u)
	}
}

func TestSummarizeTotalFromProgressWhenRequestingIsGone(t *testing.T) {
	steps := []models.MHSDeviceTestStep{
		s(30, "download", "running", "Unit 1: Water Cycle: 94% · 201 of 213 MB"),
		s(60, "download", "info", "Unit 1: Water Cycle: picked up at 201 MB after a dropped connection (4 drops so far)"),
		s(90, "download", "info", "Unit 1: Water Cycle: picked up at 201 MB after a dropped connection (5 drops so far)"),
	}
	u := Summarize(steps, "launch-ok", Context{UnitID: "unit1"}).Units[0]
	if !u.Held || u.TotalMB != 213 || u.Requested {
		t.Errorf("%+v", u)
	}
}

func TestSummarizeCachedUnit(t *testing.T) {
	steps := []models.MHSDeviceTestStep{
		s(0, "method", "info", "Not needed — Unit 1: Water Cycle is already on this device"),
		s(1, "download", "ok", "Unit 1: Water Cycle: already on this device (downloaded earlier)"),
	}
	u := Summarize(steps, "report", Context{}).Units[0]
	if !u.Cached || u.Complete || u.Held || u.Unit != "unit1" {
		t.Errorf("%+v", u)
	}
}

func TestSummarizeLaunchAndCrash(t *testing.T) {
	steps := []models.MHSDeviceTestStep{
		s(0, "page", "info", "Opened the play page for unit1 v2.8.1"),
		s(1, "launch", "running", "Loading the game loader (unit1.loader.js)…"),
		s(2, "launch", "ok", "Game loader loaded (from the downloaded files)"),
		s(2, "launch", "running", "Starting Unity…"),
		s(35, "launch", "warn", "No progress for 33 s while almost ready — the loader answered (HTTP 200 in 454 ms), so the network is fine"),
		s(42, "launch", "ok", "Unity started in 42.1 s"),
		s(42, "game", "running", "Game running — identity sent to the game"),
		s(43, "game", "ok", "Game is rendering"),
		s(300, "game", "fail", "Crash (unhandled_rejection, game): A user gesture is required to request Pointer Lock."),
		s(900, "game", "fail", "Crash (runtime_error, running): RuntimeError: memory access out of bounds"),
	}
	sum := Summarize(steps, "crash", Context{UnitID: "unit1"})
	kinds := []string{}
	for _, l := range sum.Launch {
		kinds = append(kinds, l.Kind)
	}
	want := []string{models.MHSLaunchEventLoaderOK, models.MHSLaunchEventWatchdogWarn, models.MHSLaunchEventUnityOK, models.MHSLaunchEventRendering}
	if len(kinds) != len(want) {
		t.Fatalf("launch kinds %v, want %v", kinds, want)
	}
	for i := range want {
		if kinds[i] != want[i] {
			t.Errorf("launch[%d] = %s, want %s", i, kinds[i], want[i])
		}
	}
	if sum.Launch[0].Source != "cache" || sum.Launch[1].Seconds != 33 || sum.Launch[1].Phase != "almost ready" || sum.Launch[2].Seconds != 42.1 || sum.Launch[3].Unit != "unit1" {
		t.Errorf("launch details %+v", sum.Launch)
	}
	if len(sum.Crashes) != 2 || !sum.Crashes[0].Notice || sum.Crashes[1].Notice || sum.Crashes[1].Type != "runtime_error" || sum.Crashes[1].Unit != "unit1" {
		t.Errorf("crashes %+v", sum.Crashes)
	}
	if len(sum.Units) != 0 {
		t.Errorf("a play page log has no download units: %+v", sum.Units)
	}
}

func TestSummarizeTwoUnitsInOneLog(t *testing.T) {
	steps := []models.MHSDeviceTestStep{
		s(0, "download", "running", "Requesting Unit 1: Water Cycle v2.8.1 (213 MB)"),
		s(20, "download", "ok", "Unit 1: Water Cycle: download complete — 213 MB in 0:20 (10.6 MB/s)"),
		s(25, "download", "running", "Requesting Unit 2: Water Quality v2.8.6 (156 MB)"),
		s(30, "download", "running", "Unit 2: Water Quality: 20% · 31 of 156 MB · 6.0 MB/s · 0:20 left"),
	}
	sum := Summarize(steps, "download-complete", Context{UnitID: "unit1"})
	if len(sum.Units) != 2 || sum.Units[0].Unit != "unit1" || sum.Units[1].Unit != "unit2" {
		t.Fatalf("%+v", sum.Units)
	}
	if !sum.Units[0].Complete || sum.Units[1].Complete || sum.Units[1].MaxMB != 31 {
		t.Errorf("%+v", sum.Units)
	}
}

func TestSummarizeFallsBackToRecordUnit(t *testing.T) {
	steps := []models.MHSDeviceTestStep{
		s(0, "download", "warn", "No data for 12 s — switching to the direct download in 13 s"),
		s(13, "download", "warn", "Download interrupted. Retrying in 10 s (attempt 2)"),
	}
	sum := Summarize(steps, "download-switched", Context{UnitID: "unit3"})
	if len(sum.Units) != 1 || sum.Units[0].Unit != "unit3" || sum.Units[0].Retries != 1 {
		t.Errorf("%+v", sum.Units)
	}
}
