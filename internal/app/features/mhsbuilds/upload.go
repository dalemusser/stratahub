// internal/app/features/mhsbuilds/upload.go
package mhsbuilds

import (
	"archive/zip"
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/dalemusser/stratahub/internal/app/store/mhsbuilds"
	"github.com/dalemusser/stratahub/internal/app/system/format"
	"github.com/dalemusser/stratahub/internal/app/system/auth"
	"github.com/dalemusser/stratahub/internal/app/system/viewdata"
	"github.com/dalemusser/stratahub/internal/domain/models"
	"github.com/dalemusser/waffle/pantry/storage"
	"github.com/dalemusser/waffle/pantry/templates"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.uber.org/zap"
)

var unitDirPattern = regexp.MustCompile(`^unit\d+$`)

// maxUploadSize is the maximum allowed upload size (1.5 GB).
const maxUploadSize = 1536 * 1024 * 1024

// ServeUpload renders the upload form page.
func (h *Handler) ServeUpload(w http.ResponseWriter, r *http.Request) {
	data := UploadData{
		BaseVM: viewdata.LoadBase(r, h.DB),
	}
	data.Title = "Upload MHS Build"
	templates.Render(w, r, "mhsbuilds_upload", data)
}

// HandleUpload processes the uploaded zip, analyzes it, and renders the review page.
func (h *Handler) HandleUpload(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseMultipartForm(maxUploadSize); err != nil {
		h.renderUploadError(w, r, "File too large or invalid form data.")
		return
	}

	file, header, err := r.FormFile("file")
	if err != nil {
		h.renderUploadError(w, r, "Please select a zip file to upload.")
		return
	}
	defer file.Close()

	// Default build identifier from filename (strip .zip extension)
	buildIdentifier := r.FormValue("build_identifier")
	if buildIdentifier == "" {
		buildIdentifier = strings.TrimSuffix(header.Filename, ".zip")
		buildIdentifier = strings.TrimSuffix(buildIdentifier, filepath.Ext(header.Filename))
	}

	// Save to temp file (archive/zip needs io.ReaderAt)
	tmpFile, err := os.CreateTemp("", "mhsbuild-*.zip")
	if err != nil {
		h.Log.Error("failed to create temp file", zap.Error(err))
		h.renderUploadError(w, r, "Server error creating temp file.")
		return
	}
	tmpPath := tmpFile.Name()

	if _, err := io.Copy(tmpFile, file); err != nil {
		tmpFile.Close()
		if err := os.Remove(tmpPath); err != nil {
			h.Log.Warn("failed to clean up temp file", zap.String("path", tmpPath), zap.Error(err))
		}
		h.Log.Error("failed to write temp file", zap.Error(err))
		h.renderUploadError(w, r, "Failed to save uploaded file.")
		return
	}
	tmpFile.Close()

	// Analyze the zip
	detected, err := analyzeZip(tmpPath)
	if err != nil {
		if err := os.Remove(tmpPath); err != nil {
			h.Log.Warn("failed to clean up temp file", zap.String("path", tmpPath), zap.Error(err))
		}
		h.renderUploadError(w, r, fmt.Sprintf("Failed to analyze zip: %s", err))
		return
	}
	if len(detected) == 0 {
		if err := os.Remove(tmpPath); err != nil {
			h.Log.Warn("failed to clean up temp file", zap.String("path", tmpPath), zap.Error(err))
		}
		h.renderUploadError(w, r, "No unit folders found in the zip. Expected folders like unit1/Build/, unit2/Build/, etc.")
		return
	}

	// Look up latest versions for version suggestions
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	for i := range detected {
		latest, err := h.BuildStore.LatestByUnit(ctx, detected[i].UnitID)
		if err == nil {
			detected[i].LatestVersion = latest.Version
			detected[i].SuggestedNext = incrementPatch(latest.Version)
		} else if err == mhsbuilds.ErrNotFound {
			detected[i].SuggestedNext = "1.0.0"
		}
	}

	// Generate collection name
	collName := fmt.Sprintf("Build %s — %s", buildIdentifier, time.Now().UTC().Format("2006-01-02"))

	data := ReviewData{
		BaseVM:          viewdata.LoadBase(r, h.DB),
		DetectedUnits:   detected,
		BuildIdentifier: buildIdentifier,
		CollectionName:  collName,
		TempFilePath:    tmpPath,
	}
	data.Title = "Review MHS Build"
	templates.Render(w, r, "mhsbuilds_upload_review", data)
}

