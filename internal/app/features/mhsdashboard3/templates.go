// internal/app/features/mhsdashboard3/templates.go
package mhsdashboard3

import (
	"embed"

	"github.com/dalemusser/waffle/pantry/templates"
)

//go:embed templates/*.gohtml
var FS embed.FS

func init() {
	templates.Register(templates.Set{
		Name:     "mhsdashboard3",
		FS:       FS,
		Patterns: []string{"templates/*.gohtml"},
	})
}
