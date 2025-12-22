// internal/app/bootstrap/db.go
package bootstrap

import (
	"context"

	"github.com/dalemusser/stratahub/internal/app/system/indexes"
	"github.com/dalemusser/stratahub/internal/app/system/validators"
	"github.com/dalemusser/waffle/config"
	wafflemongo "github.com/dalemusser/waffle/pantry/mongo"
	"go.uber.org/zap"
)

// ConnectDB connects to databases or other backends.
//
// WAFFLE calls this after configuration is loaded but before EnsureSchema and
// Startup. This is the place to establish connections to:
//   - Databases (MongoDB, PostgreSQL, MySQL, SQLite, etc.)
//   - Caches (Redis, Memcached)
//   - Message queues (RabbitMQ, Kafka)
//   - External services that require persistent connections
//
// Best practices:
//   - Use coreCfg.DBConnectTimeout to set connection timeouts
//   - Log connection attempts and successes for debugging
//   - Return descriptive errors if connections fail
//   - Store clients in the DBDeps struct for use in handlers
//
// StrataHub connects to MongoDB using the waffle/pantry/mongo helper,
// which handles connection pooling and ping verification.
func ConnectDB(ctx context.Context, coreCfg *config.CoreConfig, appCfg AppConfig, logger *zap.Logger) (DBDeps, error) {
	client, err := wafflemongo.Connect(ctx, appCfg.MongoURI, appCfg.MongoDatabase)
	if err != nil {
		return DBDeps{}, err
	}

	db := client.Database(appCfg.MongoDatabase)

	logger.Info("connected to MongoDB",
		zap.String("database", appCfg.MongoDatabase),
	)

	return DBDeps{
		StrataHubMongoClient:   client,
		StrataHubMongoDatabase: db,
	}, nil
}

// EnsureSchema sets up indexes or schema as needed.
//
// This runs after ConnectDB succeeds but before Startup and before the HTTP
// handler is built. It is optional—if you do not need indexes or migrations,
// you can leave this as a no-op that returns nil.
//
// This is the place to:
//   - Create database indexes for query performance
//   - Run schema migrations
//   - Validate that required collections/tables exist
//   - Set up initial data (seed data, default records)
//
// The context has a timeout based on coreCfg.IndexBootTimeout, so long-running
// migrations should respect context cancellation.
func EnsureSchema(ctx context.Context, coreCfg *config.CoreConfig, appCfg AppConfig, deps DBDeps, logger *zap.Logger) error {
	db := deps.StrataHubMongoDatabase

	// Ensure collections exist and attach JSON-Schema validators.
	// This runs first so indexes can be created on existing collections.
	logger.Info("ensuring collections and validators")
	if err := validators.EnsureAll(ctx, db); err != nil {
		logger.Error("failed to ensure validators", zap.Error(err))
		return err
	}

	// Ensure database indexes for query performance.
	logger.Info("ensuring database indexes")
	if err := indexes.EnsureAll(ctx, db); err != nil {
		logger.Error("failed to ensure indexes", zap.Error(err))
		return err
	}

	logger.Info("database schema ensured successfully")
	return nil
}