// HandleUploadConfirm processes the confirmed upload: extracts files from zip, uploads to S3, creates build + collection records.
func (h *Handler) HandleUploadConfirm(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		h.ErrLog.LogBadRequest(w, r, "Invalid form", err, "Invalid form data", "/mhsbuilds/upload")
		return
	}

	tmpPath := r.FormValue("temp_file_path")
	buildIdentifier := r.FormValue("build_identifier")
	collectionName := r.FormValue("collection_name")
	collectionDesc := r.FormValue("collection_description")

	// Clean up temp file when done
	defer func() {
		if err := os.Remove(tmpPath); err != nil {
			h.Log.Warn("failed to clean up temp file", zap.String("path", tmpPath), zap.Error(err))
		}
	}()

	// Re-analyze zip to get file listings
	detected, err := analyzeZip(tmpPath)
	if err != nil {
		h.ErrLog.LogServerError(w, r, "Failed to re-read zip file", err, "Failed to process uploaded file", "/mhsbuilds/upload")
		return
	}

	// Read versions from form
	unitVersions := make(map[string]string) // unitID -> version
	for _, d := range detected {
		version := r.FormValue("version_" + d.UnitID)
		if version == "" {
			h.renderUploadError(w, r, fmt.Sprintf("Version required for %s", d.UnitID))
			return
		}
		unitVersions[d.UnitID] = version
	}

	// Get current user for audit
	user, _ := auth.CurrentUser(r)
	var createdByID primitive.ObjectID
	if user != nil {
		createdByID, _ = primitive.ObjectIDFromHex(user.ID)
	}
	createdByName := ""
	if user != nil {
		createdByName = user.Name
	}

	// Use a long timeout for S3 uploads
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Minute)
	defer cancel()

	// Open zip for extraction
	zr, err := zip.OpenReader(tmpPath)
	if err != nil {
		h.ErrLog.LogServerError(w, r, "Failed to open zip", err, "Failed to open zip file", "/mhsbuilds/upload")
		return
	}
	defer zr.Close()

	// Detect the base path prefix (wrapper folder if any)
	basePath := detectBasePath(&zr.Reader)

	// Process each detected unit: extract files and upload to S3
	var newBuilds []models.MHSBuild
	for _, d := range detected {
		version := unitVersions[d.UnitID]
		build, err := h.uploadUnitFromZip(ctx, &zr.Reader, basePath, d.UnitID, version, buildIdentifier, createdByID, createdByName)
		if err != nil {
			h.ErrLog.LogServerError(w, r, fmt.Sprintf("Failed to upload %s", d.UnitID), err, "Failed to upload files to CDN", "/mhsbuilds/upload")
			return
		}
		newBuilds = append(newBuilds, build)
	}

	// Create the collection: inherit from latest, override uploaded units
	coll, err := h.createCollectionWithUploads(ctx, collectionName, collectionDesc, newBuilds, createdByID, createdByName)
	if err != nil {
		h.ErrLog.LogServerError(w, r, "Failed to create collection", err, "Failed to create collection", "/mhsbuilds/upload")
		return
	}

	http.Redirect(w, r, "/mhsbuilds/collections/"+coll.ID.Hex(), http.StatusSeeOther)
}

