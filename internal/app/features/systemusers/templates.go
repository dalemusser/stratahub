// internal/app/features/systemusers/templates.go
package systemusers

import (
	"embed"

	"github.com/dalemusser/waffle/templates"
)

//go:embed templates/*.gohtml
var FS embed.FS

func init() {
	templates.Register(templates.Set{
		Name:     "system_users",
		FS:       FS,
		Patterns: []string{"templates/*.gohtml"},
	})
}
