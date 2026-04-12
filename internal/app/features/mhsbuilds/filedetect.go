// internal/app/features/mhsbuilds/filedetect.go
package mhsbuilds

import (
	"strings"

	"github.com/dalemusser/stratahub/internal/domain/models"
	"github.com/dalemusser/waffle/pantry/storage"
)

// buildFilesFromS3Objects converts S3 list results into MHSBuildFile entries,
// filtering out directory markers and detecting key build files.
// Returns the file list, total size, and the data/framework/code filenames.
func buildFilesFromS3Objects(objects []storage.ObjectInfo) (files []models.MHSBuildFile, totalSize int64, dataFile, frameworkFile, codeFile string) {
	for _, obj := range objects {
		// Skip directory marker entries (paths ending in / with size 0)
		if strings.HasSuffix(obj.Path, "/") {
			continue
		}

		files = append(files, models.MHSBuildFile{Path: obj.Path, Size: obj.Size})
		totalSize += obj.Size

		name := obj.Path
		if idx := strings.LastIndex(name, "/"); idx >= 0 {
			name = name[idx+1:]
		}

		detectKeyFile(name, &dataFile, &frameworkFile, &codeFile)
	}
	return
}

// detectKeyFile checks if a filename is a key Unity build file and sets the appropriate pointer.
// Handles both old format (.unityweb extension) and new format (no .unityweb extension).
func detectKeyFile(name string, dataFile, frameworkFile, codeFile *string) {
	switch {
	// Old format: unit1.data.unityweb, unit1.framework.js.unityweb, unit1.wasm.unityweb
	case strings.HasSuffix(name, ".data.unityweb"):
		*dataFile = name
	case strings.HasSuffix(name, ".framework.js.unityweb"):
		*frameworkFile = name
	case strings.HasSuffix(name, ".wasm.unityweb"):
		*codeFile = name
	// New format: unit1.data, unit1.framework.js, unit1.wasm
	case strings.HasSuffix(name, ".data") && !strings.HasSuffix(name, ".data.unityweb"):
		if *dataFile == "" {
			*dataFile = name
		}
	case strings.HasSuffix(name, ".framework.js") && !strings.HasSuffix(name, ".framework.js.unityweb"):
		if *frameworkFile == "" {
			*frameworkFile = name
		}
	case strings.HasSuffix(name, ".wasm") && !strings.HasSuffix(name, ".wasm.unityweb"):
		if *codeFile == "" {
			*codeFile = name
		}
	}
}
