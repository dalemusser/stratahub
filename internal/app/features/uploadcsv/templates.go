// internal/app/features/uploadcsv/templates.go
package uploadcsv

import (
	"embed"

	"github.com/dalemusser/waffle/pantry/templates"
)

//go:embed templates/*.gohtml
var FS embed.FS

func init() {
	templates.Register(templates.Set{
		Name:     "uploadcsv",
		FS:       FS,
		Patterns: []string{"templates/*.gohtml"},
	})
}
