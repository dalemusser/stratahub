// internal/app/features/mhsbuilds/sync.go
package mhsbuilds

import (
	"context"
	"regexp"
	"sort"
	"strings"

	"github.com/dalemusser/stratahub/internal/domain/models"
	"github.com/dalemusser/waffle/pantry/storage"
	"go.uber.org/zap"
)

var versionDirPattern = regexp.MustCompile(`^v\d+\.\d+\.\d+$`)

// SyncResult holds the results of an S3 sync operation.
type SyncResult struct {
	Discovered int // New builds discovered and created
	Updated    int // Existing builds refreshed with current S3 data
	Unchanged  int // Existing builds with no changes
	Units      int // Total unique units found
	Ceremonies int // Ceremony versions found (the "end/" prefix)
}

// SyncS3Builds scans S3 for all unit versions — and ceremony versions under
// the "end/" prefix (models.MHSCeremonyBuildID) — and creates mhs_builds
// records for any that don't already exist in the database. Idempotent — safe
// to run multiple times. A ceremony version folder is recognised by its entry
// file (models.MHSCeremonyEntryFile); a folder without it is skipped with a
// warning rather than registered as an empty ceremony.
func (h *Handler) SyncS3Builds(ctx context.Context) (SyncResult, error) {
	var result SyncResult

	if h.MHSStorage == nil {
		return result, nil
	}

	// Discover unit+version combos by listing all objects and parsing paths.
	// We collect unique unit/version pairs from the object keys.
	type unitVersion struct {
		UnitID  string
		Version string
		Prefix  string // e.g., "unit1/v2.2.2/"
	}

	seen := make(map[string]bool) // "unit1/v2.2.2" -> true
	var toSync []unitVersion
	unitSet := make(map[string]bool)

	// List all objects — paginate if needed
	var continuationToken string
	for {
		opts := &storage.ListOptions{MaxKeys: 1000}
		if continuationToken != "" {
			opts.ContinuationToken = continuationToken
		}
		listing, err := h.MHSStorage.List(ctx, "", opts)
		if err != nil {
			// Try listing with a trailing-slash workaround
			// Some S3 configs need a non-empty prefix
			listing, err = h.MHSStorage.List(ctx, "unit", opts)
			if err != nil {
				return result, err
			}
		}

		for _, obj := range listing.Objects {
			// Parse paths like "unit1/v2.2.2/Build/unit1.data.unityweb"
			parts := strings.SplitN(obj.Path, "/", 3)
			if len(parts) < 3 {
				continue
			}
			unitID := parts[0]
			vDir := parts[1]

			isCeremony := unitID == models.MHSCeremonyBuildID
			if !isCeremony && !unitDirPattern.MatchString(unitID) {
				continue
			}
			if !versionDirPattern.MatchString(vDir) {
				continue
			}

			key := unitID + "/" + vDir
			if seen[key] {
				continue
			}
			seen[key] = true
			if isCeremony {
				result.Ceremonies++
			} else {
				unitSet[unitID] = true
			}

			version := strings.TrimPrefix(vDir, "v")
			toSync = append(toSync, unitVersion{
				UnitID:  unitID,
				Version: version,
				Prefix:  unitID + "/" + vDir + "/",
			})
		}

		if !listing.IsTruncated {
			break
		}
		continuationToken = listing.NextContinuationToken
	}

	result.Units = len(unitSet)

	// Sort for consistent processing
	sort.Slice(toSync, func(i, j int) bool {
		if toSync[i].UnitID != toSync[j].UnitID {
			return toSync[i].UnitID < toSync[j].UnitID
		}
		return toSync[i].Version < toSync[j].Version
	})

	// For each discovered unit+version, create or update the build record
	for _, uv := range toSync {
		// Discover files from S3 for this version
		fileResult, err := h.MHSStorage.List(ctx, uv.Prefix, &storage.ListOptions{MaxKeys: 500})
		if err != nil {
			h.Log.Warn("failed to list files for version", zap.String("prefix", uv.Prefix), zap.Error(err))
			continue
		}

		files, totalSize, dataFile, frameworkFile, codeFile := buildFilesFromS3Objects(fileResult.Objects)
		if len(files) == 0 {
			continue
		}

		if uv.UnitID == models.MHSCeremonyBuildID {
			if err := h.syncCeremonyVersion(ctx, uv.Version, uv.Prefix, files, totalSize, &result); err != nil {
				h.Log.Warn("failed to sync ceremony version from S3", zap.String("version", uv.Version), zap.Error(err))
			}
			continue
		}

		existing, err := h.BuildStore.GetByUnitVersion(ctx, uv.UnitID, uv.Version)
		if err == nil {
			// Record exists — check if files changed
			if existing.TotalSize == totalSize && len(existing.Files) == len(files) {
				result.Unchanged++
				continue
			}
			// Files changed — update the record (preserve build identifier)
			if err := h.BuildStore.UpdateFiles(ctx, uv.UnitID, uv.Version, files, totalSize, dataFile, frameworkFile, codeFile); err != nil {
				h.Log.Warn("failed to update build record from S3 sync",
					zap.String("unit", uv.UnitID), zap.String("version", uv.Version), zap.Error(err))
				continue
			}
			result.Updated++
			h.Log.Info("updated build from S3",
				zap.String("unit", uv.UnitID),
				zap.String("version", uv.Version),
				zap.Int("files", len(files)),
			)
			continue
		}

		// New — create the record
		build := models.MHSBuild{
			UnitID:          uv.UnitID,
			Version:         uv.Version,
			BuildIdentifier: "unknown",
			Files:           files,
			TotalSize:       totalSize,
			DataFile:        dataFile,
			FrameworkFile:   frameworkFile,
			CodeFile:        codeFile,
			CreatedByName:   "S3 Sync",
		}

		if _, err := h.BuildStore.Create(ctx, build); err != nil {
			h.Log.Warn("failed to create build record from S3 sync",
				zap.String("unit", uv.UnitID), zap.String("version", uv.Version), zap.Error(err))
			continue
		}

		result.Discovered++
		h.Log.Info("synced build from S3",
			zap.String("unit", uv.UnitID),
			zap.String("version", uv.Version),
			zap.Int("files", len(files)),
		)
	}

	return result, nil
}

