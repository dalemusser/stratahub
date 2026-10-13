package mhssteplog

import (
	"sort"
	"strings"
	"time"

	"github.com/dalemusser/stratahub/internal/domain/models"
)

// HoldTailMB is how close to the end a download has to have come for a
// stop there to count as a hold. The holds seen in Oct 2026 all stopped
// exactly 12 MB before the end of each file (the filter's scan window);
// 16 leaves room for one resumed-connection part.
const HoldTailMB = 16

// holdMinSeconds is the least time between reaching the tail and completing
// for a stop there to be a hold rather than the ordinary last part arriving.
const holdMinSeconds = 10

// Caps on the lists a summary keeps.
const (
	maxSummaryLaunch  = 24
	maxSummaryCrashes = 12
)

// Context is what the page's log context adds to a summary.
type Context struct {
	UnitID          string // the record's unit, for entries that name none
	DeviceID        string
	SWVersion       string
	DeliveryVersion string
	DownloadMode    string
}

type mbAt struct {
	mb int
	at time.Time
}

type unitAcc struct {
	models.MHSStepUnitSummary
	progress []mbAt      // progress lines (bytes reached)
	resumes  []mbAt      // "picked up at" lines
	signals  []time.Time // stalls, waits and retries
}

// Summarize reads a record's steps into a summary. It never fails: entries
// it does not understand are skipped and counted in Steps only.
func Summarize(steps []models.MHSDeviceTestStep, outcome string, ctx Context) models.MHSMemberSummary {
	s := models.MHSMemberSummary{
		V:               models.MHSMemberSummaryVersion,
		Outcome:         outcome,
		Steps:           len(steps),
		DeviceID:        ctx.DeviceID,
		SWVersion:       ctx.SWVersion,
		DeliveryVersion: ctx.DeliveryVersion,
		DownloadMode:    ctx.DownloadMode,
	}
	units := map[string]*unitAcc{}
	var order []string
	acc := func(unit string) *unitAcc {
		if unit == "" {
			unit = ctx.UnitID
		}
		if unit == "" {
			unit = "unknown"
		}
		u := units[unit]
		if u == nil {
			u = &unitAcc{}
			u.Unit = unit
			units[unit] = u
			order = append(order, unit)
		}
		return u
	}
	launchUnit := func(ev Event) string {
		if ev.Unit != "" {
			return ev.Unit
		}
		return ctx.UnitID
	}

	for _, st := range steps {
		if !st.At.IsZero() {
			if s.FirstAt.IsZero() || st.At.Before(s.FirstAt) {
				s.FirstAt = st.At
			}
			if st.At.After(s.LastAt) {
				s.LastAt = st.At
			}
		}
		ev, ok := Parse(st)
		if !ok {
			continue
		}
		switch ev.Kind {
		case KindRequesting, KindProgress, KindResumed, KindNoData, KindWaiting, KindRetry, KindDownloadFail, KindDownloadOK, KindCached, KindBgfetchRefused, KindBgfetchFrozen:
			u := acc(ev.Unit)
			u.apply(ev)
		case KindStorage:
			s.Storage = &models.MHSStepStorage{At: ev.At, UsedMB: ev.UsedMB, QuotaMB: ev.QuotaMB, Persisted: ev.Persisted}
		case KindLoaderOK, KindLoaderFail, KindUnityOK, KindUnityFail, KindWatchdogWarn, KindWatchdogFail, KindRendering:
			if len(s.Launch) >= maxSummaryLaunch {
				continue
			}
			le := models.MHSStepLaunchEvent{At: ev.At, Kind: string(ev.Kind), Unit: launchUnit(ev), Seconds: ev.Seconds, Source: ev.Source, Phase: ev.Phase, Reason: models.ClipRunes(ev.Reason, 200)}
			s.Launch = append(s.Launch, le)
		case KindCrash:
			if len(s.Crashes) >= maxSummaryCrashes {
				continue
			}
			s.Crashes = append(s.Crashes, models.MHSStepCrash{At: ev.At, Type: ev.CrashType, Phase: ev.Phase, Unit: launchUnit(ev), Notice: ev.Notice, Message: models.ClipRunes(ev.Message, 200)})
		}
	}

	sort.Strings(order)
	for _, unit := range order {
		u := units[unit]
		u.finish()
		s.Units = append(s.Units, u.MHSStepUnitSummary)
	}
	return s
}

func (u *unitAcc) touch(ev Event) {
	if ev.At.IsZero() {
		return
	}
	if u.FirstAt.IsZero() || ev.At.Before(u.FirstAt) {
		u.FirstAt = ev.At
	}
	if ev.At.After(u.LastAt) {
		u.LastAt = ev.At
	}
}

