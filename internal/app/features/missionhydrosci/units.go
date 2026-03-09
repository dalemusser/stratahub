// internal/app/features/missionhydrosci/units.go
package missionhydrosci

import (
	"fmt"
	"net/http"

	"github.com/dalemusser/stratahub/internal/app/system/auth"
	"github.com/dalemusser/stratahub/internal/app/system/viewdata"
	"github.com/dalemusser/stratahub/internal/app/system/workspace"
	"github.com/dalemusser/waffle/pantry/templates"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

// ServeUnits renders the single-launch unit page with progress tracking.
func (h *Handler) ServeUnits(w http.ResponseWriter, r *http.Request) {
	manifest := h.loadContentManifest()

	// Load user progress
	var currentUnit, userLoginID string
	var completedUnits []string
	var isComplete bool

	if user, ok := auth.CurrentUser(r); ok {
		userLoginID = user.LoginID
		wsID := workspace.IDFromRequest(r)
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
			ID:        u.ID,
			Title:     u.Title,
			Version:   u.Version,
			TotalSize: u.TotalSize,
			SizeLabel: formatBytes(u.TotalSize),
			Status:    status,
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

	data := UnitsData{
		BaseVM:         viewdata.LoadBase(r, h.DB),
		Units:          units,
		CDNBaseURL:     h.CDNBaseURL,
		CurrentUnit:    currentUnit,
		CompletedUnits: completedUnits,
		UserLoginID:    userLoginID,
		IsComplete:     isComplete,
		NextUnitID:     nextUnitID,
	}
	data.Title = "Mission HydroSci"

	templates.Render(w, r, "missionhydrosci_units", data)
}

// formatBytes converts bytes to a human-readable string.
func formatBytes(b int64) string {
	const mb = 1024 * 1024
	const gb = 1024 * mb

	switch {
	case b >= gb:
		return fmt.Sprintf("%.1f GB", float64(b)/float64(gb))
	case b >= mb:
		return fmt.Sprintf("%.1f MB", float64(b)/float64(mb))
	default:
		return fmt.Sprintf("%.1f KB", float64(b)/1024)
	}
}
