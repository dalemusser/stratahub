package organizations

import (
	"embed"

	"github.com/dalemusser/waffle/templates"
)

//go:embed templates/*.gohtml
var FS embed.FS

func init() {
	templates.Register(templates.Set{
		Name:     "organizations",
		FS:       FS,
		Patterns: []string{"templates/*.gohtml"},
	})
}
