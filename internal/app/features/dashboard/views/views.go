// internal/app/features/dashboard/views/views.go
package dashboardviews

import (
	"embed"

	"github.com/dalemusser/waffle/templates"
)

//go:embed templates/*.gohtml
var FS embed.FS

func init() {
	templates.Register(templates.Set{
		Name:     "dashboard",
		FS:       FS,
		Patterns: []string{"templates/*.gohtml"},
	})
}
