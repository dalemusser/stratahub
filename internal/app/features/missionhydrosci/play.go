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
)

// ServePlay renders the game launcher page for a specific unit.
func (h *Handler) ServePlay(w http.ResponseWriter, r *http.Request) {
	unitID := chi.URLParam(r, "unit")

	manifest, _ := h.resolveManifest(r)
	var unit *ContentManifestUnit
	var nextUnitID, nextUnitVersion string
	for i := range manifest.Units {
		if manifest.Units[i].ID == unitID {
			unit = &manifest.Units[i]
			if i+1 < len(manifest.Units) {
				nextUnitID = manifest.Units[i+1].ID
				nextUnitVersion = manifest.Units[i+1].Version
			}
			break
		}
	}

	if unit == nil || unit.Title == "" {
		http.NotFound(w, r)
		return
	}

	// Get current user for identity bridge and progress gate
	var userName, userIDHex string
	user, authenticated := auth.CurrentUser(r)
	if authenticated {
		userName = user.Name
		userIDHex = user.ID

		// Progress gate: members can only access their current or completed units.
		// Non-members (admin, coordinator, leader) can access any unit for observation/testing.
		if user.Role == "member" {
			wsID := workspace.IDFromRequest(r)
			userOID, err := primitive.ObjectIDFromHex(user.ID)
			if err == nil {
				progress, err := h.ProgressStore.GetOrCreate(r.Context(), wsID, userOID)
				if err != nil {
					h.ErrLog.LogServerError(w, r, "load MHS progress failed (play gate)", err, "Couldn't load Mission HydroSci. Please try again.", "/missionhydrosci/units")
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

	h.renderPlay(w, r, playRender{
		Unit:            *unit,
		NextUnitID:      nextUnitID,
		NextUnitVersion: nextUnitVersion,
		UserName:        userName,
		UserIDHex:       userIDHex,
		BackURL:         "/missionhydrosci/units",
	})
}

// playRender is what renderPlay needs beyond the request: the manifest unit
// to run, the identity to hand the game, and (for the device test) the
// run's ids and endpoints.
type playRender struct {
	Unit            ContentManifestUnit
	NextUnitID      string
	NextUnitVersion string
	UserName        string
	UserIDHex       string
	BackURL         string
	DeviceTest      *deviceTestPlay // nil for the normal launcher
}

// deviceTestPlay carries the device-test run into the play template.
type deviceTestPlay struct {
	ID      string // 24-hex test id = game user_id
	ShortID string
	Base    string // "/missionhydrosci/devicetest/run/<id>"
}

// renderPlay renders the Unity host page. The game-service URLs and keys are
// rendered here for every launch, student or device test alike.
func (h *Handler) renderPlay(w http.ResponseWriter, r *http.Request, p playRender) {
	// Prevent iOS PWA from HTTP-caching the play page HTML.
	// The service worker handles offline caching separately.
	w.Header().Set("Cache-Control", "no-cache, no-store")

	data := PlayData{
		BaseVM:          viewdata.LoadBase(r, h.DB),
		UnitID:          p.Unit.ID,
		UnitTitle:       p.Unit.Title,
		UnitVersion:     p.Unit.Version,
		CDNBaseURL:      h.CDNBaseURL,
		UserName:        p.UserName,
		UserIDHex:       p.UserIDHex,
		NextUnitID:      p.NextUnitID,
		NextUnitVersion: p.NextUnitVersion,
		DataFile:        p.Unit.DataFile,
		FrameworkFile:   p.Unit.FrameworkFile,
		CodeFile:        p.Unit.CodeFile,
		LogSubmitURL:    h.Services.LogSubmitURL,
		LogAuth:         h.Services.LogAuth,
		StateSaveURL:    h.Services.StateSaveURL,
		StateLoadURL:    h.Services.StateLoadURL,
		SettingsSaveURL: h.Services.SettingsSaveURL,
		SettingsLoadURL: h.Services.SettingsLoadURL,
		SaveAuth:        h.Services.SaveAuth,
		PlayBackURL:     p.BackURL,
	}
	if p.DeviceTest != nil {
		data.DeviceTest = true
		data.DeviceTestID = p.DeviceTest.ID
		data.DeviceTestShortID = p.DeviceTest.ShortID
		data.DeviceTestBase = p.DeviceTest.Base
	}
	data.Title = p.Unit.Title

	templates.Render(w, r, "missionhydrosci_play", data)
}

// RedirectToPlay handles game-initiated unit transitions in URL mode: the
// game navigates to ../unitN/index.html, which resolves here.
func (h *Handler) RedirectToPlay(w http.ResponseWriter, r *http.Request) {
	unitID := chi.URLParam(r, "unit")
	target := "/missionhydrosci/play/" + unitID
	if q := r.URL.RawQuery; q != "" {
		target += "?" + q
	}
	http.Redirect(w, r, target, http.StatusFound)
}
