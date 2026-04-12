// internal/app/features/missionhydrosci/play.go
package missionhydrosci

import (
	"net/http"

	"github.com/dalemusser/stratahub/internal/app/system/auth"
	"github.com/dalemusser/stratahub/internal/app/system/viewdata"
	"github.com/dalemusser/stratahub/internal/app/system/workspace"
	"github.com/dalemusser/waffle/pantry/templates"
	"github.com/go-chi/chi/v5"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.uber.org/zap"
)

// ServePlay renders the game launcher page for a specific unit.
func (h *Handler) ServePlay(w http.ResponseWriter, r *http.Request) {
	unitID := chi.URLParam(r, "unit")

	manifest, _ := h.resolveManifest(r)
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

	// Get current user for identity bridge and progress gate
	var userName, userLoginID string
	user, authenticated := auth.CurrentUser(r)
	if authenticated {
		userName = user.Name
		userLoginID = user.LoginID

		// Progress gate: members can only access their current or completed units.
		// Non-members (admin, coordinator, leader) can access any unit for observation/testing.
		if user.Role == "member" {
			wsID := workspace.IDFromRequest(r)
			userOID, err := primitive.ObjectIDFromHex(user.ID)
			if err == nil {
				progress, err := h.ProgressStore.GetOrCreate(r.Context(), wsID, userOID, user.LoginID)
				if err != nil {
					h.Log.Error("failed to check progress for unit gate", zap.Error(err))
					http.Error(w, "internal error", http.StatusInternalServerError)
					return
				}

				// Allow access to the current unit and any completed unit
				allowed := unitID == progress.CurrentUnit
				if !allowed {
					for _, cu := range progress.CompletedUnits {
						if cu == unitID {
							allowed = true
							break
						}
					}
				}

				if !allowed {
					// Redirect to the units page instead of showing a 403
					http.Redirect(w, r, "/missionhydrosci/units", http.StatusSeeOther)
					return
				}
			}
		}
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
		LogSubmitURL:    h.Services.LogSubmitURL,
		LogAuth:         h.Services.LogAuth,
		StateSaveURL:    h.Services.StateSaveURL,
		StateLoadURL:    h.Services.StateLoadURL,
		SettingsSaveURL: h.Services.SettingsSaveURL,
		SettingsLoadURL: h.Services.SettingsLoadURL,
		SaveAuth:        h.Services.SaveAuth,
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
