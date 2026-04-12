// internal/app/features/mhsbuilds/storage.go
package mhsbuilds

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/dalemusser/stratahub/internal/app/system/format"
	"github.com/dalemusser/stratahub/internal/app/system/viewdata"
	"github.com/dalemusser/stratahub/internal/domain/models"
	"github.com/dalemusser/waffle/pantry/storage"
	"github.com/dalemusser/waffle/pantry/templates"
	"github.com/go-chi/chi/v5"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

// ServeStorage renders the S3 storage management page.
func (h *Handler) ServeStorage(w http.ResponseWriter, r *http.Request) {
	h.renderStorage(w, r, "", "")
}

// HandleSync triggers an S3 sync and re-renders the storage page.
func (h *Handler) HandleSync(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Minute)
	defer cancel()

	result, err := h.SyncS3Builds(ctx)
	if err != nil {
		h.renderStorage(w, r, "", fmt.Sprintf("Sync failed: %s", err))
		return
	}

	msg := fmt.Sprintf("Sync complete. Found %d units. Discovered %d new, updated %d, %d unchanged.",
		result.Units, result.Discovered, result.Updated, result.Unchanged)
	h.renderStorage(w, r, msg, "")
}

// HandleDeleteBuild deletes a build's S3 files and database record.
func (h *Handler) HandleDeleteBuild(w http.ResponseWriter, r *http.Request) {
	unitID := chi.URLParam(r, "unit")
	version := chi.URLParam(r, "version")

	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Minute)
	defer cancel()

	// Check that this version is not in any collection
	if h.isBuildInCollection(ctx, unitID, version) {
		h.renderStorage(w, r, "", fmt.Sprintf("Cannot delete %s v%s — it is referenced by a collection.", unitID, version))
		return
	}

	// Delete files from S3 first — if this fails, don't delete the database record
	if h.MHSStorage != nil {
		s3Prefix := fmt.Sprintf("%s/v%s/", unitID, version)
		result, err := h.MHSStorage.List(ctx, s3Prefix, &storage.ListOptions{MaxKeys: 500})
		if err != nil {
			h.renderStorage(w, r, "", fmt.Sprintf("Failed to list S3 files for %s v%s: %s", unitID, version, err))
			return
		}
		var paths []string
		for _, obj := range result.Objects {
			paths = append(paths, obj.Path)
		}
		if len(paths) > 0 {
			if _, err := h.MHSStorage.DeleteMany(ctx, paths); err != nil {
				h.renderStorage(w, r, "", fmt.Sprintf("Failed to delete S3 files for %s v%s: %s", unitID, version, err))
				return
			}
		}
	}

	// S3 files deleted successfully — now delete the database record
	if err := h.BuildStore.DeleteByUnitVersion(ctx, unitID, version); err != nil {
		h.renderStorage(w, r, "", fmt.Sprintf("S3 files deleted but failed to remove database record for %s v%s: %s", unitID, version, err))
		return
	}

	h.renderStorage(w, r, fmt.Sprintf("Deleted %s v%s from S3 and database.", unitID, version), "")
}

// isBuildInCollection checks if a specific unit+version is referenced by any collection.
func (h *Handler) isBuildInCollection(ctx context.Context, unitID, version string) bool {
	collections, err := h.CollectionStore.List(ctx, 500)
	if err != nil {
		return true // err on the side of caution
	}
	for _, coll := range collections {
		for _, u := range coll.Units {
			if u.UnitID == unitID && u.Version == version {
				return true
			}
		}
	}
	return false
}

// collectionsForBuild returns the names of collections that reference a specific unit+version.
func (h *Handler) collectionsForBuild(collections []models.MHSCollection, unitID, version string) []string {
	var names []string
	for _, coll := range collections {
		for _, u := range coll.Units {
			if u.UnitID == unitID && u.Version == version {
				names = append(names, coll.Name)
				break
			}
		}
	}
	return names
}

func (h *Handler) renderStorage(w http.ResponseWriter, r *http.Request, syncMsg, errMsg string) {
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()

	// Load all builds from the database
	allBuilds, err := h.BuildStore.ListAll(ctx)
	if err != nil {
		h.ErrLog.LogServerError(w, r, "Failed to load builds", err, "Failed to load builds", "/mhsbuilds/collections")
		return
	}

	// Load all collections to check references
	allCollections, _ := h.CollectionStore.List(ctx, 500)

	vms := make([]StorageBuildVM, len(allBuilds))
	for i, b := range allBuilds {
		collNames := h.collectionsForBuild(allCollections, b.UnitID, b.Version)
		vms[i] = StorageBuildVM{
			ID:              b.ID.Hex(),
			UnitID:          b.UnitID,
			Version:         b.Version,
			BuildIdentifier: b.BuildIdentifier,
			FileCount:       len(b.Files),
			TotalSize:       b.TotalSize,
			SizeLabel:       format.Bytes(b.TotalSize),
			Collections:     collNames,
			CollectionCount: len(collNames),
			CanDelete:       len(collNames) == 0,
		}
	}

	data := StorageData{
		BaseVM:      viewdata.NewBaseVM(r, h.DB, "S3 Storage", "/mhsbuilds/collections"),
		Builds:      vms,
		SyncMessage: syncMsg,
		Error:       errMsg,
	}

	templates.Render(w, r, "mhsbuilds_storage", data)
}

// ServeBuildCollectionsModal renders the collections modal for a build (HTMX snippet).
func (h *Handler) ServeBuildCollectionsModal(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	oid, err := primitive.ObjectIDFromHex(idStr)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()

	build, err := h.BuildStore.GetByID(ctx, oid)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	allCollections, _ := h.CollectionStore.List(ctx, 500)
	collNames := h.collectionsForBuild(allCollections, build.UnitID, build.Version)

	data := struct {
		UnitID      string
		Version     string
		Collections []string
	}{
		UnitID:      build.UnitID,
		Version:     build.Version,
		Collections: collNames,
	}

	templates.RenderSnippet(w, "mhsbuilds_build_collections_modal", data)
}

// HandleEditBuildIdentifier updates the build identifier for a build record.
func (h *Handler) HandleEditBuildIdentifier(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	oid, err := primitive.ObjectIDFromHex(idStr)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}

	newIdentifier := strings.TrimSpace(r.FormValue("build_identifier"))

	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()

	build, err := h.BuildStore.GetByID(ctx, oid)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	// Update just the build identifier field
	_, err = h.DB.Collection("mhs_builds").UpdateOne(ctx,
		bson.M{"_id": oid},
		bson.M{"$set": bson.M{"build_identifier": newIdentifier}},
	)
	if err != nil {
		h.renderStorage(w, r, "", fmt.Sprintf("Failed to update build identifier for %s v%s: %s", build.UnitID, build.Version, err))
		return
	}

	h.renderStorage(w, r, fmt.Sprintf("Updated build identifier for %s v%s.", build.UnitID, build.Version), "")
}
