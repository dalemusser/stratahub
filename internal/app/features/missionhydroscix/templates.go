// internal/app/features/missionhydroscix/templates.go
package missionhydroscix

import (
	"embed"

	"github.com/dalemusser/waffle/pantry/templates"
)

//go:embed templates/*.gohtml
var FS embed.FS

func init() {
	templates.Register(templates.Set{
		Name:     "missionhydroscix",
		FS:       FS,
		Patterns: []string{"templates/*.gohtml"},
	})
}
