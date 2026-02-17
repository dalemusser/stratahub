// internal/app/features/mhsdelivery/units.go
package mhsdelivery

import (
	"fmt"
	"net/http"

	"github.com/dalemusser/stratahub/internal/app/system/viewdata"
	"github.com/dalemusser/waffle/pantry/templates"
)

// ServeUnits renders the unit selector page with download/play status.
func (h *Handler) ServeUnits(w http.ResponseWriter, r *http.Request) {
	manifest := h.loadContentManifest()

	units := make([]UnitVM, len(manifest.Units))
	for i, u := range manifest.Units {
		units[i] = UnitVM{
			ID:        u.ID,
			Title:     u.Title,
			TotalSize: u.TotalSize,
			SizeLabel: formatBytes(u.TotalSize),
		}
	}

	data := UnitsData{
		BaseVM:     viewdata.LoadBase(r, h.DB),
		Units:      units,
		CDNBaseURL: h.CDNBaseURL,
	}
	data.Title = "Mission HydroSci - Units"

	templates.Render(w, r, "mhs_units", data)
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
