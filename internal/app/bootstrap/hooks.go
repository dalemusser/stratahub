// internal/app/bootstrap/hooks.go
package bootstrap

import (
	"context"
	"fmt"
	"net/http"

	dashboardfeature "github.com/dalemusser/stratahub/internal/app/features/dashboard"
	_ "github.com/dalemusser/stratahub/internal/app/features/dashboard/views" // register dashboard templates

	aboutfeature "github.com/dalemusser/stratahub/internal/app/features/about"
	_ "github.com/dalemusser/stratahub/internal/app/features/about/views" // register about templates
	contactfeature "github.com/dalemusser/stratahub/internal/app/features/contact"
	_ "github.com/dalemusser/stratahub/internal/app/features/contact/views"
	healthfeature "github.com/dalemusser/stratahub/internal/app/features/health"
	homefeature "github.com/dalemusser/stratahub/internal/app/features/home"
	_ "github.com/dalemusser/stratahub/internal/app/features/home/views"   // register home templates
	_ "github.com/dalemusser/stratahub/internal/app/features/shared/views" // register shared templates
	termsfeature "github.com/dalemusser/stratahub/internal/app/features/terms"
	_ "github.com/dalemusser/stratahub/internal/app/features/terms/views" // register terms templates
	"github.com/dalemusser/stratahub/internal/app/system/auth"
	"github.com/dalemusser/waffle/app"
	"github.com/dalemusser/waffle/config"
	"github.com/dalemusser/waffle/templates"
	"github.com/dalemusser/waffle/toolkit/db/mongodb"
	"github.com/go-chi/chi/v5"
	"github.com/spf13/viper"
	"go.uber.org/zap"
)

// Hooks wires the app into WAFFLE's lifecycle.
var Hooks = app.Hooks[AppConfig, DBDeps]{
	Name:           "stratahub",
	LoadConfig:     LoadConfig,
	ValidateConfig: ValidateConfig,
	ConnectDB:      ConnectDB,
	EnsureSchema:   EnsureSchema,
	BuildHandler:   BuildHandler,
	Shutdown:       Shutdown,
}

// LoadConfig loads WAFFLE core config and app-specific config.
func LoadConfig(logger *zap.Logger) (*config.CoreConfig, AppConfig, error) {
	coreCfg, err := config.Load(logger)
	if err != nil {
		return nil, AppConfig{}, err
	}

	// App-specific defaults for StrataHub
	viper.SetDefault("stratahub_mongo_uri", "mongodb://localhost:27017")
	viper.SetDefault("stratahub_mongo_database", "strata_hub")

	appCfg := AppConfig{
		Greeting: "Hello from WAFFLE!",

		StrataHubMongoURI:      viper.GetString("stratahub_mongo_uri"),
		StrataHubMongoDatabase: viper.GetString("stratahub_mongo_database"),
	}

	logger.Info("app config loaded",
		zap.String("stratahub_mongo_uri", appCfg.StrataHubMongoURI),
		zap.String("stratahub_mongo_database", appCfg.StrataHubMongoDatabase),
	)

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

// EnsureSchema sets up indexes or schema as needed.
func EnsureSchema(ctx context.Context, coreCfg *config.CoreConfig, appCfg AppConfig, deps DBDeps, logger *zap.Logger) error {
	// TODO: create indexes, run migrations, etc.
	return nil
}

// BuildHandler constructs the HTTP handler for the service.
func BuildHandler(coreCfg *config.CoreConfig, appCfg AppConfig, deps DBDeps, logger *zap.Logger) (http.Handler, error) {
	// Initialize and boot the template engine once at startup.
	eng := templates.New(coreCfg.Env == "dev")
	if err := eng.Boot(logger); err != nil {
		logger.Error("template engine boot failed", zap.Error(err))
		return nil, err
	}
	templates.UseEngine(eng, logger)

	r := chi.NewRouter()

	// Global auth middleware: loads SessionUser into context if logged in.
	r.Use(auth.LoadSessionUser)

	// Health feature
	healthHandler := healthfeature.NewHandler(deps.StrataHubMongoClient, logger)
	r.Mount("/health", healthfeature.Routes(healthHandler))

	// Home feature
	homeHandler := homefeature.NewHandler(logger)
	r.Mount("/", homefeature.Routes(homeHandler))

	// About feature
	aboutHandler := aboutfeature.NewHandler(logger)
	r.Mount("/about", aboutfeature.Routes(aboutHandler))

	contactHandler := contactfeature.NewHandler(logger)
	r.Mount("/contact", contactfeature.Routes(contactHandler))

	// Terms
	termsHandler := termsfeature.NewHandler(logger)
	r.Mount("/terms", termsfeature.Routes(termsHandler))

	// Dashboard feature
	dashboardHandler := dashboardfeature.NewHandler(deps.StrataHubMongoDatabase, logger)
	r.Mount("/dashboard", dashboardfeature.Routes(dashboardHandler))

	return r, nil
}

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