// syncCeremonyVersion creates or refreshes the mhs_builds record for one
// ceremony version folder ("end/vX.Y.Z/").
func (h *Handler) syncCeremonyVersion(ctx context.Context, version, prefix string, files []models.MHSBuildFile, totalSize int64, result *SyncResult) error {
	entry := detectCeremonyEntry(prefix, files)
	if entry == "" {
		h.Log.Warn("ceremony version folder has no entry file — skipped",
			zap.String("version", version), zap.String("expected", models.MHSCeremonyEntryFile))
		return nil
	}
	existing, err := h.BuildStore.GetCeremony(ctx, version)
	if err == nil {
		if existing.IsCeremony() && existing.EntryFile == entry && existing.TotalSize == totalSize && len(existing.Files) == len(files) {
			result.Unchanged++
			return nil
		}
		if err := h.BuildStore.UpdateCeremonyFiles(ctx, version, files, totalSize, entry); err != nil {
			return err
		}
		result.Updated++
		h.Log.Info("updated ceremony from S3", zap.String("version", version), zap.Int("files", len(files)))
		return nil
	}
	build := models.MHSBuild{
		UnitID:          models.MHSCeremonyBuildID,
		Kind:            models.MHSBuildKindCeremony,
		EntryFile:       entry,
		Version:         version,
		BuildIdentifier: "mhs-gameplay-end",
		Files:           files,
		TotalSize:       totalSize,
		CreatedByName:   "S3 Sync",
	}
	if _, err := h.BuildStore.Create(ctx, build); err != nil {
		return err
	}
	result.Discovered++
	h.Log.Info("synced ceremony from S3", zap.String("version", version), zap.Int("files", len(files)))
	return nil
}
