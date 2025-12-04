// internal/app/bootstrap/hooks.go
package bootstrap

import (
	"github.com/dalemusser/waffle/app"
)

// Hooks wires the app into WAFFLE's lifecycle.
var Hooks = app.Hooks[AppConfig, DBDeps]{
	Name:           "stratahub",
	LoadConfig:     LoadConfig,
	ValidateConfig: ValidateConfig,
	ConnectDB:      ConnectDB,
	EnsureSchema:   EnsureSchema,
	Startup:        Startup,
	BuildHandler:   BuildHandler,
	Shutdown:       Shutdown,
}
