// internal/app/bootstrap/shutdown.go
package bootstrap

import (
	"context"

	"github.com/dalemusser/waffle/config"
	"go.uber.org/zap"
)

// Shutdown cleanly tears down DB connections and other resources.
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
