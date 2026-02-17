// internal/app/features/mhsdelivery/sw.go
package mhsdelivery

import (
	"embed"
	"io"
	"net/http"
)

//go:embed static/sw.js static/sw-cache.js static/sw-background-fetch.js
var swFS embed.FS

// ServeServiceWorker concatenates the service worker JS files and serves
// them as a single /sw.js response. The file must be served from the root
// path so the service worker can control the entire origin.
func (h *Handler) ServeServiceWorker(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/javascript")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Service-Worker-Allowed", "/")

	files := []string{
		"static/sw-cache.js",
		"static/sw-background-fetch.js",
		"static/sw.js",
	}

	for _, name := range files {
		f, err := swFS.Open(name)
		if err != nil {
			h.Log.Error("failed to open SW file: " + name)
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		io.Copy(w, f)
		f.Close()
		w.Write([]byte("\n"))
	}
}
