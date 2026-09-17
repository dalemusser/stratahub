// internal/app/features/missionhydrosci/logs_health.go
//
// Logging health of a launch: while the game runs, is the log service
// receiving its gameplay entries? The game reports nothing when its own
// logging fails (docs/mission-hydrosci/mhs-game-logging-silent-failure-plan.md
// §2), so the check is made here, on the page's heartbeat, against the log
// service's collection, and the answer travels back in the heartbeat reply.
package missionhydrosci

import (
	"context"
	"time"

	"github.com/dalemusser/stratahub/internal/domain/models"
	"go.uber.org/zap"
)

const (
	logsHealthGame = "mhs"

	// Check on every logsCheckEvery-th beat until the service has seen an
	// entry from this launch, then every logsRecheckEvery-th so a sender
	// that dies mid-session is still caught. Beats are 30 s apart, so the
	// first check lands about 4.5 minutes into play.
	logsCheckEvery   = 10
	logsRecheckEvery = 20

	// The page is told "nothing is arriving" only after this much play, so
	// a slow start never trips the notice; the same rule stamps the flag.
	logsFlagAfterMs = 4 * 60 * 1000

	// Budget for the read. Past it the state is "unknown", never "none".
	logsCheckBudget = 3 * time.Second

	// Entries created a little before the launch record count as this
	// launch's: a member's record is created at first frame, after the
	// game's earliest events may already have gone out.
	logsCheckSlack = 2 * time.Minute
)

// heartbeatReply is the JSON body every heartbeat returns.
type heartbeatReply struct {
	// Logs is the launch's logging-health state the page should act on:
	// models.MHSLogsStateSeen, None or Unknown; empty before the first check.
	Logs string `json:"logs,omitempty"`
}

// gameProducingEvents reports whether the game has been generating events it
// could not send: it writes every unsent event to its PlayerPrefs store, so
// a store that has grown since launch, one near Unity's cap, or a
// cache-write error the page saw all mean events exist. Without that, "no
// entries arriving" is an idle game, not a failure. A page that cannot read
// the store (no IndexedDB, an older browser) reports 0 and is treated as
// idle, so it never raises a false alarm; the cache-error hook still covers
// it at the cap.
func gameProducingEvents(rec models.MHSDeviceTest, beat models.MHSDeviceTestHeartbeat) bool {
	if beat.CacheErrors > 0 {
		return true
	}
	if beat.PlayerPrefsBytes <= 0 {
		return false
	}
	if beat.PlayerPrefsBytes*100 >= int64(models.MHSPlayerPrefsWarnPercent)*models.MHSPlayerPrefsCapBytes {
		return true
	}
	if rec.PlayerPrefsBytesAtLaunch <= 0 {
		return false // this beat is the baseline; growth is measured from here
	}
	return beat.PlayerPrefsBytes-rec.PlayerPrefsBytesAtLaunch >= models.MHSPlayerPrefsGrowthBytes
}

// logsHealthAfterBeat runs the check when this beat is due and returns the
// state for the page. rec is the record as it was before the beat was
// appended (so its count is one behind); gameUserID is the id the game logs
// under: the member's user id, or the device-test run id.
func (h *Handler) logsHealthAfterBeat(ctx context.Context, rec models.MHSDeviceTest, beat models.MHSDeviceTestHeartbeat, gameUserID string) heartbeatReply {
	reply := heartbeatReply{Logs: rec.LogsState}
	count := rec.HeartbeatCount + 1
	every := logsCheckEvery
	if rec.LogsSeenAt != nil {
		every = logsRecheckEvery
	}
	if count%every != 0 {
		return reply
	}

	since := rec.StartedAt
	if rec.Kind == models.MHSDeviceTestKindDeviceTest && rec.LastLaunchAt != nil {
		// A device-test run starts at the form, possibly long before launch.
		since = *rec.LastLaunchAt
	}

	state := models.MHSLogsStateUnknown
	if h.Logs != nil && gameUserID != "" {
		cctx, cancel := context.WithTimeout(ctx, logsCheckBudget)
		defer cancel()
		seen, err := h.Logs.HasEntrySince(cctx, logsHealthGame, gameUserID, since.Add(-logsCheckSlack))
		switch {
		case err != nil:
			h.Log.Warn("logging-health check failed", zap.String("record", rec.ID.Hex()), zap.Error(err))
		case seen:
			state = models.MHSLogsStateSeen
		case gameProducingEvents(rec, beat):
			state = models.MHSLogsStateNone
		default:
			// Nothing arrived, but the game shows no sign of producing
			// events either (its store has not grown): a menu screen, not
			// a failure. Keep checking.
			state = models.MHSLogsStateQuiet
		}
	}

	flag := state == models.MHSLogsStateNone && beat.ElapsedMs >= logsFlagAfterMs
	if err := h.DeviceTestStore.SetLogsState(ctx, rec.WorkspaceID, rec.ID, state, flag); err != nil {
		h.Log.Warn("logging-health state not stored", zap.String("record", rec.ID.Hex()), zap.Error(err))
	}
	if state == models.MHSLogsStateNone && !flag {
		// Nothing has arrived yet, but it is early: the page keeps what it
		// knew rather than being told of a failure.
		if reply.Logs == "" || reply.Logs == models.MHSLogsStateNone {
			reply.Logs = models.MHSLogsStateUnknown
		}
		return reply
	}
	if flag {
		h.Log.Info("logging-health: playing with no log entries arriving",
			zap.String("record", rec.ID.Hex()), zap.String("kind", rec.Kind),
			zap.String("unit", rec.UnitID+" v"+rec.UnitVersion), zap.Int64("elapsed_ms", beat.ElapsedMs))
	}
	reply.Logs = state
	return reply
}
