// internal/app/features/groups/templates.go
package groups

import (
	"embed"

	"github.com/dalemusser/waffle/templates"
)

//go:embed templates/*.gohtml
var FS embed.FS

func init() {
	templates.Register(templates.Set{
		Name:     "groups",
		FS:       FS,
		Patterns: []string{"templates/*.gohtml"},
	})
}
