// internal/app/bootstrap/config.go
package bootstrap

import (
	"context"
	"fmt"

	"github.com/dalemusser/stratahub/internal/app/system/auth"
	"github.com/dalemusser/waffle/config"
	"github.com/dalemusser/waffle/toolkit/db/mongodb"
	"github.com/spf13/viper"
	"go.uber.org/zap"
)

// LoadConfig loads WAFFLE core config and app-specific config.
func LoadConfig(logger *zap.Logger) (*config.CoreConfig, AppConfig, error) {
	coreCfg, err := config.Load(logger)
	if err != nil {
		return nil, AppConfig{}, err
	}

	// App-specific defaults for StrataHub
	viper.SetDefault("stratahub_mongo_uri", "mongodb://localhost:27017")
	viper.SetDefault("stratahub_mongo_database", "strata_hub")

	// Session defaults: in dev, these can be overridden via env or config file.
	// In production, you MUST override session_key with a strong secret.
	viper.SetDefault("stratahub_session_key", "dev-only-change-me-please-0123456789ABCDEF")
	viper.SetDefault("stratahub_session_domain", "") // blank means current host

	appCfg := AppConfig{
		Greeting:               "Hello from WAFFLE!",
		StrataHubMongoURI:      viper.GetString("stratahub_mongo_uri"),
		StrataHubMongoDatabase: viper.GetString("stratahub_mongo_database"),
		SessionKey:             viper.GetString("stratahub_session_key"),
		SessionDomain:          viper.GetString("stratahub_session_domain"),
	}

	logger.Info("app config loaded",
		zap.String("stratahub_mongo_uri", appCfg.StrataHubMongoURI),
		zap.String("stratahub_mongo_database", appCfg.StrataHubMongoDatabase),
		zap.String("session_domain", appCfg.SessionDomain),
	)

	// Initialize the session store using app config + core env
	secure := coreCfg.Env == "prod"
	if err := auth.InitSessionStore(appCfg.SessionKey, appCfg.SessionDomain, secure, logger); err != nil {
		logger.Error("session store init failed", zap.Error(err))
		return nil, AppConfig{}, err
	}

	return coreCfg, appCfg, nil
}

func ValidateConfig(coreCfg *config.CoreConfig, appCfg AppConfig, logger *zap.Logger) error {
	if err := mongodb.ValidateURI(appCfg.StrataHubMongoURI); err != nil {
		logger.Error("invalid StrataHub Mongo URI", zap.Error(err))
		return fmt.Errorf("invalid StrataHub Mongo URI: %w", err)
	}
	// Add any additional app-specific validation here later.
	return nil
}

// ConnectDB connects to databases or other backends.
func ConnectDB(ctx context.Context, coreCfg *config.CoreConfig, appCfg AppConfig, logger *zap.Logger) (DBDeps, error) {
	logger.Info("Connecting to MongoDB...",
		zap.String("uri", appCfg.StrataHubMongoURI),
		zap.String("database", appCfg.StrataHubMongoDatabase),
	)

	client, err := mongodb.Connect(ctx, appCfg.StrataHubMongoURI, appCfg.StrataHubMongoDatabase)

	if err != nil {
		logger.Error("MongoDB connection failed", zap.Error(err))
		return DBDeps{}, err
	}

	deps := DBDeps{
		StrataHubMongoClient:   client,
		StrataHubMongoDatabase: client.Database(appCfg.StrataHubMongoDatabase),
	}

	logger.Info("MongoDB connected",
		zap.String("database", appCfg.StrataHubMongoDatabase),
	)

	return deps, nil
}
