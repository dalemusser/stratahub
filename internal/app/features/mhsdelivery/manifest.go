// internal/app/features/mhsdelivery/manifest.go
package mhsdelivery

import (
	"encoding/json"
	"net/http"
)

// pwaManifest is the PWA web app manifest structure.
type pwaManifest struct {
	Name            string        `json:"name"`
	ShortName       string        `json:"short_name"`
	Description     string        `json:"description"`
	StartURL        string        `json:"start_url"`
	Display         string        `json:"display"`
	BackgroundColor string        `json:"background_color"`
	ThemeColor      string        `json:"theme_color"`
	Icons           []pwaIcon     `json:"icons"`
}

type pwaIcon struct {
	Src   string `json:"src"`
	Sizes string `json:"sizes"`
	Type  string `json:"type"`
}

// ServeManifest serves the PWA manifest.json.
func (h *Handler) ServeManifest(w http.ResponseWriter, r *http.Request) {
	m := pwaManifest{
		Name:            "Mission HydroSci",
		ShortName:       "MHS",
		Description:     "Mission HydroSci - Science Adventure Game",
		StartURL:        "/mhs/units",
		Display:         "standalone",
		BackgroundColor: "#1e3a5f",
		ThemeColor:      "#1e3a5f",
		Icons: []pwaIcon{
			{Src: "/assets/mhs/icon-192.png", Sizes: "192x192", Type: "image/png"},
			{Src: "/assets/mhs/icon-512.png", Sizes: "512x512", Type: "image/png"},
		},
	}

	w.Header().Set("Content-Type", "application/manifest+json")
	json.NewEncoder(w).Encode(m)
}
