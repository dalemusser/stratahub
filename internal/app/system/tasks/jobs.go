// internal/app/system/tasks/jobs.go
package tasks

import (
	"context"
	"time"

	"github.com/dalemusser/stratahub/internal/app/store/oauthstate"
	"github.com/dalemusser/stratahub/internal/app/store/sessions"
	"go.uber.org/zap"
)

// InactiveSessionCleanupJob creates a job that closes sessions inactive for the given threshold.
// Unlike session expiration (which deletes), this marks sessions as ended for audit purposes.
func InactiveSessionCleanupJob(sessStore *sessions.Store, logger *zap.Logger, threshold time.Duration) Job {
	return Job{
		Name:     "inactive-session-cleanup",
		Interval: 1 * time.Minute, // Check every minute
		Run: func(ctx context.Context) error {
			count, err := sessStore.CloseInactiveSessions(ctx, threshold)
			if err != nil {
				return err
			}
			if count > 0 {
				logger.Info("closed inactive sessions",
					zap.Int64("count", count),
					zap.Duration("threshold", threshold))
			}
			return nil
		},
	}
}

// OAuthStateCleanupJob creates a job that removes expired OAuth state tokens.
// This is a backup for when MongoDB's TTL index cleanup is delayed.
func OAuthStateCleanupJob(stateStore *oauthstate.Store, logger *zap.Logger) Job {
	return Job{
		Name:     "oauth-state-cleanup",
		Interval: 1 * time.Hour, // Run hourly
		Run: func(ctx context.Context) error {
			count, err := stateStore.CleanupExpired(ctx)
			if err != nil {
				return err
			}
			if count > 0 {
				logger.Debug("cleaned up expired OAuth states", zap.Int64("count", count))
			}
			return nil
		},
	}
}
