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
	CurrentUnit          string          // e.g., "unit3" or "complete"
	IsComplete           bool            // True when all units are done
	CeremonyURL          string          // "/missionhydrosci/ceremony" when the collection has a ceremony (shown once complete)
	Ceremony             *CeremonyCardVM // the ceremony row at the end of the unit list; nil when the collection has none
	NextUnitID           string          // Unit after CurrentUnit, empty if last/complete
	CollectionOverride   bool            // True when a per-user override is active (drives the "(override)" badge)
	ActiveCollectionName string          // Name of the effective collection being used (version line)
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
	CeremonyURL     string // where the game's EndGame lands when the collection has a ceremony ("" = units page)
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

	// Device test (docs/mission-hydrosci/mhs-loading-status-and-unit2-device-test-plan.md):
	// the page runs one unit for an anonymous tester. DeviceTest switches the
	// template's completion, back-button and reporting branches.
	DeviceTest        bool
	DeviceTestID      string // 24-hex test id (also the game's user_id)
	DeviceTestShortID string // last 6 hex characters, quoted back to us by testers
	DeviceTestBase    string // "/missionhydrosci/devicetest/run/<id>"
	PlayBackURL       string // where the back button goes
}

// CeremonyCardVM is the end-of-game ceremony's row in the unit list: always
// visible when the collection carries one, so students see what follows
// Unit 5 and staff can see (and open) the version in use.
type CeremonyCardVM struct {
	Version   string
	SizeLabel string
	URL       string
	Label     string // "After Unit 5" / "Watch" / "Open (preview)"
	CanOpen   bool   // Label is a link
}

// CeremonyData is the view model for the end-of-game ceremony host page
// (docs/mission-hydrosci/mhs-end-ceremony-plan.md D3; the bundle's contract
// is mhs-gameplay-end/docs/embed-api.md).
type CeremonyData struct {
	viewdata.BaseVM
	Base      string // "/missionhydrosci/content/end/v<version>/" — every bundle file resolves under it
	Version   string // ceremony version from the resolved collection
	ScoresURL string // the EA-scores endpoint the embed polls
	ReturnURL string // where the Exit control goes
	ExitLabel string
	Dev       bool // reviewer's beat selector (staff only)
	Preview   bool // a staff preview of a student's ceremony: no viewed marks, no step log
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

// ContentManifestCeremony is the end-of-game ceremony in the content
// manifest: a separate block beside the units (it is not a unit: it is never
// downloaded ahead, listed, or counted toward completion), served at
// CDNBaseURL + "/" + file.path like everything else.
type ContentManifestCeremony struct {
	ID              string                `json:"id"` // "end"
	Version         string                `json:"version"`
	BuildIdentifier string                `json:"buildIdentifier,omitempty"`
	Entry           string                `json:"entry"` // "lib/embed.js", relative to the version folder
	Files           []ContentManifestFile `json:"files"`
	TotalSize       int64                 `json:"totalSize"`
}

// ContentManifest is the JSON response for the content manifest API.
type ContentManifest struct {
	CDNBaseURL string                   `json:"cdnBaseUrl"`
	Units      []ContentManifestUnit    `json:"units"`
	Ceremony   *ContentManifestCeremony `json:"ceremony,omitempty"`
	Tuning     *ManifestTuning          `json:"tuning,omitempty"`
	Probes     []ManifestProbe          `json:"probes,omitempty"`
}