// uploadUnitFromZip extracts files for one unit from the zip, uploads each to S3, and creates the MHSBuild record.
func (h *Handler) uploadUnitFromZip(ctx context.Context, zr *zip.Reader, basePath, unitID, version, buildIdentifier string, createdByID primitive.ObjectID, createdByName string) (models.MHSBuild, error) {
	prefix := basePath + unitID + "/"
	var files []models.MHSBuildFile
	var totalSize int64
	var dataFile, frameworkFile, codeFile string

	for _, f := range zr.File {
		if f.FileInfo().IsDir() {
			continue
		}
		if !strings.HasPrefix(f.Name, prefix) {
			continue
		}

		// Relative path within the unit (e.g., "Build/unit1.data.unityweb")
		relPath := strings.TrimPrefix(f.Name, prefix)
		// Guard against path traversal from malicious zips
		if strings.Contains(relPath, "..") || filepath.IsAbs(relPath) {
			continue
		}
		// S3 destination: unitID/vVersion/relPath
		s3Key := fmt.Sprintf("%s/v%s/%s", unitID, version, relPath)

		rc, err := f.Open()
		if err != nil {
			return models.MHSBuild{}, fmt.Errorf("open %s: %w", f.Name, err)
		}

		// Read file into memory for upload (individual files within a unit build are manageable)
		data, err := io.ReadAll(rc)
		rc.Close()
		if err != nil {
			return models.MHSBuild{}, fmt.Errorf("read %s: %w", f.Name, err)
		}

		err = h.MHSStorage.Put(ctx, s3Key, bytes.NewReader(data), &storage.PutOptions{})
		if err != nil {
			return models.MHSBuild{}, fmt.Errorf("upload %s to S3: %w", s3Key, err)
		}

		size := int64(len(data))
		files = append(files, models.MHSBuildFile{Path: s3Key, Size: size})
		totalSize += size

		// Detect key build files
		name := filepath.Base(f.Name)
		detectKeyFile(name, &dataFile, &frameworkFile, &codeFile)
	}

	if len(files) == 0 {
		return models.MHSBuild{}, fmt.Errorf("no files found for %s in zip", unitID)
	}

	build := models.MHSBuild{
		UnitID:          unitID,
		Version:         version,
		BuildIdentifier: buildIdentifier,
		Files:           files,
		TotalSize:       totalSize,
		DataFile:        dataFile,
		FrameworkFile:   frameworkFile,
		CodeFile:        codeFile,
		CreatedByID:     createdByID,
		CreatedByName:   createdByName,
	}

	_, err := h.BuildStore.Create(ctx, build)
	if err == mhsbuilds.ErrDuplicate {
		// Build already exists for this unit+version — update the file data
		if updateErr := h.BuildStore.UpdateFiles(ctx, unitID, version, files, totalSize, dataFile, frameworkFile, codeFile); updateErr != nil {
			return models.MHSBuild{}, fmt.Errorf("update build record for %s v%s: %w", unitID, version, updateErr)
		}
	} else if err != nil {
		return models.MHSBuild{}, fmt.Errorf("save build record for %s v%s: %w", unitID, version, err)
	}

	h.Log.Info("uploaded MHS unit build",
		zap.String("unit", unitID),
		zap.String("version", version),
		zap.Int("files", len(files)),
		zap.Int64("totalSize", totalSize),
	)

	return build, nil
}

// createCollectionWithUploads creates a new collection inheriting from the latest existing collection
// and overriding units that were just uploaded.
func (h *Handler) createCollectionWithUploads(ctx context.Context, name, description string, newBuilds []models.MHSBuild, createdByID primitive.ObjectID, createdByName string) (models.MHSCollection, error) {
	// Build a map of new builds by unit ID
	newBuildMap := make(map[string]models.MHSBuild, len(newBuilds))
	for _, b := range newBuilds {
		newBuildMap[b.UnitID] = b
	}

	// Start from the latest collection to inherit unchanged units
	var units []models.MHSCollectionUnit
	latest, err := h.CollectionStore.Latest(ctx)
	if err == nil {
		// Inherit all units from latest, overriding the uploaded ones
		for _, u := range latest.Units {
			if nb, ok := newBuildMap[u.UnitID]; ok {
				units = append(units, buildToCollectionUnit(nb))
				delete(newBuildMap, u.UnitID)
			} else {
				units = append(units, u)
			}
		}
	}

	// Add any new units not in the previous collection
	for _, nb := range newBuildMap {
		units = append(units, buildToCollectionUnit(nb))
	}

	if len(units) == 0 {
		return models.MHSCollection{}, fmt.Errorf("no units to include in collection")
	}

	// Sort units by unit ID for consistent ordering
	sort.Slice(units, func(i, j int) bool {
		return units[i].UnitID < units[j].UnitID
	})

	coll := models.MHSCollection{
		Name:          name,
		Description:   description,
		Units:         units,
		CreatedByID:   createdByID,
		CreatedByName: createdByName,
	}

	id, err := h.CollectionStore.Create(ctx, coll)
	if err != nil {
		return coll, err
	}
	coll.ID = id
	return coll, nil
}

