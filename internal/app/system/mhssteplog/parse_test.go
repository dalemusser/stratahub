package mhssteplog

import (
	"testing"
	"time"

	"github.com/dalemusser/stratahub/internal/domain/models"
)

func step(stepID, state, msg string, detail map[string]string) models.MHSDeviceTestStep {
	return models.MHSDeviceTestStep{At: time.Date(2026, 10, 12, 17, 10, 0, 0, time.UTC), Step: stepID, State: state, Msg: msg, Detail: detail}
}

func TestParseDownloadSentences(t *testing.T) {
	tests := []struct {
		name string
		st   models.MHSDeviceTestStep
		want Event
	}{
		{"requesting", step("download", "running", "Requesting Unit 1: Water Cycle v2.8.1 (213 MB)", nil),
			Event{Kind: KindRequesting, Unit: "unit1", Title: "Unit 1: Water Cycle", Version: "2.8.1", TotalMB: 213}},
		{"progress with rate", step("download", "running", "Unit 1: Water Cycle: 94% · 201 of 213 MB · 9.3 MB/s · 0:01 left", nil),
			Event{Kind: KindProgress, Unit: "unit1", Title: "Unit 1: Water Cycle", Pct: 94, MB: 201, TotalMB: 213, MBps: 9.3}},
		{"progress first sample", step("download", "running", "Unit 2: Water Quality: 0% · 0 of 156 MB", nil),
			Event{Kind: KindProgress, Unit: "unit2", Title: "Unit 2: Water Quality", TotalMB: 156}},
		{"progress by unit id title", step("download", "running", "unit3: 40% · 62 of 156 MB", nil),
			Event{Kind: KindProgress, Unit: "unit3", Title: "unit3", Pct: 40, MB: 62, TotalMB: 156}},
		{"resumed first drop", step("download", "info", "Unit 1: Water Cycle: picked up at 201 MB after a dropped connection", nil),
			Event{Kind: KindResumed, Unit: "unit1", Title: "Unit 1: Water Cycle", MB: 201, Drops: 1}},
		{"resumed with count", step("download", "info", "Unit 1: Water Cycle: picked up at 201 MB after a dropped connection (3 drops so far)", nil),
			Event{Kind: KindResumed, Unit: "unit1", Title: "Unit 1: Water Cycle", MB: 201, Drops: 3}},
		{"stall", step("download", "warn", "Unit 1: Water Cycle: no data received for 30 s — restarting the download", nil),
			Event{Kind: KindNoData, Unit: "unit1", Title: "Unit 1: Water Cycle", Seconds: 30}},
		{"waiting before switch", step("download", "warn", "No data for 12 s — switching to the direct download in 13 s", nil),
			Event{Kind: KindWaiting, Seconds: 12}},
		{"retry in", step("download", "warn", "Download interrupted. Retrying in 10 s (attempt 3)", nil),
			Event{Kind: KindRetry, Seconds: 10, Attempt: 3, Message: "Download interrupted."}},
		{"retry now", step("download", "warn", "The content server did not answer. Retrying now (attempt 1)", nil),
			Event{Kind: KindRetry, Attempt: 1, Message: "The content server did not answer."}},
		{"fail with class and raw", step("download", "fail", "Unit 1: Water Cycle: Download failed [network:timeout] — TypeError: Failed to fetch", map[string]string{"path": "fallback"}),
			Event{Kind: KindDownloadFail, Unit: "unit1", Title: "Unit 1: Water Cycle", Message: "Download failed", ErrorClass: "network", Reason: "timeout", Path: "fallback"}},
		{"fail class only", step("download", "fail", "Unit 2: Water Quality: 6 dropped connections resumed, then: no data [stalled]", map[string]string{"path": "fallback"}),
			Event{Kind: KindDownloadFail, Unit: "unit2", Title: "Unit 2: Water Quality", Message: "6 dropped connections resumed, then: no data", ErrorClass: "stalled", Path: "fallback"}},
		{"fail bare", step("download", "fail", "Unit 2: Water Quality: Download failed", nil),
			Event{Kind: KindDownloadFail, Unit: "unit2", Title: "Unit 2: Water Quality", Message: "Download failed"}},
		{"complete", step("download", "ok", "Unit 1: Water Cycle: download complete — 213 MB in 0:22 (9.7 MB/s)", nil),
			Event{Kind: KindDownloadOK, Unit: "unit1", Title: "Unit 1: Water Cycle", MB: 213, Seconds: 22, MBps: 9.7}},
		{"complete minutes", step("download", "ok", "Unit 3: Watersheds: download complete — 156 MB in 1:05 (2.4 MB/s)", nil),
			Event{Kind: KindDownloadOK, Unit: "unit3", Title: "Unit 3: Watersheds", MB: 156, Seconds: 65, MBps: 2.4}},
		{"complete without timing", step("download", "ok", "Unit 3: Watersheds: download complete", nil),
			Event{Kind: KindDownloadOK, Unit: "unit3", Title: "Unit 3: Watersheds"}},
		{"cached", step("download", "ok", "Unit 1: Water Cycle: already on this device (downloaded earlier)", nil),
			Event{Kind: KindCached, Unit: "unit1", Title: "Unit 1: Water Cycle"}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := Parse(tc.st)
			if !ok {
				t.Fatalf("Parse(%q) not recognised", tc.st.Msg)
			}
			assertEvent(t, got, tc.want)
		})
	}
}

