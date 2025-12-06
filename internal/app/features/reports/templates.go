// internal/app/features/reports/templates.go
package reports

import (
	"embed"

	"github.com/dalemusser/waffle/templates"
)

//go:embed templates/*.gohtml
var FS embed.FS

func init() {
	templates.Register(templates.Set{
		Name:     "reports",
		FS:       FS,
		Patterns: []string{"templates/*.gohtml"},
	})
}
