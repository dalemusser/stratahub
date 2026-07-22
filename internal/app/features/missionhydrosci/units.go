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
	var currentUnit string
	var completedUnits []string
	var isComplete bool
	wsID := workspace.IDFromRequest(r)

	if user, ok := auth.CurrentUser(r); ok {
		userID, err := primitive.ObjectIDFromHex(user.ID)
		if err != nil {
			h.ErrLog.LogServerError(w, r, "invalid user id on MHS units page", err, "Something went wrong loading Mission HydroSci.", "/dashboard")
			return
		}
		// GetOrCreate returns a record on success (defaulting a new user to
		// unit1). A non-nil error is a real DB failure — surface it rather than
		// rendering the page as a brand-new user on unit1, which would show a
		// mid-course student the wrong current unit and could gate them out of
		// their real one.
		progress, err := h.ProgressStore.GetOrCreate(r.Context(), wsID, userID)
		if err != nil {
			h.ErrLog.LogServerError(w, r, "load MHS progress failed", err, "Couldn't load your Mission HydroSci progress. Please try again.", "/dashboard")
			return
		}
		currentUnit = progress.CurrentUnit
		completedUnits = progress.CompletedUnits
		isComplete = progress.CurrentUnit == "complete"
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

	// Resolve effective collection info for the read-only version line.
	collInfo := h.resolveEffectiveCollectionInfo(r)

	data := UnitsData{
		BaseVM:               viewdata.LoadBase(r, h.DB),
		Units:                units,
		CurrentUnit:          currentUnit,
		IsComplete:           isComplete,
		NextUnitID:           nextUnitID,
		CollectionOverride:   collInfo.IsOverride,
		ActiveCollectionName: collInfo.Name,
	}
	data.Title = "Mission HydroSci"

	templates.Render(w, r, "missionhydrosci_units", data)
}