func TestParsePrefersDetail(t *testing.T) {
	st := step("download", "running", "Unit 1: Water Cycle: 94% · 201 of 213 MB", map[string]string{"unit": "unit1", "mb": "202", "total_mb": "214", "pct": "94", "mbps": "9.3"})
	got, ok := Parse(st)
	if !ok {
		t.Fatal("not recognised")
	}
	assertEvent(t, got, Event{Kind: KindProgress, Unit: "unit1", Title: "Unit 1: Water Cycle", Pct: 94, MB: 202, TotalMB: 214, MBps: 9.3})

	// A failure whose class is only in the detail (older sentences lacked it).
	st = step("download", "fail", "Unit 2: Water Quality: Download failed", map[string]string{"path": "background", "error_class": "quota", "reason": "QUOTA_EXCEEDED", "version": "2.8.6"})
	got, _ = Parse(st)
	assertEvent(t, got, Event{Kind: KindDownloadFail, Unit: "unit2", Title: "Unit 2: Water Quality", Version: "2.8.6", Message: "Download failed", ErrorClass: "quota", Reason: "QUOTA_EXCEEDED", Path: "background"})

	// Numbers arrive as JSON text; a float for an int field still reads.
	st = step("download", "ok", "Unit 1: Water Cycle: download complete — 213 MB in 0:22 (9.7 MB/s)", map[string]string{"mb": "213", "seconds": "22.0", "mbps": "9.7"})
	got, _ = Parse(st)
	if got.Seconds != 22 || got.MB != 213 {
		t.Fatalf("detail numbers: %+v", got)
	}
}

