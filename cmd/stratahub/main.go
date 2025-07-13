// cmd/stratahub/main.go
package main

import (
	"context"
	"log"
	"os"

	"github.com/dalemusser/gowebcore/auth"
	"github.com/dalemusser/gowebcore/config"
	"github.com/dalemusser/gowebcore/db"
	"github.com/dalemusser/gowebcore/logger"
	"github.com/dalemusser/gowebcore/server"
	"github.com/go-chi/chi/v5"

	"github.com/dalemusser/stratahub/internal/platform/handler"
	"github.com/dalemusser/stratahub/internal/routes"
)

/*
---------------------------------------------------------------------------
Application-specific configuration additions
---------------------------------------------------------------------------
config.Base already gives you:

  - Addr        string   (":8080" etc.)
  - EnableTLS   bool     (true → Let’s Encrypt or cert / key)
  - LogLevel    string   ("info", "debug" …)

We embed it and extend with the fields StrataHub needs.
*/
type AppCfg struct {
	config.Base

	MongoURI string `mapstructure:"mongo_uri"`
	MongoDB  string `mapstructure:"mongo_db"`

	// Two secrets for the secure-cookie session (required by gowebcore/auth)
	SessionHashKey  string `mapstructure:"session_hash_key"`  // 32–64 chars
	SessionBlockKey string `mapstructure:"session_block_key"` // 16, 24, 32 chars
}

func main() {
	//----------------------------------------------------------------------
	// 1. Load configuration (flags → env → file → defaults).
	//----------------------------------------------------------------------
	var cfg AppCfg
	if err := config.Load(&cfg, config.WithEnvPrefix("STRATA")); err != nil {
		log.Fatalf("config load: %v", err)
	}

	//----------------------------------------------------------------------
	// 2. Initialise structured slog logger at the configured level.
	//----------------------------------------------------------------------
	logger.Init(cfg.LogLevel)

	//----------------------------------------------------------------------
	// 3. Build shared infrastructure: DB manager and auth session.
	//----------------------------------------------------------------------
	mgr := db.NewManager()
	// Example: _ = mgr.Register("primary", cfg.MongoURI, cfg.MongoDB)

	// auth.NewSession(hashKey, blockKey) – both are []byte
	sess := auth.NewSession(
		[]byte(cfg.SessionHashKey),
		[]byte(cfg.SessionBlockKey),
	)

	h := handler.New(&cfg.Base, mgr, sess)

	//----------------------------------------------------------------------
	// 4. Compose the router and mount every feature slice.
	//----------------------------------------------------------------------
	r := chi.NewRouter()
	routes.RegisterAll(r, h)

	//----------------------------------------------------------------------
	// 5. Start the HTTP(S) server.  For Let’s Encrypt pass empty cert / key.
	//----------------------------------------------------------------------
	srv := server.New(cfg.Base, r)

	ctx := context.Background()
	if err := server.Serve(ctx, srv, "", ""); err != nil && err != context.Canceled {
		log.Printf("server stopped: %v", err)
		os.Exit(1)
	}
}
