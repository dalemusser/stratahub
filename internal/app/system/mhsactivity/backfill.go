package mhsactivity

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/dalemusser/stratahub/internal/app/store/mhsactivitydays"
	"github.com/dalemusser/stratahub/internal/app/store/mhsdevicetests"
	"github.com/dalemusser/stratahub/internal/app/system/mhssteplog"
	"github.com/dalemusser/stratahub/internal/app/system/timeouts"
	"github.com/dalemusser/stratahub/internal/domain/models"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.uber.org/zap"
)

// ErrBackfillRunning is returned when a backfill is already running.
var ErrBackfillRunning = errors.New("mhsactivity: a backfill is already running")

// BackfillRequest asks for a range of closed days to be (re)computed and
// stored, oldest first.
type BackfillRequest struct {
	WorkspaceID primitive.ObjectID
	From, To    string // inclusive day strings
	Stamp       bool   // also stamp summaries onto load records that lack one
	Force       bool   // recompute days that are already final
}

// BackfillStatus is the progress of the current or last backfill.
type BackfillStatus struct {
	Running    bool
	Request    BackfillRequest
	StartedAt  time.Time
	FinishedAt *time.Time
	Total      int // zone-days in the range
	Done       int
	Open       int // not closed yet, left alone
	Skipped    int // already final (without force)
	Written    int // documents stored
	Stamped    int // summaries stamped
	Current    string
	Errors     []string
	Stopped    bool
}

// Backfiller runs one backfill at a time in the background.
type Backfiller struct {
	deps  Deps
	store *mhsactivitydays.Store
	tests *mhsdevicetests.Store
	Now   func() time.Time // the clock that decides which days are closed (tests)

	mu     sync.Mutex
	status BackfillStatus
	cancel context.CancelFunc
}

// NewBackfiller wires a backfiller.
func NewBackfiller(deps Deps, store *mhsactivitydays.Store, tests *mhsdevicetests.Store) *Backfiller {
	return &Backfiller{deps: deps, store: store, tests: tests, Now: func() time.Time { return time.Now().UTC() }}
}

// Status returns a copy of the progress.
func (b *Backfiller) Status() BackfillStatus {
	b.mu.Lock()
	defer b.mu.Unlock()
	st := b.status
	st.Errors = append([]string(nil), st.Errors...)
	return st
}

// Start begins a backfill, or returns ErrBackfillRunning.
func (b *Backfiller) Start(req BackfillRequest) error {
	days, err := DaysBetween(req.From, req.To)
	if err != nil {
		return err
	}
	b.mu.Lock()
	if b.status.Running {
		b.mu.Unlock()
		return ErrBackfillRunning
	}
	ctx, cancel := context.WithCancel(context.Background())
	b.cancel = cancel
	b.status = BackfillStatus{Running: true, Request: req, StartedAt: time.Now().UTC()}
	b.mu.Unlock()
	go b.run(ctx, req, days)
	return nil
}

// Stop cancels the running backfill, if any.
func (b *Backfiller) Stop() {
	b.mu.Lock()
	cancel := b.cancel
	b.mu.Unlock()
	if cancel != nil {
		cancel()
	}
}

func (b *Backfiller) update(f func(st *BackfillStatus)) {
	b.mu.Lock()
	f(&b.status)
	b.mu.Unlock()
}

func (b *Backfiller) run(ctx context.Context, req BackfillRequest, days []string) {
	log := b.deps.logger()
	finish := func() {
		now := time.Now().UTC()
		b.update(func(st *BackfillStatus) {
			st.Running = false
			st.FinishedAt = &now
			st.Current = ""
		})
	}
	defer finish()
	zones, err := ZonesFor(ctx, b.deps.DB, req.WorkspaceID, nil)
	if err != nil {
		b.update(func(st *BackfillStatus) { st.Errors = append(st.Errors, "zones: "+err.Error()) })
		return
	}
	b.update(func(st *BackfillStatus) { st.Total = len(days) * len(zones) })
	now := b.Now()
	for _, day := range days {
		for _, z := range zones {
			if ctx.Err() != nil {
				b.update(func(st *BackfillStatus) { st.Stopped = true })
				return
			}
			b.update(func(st *BackfillStatus) { st.Current = day + " " + z.TZ })
			if !Closed(day, z.Loc, b.deps.EffectiveCloseAfter(), now) {
				b.update(func(st *BackfillStatus) { st.Done++; st.Open++ })
				continue
			}
			dctx, cancel := context.WithTimeout(ctx, timeouts.Batch())
			if req.Stamp {
				n, err := b.stamp(dctx, req.WorkspaceID, day, z)
				if err != nil {
					b.addError(fmt.Sprintf("%s %s stamp: %v", day, z.TZ, err))
				}
				b.update(func(st *BackfillStatus) { st.Stamped += n })
			}
			res, err := RollupDay(dctx, b.deps, b.store, req.WorkspaceID, day, z, req.Force)
			cancel()
			if err != nil {
				b.addError(fmt.Sprintf("%s %s: %v", day, z.TZ, err))
				log.Warn("mhs activity backfill failed", zap.String("day", day), zap.String("zone", z.TZ), zap.Error(err))
			}
			b.update(func(st *BackfillStatus) {
				st.Done++
				if res.Skipped {
					st.Skipped++
				}
				st.Written += res.Docs
			})
		}
	}
}

func (b *Backfiller) addError(msg string) {
	b.update(func(st *BackfillStatus) {
		if len(st.Errors) < 20 {
			st.Errors = append(st.Errors, msg)
		}
	})
}

// stamp writes a summary onto every load record of the zone-day that has
// none (or an older one), so later computations never read steps again.
func (b *Backfiller) stamp(ctx context.Context, wsID primitive.ObjectID, day string, zone Zone) (int, error) {
	start, end, err := DayBounds(day, zone.Loc)
	if err != nil {
		return 0, err
	}
	recs, err := b.tests.ListMemberBetween(ctx, wsID, start, end.Add(b.deps.EffectiveCloseAfter()))
	if err != nil {
		return 0, err
	}
	var ids []primitive.ObjectID
	byID := map[primitive.ObjectID]models.MHSDeviceTest{}
	for _, rec := range recs {
		if rec.Summary != nil && rec.Summary.V == models.MHSMemberSummaryVersion {
			continue
		}
		ids = append(ids, rec.ID)
		byID[rec.ID] = rec
	}
	if len(ids) == 0 {
		return 0, nil
	}
	steps, err := b.tests.StepsByIDs(ctx, wsID, ids)
	if err != nil {
		return 0, err
	}
	n := 0
	for _, id := range ids {
		rec := byID[id]
		sum := mhssteplog.Summarize(steps[id], outcomeOf(rec), mhssteplog.Context{UnitID: rec.UnitID, DeviceID: rec.DeviceID})
		if err := b.tests.SetSummary(ctx, wsID, id, sum); err != nil {
			return n, err
		}
		n++
	}
	return n, nil
}
