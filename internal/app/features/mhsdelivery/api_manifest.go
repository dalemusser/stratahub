// internal/app/features/mhsdelivery/api_manifest.go
package mhsdelivery

import (
	"embed"
	"encoding/json"
	"net/http"
	"sync"

	"go.uber.org/zap"
)

//go:embed mhs_content_manifest.json
var staticFS embed.FS

var (
	cachedManifest *ContentManifest
	manifestOnce   sync.Once
)

// loadContentManifest loads and caches the content manifest from the embedded JSON.
func (h *Handler) loadContentManifest() ContentManifest {
	manifestOnce.Do(func() {
		data, err := staticFS.ReadFile("mhs_content_manifest.json")
		if err != nil {
			h.Log.Error("failed to read content manifest", zap.Error(err))
			cachedManifest = &ContentManifest{}
			return
		}

		var m ContentManifest
		if err := json.Unmarshal(data, &m); err != nil {
			h.Log.Error("failed to parse content manifest", zap.Error(err))
			cachedManifest = &ContentManifest{}
			return
		}

		// Override CDN base URL from config
		m.CDNBaseURL = h.CDNBaseURL
		cachedManifest = &m
	})

	// Always apply current CDN URL (in case config differs from cached)
	result := *cachedManifest
	result.CDNBaseURL = h.CDNBaseURL
	return result
}

// ServeContentManifest returns the content manifest as JSON.
func (h *Handler) ServeContentManifest(w http.ResponseWriter, r *http.Request) {
	manifest := h.loadContentManifest()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(manifest)
}
