// internal/app/features/login/templates.go
package terms

import (
	"embed"

	"github.com/dalemusser/waffle/templates"
)

//go:embed templates/*.gohtml
var FS embed.FS

func init() {
	templates.Register(templates.Set{
		Name:     "terms",
		FS:       FS,
		Patterns: []string{"templates/*.gohtml"},
	})
}
