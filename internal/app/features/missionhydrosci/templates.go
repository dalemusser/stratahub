// internal/app/features/missionhydrosci/templates.go
package missionhydrosci

import (
	"embed"

	"github.com/dalemusser/waffle/pantry/templates"
)

//go:embed templates/*.gohtml
var FS embed.FS

func init() {
	templates.Register(templates.Set{
		Name:     "missionhydrosci",
		FS:       FS,
		Patterns: []string{"templates/*.gohtml"},
	})
}
