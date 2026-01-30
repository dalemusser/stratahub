// internal/app/features/mhsdashboard3/types.go
package mhsdashboard3

import (
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

// GroupOption represents a group in the dropdown selector.
type GroupOption struct {
	ID       string
	Name     string
	Selected bool
}

// CellData represents a single cell in the progress grid.
type CellData struct {
	Value        int    // 0 = not started, 1 = needs review, 2 = completed
	IsUnitStart  bool   // True if this is the first cell in a unit
	CellClass    string // CSS class for the cell background
	BorderClass  string // CSS class for the border
	PointID      string // Progress point ID (e.g., "u1p1")
	PointTitle   string // Progress point title
	StudentName  string // Student name for this row
	ReviewReason string // Reason for needing review (only for warning cells)
}

// MemberRow represents a single row of progress data for a member.
type MemberRow struct {
	ID      string
	Name    string
	IsEven  bool       // For alternating row colors
	Cells   []CellData // Pre-computed cell data
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

	Groups        []GroupOption
	SelectedGroup string
	GroupName     string
	MemberCount   int
	LastUpdated   string
	TimezoneAbbr  string // Timezone abbreviation (e.g., "MST", "EST")

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
