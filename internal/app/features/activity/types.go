// internal/app/features/activity/types.go
package activity

// Terminology: User Identifiers
//   - UserID / userID / user_id: The MongoDB ObjectID (_id) that uniquely identifies a user record
//   - LoginID / loginID / login_id: The human-readable string users type to log in

import (
	"time"

	"github.com/dalemusser/stratahub/internal/app/system/viewdata"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

// Status represents a user's online status.
type Status string

const (
	StatusOnline  Status = "online"  // Heartbeat within 2 minutes
	StatusIdle    Status = "idle"    // Heartbeat 2-10 minutes ago
	StatusOffline Status = "offline" // No active session or heartbeat > 10 minutes
)

// OnlineThreshold is the duration within which a user is considered "online".
const OnlineThreshold = 2 * time.Minute

// IdleThreshold is the duration after which a user is considered "idle" (but not offline).
const IdleThreshold = 10 * time.Minute

// memberRow represents a member in the activity dashboard.
type memberRow struct {
	ID              string
	Name            string
	LoginID         string
	Email           string
	OrgName         string
	GroupName       string
	Status          Status
	StatusLabel     string
	CurrentActivity string
	TimeTodayMins   int    // For sorting
	TimeTodayStr    string // Pre-formatted "Xh Ym" or "X min"
	LastActiveAt    *time.Time
}

// groupOption represents a group for the filter dropdown.
type groupOption struct {
	ID       string
	Name     string
	Selected bool
}

// dashboardData is the view model for the real-time dashboard.
type dashboardData struct {
	viewdata.BaseVM

	// Filter state
	SelectedGroup string
	Groups        []groupOption
	StatusFilter  string // "all", "online", "idle", "offline"
	SearchQuery   string

	// Sorting
	SortBy  string // "name", "group", "time"
	SortDir string // "asc", "desc"

	// Pagination
	Page       int
	Total      int
	RangeStart int
	RangeEnd   int
	HasPrev    bool
	HasNext    bool
	PrevPage   int
	NextPage   int

	// Summary stats (before filtering)
	TotalMembers int
	OnlineCount  int
	IdleCount    int
	OfflineCount int

	// Member rows (paginated)
	Members []memberRow
}

// summaryRow represents a member in the weekly summary view.
type summaryRow struct {
	ID           string
	Name         string
	Email        string
	GroupName    string
	SessionCount int
	TotalTimeStr string // Pre-formatted "Xh Ym" or "X min"
	OutsideClass int    // Sessions at unusual times
}

// summaryData is the view model for the weekly summary view.
type summaryData struct {
	viewdata.BaseVM

	// Filter state
	SelectedGroup string
	Groups        []groupOption
	WeekStart     string
	WeekEnd       string

	// Member rows
	Members []summaryRow
}

// activityEvent represents an event in the student detail timeline.
type activityEvent struct {
	Time        time.Time
	TimeLabel   string
	EventType   string
	Description string
}

// sessionBlock represents a session in the student detail view.
type sessionBlock struct {
	Date         string
	LoginTime    string
	LogoutTime   string
	Duration     string
	EndReason    string
	Events       []activityEvent
}

// memberDetailData is the view model for the student detail view.
type memberDetailData struct {
	viewdata.BaseVM

	// Member info
	MemberID   string
	MemberName string
	LoginID    string
	Email      string
	GroupNames string
	OrgName    string

	// Timezone info
	TimezoneName  string // e.g., "America/Denver"
	TimezoneLabel string // e.g., "MST" or "MDT"

	// Stats
	TotalSessions    int
	TotalTimeStr     string // Pre-formatted "Xh Ym" or "X min"
	AvgSessionMins   int
	ResourceLaunches int

	// Session history (most recent first)
	Sessions []sessionBlock
}

// leaderGroup represents a group that the leader leads.
type leaderGroup struct {
	ID   primitive.ObjectID
	Name string
}

// exportData is the view model for the export page.
type exportData struct {
	viewdata.BaseVM

	// Filter state
	SelectedGroup string
	SelectedOrg   string
	Groups        []groupOption
	Orgs          []orgOption
	StartDate     string
	EndDate       string

	// Aggregated stats (for the summary section)
	TotalSessions    int
	TotalUsers       int
	TotalDurationStr string // Pre-formatted "Xh Ym" or "X min"
	AvgSessionMins   int
	PeakHour         string
	MostActiveDay    string
}

// orgOption represents an organization for the filter dropdown.
type orgOption struct {
	ID       string
	Name     string
	Selected bool
}

// sessionExportRow represents a session for CSV/JSON export.
type sessionExportRow struct {
	UserID       string    `json:"user_id"`
	UserName     string    `json:"user_name"`
	Email        string    `json:"email"`
	Organization string    `json:"organization"`
	Group        string    `json:"group"`
	LoginAt      time.Time `json:"login_at"`
	LogoutAt     string    `json:"logout_at"` // string to handle nil
	EndReason    string    `json:"end_reason"`
	DurationSecs int64     `json:"duration_secs"`
	IP           string    `json:"ip"`
}

// activityExportRow represents an activity event for CSV/JSON export.
type activityExportRow struct {
	UserID       string                 `json:"user_id"`
	UserName     string                 `json:"user_name"`
	SessionID    string                 `json:"session_id"`
	Timestamp    time.Time              `json:"timestamp"`
	EventType    string                 `json:"event_type"`
	ResourceName string                 `json:"resource_name,omitempty"`
	Details      map[string]interface{} `json:"details,omitempty"`
}

// aggregateStats holds computed statistics for a date range.
type aggregateStats struct {
	TotalSessions     int
	TotalUsers        int
	TotalDurationSecs int64
	SessionsByHour    map[int]int    // hour -> count
	SessionsByDay     map[string]int // weekday -> count
}
