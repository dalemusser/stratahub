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

	// Get all builds grouped by unit (ceremony builds kept apart)
	allBuilds, _ := h.BuildStore.ListAll(ctx)
	buildsByUnit, ceremonies := splitBuilds(allBuilds)

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

	// Pre-fill versions (and the ceremony) from latest collection
	latestMap := make(map[string]models.MHSCollectionUnit)
	ceremonyVersion := ""
	if latest, err := h.CollectionStore.Latest(ctx); err == nil {
		for _, u := range latest.Units {
			latestMap[u.UnitID] = u
		}
		if latest.HasCeremony() {
			ceremonyVersion = latest.Ceremony.Version
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
		Ceremony:       ceremonyRow(ceremonies, ceremonyVersion),
		CollectionName: fmt.Sprintf("Manual Collection — %s", time.Now().UTC().Format("2006-01-02")),
	}
	data.Title = "Create Collection Manually"
	templates.Render(w, r, "mhsbuilds_manual", data)
}

// splitBuilds groups unit builds by unit id and returns the ceremony builds
// separately (they share the store but are never unit rows).
func splitBuilds(all []models.MHSBuild) (map[string][]models.MHSBuild, []models.MHSBuild) {
	byUnit := make(map[string][]models.MHSBuild)
	var ceremonies []models.MHSBuild
	for _, b := range all {
		if b.IsCeremony() || b.UnitID == models.MHSCeremonyBuildID {
			ceremonies = append(ceremonies, b)
			continue
		}
		byUnit[b.UnitID] = append(byUnit[b.UnitID], b)
	}
	return byUnit, ceremonies
}

// ceremonyRow builds the ceremony select for a form: the available ceremony
// versions with `selected` marked ("" = none).
func ceremonyRow(ceremonies []models.MHSBuild, selected string) CeremonyRow {
	row := CeremonyRow{Version: selected}
	for _, b := range ceremonies {
		row.AvailableVersions = append(row.AvailableVersions, ManualVersionOption{
			Version:         b.Version,
			BuildIdentifier: b.BuildIdentifier,
			Selected:        b.Version == selected,
		})
	}
	return row
}

// ceremonyFromForm resolves the form's ceremony selection (field
// "version_end": "" or "none" = no ceremony) to a collection reference,
// verifying the build record exists. Returns (nil, "") for none.
func (h *Handler) ceremonyFromForm(ctx context.Context, r *http.Request) (*models.MHSCollectionCeremony, string) {
	v := strings.TrimSpace(r.FormValue("version_" + models.MHSCeremonyBuildID))
	if v == "" || v == "none" {
		return nil, ""
	}
	build, err := h.BuildStore.GetCeremony(ctx, v)
	if err == mhsbuilds.ErrNotFound || (err == nil && !build.IsCeremony()) {
		return nil, fmt.Sprintf("No ceremony build record found for v%s. Try syncing from S3 first.", v)
	}
	if err != nil {
		return nil, fmt.Sprintf("Failed to check the ceremony build: %s", err)
	}
	return &models.MHSCollectionCeremony{Version: build.Version, BuildIdentifier: build.BuildIdentifier}, ""
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

	// Collect unit IDs from form fields (version_unit1, version_unit2, etc.);
	// the ceremony select (version_end) is not a unit.
	var unitIDs []string
	for key := range r.Form {
		if strings.HasPrefix(key, "version_") && key != "version_"+models.MHSCeremonyBuildID {
			unitIDs = append(unitIDs, strings.TrimPrefix(key, "version_"))
		}
	}
	sort.Strings(unitIDs)

	ceremony, ceremonyErr := h.ceremonyFromForm(ctx, r)
	if ceremonyErr != "" {
		h.renderManualError(w, r, ceremonyErr)
		return
	}

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
		Ceremony:      ceremony,
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
	buildsByUnit, ceremonies := splitBuilds(allBuilds)

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
		Ceremony:       ceremonyRow(ceremonies, strings.TrimSpace(r.FormValue("version_"+models.MHSCeremonyBuildID))),
		CollectionName: r.FormValue("collection_name"),
		Error:          msg,
	}
	data.Title = "Create Collection Manually"
	templates.Render(w, r, "mhsbuilds_manual", data)
}
