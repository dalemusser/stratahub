// internal/app/features/mhsdelivery/types.go
package mhsdelivery

import (
	"github.com/dalemusser/stratahub/internal/app/system/viewdata"
)

// UnitVM represents a unit in the unit list UI.
type UnitVM struct {
	ID        string
	Title     string
	TotalSize int64  // Total download size in bytes
	SizeLabel string // Human-readable size (e.g., "45 MB")
}

// UnitsData is the view model for the unit selector page.
type UnitsData struct {
	viewdata.BaseVM
	Units      []UnitVM
	CDNBaseURL string
}

// PlayData is the view model for the game launcher page.
type PlayData struct {
	viewdata.BaseVM
	UnitID      string
	UnitTitle   string
	UnitVersion string
	CDNBaseURL  string
}

// OfflineData is the view model for the offline fallback page.
type OfflineData struct {
	viewdata.BaseVM
}

// ContentManifestFile represents a single file in the content manifest.
type ContentManifestFile struct {
	Path string `json:"path"`
	Size int64  `json:"size"`
}

// ContentManifestUnit represents a unit in the content manifest.
type ContentManifestUnit struct {
	ID        string                `json:"id"`
	Title     string                `json:"title"`
	Version   string                `json:"version"`
	Files     []ContentManifestFile `json:"files"`
	TotalSize int64                 `json:"totalSize"`
}

// ContentManifest is the JSON response for the content manifest API.
type ContentManifest struct {
	CDNBaseURL string                `json:"cdnBaseUrl"`
	Units      []ContentManifestUnit `json:"units"`
}
