// internal/app/features/mhsbuilds/templates.go
package mhsbuilds

import (
	"embed"

	"github.com/dalemusser/waffle/pantry/templates"
)

//go:embed templates/*.gohtml
var FS embed.FS

func init() {
	templates.Register(templates.Set{
		Name:     "mhsbuilds",
		FS:       FS,
		Patterns: []string{"templates/*.gohtml"},
	})
}
