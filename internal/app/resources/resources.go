// internal/app/resources/resources.go
package resources

import (
	"embed"
	"io/fs"
	"net/http"
	"sync"

	"github.com/dalemusser/waffle/pantry/templates"
)

// Embed the shared template files and configuration files.
//
//go:embed templates/*.gohtml mhs_progress_points.json
var FS embed.FS

// Embed assets (CSS, JS) for bundling into the binary.
//
//go:embed assets/css/*.css assets/js/*.js
var AssetsFS embed.FS

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

// AssetsHandler returns an http.Handler that serves embedded asset files.
// The urlPrefix is stripped from the request path (e.g., "/assets").
// Files are served from the "assets" subdirectory of the embedded FS.
func AssetsHandler(urlPrefix string) http.Handler {
	subFS, err := fs.Sub(AssetsFS, "assets")
	if err != nil {
		panic("failed to get assets subdirectory: " + err.Error())
	}

	fileServer := http.FileServer(http.FS(subFS))
	return http.StripPrefix(urlPrefix, fileServer)
}
