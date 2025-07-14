// cmd/stratahub/main.go
package main

import (
	"context"
	"log"
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
| Extended application configuration                                         |
*─────────────────────────────────────────────────────────────────────────────*/

type AppCfg struct {
	config.Base            // addr, TLS flags, log-level, …
	MongoURI        string `mapstructure:"mongo_uri"`
	MongoDB         string `mapstructure:"mongo_db"`
	SessionHashKey  string `mapstructure:"session_hash_key"`  // ≥32 rand chars
	SessionBlockKey string `mapstructure:"session_block_key"` // 16/24/32 chars
}

/*─────────────────────────────────────────────────────────────────────────────*
| Entry point                                                                 |
*─────────────────────────────────────────────────────────────────────────────*/

func main() {
	//----------------------------------------------------------------------
	// 1. Load configuration (flags → env → YAML → defaults)
	//----------------------------------------------------------------------
	var cfg AppCfg
	if err := config.Load(&cfg, config.WithEnvPrefix("STRATA")); err != nil {
		log.Fatalf("config load: %v", err)
	}

	//----------------------------------------------------------------------
	// 2. Initialise structured slog via gowebcore
	//----------------------------------------------------------------------
	logger.Init(cfg.LogLevel)

	//----------------------------------------------------------------------
	// 3. Shared infrastructure (DB, sessions, handler)
	//----------------------------------------------------------------------
	dbMgr := db.NewManager()
	// Example: _ = dbMgr.Register("primary", cfg.MongoURI, cfg.MongoDB)

	sessMgr := session.New(
		[]byte(cfg.SessionHashKey),
		[]byte(cfg.SessionBlockKey),
	)

	h := handler.New(&cfg.Base, dbMgr, sessMgr)

	//----------------------------------------------------------------------
	// 4. Compose router and mount all feature slices
	//----------------------------------------------------------------------
	r := chi.NewRouter()
	routes.RegisterAll(r, h)

	//----------------------------------------------------------------------
	// 5. Start HTTP(S) server   (Let’s Encrypt auto-handled when EnableTLS=true)
	//----------------------------------------------------------------------
	srv := server.New(cfg.Base, r)

	ctx := context.Background()
	if err := server.Serve(ctx, srv, "", ""); err != nil && err != context.Canceled {
		log.Printf("server stopped: %v", err)
		os.Exit(1)
	}
}
