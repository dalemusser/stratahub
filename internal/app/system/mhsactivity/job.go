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
	OutdatedPer int           // stored zone-days of an older version recomputed per pass (default 12)
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
	if cfg.OutdatedPer <= 0 {
		cfg.OutdatedPer = 24
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
		// Days stored by an earlier reading of the data: a few per pass,
		// oldest first, until none are left.
		if n, err := recomputeOutdated(ctx, deps, store, ws.ID, zones, cfg.OutdatedPer); err != nil {
			errs = append(errs, fmt.Errorf("%s outdated: %w", ws.Subdomain, err))
		} else if n > 0 {
			log.Info("mhs activity rollup: recomputed outdated days", zap.String("workspace", ws.Subdomain), zap.Int("zone_days", n))
		}
	}
	return errors.Join(errs...)
}

// recomputeOutdated rolls up again, at most limit zone-days, the days whose
// marks carry a version older than models.MHSActivityVersion.
func recomputeOutdated(ctx context.Context, deps Deps, store *mhsactivitydays.Store, wsID primitive.ObjectID, zones []Zone, limit int) (int, error) {
	marks, err := store.OutdatedMarks(ctx, wsID, models.MHSActivityVersion, limit*8)
	if err != nil || len(marks) == 0 {
		return 0, err
	}
	zoneOf := map[primitive.ObjectID]int{}
	for i, z := range zones {
		for _, org := range z.OrgIDs {
			zoneOf[org] = i
		}
	}
	type zd struct {
		zone int
		day  string
	}
	seen := map[zd]bool{}
	var todo []zd
	for _, m := range marks {
		zi, ok := zoneOf[m.OrganizationID]
		if !ok {
			// An organization that is gone: mark it current so it stops
			// coming back.
			m.Version = models.MHSActivityVersion
			_ = store.SetMark(ctx, m)
			continue
		}
		k := zd{zi, m.Day}
		if !seen[k] {
			seen[k] = true
			todo = append(todo, k)
		}
	}
	n := 0
	var errs []error
	for _, k := range todo {
		if n >= limit {
			break
		}
		dctx, cancel := context.WithTimeout(ctx, timeouts.Batch())
		_, err := RollupDay(dctx, deps, store, wsID, k.day, zones[k.zone], true)
		cancel()
		if err != nil {
			errs = append(errs, fmt.Errorf("%s %s: %w", zones[k.zone].TZ, k.day, err))
			continue
		}
		n++
	}
	return n, errors.Join(errs...)
}