func TestParseMethodStorageLaunchGame(t *testing.T) {
	yes := true
	no := false
	tests := []struct {
		name string
		st   models.MHSDeviceTestStep
		want Event
	}{
		{"refused", step("method", "warn", "The browser refused the background download (BackgroundFetch is disabled by policy) — using the direct download; keep this tab open", nil),
			Event{Kind: KindBgfetchRefused, Reason: "BackgroundFetch is disabled by policy"}},
		{"refused with detail", step("method", "warn", "The browser refused the background download (x) — using the direct download; keep this tab open", map[string]string{"unit": "unit1", "kind": "bgfetch-refused", "reason": "x"}),
			Event{Kind: KindBgfetchRefused, Unit: "unit1", Reason: "x"}},
		{"frozen", step("method", "warn", "Background download stopped receiving data for 25 s — switched to the direct download (keep this tab open)", map[string]string{"quiet_s": "25", "bf_downloaded": "1"}),
			Event{Kind: KindBgfetchFrozen, Seconds: 25}},
		{"method info ignored", step("method", "info", "Direct download — this device paused background downloads earlier; keep this tab open", nil), Event{}},
		{"storage persisted", step("storage", "ok", "Storage: 620 MB used of 10860 MB (6%) · persisted: yes", nil),
			Event{Kind: KindStorage, UsedMB: 620, QuotaMB: 10860, Persisted: &yes}},
		{"storage not persisted", step("storage", "warn", "Storage: 400 MB used of 2048 MB (20%) · persisted: no — the browser may evict downloads under pressure", nil),
			Event{Kind: KindStorage, UsedMB: 400, QuotaMB: 2048, Persisted: &no}},
		{"storage unknown persistence", step("storage", "ok", "Storage: 400 MB used of 2048 MB (20%)", nil),
			Event{Kind: KindStorage, UsedMB: 400, QuotaMB: 2048}},
		{"other storage line ignored", step("storage", "info", "Cache check: unit1 cached", nil), Event{}},
		{"loader from cache", step("launch", "ok", "Game loader loaded (from the downloaded files)", nil),
			Event{Kind: KindLoaderOK, Source: "cache"}},
		{"loader from network", step("launch", "ok", "Game loader loaded (from the network)", nil),
			Event{Kind: KindLoaderOK, Source: "network"}},
		{"loader unknown source", step("launch", "ok", "Game loader loaded", nil), Event{Kind: KindLoaderOK}},
		{"unity started", step("launch", "ok", "Unity started in 42.1 s", nil), Event{Kind: KindUnityOK, Seconds: 42.1}},
		{"watchdog warn", step("launch", "warn", "No progress for 33 s while almost ready — the loader answered (HTTP 200 in 454 ms), so the network is fine", nil),
			Event{Kind: KindWatchdogWarn, Seconds: 33, Phase: "almost ready", Message: "the loader answered (HTTP 200 in 454 ms), so the network is fine"}},
		{"watchdog fail", step("launch", "fail", "Still no progress after 90 s — stuck almost ready, even straight from the content server. The game files are not reaching the page on this device.", nil),
			Event{Kind: KindWatchdogFail, Seconds: 90, Phase: "almost ready", Fallback: true}},
		{"watchdog fail no fallback", step("launch", "fail", "Still no progress after 90 s — stuck loading the game loader. The game files are not reaching the page on this device.", nil),
			Event{Kind: KindWatchdogFail, Seconds: 90, Phase: "loading the game loader"}},
		{"unity failed", step("launch", "fail", "Unity failed to start: undefined", nil), Event{Kind: KindUnityFail, Reason: "undefined"}},
		{"loader failed", step("launch", "fail", "The game loader failed to load — the unit may not be downloaded, or the content server is unreachable", nil),
			Event{Kind: KindLoaderFail, Reason: "loader-script-error"}},
		{"launch running ignored", step("launch", "running", "Starting Unity…", nil), Event{}},
		{"rendering", step("game", "ok", "Game is rendering", map[string]string{"unit": "unit1"}), Event{Kind: KindRendering, Unit: "unit1"}},
		{"crash notice", step("game", "fail", "Crash (unhandled_rejection, game): A user gesture is required to request Pointer Lock.", nil),
			Event{Kind: KindCrash, CrashType: "unhandled_rejection", Phase: "game", Message: "A user gesture is required to request Pointer Lock.", Notice: true}},
		{"crash real", step("game", "fail", "Crash (runtime_error, running): RuntimeError: memory access out of bounds", nil),
			Event{Kind: KindCrash, CrashType: "runtime_error", Phase: "running", Message: "RuntimeError: memory access out of bounds"}},
		{"game warn ignored", step("game", "warn", "Connection lost — the game keeps playing", nil), Event{}},
		{"page", step("page", "info", "Opened the play page for unit1 v2.8.1", nil), Event{Kind: KindPage, Unit: "unit1", Version: "2.8.1"}},
		{"page units", step("page", "info", "Opened the units page", nil), Event{Kind: KindPage}},
		{"sw ignored", step("sw", "ok", "Service worker active (v1.0.16)", nil), Event{}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := Parse(tc.st)
			if tc.want.Kind == "" {
				if ok {
					t.Fatalf("Parse(%q) recognised as %s, want ignored", tc.st.Msg, got.Kind)
				}
				return
			}
			if !ok {
				t.Fatalf("Parse(%q) not recognised", tc.st.Msg)
			}
			assertEvent(t, got, tc.want)
		})
	}
}

