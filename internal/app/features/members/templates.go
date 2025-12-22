// internal/app/features/members/templates.go
package members

import (
	"embed"

	"github.com/dalemusser/waffle/pantry/templates"
)

//go:embed templates/*.gohtml
var FS embed.FS

func init() {
	templates.Register(templates.Set{
		Name:     "members",
		FS:       FS,
		Patterns: []string{"templates/*.gohtml"},
	})
}
