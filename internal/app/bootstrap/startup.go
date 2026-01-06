// internal/app/bootstrap/startup.go
package bootstrap

import (
	"context"
	"time"

	"github.com/dalemusser/stratahub/internal/app/resources"
	"github.com/dalemusser/stratahub/internal/app/store/activity"
	"github.com/dalemusser/stratahub/internal/app/store/audit"
	"github.com/dalemusser/stratahub/internal/app/store/emailverify"
	"github.com/dalemusser/stratahub/internal/app/store/sessions"
	"github.com/dalemusser/stratahub/internal/app/system/workers"
	"github.com/dalemusser/waffle/config"
	"go.uber.org/zap"
)

// Startup runs once after DB connections and schema/index setup are complete,
// but before the HTTP handler is built and requests are served.
//
// This is the place for one-time initialization that depends on having live
// database connections and fully loaded configuration. Unlike ConnectDB and
// EnsureSchema which focus on infrastructure, Startup is for application-level
// initialization.
//
// Common uses for Startup:
//   - Load shared templates from the resources directory
//   - Warm caches with frequently accessed data
//   - Initialize in-memory lookup tables
//   - Validate external service connectivity
//   - Set up background workers or scheduled tasks
//   - Perform health checks on dependencies
//
// Returning a non-nil error will abort startup and prevent the server from
// starting. Returning nil signals that initialization succeeded.
//
// The context will be cancelled if the process is asked to shut down while
// Startup is running; honor it in any long-running work.
//
// StrataHub uses Startup to load shared templates (layouts, navigation, etc.)
// that are used across all features.
func Startup(ctx context.Context, coreCfg *config.CoreConfig, appCfg AppConfig, deps DBDeps, logger *zap.Logger) error {
	resources.LoadSharedTemplates()

	// Ensure indexes for email verification store
	emailVerifyStore := emailverify.New(deps.StrataHubMongoDatabase, appCfg.EmailVerifyExpiry)
	if err := emailVerifyStore.EnsureIndexes(ctx); err != nil {
		logger.Error("failed to ensure email verify indexes", zap.Error(err))
		return err
	}

	// Ensure indexes for audit store
	auditStore := audit.New(deps.StrataHubMongoDatabase)
	if err := auditStore.EnsureIndexes(ctx); err != nil {
		logger.Error("failed to ensure audit indexes", zap.Error(err))
		return err
	}

	// Ensure indexes for sessions store (activity tracking)
	sessionsStore := sessions.New(deps.StrataHubMongoDatabase)
	if err := sessionsStore.EnsureIndexes(ctx); err != nil {
		logger.Error("failed to ensure sessions indexes", zap.Error(err))
		return err
	}

	// Ensure indexes for activity events store
	activityStore := activity.New(deps.StrataHubMongoDatabase)
	if err := activityStore.EnsureIndexes(ctx); err != nil {
		logger.Error("failed to ensure activity indexes", zap.Error(err))
		return err
	}

	// Start session cleanup background worker
	// Runs every minute, closes sessions inactive for more than 10 minutes
	deps.SessionCleanupWorker = workers.NewSessionCleanup(
		sessionsStore,
		logger,
		1*time.Minute,  // check every minute
		10*time.Minute, // close sessions inactive for 10+ minutes
	)
	deps.SessionCleanupWorker.Start()

	return nil
}
