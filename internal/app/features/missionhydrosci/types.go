// internal/app/features/missionhydrosci/types.go
package missionhydrosci

import (
	"github.com/dalemusser/stratahub/internal/app/system/viewdata"
)

// UnitVM represents a unit in the unit list UI.
type UnitVM struct {
	ID        string
	Title     string
	Version   string
	TotalSize int64  // Total download size in bytes
	SizeLabel string // Human-readable size (e.g., "45 MB")
	Status    string // "completed", "current", "future"
}

// UnitsData is the view model for the unit selector page.
type UnitsData struct {
	viewdata.BaseVM
	Units          []UnitVM
	CDNBaseURL     string
	CurrentUnit    string   // e.g., "unit3" or "complete"
	CompletedUnits []string // e.g., ["unit1", "unit2"]
	UserLoginID    string   // For building the play URL with ?id=
	IsComplete     bool     // True when all units are done
	NextUnitID     string   // Unit after CurrentUnit, empty if last/complete
	MHSMemberAuth  string   // "trust", "keyword", "staffauth" — controls member auth modal
}

// PlayData is the view model for the game launcher page.
type PlayData struct {
	viewdata.BaseVM
	UnitID          string
	UnitTitle       string
	UnitVersion     string
	CDNBaseURL      string
	UserName        string // Injected into page for Unity identity bridge
	UserLoginID     string // Injected into page for Unity identity bridge
	NextUnitID      string // Next unit after this one, empty if last
	NextUnitVersion string // Version of the next unit
	DataFile        string // Build file name for data (e.g., "unit1.data" or "unit2.data.unityweb")
	FrameworkFile   string // Build file name for framework
	CodeFile        string // Build file name for wasm

	// Game service config (injected into __mhsBridgeConfig for new builds)
	// Each URL is a full endpoint (e.g., "https://log.adroit.games/api/log/submit")
	LogSubmitURL     string
	LogAuth          string
	StateSaveURL     string
	StateLoadURL     string
	SettingsSaveURL  string
	SettingsLoadURL  string
	SaveAuth         string
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
	ID            string                `json:"id"`
	Title         string                `json:"title"`
	Version       string                `json:"version"`
	DataFile      string                `json:"dataFile"`
	FrameworkFile string                `json:"frameworkFile"`
	CodeFile      string                `json:"codeFile"`
	Files         []ContentManifestFile `json:"files"`
	TotalSize     int64                 `json:"totalSize"`
}

// ContentManifest is the JSON response for the content manifest API.
type ContentManifest struct {
	CDNBaseURL string                `json:"cdnBaseUrl"`
	Units      []ContentManifestUnit `json:"units"`
}
