package main

import (
	"context"
	"log"

	"github.com/dalemusser/stratahub/internal/app/bootstrap"
	"github.com/dalemusser/stratahub/internal/app/system/viewdata"
	"github.com/dalemusser/waffle/app"
)

// BuildTime is set at compile time via -ldflags.
// Example: go build -ldflags "-X main.BuildTime=20260323-184500"
var BuildTime = "dev"

// main is the entry point for a WAFFLE application.
//
// WAFFLE owns the full lifecycle of the service. This includes:
//   - loading configuration (core + app)
//   - establishing database connections
//   - running optional schema/index initialization
//   - executing any app-defined Startup initialization
//   - constructing the HTTP handler
//   - running the HTTP/HTTPS server with graceful shutdown
//
// The bootstrap.Hooks value wires this application into WAFFLE's lifecycle.
// app.Run executes the lifecycle in the correct order, blocking until the
// service shuts down. Any error is considered fatal and terminates the process.
func main() {
	viewdata.SetBuildTime(BuildTime)
	if err := app.Run(context.Background(), bootstrap.Hooks); err != nil {
		log.Fatal(err)
	}
}
