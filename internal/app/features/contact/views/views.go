// internal/app/features/contact/views/views.go
package contact

import (
	"embed"

	"github.com/dalemusser/waffle/templates"
)

//go:embed templates/*.gohtml
var FS embed.FS

func init() {
	templates.Register(templates.Set{
		Name:     "contact",
		FS:       FS,
		Patterns: []string{"templates/*.gohtml"},
	})
}
