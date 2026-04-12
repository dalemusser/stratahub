// internal/app/features/missionhydrosci/units.go
package missionhydrosci

import (
	"net/http"

	"github.com/dalemusser/stratahub/internal/app/system/auth"
	"github.com/dalemusser/stratahub/internal/app/system/format"
	"github.com/dalemusser/stratahub/internal/app/system/viewdata"
	"github.com/dalemusser/stratahub/internal/app/system/workspace"
	"github.com/dalemusser/waffle/pantry/templates"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

// ServeUnits renders the single-launch unit page with progress tracking.
func (h *Handler) ServeUnits(w http.ResponseWriter, r *http.Request) {
	manifest, _ := h.resolveManifest(r)

	// Load user progress
	var currentUnit, userLoginID string
	var completedUnits []string
	var isComplete bool
	wsID := workspace.IDFromRequest(r)

	if user, ok := auth.CurrentUser(r); ok {
		userLoginID = user.LoginID
		if userID, err := primitive.ObjectIDFromHex(user.ID); err == nil {
			progress, err := h.ProgressStore.GetOrCreate(r.Context(), wsID, userID, user.LoginID)
			if err == nil {
				currentUnit = progress.CurrentUnit
				completedUnits = progress.CompletedUnits
				isComplete = progress.CurrentUnit == "complete"
			}
		}
	}
	if currentUnit == "" {
		currentUnit = "unit1"
	}

	// Build completed set for quick lookup
	completedSet := make(map[string]bool, len(completedUnits))
	for _, u := range completedUnits {
		completedSet[u] = true
	}

	units := make([]UnitVM, len(manifest.Units))
	for i, u := range manifest.Units {
		var status string
		switch {
		case completedSet[u.ID]:
			status = "completed"
		case u.ID == currentUnit:
			status = "current"
		default:
			status = "future"
		}

		units[i] = UnitVM{
			ID:              u.ID,
			Title:           u.Title,
			Version:         u.Version,
			BuildIdentifier: u.BuildIdentifier,
			TotalSize:       u.TotalSize,
			SizeLabel:       format.Bytes(u.TotalSize),
			Status:          status,
		}
	}

	if completedUnits == nil {
		completedUnits = []string{}
	}

	// Compute next unit ID
	var nextUnitID string
	if !isComplete {
		for i, u := range manifest.Units {
			if u.ID == currentUnit && i+1 < len(manifest.Units) {
				nextUnitID = manifest.Units[i+1].ID
				break
			}
		}
	}

	// Load workspace MHS member auth setting
	mhsMemberAuth := "staffauth"
	if settings, err := h.SettingsStore.Get(r.Context(), wsID); err == nil {
		mhsMemberAuth = settings.GetMHSMemberAuth()
	}

	// Resolve effective collection info
	collInfo := h.resolveEffectiveCollectionInfo(r)

	data := UnitsData{
		BaseVM:                 viewdata.LoadBase(r, h.DB),
		Units:                  units,
		CDNBaseURL:             h.CDNBaseURL,
		CurrentUnit:            currentUnit,
		CompletedUnits:         completedUnits,
		UserLoginID:            userLoginID,
		IsComplete:             isComplete,
		NextUnitID:             nextUnitID,
		MHSMemberAuth:          mhsMemberAuth,
		CollectionOverride:     collInfo.IsOverride,
		CollectionOverrideName: collInfo.Name,
		ActiveCollectionName:   collInfo.Name,
		ActiveCollectionID:     collInfo.ID,
		ActiveCollectionDesc:   collInfo.Description,
	}
	data.Title = "Mission HydroSci"

	templates.Render(w, r, "missionhydrosci_units", data)
}