func TestUnitID(t *testing.T) {
	for in, want := range map[string]string{"Unit 1": "unit1", "unit1": "unit1", "Unit 3: Watersheds": "unit3", "UNIT 6": "unit6", "end": "end", "": "", "Water Cycle": ""} {
		if got := unitID(in); got != want {
			t.Errorf("unitID(%q) = %q, want %q", in, got, want)
		}
	}
}

func assertEvent(t *testing.T, got, want Event) {
	t.Helper()
	if got.Kind != want.Kind {
		t.Errorf("Kind = %s, want %s", got.Kind, want.Kind)
	}
	if got.Unit != want.Unit {
		t.Errorf("Unit = %q, want %q", got.Unit, want.Unit)
	}
	if want.Title != "" && got.Title != want.Title {
		t.Errorf("Title = %q, want %q", got.Title, want.Title)
	}
	if got.Version != want.Version {
		t.Errorf("Version = %q, want %q", got.Version, want.Version)
	}
	if got.MB != want.MB || got.TotalMB != want.TotalMB || got.Pct != want.Pct || got.Drops != want.Drops || got.Attempt != want.Attempt {
		t.Errorf("numbers = mb %d total %d pct %d drops %d attempt %d, want mb %d total %d pct %d drops %d attempt %d",
			got.MB, got.TotalMB, got.Pct, got.Drops, got.Attempt, want.MB, want.TotalMB, want.Pct, want.Drops, want.Attempt)
	}
	if got.Seconds != want.Seconds || got.MBps != want.MBps {
		t.Errorf("Seconds/MBps = %v/%v, want %v/%v", got.Seconds, got.MBps, want.Seconds, want.MBps)
	}
	if got.ErrorClass != want.ErrorClass || got.Reason != want.Reason || got.Path != want.Path || got.Source != want.Source || got.Phase != want.Phase || got.CrashType != want.CrashType {
		t.Errorf("strings = class %q reason %q path %q source %q phase %q type %q, want class %q reason %q path %q source %q phase %q type %q",
			got.ErrorClass, got.Reason, got.Path, got.Source, got.Phase, got.CrashType, want.ErrorClass, want.Reason, want.Path, want.Source, want.Phase, want.CrashType)
	}
	if want.Message != "" && got.Message != want.Message {
		t.Errorf("Message = %q, want %q", got.Message, want.Message)
	}
	if got.Notice != want.Notice || got.Fallback != want.Fallback {
		t.Errorf("Notice/Fallback = %v/%v, want %v/%v", got.Notice, got.Fallback, want.Notice, want.Fallback)
	}
	if got.UsedMB != want.UsedMB || got.QuotaMB != want.QuotaMB {
		t.Errorf("storage = %d/%d, want %d/%d", got.UsedMB, got.QuotaMB, want.UsedMB, want.QuotaMB)
	}
	if (got.Persisted == nil) != (want.Persisted == nil) || (got.Persisted != nil && *got.Persisted != *want.Persisted) {
		t.Errorf("Persisted = %v, want %v", got.Persisted, want.Persisted)
	}
}
