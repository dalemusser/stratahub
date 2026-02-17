// internal/app/features/mhsdelivery/content.go
package mhsdelivery

import (
	"net/http"
	"strings"
)

// ContentFallback returns an http.Handler that redirects /mhs/content/* requests
// to the CDN. When the service worker is active it intercepts these requests and
// serves from cache, so this fallback is only hit without a service worker.
func (h *Handler) ContentFallback() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Extract the path after /mhs/content/
		path := r.URL.Path
		prefix := "/mhs/content/"
		if !strings.HasPrefix(path, prefix) {
			http.NotFound(w, r)
			return
		}
		relPath := strings.TrimPrefix(path, prefix)
		target := h.CDNBaseURL + "/" + relPath
		http.Redirect(w, r, target, http.StatusFound)
	})
}
