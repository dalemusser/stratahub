// internal/app/features/home/views/views.go
package home

import (
	"embed"

	"github.com/dalemusser/waffle/templates"
)

//go:embed templates/*.gohtml
var FS embed.FS

func init() {
	templates.Register(templates.Set{
		Name:     "home",
		FS:       FS,
		Patterns: []string{"templates/*.gohtml"},
	})
}
