// internal/app/features/missionhydrosci/types.go
package missionhydrosci

import (
	"github.com/dalemusser/stratahub/internal/app/system/viewdata"
)

// UnitVM represents a unit in the unit list UI.
type UnitVM struct {
	ID              string
	Title           string
	Version         string
	BuildIdentifier string // CI/CD build name for traceability
	TotalSize       int64  // Total download size in bytes
	SizeLabel       string // Human-readable size (e.g., "45 MB")
	Status          string // "completed", "current", "future"
}

// UnitsData is the view model for the unit selector page. Since the redesign,
// the launcher is status-only: per-unit actions, the auth modal, and version
// switching live on the Manage page, so the fields those needed
// (CompletedUnits, UserIDHex, MHSMemberAuth, CDNBaseURL, the override/active
// collection detail) are no longer rendered here and were removed.
type UnitsData struct {
	viewdata.BaseVM
	Units                []UnitVM
	CurrentUnit          string // e.g., "unit3" or "complete"
	IsComplete           bool   // True when all units are done
	NextUnitID           string // Unit after CurrentUnit, empty if last/complete
	CollectionOverride   bool   // True when a per-user override is active (drives the "(override)" badge)
	ActiveCollectionName string // Name of the effective collection being used (version line)
}

// PlayData is the view model for the game launcher page.
type PlayData struct {
	viewdata.BaseVM
	UnitID          string
	UnitTitle       string
	UnitVersion     string
	CDNBaseURL      string
	UserName        string // Injected into page for Unity identity bridge
	UserIDHex       string // Hex of users._id; injected into Unity identity bridge
	NextUnitID      string // Next unit after this one, empty if last
	NextUnitVersion string // Version of the next unit
	DataFile        string // Build file name for data (e.g., "unit1.data" or "unit2.data.unityweb")
	FrameworkFile   string // Build file name for framework
	CodeFile        string // Build file name for wasm

	// Game service config (injected into __mhsBridgeConfig for new builds)
	// Each URL is a full endpoint (e.g., "https://log.adroit.games/api/log/submit")
	LogSubmitURL    string
	LogAuth         string
	StateSaveURL    string
	StateLoadURL    string
	SettingsSaveURL string
	SettingsLoadURL string
	SaveAuth        string
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
	ID              string                `json:"id"`
	Title           string                `json:"title"`
	Version         string                `json:"version"`
	BuildIdentifier string                `json:"buildIdentifier,omitempty"`
	DataFile        string                `json:"dataFile"`
	FrameworkFile   string                `json:"frameworkFile"`
	CodeFile        string                `json:"codeFile"`
	Files           []ContentManifestFile `json:"files"`
	TotalSize       int64                 `json:"totalSize"`
}

// ManifestTuning carries the client download timing values (milliseconds)
// the delivery JS applies in place of its built-in defaults. Served with the
// manifest so thresholds can be adjusted per deployment without a JS deploy.
// Zero fields are omitted and the client keeps its default for them.
type ManifestTuning struct {
	FrozenSwitchMs  int `json:"frozenSwitchMs,omitempty"`  // Background Fetch quiet this long (visible tab) => switch to the direct download
	FallbackStallMs int `json:"fallbackStallMs,omitempty"` // direct (SW) download quiet this long => auto-resume once, then Stalled
	BgStallMs       int `json:"bgStallMs,omitempty"`       // Background Fetch quiet this long => Stalled (normally never reached)
	KeepaliveMs     int `json:"keepaliveMs,omitempty"`     // how often pages nudge the SW to keep progress alive
}

// ManifestProbe is a reachability probe the client runs before its first
// download (no-cors GET; any response counts as reachable). Used to tell a
// blocked game-service host from a working one.
type ManifestProbe struct {
	Name string `json:"name"`
	URL  string `json:"url"`
}

// ContentManifest is the JSON response for the content manifest API.
type ContentManifest struct {
	CDNBaseURL string                `json:"cdnBaseUrl"`
	Units      []ContentManifestUnit `json:"units"`
	Tuning     *ManifestTuning       `json:"tuning,omitempty"`
	Probes     []ManifestProbe       `json:"probes,omitempty"`
}
