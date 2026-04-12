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
	IsInCurrentUnit bool   // True if cell belongs to the unit the student is currently in (grader data)
	IsInMHSUnit     bool   // True if cell belongs to the unit from Mission HydroSci progress
	CellClass       string // CSS class for the cell background
	BorderClass     string // CSS class for the border
	PointID         string // Progress point ID (e.g., "u1p1")
	PointTitle      string // Progress point title
	StudentName     string // Student name for this row
	ReviewReason    string // Reason for flagged cells
	DurationDisplay       string // Formatted wall-clock completion time (e.g., "12:34" or "1:23:45")
	ActiveDurationDisplay string // Formatted active time excluding gaps
	MistakeCount          int    // Number of mistakes/negative events (-1 = no data)
	AttemptCount          int    // Number of attempts for this point (0 = no data)
	Skipped               bool   // True if this active point was skipped (later points are passed)
}

// DeviceInfo represents a device's readiness status for display.
type DeviceInfo struct {
	DeviceType    string
	DeviceDetails map[string]string // Rich device info (browser, OS, screen, etc.)
	PWAInstalled  bool
	UnitStatus    map[string]string // "unit1" → "cached" etc.
	StorageUsage  int64
	StorageQuota  int64
	StoragePct    int    // Pre-computed storage percentage (0-100)
	StorageUsed   string // Human-readable used storage (e.g., "1.2 GB")
	StorageTotal  string // Human-readable total storage (e.g., "4.8 GB")
	LastSeen      time.Time
	IsStale       bool // last_seen > 7 days ago
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

	// Collection info
	HasCollectionOverride bool   // True if user has a per-user collection override
	CollectionName        string // Name of the effective collection (override or group/workspace)
}

// UnitHeader represents header info for a unit.
type UnitHeader struct {
	ID             string
	Title          string
	Width          int // Width in pixels (28px per progress point)
	AnalyticsWidth int // Width in pixels (64px per progress point)
	PointCount     int
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

	EnableClaudeSummaries bool // Whether AI summaries feature is enabled
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

	EnableClaudeSummaries bool // Whether AI summaries feature is enabled

	IsAdmin bool // Whether the current user is admin/coordinator (for debug tab)
}

// --- Debug tab types ---

// DebugStudentRow represents a student in the debug student list with anomaly counts.
type DebugStudentRow struct {
	ID             string
	Name           string
	LoginID        string
	PencilCount    int // active grades (started, never finished)
	EmptyCount     int // missing grades (expected but not present)
	DuplicateCount int // duplicate trigger events
	TotalAnomalies int
	TotalEvents    int64 // total log entries for this student
}

// DebugStudentsData is the view model for the debug student list.
type DebugStudentsData struct {
	GroupName string
	Students  []DebugStudentRow
}

// DebugDetailData is the view model for a single student's debug detail view.
type DebugDetailData struct {
	StudentID    string
	StudentName  string
	LoginID      string
	SelectedUnit string // "" for all, "unit1"-"unit5" for filtered
	Anomalies    []DebugAnomaly
	Timeline     []TimelineEntry
	TotalEvents  int
	UnitOptions  []string // ["unit1", "unit2", ..., "unit5"]
	GroupID      string
	TimezoneAbbr string // e.g., "CST", "UTC"
}

// DebugAnomaly represents a detected issue in a student's gameplay data.
type DebugAnomaly struct {
	Type        string // "pencil", "empty", "duplicate", "gap", "event_no_grade"
	Severity    string // "error", "warning", "info"
	PointID     string // e.g., "u3p2" (empty for non-point anomalies)
	Unit        string // e.g., "unit3"
	Description string
	Timestamp   string // when the issue occurred
}

// TimelineEntry represents a single event in the annotated timeline.
type TimelineEntry struct {
	ID              string
	EventType       string
	EventKey        string
	SceneName       string
	ServerTimestamp  time.Time
	TimestampStr    string // pre-formatted for display
	Data            map[string]interface{}
	DataSummary     string // compact string representation of data
	Unit            string // derived from sceneName

	// Annotations from grading rules
	IsStartEvent bool     // this event starts a progress point
	IsEndEvent   bool     // this event ends/triggers a progress point
	PointIDs     []string // which progress point(s) this relates to
	Annotation   string   // human-readable label
	KeyRole      string   // "start", "end", "yellow", "positive", "negative", etc.

	// Classification
	Category    string // "waypoint", "dialogue", "quest", "position", "gameplay", "system"
	IsAnomaly   bool
	AnomalyNote string // e.g., "duplicate EndOfUnit"

	// Gap from previous event
	GapSeconds float64
	GapDisplay string // e.g., "4m 10s" — only set if gap > 30s

	// Date and elapsed time
	DateStr              string // date portion, e.g. "Apr 1, 2026" — only set on first event of a new day
	ElapsedDisplay       string // wall-clock elapsed from first event, e.g. "12:34"
	ActiveElapsedDisplay string // elapsed excluding gaps > threshold, e.g. "8:21"
}
