package mhsactivity

import (
	"testing"
	"time"
)

func TestDayBoundsAcrossDaylightSaving(t *testing.T) {
	chicago, err := time.LoadLocation("America/Chicago")
	if err != nil {
		t.Skip("tz database unavailable")
	}
	// 2026-11-01: clocks fall back at 2 am, a 25-hour day.
	start, end, err := DayBounds("2026-11-01", chicago)
	if err != nil {
		t.Fatal(err)
	}
	if got := end.Sub(start); got != 25*time.Hour {
		t.Errorf("2026-11-01 Chicago is %v long, want 25h", got)
	}
	if start.UTC().Format(time.RFC3339) != "2026-11-01T05:00:00Z" || end.UTC().Format(time.RFC3339) != "2026-11-02T06:00:00Z" {
		t.Errorf("bounds %v .. %v", start.UTC(), end.UTC())
	}
	// 2026-03-08: clocks spring forward, a 23-hour day.
	start, end, _ = DayBounds("2026-03-08", chicago)
	if got := end.Sub(start); got != 23*time.Hour {
		t.Errorf("2026-03-08 Chicago is %v long, want 23h", got)
	}
	// An ordinary day in Los Angeles.
	la, _ := time.LoadLocation("America/Los_Angeles")
	start, end, _ = DayBounds("2026-10-12", la)
	if start.UTC().Hour() != 7 || end.Sub(start) != 24*time.Hour {
		t.Errorf("LA bounds %v .. %v", start.UTC(), end.UTC())
	}
	if _, _, err := DayBounds("2026-13-40", la); err == nil {
		t.Error("bad day accepted")
	}
}

func TestLocalDayAndClosed(t *testing.T) {
	chicago, _ := time.LoadLocation("America/Chicago")
	// 2026-10-13 04:00 UTC is still 2026-10-12 23:00 in Chicago.
	at := time.Date(2026, 10, 13, 4, 0, 0, 0, time.UTC)
	if got := LocalDay(at, chicago); got != "2026-10-12" {
		t.Errorf("LocalDay = %s", got)
	}
	if got := LocalDay(at, time.UTC); got != "2026-10-13" {
		t.Errorf("LocalDay UTC = %s", got)
	}
	// The Chicago day 2026-10-12 ends 05:00 UTC on the 13th; with 6 h it
	// closes at 11:00 UTC.
	if Closed("2026-10-12", chicago, 6*time.Hour, time.Date(2026, 10, 13, 10, 59, 0, 0, time.UTC)) {
		t.Error("closed too early")
	}
	if !Closed("2026-10-12", chicago, 6*time.Hour, time.Date(2026, 10, 13, 11, 0, 0, 0, time.UTC)) {
		t.Error("not closed at 11:00 UTC")
	}
	if Closed("bad", chicago, time.Hour, time.Now()) {
		t.Error("a bad day is never closed")
	}
}

func TestDaysBetweenAndAddDays(t *testing.T) {
	days, err := DaysBetween("2026-10-07", "2026-10-09")
	if err != nil || len(days) != 3 || days[0] != "2026-10-07" || days[2] != "2026-10-09" {
		t.Errorf("DaysBetween = %v, %v", days, err)
	}
	if _, err := DaysBetween("2026-10-09", "2026-10-07"); err == nil {
		t.Error("reversed range accepted")
	}
	if d, _ := AddDays("2026-10-31", 1); d != "2026-11-01" {
		t.Errorf("AddDays = %s", d)
	}
	if d, _ := AddDays("2026-01-01", -1); d != "2025-12-31" {
		t.Errorf("AddDays back = %s", d)
	}
}

func TestLocationForFallsBackToUTC(t *testing.T) {
	if loc, fb := LocationFor(""); loc != time.UTC || !fb {
		t.Error("empty zone should fall back")
	}
	if loc, fb := LocationFor("Mars/Olympus"); loc != time.UTC || !fb {
		t.Error("unknown zone should fall back")
	}
	if loc, fb := LocationFor("America/Denver"); loc.String() != "America/Denver" || fb {
		t.Error("known zone should load")
	}
}
