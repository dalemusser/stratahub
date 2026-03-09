// internal/app/features/missionhydrosci/play.go
package missionhydrosci

import (
	"net/http"

	"github.com/dalemusser/stratahub/internal/app/system/auth"
	"github.com/dalemusser/stratahub/internal/app/system/viewdata"
	"github.com/dalemusser/waffle/pantry/templates"
	"github.com/go-chi/chi/v5"
)

// ServePlay renders the game launcher page for a specific unit.
func (h *Handler) ServePlay(w http.ResponseWriter, r *http.Request) {
	unitID := chi.URLParam(r, "unit")

	manifest := h.loadContentManifest()
	var unitTitle, unitVersion string
	var nextUnitID, nextUnitVersion string
	var dataFile, frameworkFile, codeFile string
	for i, u := range manifest.Units {
		if u.ID == unitID {
			unitTitle = u.Title
			unitVersion = u.Version
			dataFile = u.DataFile
			frameworkFile = u.FrameworkFile
			codeFile = u.CodeFile
			if i+1 < len(manifest.Units) {
				nextUnitID = manifest.Units[i+1].ID
				nextUnitVersion = manifest.Units[i+1].Version
			}
			break
		}
	}

	if unitTitle == "" {
		http.NotFound(w, r)
		return
	}

	// Get current user for identity bridge (Unity jslib calls /api/user)
	var userName, userLoginID string
	if user, ok := auth.CurrentUser(r); ok {
		userName = user.Name
		userLoginID = user.LoginID
	}

	// Prevent iOS PWA from HTTP-caching the play page HTML.
	// The service worker handles offline caching separately.
	w.Header().Set("Cache-Control", "no-cache, no-store")

	data := PlayData{
		BaseVM:          viewdata.LoadBase(r, h.DB),
		UnitID:          unitID,
		UnitTitle:       unitTitle,
		UnitVersion:     unitVersion,
		CDNBaseURL:      h.CDNBaseURL,
		UserName:        userName,
		UserLoginID:     userLoginID,
		NextUnitID:      nextUnitID,
		NextUnitVersion: nextUnitVersion,
		DataFile:        dataFile,
		FrameworkFile:   frameworkFile,
		CodeFile:        codeFile,
	}
	data.Title = unitTitle

	templates.Render(w, r, "missionhydrosci_play", data)
}

// RedirectToPlay handles game-initiated unit transitions that bypass PWA mode.
// When MHSBridge navigates to ../unit2/index.html (URL mode), the browser resolves
// it to /missionhydrosci/unit2/index.html. This redirects to /missionhydrosci/play/unit2.
func (h *Handler) RedirectToPlay(w http.ResponseWriter, r *http.Request) {
	unitID := chi.URLParam(r, "unit")
	target := "/missionhydrosci/play/" + unitID
	if q := r.URL.RawQuery; q != "" {
		target += "?" + q
	}
	http.Redirect(w, r, target, http.StatusFound)
}
