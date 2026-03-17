// internal/app/features/mhsdashboard/types.go
package mhsdashboard

import (
	"time"

	"github.com/dalemusser/stratahub/internal/app/system/viewdata"
)

// ProgressPoint represents a single progress point within a unit.
type ProgressPoint struct {
	ID          string `json:"id"`
	ShortName   string `json:"short_name"`
	Description string `json:"description"`
}

// Unit represents a curriculum unit with its progress points.
type Unit struct {
	ID             string          `json:"id"`
	Title          string          `json:"title"`
	ProgressPoints []ProgressPoint `json:"progress_points"`
}

// ProgressConfig holds the curriculum structure loaded from JSON.
type ProgressConfig struct {
	Units []Unit `json:"units"`
}

// GroupOption represents a group in the dropdown selector (leader view).
type GroupOption struct {
	ID       string
	Name     string
	Selected bool
}

// OrgOption represents an organization in the org dropdown (admin view).
type OrgOption struct {
	ID         string
	Name       string
	GroupCount int
	Selected   bool
}

// GroupOptionEx represents a group in the styled group dropdown (admin view).
type GroupOptionEx struct {
	ID          string
	Name        string
	OrgID       string
	MemberCount int
	Selected    bool
}

// CellData represents a single cell in the progress grid.
type CellData struct {
	Value           int    // 0 = pending, 1 = flagged, 2 = passed, 3 = active
	IsUnitStart     bool   // True if this is the first cell in a unit
	IsInCurrentUnit bool   // True if cell belongs to the unit the student is currently in
	CellClass       string // CSS class for the cell background
	BorderClass     string // CSS class for the border
	PointID         string // Progress point ID (e.g., "u1p1")
	PointTitle      string // Progress point title
	StudentName     string // Student name for this row
	ReviewReason    string // Reason for flagged cells
}

// DeviceInfo represents a device's readiness status for display.
type DeviceInfo struct {
	DeviceType   string
	PWAInstalled bool
	UnitStatus   map[string]string // "unit1" → "cached" etc.
	StorageUsage int64
	StorageQuota int64
	StoragePct   int // Pre-computed storage percentage (0-100)
	LastSeen     time.Time
	IsStale      bool // last_seen > 7 days ago
}

// MemberRow represents a single row of progress data for a member.
type MemberRow struct {
	ID           string
	Name         string
	IsEven       bool              // For alternating row colors
	Cells        []CellData        // Pre-computed cell data
	Devices      []DeviceInfo      // Device readiness info
	UnitProgress map[string]string // unit ID → "completed"/"current"/"future"
	CurrentUnit  string            // Unit ID the student is currently in (from grader)
}

// UnitHeader represents header info for a unit.
type UnitHeader struct {
	ID         string
	Title      string
	Width      int // Width in pixels (28px per progress point)
	PointCount int
}

// PointHeader represents header info for a progress point.
type PointHeader struct {
	ID          string
	ShortName   string
	Description string
	IsUnitStart bool
}

// DashboardData is the view model for the main dashboard page.
type DashboardData struct {
	viewdata.BaseVM

	Groups        []GroupOption   // Leader view: flat group list
	SelectedGroup string
	GroupName     string
	MemberCount   int
	LastUpdated   string
	TimezoneAbbr  string // Timezone abbreviation (e.g., "MST", "EST")

	// Admin/coordinator view: org + group dropdowns
	IsAdmin     bool             // true for admin/coordinator/superadmin
	Orgs        []OrgOption      // Organization options
	SelectedOrg string           // Selected org ID hex
	GroupsEx    []GroupOptionEx   // Groups with org association + member counts

	UnitHeaders  []UnitHeader
	PointHeaders []PointHeader
	Members      []MemberRow

	SortBy  string // Sort field (currently only "name")
	SortDir string // Sort direction: "asc" or "desc"
}

// GridData is the view model for the HTMX-refreshed grid content.
type GridData struct {
	SelectedGroup string
	GroupName     string
	MemberCount   int
	LastUpdated   string

	UnitHeaders  []UnitHeader
	PointHeaders []PointHeader
	Members      []MemberRow

	// CSRF token for refresh requests
	CSRFToken string

	SortBy  string // Sort field (currently only "name")
	SortDir string // Sort direction: "asc" or "desc"
}
