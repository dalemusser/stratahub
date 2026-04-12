// internal/app/features/mhsbuilds/manual.go
package mhsbuilds

import (
	"context"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/dalemusser/stratahub/internal/app/store/mhsbuilds"
	"github.com/dalemusser/stratahub/internal/app/system/auth"
	"github.com/dalemusser/stratahub/internal/app/system/viewdata"
	"github.com/dalemusser/stratahub/internal/domain/models"
	"github.com/dalemusser/waffle/pantry/templates"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

// defaultUnitIDs is the list of units to show in the manual form.
var defaultUnitIDs = []string{"unit1", "unit2", "unit3", "unit4", "unit5"}

// ServeManual renders the manual collection creation form.
func (h *Handler) ServeManual(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Minute)
	defer cancel()

	// Sync from S3 to ensure version dropdowns are current.
	// Uses a separate short timeout so a slow S3 doesn't block the page indefinitely.
	syncCtx, syncCancel := context.WithTimeout(ctx, 30*time.Second)
	h.SyncS3Builds(syncCtx)
	syncCancel()

	// Get all builds grouped by unit
	allBuilds, _ := h.BuildStore.ListAll(ctx)
	buildsByUnit := make(map[string][]models.MHSBuild)
	for _, b := range allBuilds {
		buildsByUnit[b.UnitID] = append(buildsByUnit[b.UnitID], b)
	}

	// Determine which units to show — from latest collection + any units discovered in S3
	unitIDs := make(map[string]bool)
	for _, id := range defaultUnitIDs {
		unitIDs[id] = true
	}
	for id := range buildsByUnit {
		unitIDs[id] = true
	}
	var sortedUnitIDs []string
	for id := range unitIDs {
		sortedUnitIDs = append(sortedUnitIDs, id)
	}
	sort.Strings(sortedUnitIDs)

	// Pre-fill versions from latest collection
	latestMap := make(map[string]models.MHSCollectionUnit)
	if latest, err := h.CollectionStore.Latest(ctx); err == nil {
		for _, u := range latest.Units {
			latestMap[u.UnitID] = u
		}
	}

	rows := make([]ManualUnitRow, len(sortedUnitIDs))
	for i, id := range sortedUnitIDs {
		row := ManualUnitRow{UnitID: id}
		if u, ok := latestMap[id]; ok {
			row.Version = u.Version
			row.BuildIdentifier = u.BuildIdentifier
		}

		// Build version dropdown options
		if builds, ok := buildsByUnit[id]; ok {
			for _, b := range builds {
				row.AvailableVersions = append(row.AvailableVersions, ManualVersionOption{
					Version:         b.Version,
					BuildIdentifier: b.BuildIdentifier,
					Selected:        b.Version == row.Version,
				})
			}
		}

		rows[i] = row
	}

	data := ManualData{
		BaseVM:         viewdata.LoadBase(r, h.DB),
		Units:          rows,
		CollectionName: fmt.Sprintf("Manual Collection — %s", time.Now().UTC().Format("2006-01-02")),
	}
	data.Title = "Create Collection Manually"
	templates.Render(w, r, "mhsbuilds_manual", data)
}

