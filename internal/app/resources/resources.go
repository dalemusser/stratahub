// internal/app/resources/resources.go
package resources

import (
	"embed"
	"sync"

	"github.com/dalemusser/waffle/pantry/templates"
)

// Embed the shared template files.
//
//go:embed templates/*.gohtml
var FS embed.FS

var registerOnce sync.Once

func LoadSharedTemplates() {
	registerOnce.Do(func() {
		templates.Register(templates.Set{
			Name:     "shared",
			FS:       FS,
			Patterns: []string{"templates/*.gohtml"},
		})
	})
}
