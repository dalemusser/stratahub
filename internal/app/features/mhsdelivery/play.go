// internal/app/features/mhsdelivery/play.go
package mhsdelivery

import (
	"net/http"

	"github.com/dalemusser/stratahub/internal/app/system/viewdata"
	"github.com/dalemusser/waffle/pantry/templates"
	"github.com/go-chi/chi/v5"
)

// ServePlay renders the game launcher page for a specific unit.
func (h *Handler) ServePlay(w http.ResponseWriter, r *http.Request) {
	unitID := chi.URLParam(r, "unit")

	manifest := h.loadContentManifest()
	var unitTitle, unitVersion string
	for _, u := range manifest.Units {
		if u.ID == unitID {
			unitTitle = u.Title
			unitVersion = u.Version
			break
		}
	}

	if unitTitle == "" {
		http.NotFound(w, r)
		return
	}

	data := PlayData{
		BaseVM:      viewdata.LoadBase(r, h.DB),
		UnitID:      unitID,
		UnitTitle:   unitTitle,
		UnitVersion: unitVersion,
		CDNBaseURL:  h.CDNBaseURL,
	}
	data.Title = unitTitle

	templates.Render(w, r, "mhs_play", data)
}