// HandleManual processes the manual collection creation form.
// It verifies that the specified files exist in S3 and creates build + collection records.
func (h *Handler) HandleManual(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		h.ErrLog.LogBadRequest(w, r, "Invalid form", err, "Invalid form data", "/mhsbuilds/manual")
		return
	}

	collectionName := strings.TrimSpace(r.FormValue("collection_name"))
	collectionDesc := strings.TrimSpace(r.FormValue("collection_description"))
	if collectionName == "" {
		collectionName = fmt.Sprintf("Manual Collection — %s", time.Now().UTC().Format("2006-01-02"))
	}

	// Get current user
	user, _ := auth.CurrentUser(r)
	var createdByID primitive.ObjectID
	var createdByName string
	if user != nil {
		createdByID, _ = primitive.ObjectIDFromHex(user.ID)
		createdByName = user.Name
	}

	// Use a generous timeout for S3 operations
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Minute)
	defer cancel()

	// Collect unit IDs from form fields (version_unit1, version_unit2, etc.)
	var unitIDs []string
	for key := range r.Form {
		if strings.HasPrefix(key, "version_") {
			unitIDs = append(unitIDs, strings.TrimPrefix(key, "version_"))
		}
	}
	sort.Strings(unitIDs)

	var units []models.MHSCollectionUnit

	for _, unitID := range unitIDs {
		version := strings.TrimSpace(r.FormValue("version_" + unitID))
		if version == "" {
			continue // skip units with no version selected
		}

		// Verify the build record exists
		existing, err := h.BuildStore.GetByUnitVersion(ctx, unitID, version)
		if err == mhsbuilds.ErrNotFound {
			h.renderManualError(w, r, fmt.Sprintf("No build record found for %s v%s. Try syncing from S3 first.", unitID, version))
			return
		}
		if err != nil {
			h.ErrLog.LogServerError(w, r, "Failed to check build", err, "Failed to check build records", "/mhsbuilds/manual")
			return
		}

		// Store reference only — file data lives in mhs_builds
		units = append(units, models.MHSCollectionUnit{
			UnitID:          unitID,
			Title:           unitTitle(unitID),
			Version:         existing.Version,
			BuildIdentifier: existing.BuildIdentifier,
		})
	}

	if len(units) == 0 {
		h.renderManualError(w, r, "At least one unit version must be selected.")
		return
	}

	// Sort units
	sort.Slice(units, func(i, j int) bool {
		return units[i].UnitID < units[j].UnitID
	})

	coll := models.MHSCollection{
		Name:          collectionName,
		Description:   collectionDesc,
		Units:         units,
		CreatedByID:   createdByID,
		CreatedByName: createdByName,
	}

	id, err := h.CollectionStore.Create(ctx, coll)
	if err != nil {
		h.ErrLog.LogServerError(w, r, "Failed to create collection", err, "Failed to create collection", "/mhsbuilds/manual")
		return
	}

	http.Redirect(w, r, "/mhsbuilds/collections/"+id.Hex(), http.StatusSeeOther)
}

func (h *Handler) renderManualError(w http.ResponseWriter, r *http.Request, msg string) {
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()

	// Discover all units from builds (same logic as ServeManual)
	allBuilds, _ := h.BuildStore.ListAll(ctx)
	buildsByUnit := make(map[string][]models.MHSBuild)
	for _, b := range allBuilds {
		buildsByUnit[b.UnitID] = append(buildsByUnit[b.UnitID], b)
	}

	unitIDs := make(map[string]bool)
	for _, id := range defaultUnitIDs {
		unitIDs[id] = true
	}
	for id := range buildsByUnit {
		unitIDs[id] = true
	}
	var sortedUnitIDs []string
	for id := range unitIDs {
		sortedUnitIDs = append(sortedUnitIDs, id)
	}
	sort.Strings(sortedUnitIDs)

	// Pre-fill from latest collection
	latestMap := make(map[string]models.MHSCollectionUnit)
	if latest, err := h.CollectionStore.Latest(ctx); err == nil {
		for _, u := range latest.Units {
			latestMap[u.UnitID] = u
		}
	}

	rows := make([]ManualUnitRow, len(sortedUnitIDs))
	for i, id := range sortedUnitIDs {
		// Preserve form selection if present, otherwise use latest collection
		selectedVersion := r.FormValue("version_" + id)
		if selectedVersion == "" {
			if u, ok := latestMap[id]; ok {
				selectedVersion = u.Version
			}
		}

		row := ManualUnitRow{UnitID: id, Version: selectedVersion}
		if builds, ok := buildsByUnit[id]; ok {
			for _, b := range builds {
				row.AvailableVersions = append(row.AvailableVersions, ManualVersionOption{
					Version:         b.Version,
					BuildIdentifier: b.BuildIdentifier,
					Selected:        b.Version == selectedVersion,
				})
			}
		}
		rows[i] = row
	}

	data := ManualData{
		BaseVM:         viewdata.LoadBase(r, h.DB),
		Units:          rows,
		CollectionName: r.FormValue("collection_name"),
		Error:          msg,
	}
	data.Title = "Create Collection Manually"
	templates.Render(w, r, "mhsbuilds_manual", data)
}
