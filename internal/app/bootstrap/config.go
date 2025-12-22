// internal/app/bootstrap/config.go
package bootstrap

import (
	"fmt"

	"github.com/dalemusser/waffle/config"
	wafflemongo "github.com/dalemusser/waffle/pantry/mongo"
	"go.uber.org/zap"
)

// appConfigKeys defines the configuration keys for StrataHub.
// These are loaded via WAFFLE's config system with support for:
//   - Config files: mongo_uri, session_name, etc.
//   - Environment variables: STRATAHUB_MONGO_URI, STRATAHUB_SESSION_NAME, etc.
//   - Command-line flags: --mongo_uri, --session_name, etc.
var appConfigKeys = []config.AppKey{
	{Name: "mongo_uri", Default: "mongodb://localhost:27017", Desc: "MongoDB connection URI"},
	{Name: "mongo_database", Default: "strata_hub", Desc: "MongoDB database name"},
	{Name: "session_key", Default: "dev-only-change-me-please-0123456789ABCDEF", Desc: "Session signing key (must be strong in production)"},
	{Name: "session_name", Default: "stratahub-session", Desc: "Session cookie name"},
	{Name: "session_domain", Default: "", Desc: "Session cookie domain (blank means current host)"},
}

// LoadConfig loads WAFFLE core config and app-specific config.
//
// It is called early in startup so that both WAFFLE and the app have
// access to configuration before any backends or handlers are built.
// CoreConfig comes from the shared WAFFLE layer; AppConfig is specific
// to this app and can be extended as the app grows.
//
// WAFFLE's config.LoadWithAppConfig handles:
//   - Loading from .env files
//   - Loading from config.yaml/json/toml files
//   - Reading environment variables (WAFFLE_* for core, STRATAHUB_* for app)
//   - Parsing command-line flags
//   - Merging with precedence: flags > env > files > defaults
func LoadConfig(logger *zap.Logger) (*config.CoreConfig, AppConfig, error) {
	coreCfg, appValues, err := config.LoadWithAppConfig(logger, "STRATAHUB", appConfigKeys)
	if err != nil {
		return nil, AppConfig{}, err
	}

	appCfg := AppConfig{
		MongoURI:      appValues.String("mongo_uri"),
		MongoDatabase: appValues.String("mongo_database"),
		SessionKey:    appValues.String("session_key"),
		SessionName:   appValues.String("session_name"),
		SessionDomain: appValues.String("session_domain"),
	}

	return coreCfg, appCfg, nil
}

// ValidateConfig performs app-specific config validation.
//
// Return nil to accept the loaded config, or an error to abort startup.
// This is the right place to enforce required fields or invariants that
// involve both the core and app configs.
//
// StrataHub validates the MongoDB URI format to catch configuration
// errors early, before attempting to connect.
func ValidateConfig(coreCfg *config.CoreConfig, appCfg AppConfig, logger *zap.Logger) error {
	if err := wafflemongo.ValidateURI(appCfg.MongoURI); err != nil {
		logger.Error("invalid MongoDB URI", zap.Error(err))
		return fmt.Errorf("invalid MongoDB URI: %w", err)
	}
	// Add any additional app-specific validation here.
	return nil
}
