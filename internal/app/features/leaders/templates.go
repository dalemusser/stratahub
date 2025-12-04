// internal/app/features/leaders/templates.go
package leaders

import (
	"embed"

	"github.com/dalemusser/waffle/templates"
)

//go:embed templates/*.gohtml
var FS embed.FS

func init() {
	templates.Register(templates.Set{
		Name:     "admin_leaders",
		FS:       FS,
		Patterns: []string{"templates/*.gohtml"},
	})
}
