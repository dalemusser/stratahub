// internal/app/features/shared/views/views.go
package shared

import (
	"embed"

	"github.com/dalemusser/waffle/templates"
)

// Embed the shared template files.
//
//go:embed templates/*.gohtml
var FS embed.FS

func init() {
	templates.Register(templates.Set{
		Name:     "shared",
		FS:       FS,
		Patterns: []string{"templates/*.gohtml"},
	})
}
