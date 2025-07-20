// cmd/stratahub/main.go
package main

import (
	"context"
	"os"

	"github.com/go-chi/chi/v5"

	"github.com/dalemusser/gowebcore/config"
	"github.com/dalemusser/gowebcore/db"
	"github.com/dalemusser/gowebcore/logger"
	"github.com/dalemusser/gowebcore/server"

	"github.com/dalemusser/stratahub/internal/platform/handler"
	"github.com/dalemusser/stratahub/internal/platform/session"
	"github.com/dalemusser/stratahub/internal/routes"
)

/*─────────────────────────────────────────────────────────────────────────────*
| Extended application configuration                                         *
*─────────────────────────────────────────────────────────────────────────────*/

type AppCfg struct {
	config.Base     `mapstructure:",squash"` // flattens Base keys / host, ports, TLS flags,
	MongoURI        string                   `mapstructure:"mongo_uri"`
	MongoDB         string                   `mapstructure:"mongo_db"`
	SessionHashKey  string                   `mapstructure:"session_hash_key"`  // ≥32 random chars
	SessionBlockKey string                   `mapstructure:"session_block_key"` // 16 / 24 / 32 chars
}

/*─────────────────────────────────────────────────────────────────────────────*
| Entry point                                                                 |
*─────────────────────────────────────────────────────────────────────────────*/

func main() {
	//----------------------------------------------------------------------
	// 0. Initialise logger with a safe default so we can log early errors.
	//----------------------------------------------------------------------
	logger.Init("info")

	//----------------------------------------------------------------------
	// 1. Load configuration (flags → env → YAML → defaults).
	//----------------------------------------------------------------------
	var cfg AppCfg
	if err := config.Load(
		&cfg,
		config.WithEnvPrefix("STRATA"),
		config.WithConfigFile("config.toml"),
	); err != nil {
		logger.Error("config load failed", "err", err)
		os.Exit(1)
	}

	//----------------------------------------------------------------------
	// 2. Re-initialise logger with the configured level.
	//----------------------------------------------------------------------
	logger.Init(cfg.LogLevel)

	logger.Info("cfg.LogLevel after Load", "lvl", cfg.LogLevel)
	//logger.Info("cfg.MongoURI  after Load", "uri", cfg.MongoURI)

	logger.Debug("effective Mongo URI", "uri", cfg.MongoURI)

	//----------------------------------------------------------------------
	// 3. Shared infrastructure (DB, sessions, handler).
	//----------------------------------------------------------------------
	dbMgr := db.NewManager()
	// Open a Mongo connection and cache it under the alias "primary".
	if err := dbMgr.Add("primary", cfg.MongoURI, cfg.MongoDB); err != nil {
		logger.Error("mongo connect failed", "err", err)
		os.Exit(1)
	}

	/*
		// adding another mongo database connection
		_ = dbMgr.Add("analytics", cfg.AnalyticsURI, "analytics_db")
	*/

	sessMgr := session.New(
		[]byte(cfg.SessionHashKey),
		[]byte(cfg.SessionBlockKey),
	)

	h := handler.New(&cfg.Base, dbMgr, sessMgr)

	//----------------------------------------------------------------------
	// 4. Compose router and mount all feature slices.
	//----------------------------------------------------------------------
	r := chi.NewRouter()
	routes.RegisterAll(r, h)

	//----------------------------------------------------------------------
	// 5. Build HTTP(S) server and serve with graceful shutdown.
	//----------------------------------------------------------------------
	srv := server.New(cfg.Base, r)

	ctx := context.Background()
	if err := server.Serve(ctx, srv, "", ""); err != nil && err != context.Canceled {
		logger.Error("server stopped", "err", err)
	}
}
