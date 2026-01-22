// internal/app/features/mhsdashboard/templates.go
package mhsdashboard

import (
	"embed"

	"github.com/dalemusser/waffle/pantry/templates"
)

//go:embed templates/*.gohtml
var FS embed.FS

func init() {
	templates.Register(templates.Set{
		Name:     "mhsdashboard",
		FS:       FS,
		Patterns: []string{"templates/*.gohtml"},
	})
}
