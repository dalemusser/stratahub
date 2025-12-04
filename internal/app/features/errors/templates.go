// internal/app/features/errors/templates.go
package errors

import (
	"embed"

	"github.com/dalemusser/waffle/templates"
)

//go:embed templates/*.gohtml
var FS embed.FS

func init() {
	templates.Register(templates.Set{
		Name:     "errors",
		FS:       FS,
		Patterns: []string{"templates/*.gohtml"},
	})
}
