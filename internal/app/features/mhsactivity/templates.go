// internal/app/features/mhsactivity/templates.go
package mhsactivity

import (
	"embed"

	"github.com/dalemusser/waffle/pantry/templates"
)

//go:embed templates/*.gohtml
var FS embed.FS

func init() {
	templates.Register(templates.Set{
		Name:     "mhsactivity",
		FS:       FS,
		Patterns: []string{"templates/*.gohtml"},
	})
}