// buildToCollectionUnit converts an MHSBuild to an MHSCollectionUnit.
// buildToCollectionUnit creates a collection unit reference from a build record.
// File data is stored in mhs_builds and looked up at serving time.
func buildToCollectionUnit(b models.MHSBuild) models.MHSCollectionUnit {
	return models.MHSCollectionUnit{
		UnitID:          b.UnitID,
		Title:           unitTitle(b.UnitID),
		Version:         b.Version,
		BuildIdentifier: b.BuildIdentifier,
	}
}

// unitTitles maps unit IDs to display titles.
var unitTitles = map[string]string{
	"unit1": "Unit 1: Water Cycle",
	"unit2": "Unit 2: Water Quality",
	"unit3": "Unit 3: Watersheds",
	"unit4": "Unit 4: Flooding",
	"unit5": "Unit 5: Water Resources",
	"unit6": "Unit 6",
}

func unitTitle(id string) string {
	if t, ok := unitTitles[id]; ok {
		return t
	}
	return id
}

// analyzeZip opens a zip file and detects which units it contains.
func analyzeZip(path string) ([]DetectedUnit, error) {
	zr, err := zip.OpenReader(path)
	if err != nil {
		return nil, fmt.Errorf("cannot open zip: %w", err)
	}
	defer zr.Close()

	basePath := detectBasePath(&zr.Reader)

	// Collect files per unit
	unitFiles := make(map[string][]zip.File)
	for _, f := range zr.File {
		if f.FileInfo().IsDir() {
			continue
		}
		name := strings.TrimPrefix(f.Name, basePath)
		parts := strings.SplitN(name, "/", 2)
		if len(parts) < 2 {
			continue
		}
		dir := parts[0]
		if !unitDirPattern.MatchString(dir) {
			continue
		}
		unitFiles[dir] = append(unitFiles[dir], *f)
	}

	var detected []DetectedUnit
	for unitID, files := range unitFiles {
		var totalSize int64
		for _, f := range files {
			totalSize += int64(f.UncompressedSize64)
		}
		detected = append(detected, DetectedUnit{
			UnitID:    unitID,
			FileCount: len(files),
			TotalSize: totalSize,
			SizeLabel: format.Bytes(totalSize),
		})
	}

	// Sort by unit ID
	sort.Slice(detected, func(i, j int) bool {
		return detected[i].UnitID < detected[j].UnitID
	})

	return detected, nil
}

// detectBasePath figures out if the zip has a wrapper directory.
// Returns the prefix to strip (e.g., "BuildOutput/" or "").
func detectBasePath(zr *zip.Reader) string {
	// Find all entries that contain /Build/ or /StreamingAssets/
	for _, f := range zr.File {
		// Look for a path like "something/unit1/Build/..."
		idx := strings.Index(f.Name, "/Build/")
		if idx < 0 {
			idx = strings.Index(f.Name, "/StreamingAssets/")
		}
		if idx < 0 {
			continue
		}
		// Everything before the unitN/ folder
		prefix := f.Name[:idx]
		parts := strings.Split(prefix, "/")
		if len(parts) == 0 {
			continue
		}
		// The last part should be the unit folder
		unitDir := parts[len(parts)-1]
		if unitDirPattern.MatchString(unitDir) {
			// Base path is everything before the unit folder
			if len(parts) == 1 {
				return "" // unit folder is at root
			}
			return strings.Join(parts[:len(parts)-1], "/") + "/"
		}
	}
	return ""
}

// incrementPatch increments the patch version of a semver string.
// "2.2.2" -> "2.2.3", "1.0.0" -> "1.0.1"
func incrementPatch(version string) string {
	parts := strings.Split(version, ".")
	if len(parts) != 3 {
		return version
	}
	patch, err := strconv.Atoi(parts[2])
	if err != nil {
		return version
	}
	return fmt.Sprintf("%s.%s.%d", parts[0], parts[1], patch+1)
}


func (h *Handler) renderUploadError(w http.ResponseWriter, r *http.Request, msg string) {
	data := UploadData{
		BaseVM: viewdata.LoadBase(r, h.DB),
		Error:  msg,
	}
	data.Title = "Upload MHS Build"
	templates.Render(w, r, "mhsbuilds_upload", data)
}