func (u *unitAcc) apply(ev Event) {
	u.touch(ev)
	if ev.Title != "" && u.Title == "" {
		u.Title = ev.Title
	}
	if ev.Version != "" {
		u.Version = ev.Version
	}
	switch ev.Kind {
	case KindRequesting:
		u.Requested = true
		if ev.TotalMB > u.TotalMB {
			u.TotalMB = ev.TotalMB
		}
	case KindProgress:
		u.Progress++
		if ev.TotalMB > u.TotalMB {
			u.TotalMB = ev.TotalMB
		}
		if ev.MB > u.MaxMB {
			u.MaxMB = ev.MB
		}
		u.progress = append(u.progress, mbAt{ev.MB, ev.At})
	case KindResumed:
		if ev.Drops > u.Drops {
			u.Drops = ev.Drops
		}
		if ev.MB > u.MaxMB {
			u.MaxMB = ev.MB
		}
		u.resumes = append(u.resumes, mbAt{ev.MB, ev.At})
	case KindNoData:
		u.Stalls++
		u.signals = append(u.signals, ev.At)
	case KindWaiting:
		// Background-fetch silence before a switch: a stall signal too.
		u.signals = append(u.signals, ev.At)
	case KindRetry:
		u.Retries++
		u.signals = append(u.signals, ev.At)
		if ev.ErrorClass != "" {
			u.LastErrorClass = ev.ErrorClass
		}
		if ev.Path != "" {
			u.LastPath = ev.Path
		}
	case KindDownloadFail:
		u.Fails++
		if ev.ErrorClass != "" {
			u.LastErrorClass = ev.ErrorClass
		}
		u.LastErrorText = models.ClipRunes(strings.TrimSpace(ev.Message), 160)
		if ev.Path != "" {
			u.LastPath = ev.Path
		}
	case KindDownloadOK:
		if !u.Complete || (u.CompleteAt != nil && ev.At.Before(*u.CompleteAt)) {
			at := ev.At
			u.Complete = true
			u.CompleteAt = &at
			u.CompleteMB = ev.MB
			u.CompleteSeconds = int(ev.Seconds + 0.5)
			u.CompleteMBps = ev.MBps
		}
		if ev.MB > u.TotalMB {
			u.TotalMB = ev.MB
		}
	case KindCached:
		u.Cached = true
	case KindBgfetchRefused:
		u.BgfetchRefused = true
	case KindBgfetchFrozen:
		u.BgfetchFrozen = true
		if ev.MB > u.MaxMB {
			u.MaxMB = ev.MB
		}
	}
}

// finish decides whether the download was held near its end: progress
// reached the tail (the last HoldTailMB of the file) and then the connection
// kept dropping and picking up at the same point, or stalled, instead of
// finishing. A tail that arrives after the usual pause of the last part is
// not a hold; one resumed connection that completes at once is not either.
func (u *unitAcc) finish() {
	if u.Cached || u.TotalMB <= 0 {
		return
	}
	tailStart := u.TotalMB - HoldTailMB
	var reachedAt time.Time
	holdMB := 0
	for _, p := range u.progress {
		if p.mb >= tailStart && p.mb < u.TotalMB {
			reachedAt, holdMB = p.at, p.mb
			break
		}
	}
	tailResumes := 0
	for _, r := range u.resumes {
		if r.mb < tailStart {
			continue
		}
		tailResumes++
		if tailResumes == 1 {
			// The pick-up point is where the file stopped.
			holdMB = r.mb
			if reachedAt.IsZero() || r.at.Before(reachedAt) {
				reachedAt = r.at
			}
		}
	}
	if reachedAt.IsZero() {
		return // the download never reached the tail
	}
	signals := 0
	for _, at := range u.signals {
		if at.Before(reachedAt) {
			continue
		}
		if u.Complete && u.CompleteAt != nil && !at.Before(*u.CompleteAt) {
			continue
		}
		signals++
	}
	held := tailResumes >= 2 || (tailResumes == 1 && signals > 0) || (!u.Complete && signals > 0)
	if !held {
		return
	}
	if u.Complete && u.CompleteAt != nil {
		gap := u.CompleteAt.Sub(reachedAt)
		if gap < holdMinSeconds*time.Second {
			return
		}
		u.HoldSeconds = int(gap / time.Second)
	}
	u.Held = true
	u.HeldAtMB = holdMB
	at := reachedAt
	u.HeldAt = &at
}
