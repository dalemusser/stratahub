// internal/app/bootstrap/startup.go
package bootstrap

import (
	"context"

	"github.com/dalemusser/stratahub/internal/app/resources"
	"github.com/dalemusser/waffle/config"
	"go.uber.org/zap"
)

// Startup runs one-time application initialization after DB connections and
// schema setup are complete, but before the HTTP handler is built. It is the
// place to load shared resources (like templates), warm caches, or perform
// any app-wide setup that depends on config and backends.
func Startup(ctx context.Context, coreCfg *config.CoreConfig, appCfg AppConfig, deps DBDeps, logger *zap.Logger) error {
	resources.LoadSharedTemplates()
	return nil
}
