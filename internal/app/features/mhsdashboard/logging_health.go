// internal/app/features/mhsdashboard/logging_health.go
//
// The Devices tab's "Logs" column: whether the game's activity record (its
// gameplay logs) reached the log service from a device the last time it
// played. The game itself never reports a logging failure, so this comes
// from the launch record's server-side check and from the page's readings
// (docs/mission-hydrosci/mhs-game-logging-silent-failure-plan.md §5.2).
package mhsdashboard

import (
	"fmt"
	"time"

	"github.com/dalemusser/stratahub/internal/domain/models"
)

// loggingRemedy is what a teacher can do on the device. It is the same text
// the play page shows the teacher under "What to do".
const loggingRemedy = " What to do: nothing on the device. The game keeps its record there, and the next time the game is opened on this device the page removes what stopped it sending, so the saved record is sent then (use good Wi-Fi); this column shows the result after about five minutes of play. If the device is flagged again after that session, the school network may be blocking the game's log service. See the Teacher Guide, \"Game activity not recorded\"."

// fillLoggingHealth sets the "Logs" fields of a device from the newest
// launch record on it (nil when none in the window) and the launcher's last
// reading of the game's PlayerPrefs store.
func fillLoggingHealth(d *DeviceInfo, s models.MHSDeviceStatus, launch *models.MHSDeviceTest, loc *time.Location) {
	if s.PlayerPrefsBytes != nil {
		d.PrefsBytes = *s.PlayerPrefsBytes
	}
	if launch != nil {
		d.LogsState = launch.LogsState
		if launch.LogsSeenAt != nil {
			d.LogsSeenAt = launch.LogsSeenAt.In(loc)
		}
		if launch.LogsCheckedAt != nil {
			d.LogsCheckedAt = launch.LogsCheckedAt.In(loc)
		}
		d.CacheErrors = launch.CacheErrors
		// The play page's reading is the newer one when the device played
		// after the launcher last reported.
		if launch.PlayerPrefsBytes > 0 && (s.PlayerPrefsBytes == nil || launch.StartedAt.After(s.LastSeen)) {
			d.PrefsBytes = launch.PlayerPrefsBytes
		}
	}
	d.PrefsPct = int(d.PrefsBytes * 100 / models.MHSPlayerPrefsCapBytes)

	switch {
	case d.CacheErrors > 0:
		d.LoggingProblem = true
		d.LoggingText = "Cache full"
		d.LoggingTitle = "Game activity is not being recorded on this device: in the latest play session the game reported it could not save its log cache (its local store is full)." + loggingRemedy
	case d.LogsState == models.MHSLogsStateNone:
		d.LoggingProblem = true
		d.LoggingText = "Not recorded"
		d.LoggingTitle = "Game activity is not being recorded on this device: the latest play session ran for several minutes with no gameplay logs reaching the server." + loggingRemedy
	case d.PrefsPct >= models.MHSPlayerPrefsWarnPercent:
		d.LoggingProblem = true
		d.LoggingText = "Nearly full"
		d.LoggingTitle = fmt.Sprintf("The game's local log store on this device is %d%% full (%d KB of %d KB): gameplay logs have been piling up unsent for a long time.", d.PrefsPct, d.PrefsBytes/1024, models.MHSPlayerPrefsCapBytes/1024) + loggingRemedy
	case (d.LogsState == models.MHSLogsStateSeen || d.LogsState == models.MHSLogsStateQuiet) && !d.LogsSeenAt.IsZero():
		// Quiet after a confirmation is an idle stretch, not a failure: the
		// last confirmation stands.
		d.LoggingTitle = "Gameplay logs from this device reached the server at " + d.LogsSeenAt.Format("Jan 2, 3:04 PM") + "."
	case d.LogsState == models.MHSLogsStateQuiet:
		d.LoggingTitle = "No verdict: the latest play session on this device produced no game activity to record (the game sat at a menu screen or in a background tab), so there was nothing to check."
	case launch != nil:
		d.LoggingTitle = "No verdict yet: the latest play session on this device was too short for a check (the first check comes after about five minutes of play)."
	default:
		d.LoggingTitle = "No play session has been recorded on this device yet."
	}
}
