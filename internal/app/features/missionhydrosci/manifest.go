// internal/app/features/missionhydrosci/manifest.go
package missionhydrosci

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"

	"github.com/dalemusser/waffle/pantry/templates"
)

// pwaManifest is the PWA web app manifest structure.
type pwaManifest struct {
	Name            string    `json:"name"`
	ShortName       string    `json:"short_name"`
	Description     string    `json:"description"`
	StartURL        string    `json:"start_url"`
	Scope           string    `json:"scope"`
	Display         string    `json:"display"`
	BackgroundColor string    `json:"background_color"`
	ThemeColor      string    `json:"theme_color"`
	Icons           []pwaIcon `json:"icons"`
}

type pwaIcon struct {
	Src   string `json:"src"`
	Sizes string `json:"sizes"`
	Type  string `json:"type"`
}

// manifestData is the canonical manifest, built once and reused by
// both the HTTP handler and the content-hash template function.
var manifestData = pwaManifest{
	Name:            "Mission HydroSci",
	ShortName:       "MHS",
	Description:     "Mission HydroSci - Science Adventure Game",
	StartURL:        "/missionhydrosci/units",
	Scope:           "/",
	Display:         "standalone",
	BackgroundColor: "#1e3a5f",
	ThemeColor:      "#1e3a5f",
	Icons: []pwaIcon{
		{Src: "/assets/mhs/icon-192.png", Sizes: "192x192", Type: "image/png"},
		{Src: "/assets/mhs/icon-512.png", Sizes: "512x512", Type: "image/png"},
	},
}

func init() {
	data, _ := json.Marshal(manifestData)
	h := sha256.Sum256(data)
	version := hex.EncodeToString(h[:5]) // 10 hex chars, matches assets.ContentHash
	templates.RegisterFunc("missionhydrosciManifestVersion", func() string { return version })
}

// ServeManifest serves the PWA manifest.json.
func (h *Handler) ServeManifest(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/manifest+json")
	json.NewEncoder(w).Encode(manifestData)
}
