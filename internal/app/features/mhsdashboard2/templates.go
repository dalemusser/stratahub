// internal/app/features/mhsdashboard2/templates.go
package mhsdashboard2

import (
	"embed"

	"github.com/dalemusser/waffle/pantry/templates"
)

//go:embed templates/*.gohtml
var FS embed.FS

func init() {
	templates.Register(templates.Set{
		Name:     "mhsdashboard2",
		FS:       FS,
		Patterns: []string{"templates/*.gohtml"},
	})
}
