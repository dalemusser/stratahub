// internal/app/bootstrap/shutdown.go
package bootstrap

import (
	"context"

	"github.com/dalemusser/waffle/config"
	"go.uber.org/zap"
)

// Shutdown is an optional hook invoked during WAFFLE's shutdown phase.
//
// This function is called after the HTTP server has stopped accepting new
// requests and existing requests have been drained (or the shutdown timeout
// has elapsed). It is your opportunity to gracefully clean up resources.
//
// The context provided has a timeout (default 10 seconds) and should be
// respected—if cleanup takes too long, the context will be cancelled.
//
// Common uses for Shutdown:
//   - Close database connections
//   - Flush pending writes to external services
//   - Stop background workers gracefully
//   - Close message queue channels
//   - Release file handles or network connections
//
// If an error is returned, it will be logged but won't prevent the process
// from exiting. However, returning nil on success helps ensure clean shutdown
// behavior and accurate logging.
//
// StrataHub disconnects the MongoDB client to release connection pool resources.
func Shutdown(ctx context.Context, coreCfg *config.CoreConfig, appCfg AppConfig, deps DBDeps, logger *zap.Logger) error {
	if deps.StrataHubMongoClient != nil {
		logger.Info("disconnecting StrataHub MongoDB client")
		if err := deps.StrataHubMongoClient.Disconnect(ctx); err != nil {
			logger.Error("MongoDB disconnect failed", zap.Error(err))
			return err
		}
	}
	return nil
}
