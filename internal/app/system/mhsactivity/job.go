package mhsactivity

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/dalemusser/stratahub/internal/app/store/mhsactivitydays"
	"github.com/dalemusser/stratahub/internal/app/store/workspaces"
	"github.com/dalemusser/stratahub/internal/app/system/tasks"
	"github.com/dalemusser/stratahub/internal/app/system/timeouts"
	"github.com/dalemusser/stratahub/internal/domain/models"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.uber.org/zap"
)

// maxPartialAttempts is how many times a closed day whose computation had a
// source missing is retried before it is stored as final anyway (with the
// partial flag, which the page shows).
const maxPartialAttempts = 12

// EffectiveCloseAfter is the configured close-after, never below MinCloseAfter.
func (d Deps) EffectiveCloseAfter() time.Duration { return d.closeAfter() }

// RollupResult says what RollupDay did.
type RollupResult struct {
	Skipped bool // every organization of the zone already had a final, current mark
	Docs    int
	Partial bool
	Missing []string
}

// RollupDay computes one zone-day, stores its documents and writes a mark
// per organization. Without force, a zone whose organizations all carry a
// final mark of the current version is skipped. A partial result (a source
// was unavailable) is stored but its marks stay non-final until
// maxPartialAttempts, so the job keeps retrying it.
func RollupDay(ctx context.Context, deps Deps, store *mhsactivitydays.Store, wsID primitive.ObjectID, day string, zone Zone, force bool) (RollupResult, error) {
	var res RollupResult
	marks, err := store.GetMarks(ctx, wsID, day, day, zone.OrgIDs)
	if err != nil {
		return res, err
	}
	if !force {
		allFinal := true
		for _, org := range zone.OrgIDs {
			m, ok := marks[mhsactivitydays.MarkKey(org, day)]
			if !ok || !m.Final || m.Version != models.MHSActivityVersion {
				allFinal = false
				break
			}
		}
		if allFinal {
			res.Skipped = true
			return res, nil
		}
	}
	docs, err := ComputeDay(ctx, deps, wsID, day, zone)
	if err != nil {
		return res, err
	}
	res.Docs = len(docs)
	if len(docs) > 0 {
		res.Partial = docs[0].Partial
		res.Missing = docs[0].Missing
	}
	if err := store.ReplaceDay(ctx, wsID, day, zone.OrgIDs, docs); err != nil {
		return res, err
	}
	now := time.Now().UTC()
	perOrg := map[primitive.ObjectID]int{}
	for _, d := range docs {
		if d.Role == models.MHSActivityRoleMember {
			perOrg[d.OrganizationID]++
		}
	}
	for _, org := range zone.OrgIDs {
		attempts := 1
		if m, ok := marks[mhsactivitydays.MarkKey(org, day)]; ok && !force {
			attempts = m.Attempts + 1
		}
		mark := models.MHSActivityMark{
			WorkspaceID:    wsID,
			OrganizationID: org,
			Day:            day,
			TZ:             zone.TZ,
			Final:          !res.Partial || attempts >= maxPartialAttempts,
			Partial:        res.Partial,
			Missing:        res.Missing,
			Attempts:       attempts,
			Students:       perOrg[org],
			ComputedAt:     now,
			Version:        models.MHSActivityVersion,
		}
		if err := store.SetMark(ctx, mark); err != nil {
			return res, err
		}
	}
	return res, nil
}

// JobConfig shapes the rollup job.
type JobConfig struct {
	Interval    time.Duration // how often the job looks for closed days (default 15 min)
	CatchUpDays int           // how many days back it looks (default 3)
	Now         func() time.Time
}

// RollupJob returns the scheduled job that rolls up closed days: for every
// workspace and time zone, the last CatchUpDays local days that are closed
// and not yet final. Days further back are the backfill's business.
func RollupJob(deps Deps, store *mhsactivitydays.Store, wsStore *workspacestore.Store, cfg JobConfig) tasks.Job {
	if cfg.Interval <= 0 {
		cfg.Interval = 15 * time.Minute
	}
	if cfg.CatchUpDays <= 0 {
		cfg.CatchUpDays = 3
	}
	if cfg.Now == nil {
		cfg.Now = func() time.Time { return time.Now().UTC() }
	}
	return tasks.Job{
		Name:     "mhs-activity-rollup",
		Interval: cfg.Interval,
		Run: func(ctx context.Context) error {
			return RunRollup(ctx, deps, store, wsStore, cfg)
		},
	}
}

// RunRollup is one pass of the rollup job.
func RunRollup(ctx context.Context, deps Deps, store *mhsactivitydays.Store, wsStore *workspacestore.Store, cfg JobConfig) error {
	log := deps.logger()
	now := cfg.Now()
	wss, err := wsStore.Find(ctx, bson.M{})
	if err != nil {
		return err
	}
	var errs []error
	for _, ws := range wss {
		zones, err := ZonesFor(ctx, deps.DB, ws.ID, nil)
		if err != nil {
			errs = append(errs, fmt.Errorf("%s zones: %w", ws.Subdomain, err))
			continue
		}
		for _, z := range zones {
			today := LocalDay(now, z.Loc)
			for back := cfg.CatchUpDays; back >= 1; back-- {
				day, err := AddDays(today, -back)
				if err != nil {
					continue
				}
				if !Closed(day, z.Loc, deps.EffectiveCloseAfter(), now) {
					continue
				}
				dctx, cancel := context.WithTimeout(ctx, timeouts.Batch())
				res, err := RollupDay(dctx, deps, store, ws.ID, day, z, false)
				cancel()
				if err != nil {
					errs = append(errs, fmt.Errorf("%s %s %s: %w", ws.Subdomain, z.TZ, day, err))
					log.Warn("mhs activity rollup failed", zap.String("workspace", ws.Subdomain), zap.String("zone", z.TZ), zap.String("day", day), zap.Error(err))
					continue
				}
				if !res.Skipped {
					log.Info("mhs activity rollup", zap.String("workspace", ws.Subdomain), zap.String("zone", z.TZ), zap.String("day", day),
						zap.Int("docs", res.Docs), zap.Bool("partial", res.Partial), zap.Strings("missing", res.Missing))
				}
			}
		}
	}
	return errors.Join(errs...)
}
