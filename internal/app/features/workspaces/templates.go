// internal/app/features/workspaces/templates.go
package workspaces

import (
	"embed"

	"github.com/dalemusser/waffle/pantry/templates"
)

//go:embed templates/*.gohtml
var FS embed.FS

func init() {
	templates.Register(templates.Set{
		Name:     "workspaces",
		FS:       FS,
		Patterns: []string{"templates/*.gohtml"},
	})
}
