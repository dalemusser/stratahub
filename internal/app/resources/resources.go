// internal/app/resources/resources.go
package resources

import (
	"embed"
	"io/fs"
	"net/http"
	"sync"

	"github.com/dalemusser/waffle/pantry/assets"
	"github.com/dalemusser/waffle/pantry/templates"
)

// Embed the shared template files and configuration files.
//
//go:embed templates/*.gohtml mhs_progress_points.json
var FS embed.FS

// Embed assets (CSS, JS) for bundling into the binary.
//
//go:embed assets/css/*.css assets/js/*.js assets/mhs/*
var AssetsFS embed.FS

var (
	tailwindVersion = assets.ContentHash(AssetsFS, "assets/css/tailwind.css")
	tiptapVersion   = assets.ContentHash(AssetsFS, "assets/css/tiptap.css")
	htmxVersion     = assets.ContentHash(AssetsFS, "assets/js/htmx.min.js")
)

func init() {
	templates.RegisterFunc("tailwindVersion", func() string { return tailwindVersion })
	templates.RegisterFunc("tiptapVersion", func() string { return tiptapVersion })
	templates.RegisterFunc("htmxVersion", func() string { return htmxVersion })
}

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
