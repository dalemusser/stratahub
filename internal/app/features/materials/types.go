// internal/app/features/materials/types.go
package materials

import (
	"html/template"

	"github.com/dalemusser/stratahub/internal/app/system/viewdata"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

// ========================= ADMIN VIEW MODELS ======================

// listItem is a summary row for display in the admin materials list.
type listItem struct {
	ID          primitive.ObjectID
	Title       string
	TitleCI     string // case-insensitive title for cursor building
	Subject     string
	Type        string
	Status      string
	HasFile     bool   // true if material has an uploaded file
	Description string
}

// listData provides template data for the admin materials list.
type listData struct {
	viewdata.BaseVM

	Q     string
	Items []listItem

	// Pagination
	Shown      int
	Total      int64
	HasPrev    bool
	HasNext    bool
	PrevCursor string
	NextCursor string
	RangeStart int
	RangeEnd   int
	PrevStart  int
	NextStart  int

	// Permission flags
	CanCreate bool // True for admin; false for coordinator (view-only)
}

// manageModalData is used to render the admin "Manage Material" modal.
type manageModalData struct {
	ID          string
	Title       string
	Subject     string
	Type        string
	Status      string
	HasFile     bool
	FileName    string
	Description string
	BackURL     string
	CanEdit     bool   // True for admin; false for coordinator (view-only)
	CSRFToken   string
}

// MaterialTypeOption is used to populate the material type select menu.
type MaterialTypeOption struct {
	ID    string
	Label string
}

// materialFormVM is the unified form view-model used by the New and Edit
// admin flows. New and Edit handlers populate this and then render the
// corresponding templates.
type materialFormVM struct {
	viewdata.BaseVM

	ID            string
	MaterialTitle string
	Subject       string
	Description   string
	LaunchURL     string
	Type          string

	Status              string
	DefaultInstructions string

	// File info (for edit form)
	HasFile  bool
	FileName string
	FileSize int64

	// Navigation / redirects
	SubmitReturn string
	DeleteReturn string

	// Error message to show above the form
	Error template.HTML

	// Populated with models.MaterialTypes as ID + label
	TypeOptions []MaterialTypeOption
}

// viewData is the view-only model for the admin material detail page.
type viewData struct {
	viewdata.BaseVM

	ID                  string
	MaterialTitle       string
	Subject             string
	Description         string
	LaunchURL           string
	Type                string
	Status              string
	HasFile             bool
	FileName            string
	FileSize            int64
	DefaultInstructions template.HTML // HTML content, sanitized for safe rendering
	AssignmentCount     int64

	// Permission flags
	CanEdit bool // True for admin; false for coordinator (view-only)
}

// ========================= ASSIGNMENT VIEW MODELS ======================

// orgPaneRow is a single organization in the assignment picker left pane.
type orgPaneRow struct {
	ID          primitive.ObjectID
	Name        string
	NameCI      string
	LeaderCount int64
	Selected    bool
}

// leaderPaneRow is a single leader in the assignment picker right pane.
type leaderPaneRow struct {
	ID       primitive.ObjectID
	FullName string
	Email    string
	Selected bool
}

// assignData is the view-model for the assignment picker page.
type assignData struct {
	viewdata.BaseVM

	// Material info
	MaterialID    string
	MaterialTitle string

	// Left pane - Organizations
	OrgRows         []orgPaneRow
	SelectedOrg     string
	SelectedOrgName string
	OrgSearch       string
	OrgShown        int
	OrgTotal        int64
	OrgHasPrev      bool
	OrgHasNext      bool
	OrgPrevCur      string
	OrgNextCur      string

	// Right pane - Leaders (loaded via HTMX when org is selected)
	LeaderRows         []leaderPaneRow
	SelectedAll        bool   // true if assigning to org (all leaders)
	SelectedLeaderID   string // selected leader ID (if not all)
	SelectedLeaderName string // selected leader name (if not all)
	LeaderSearch       string
	LeaderShown        int
	LeaderTotal        int64
	LeaderHasPrev      bool
	LeaderHasNext      bool
	LeaderPrevCur      string
	LeaderNextCur      string

	// Error
	Error template.HTML
}

// assignFormData is the view-model for the assignment form page (step 2).
type assignFormData struct {
	viewdata.BaseVM

	// Material info
	MaterialID    string
	MaterialTitle string
	HasFile       bool
	LaunchURL     string

	// Target info
	OrgID      string
	LeaderID   string
	TargetName string
	IsOrgWide  bool

	// Form fields
	VisibleFrom  string
	VisibleUntil string
	Directions   string

	// Timezone for date interpretation
	TimeZone string // friendly label (e.g., "Eastern Time (US & Canada)")

	// Error
	Error template.HTML
}

// leadersPaneData is the HTMX partial for the leaders pane.
type leadersPaneData struct {
	MaterialID    string
	SelectedOrg   string
	OrgName       string
	LeaderRows    []leaderPaneRow
	SelectedAll   bool
	LeaderSearch  string
	LeaderShown   int
	LeaderTotal   int64
	LeaderHasPrev bool
	LeaderHasNext bool
	LeaderPrevCur string
	LeaderNextCur string
}

// assignmentListItem is a single assignment row.
type assignmentListItem struct {
	ID            string
	MaterialID    string
	MaterialTitle string
	TargetName    string // Org name or Leader name
	TargetType    string // "organization" or "leader"
	VisibleFrom   string
	VisibleUntil  string
	CreatedAt     string
}

// assignmentListData is the view-model for the assignments list (per material).
type assignmentListData struct {
	viewdata.BaseVM

	MaterialID    string
	MaterialTitle string
	Items         []assignmentListItem
	Shown         int
}

// allAssignmentsListData is the view-model for the global assignments list.
type allAssignmentsListData struct {
	viewdata.BaseVM

	Items []assignmentListItem
	Shown int
}

// assignmentManageModalData is the view-model for the assignment manage modal.
type assignmentManageModalData struct {
	ID            string
	MaterialID    string
	MaterialTitle string
	TargetName    string
	TargetType    string
	VisibleFrom   string
	VisibleUntil  string
	Directions    string
	BackURL       string
	CSRFToken     string
}

// assignmentViewData is the view-model for viewing an assignment.
type assignmentViewData struct {
	viewdata.BaseVM

	ID            string
	MaterialID    string
	MaterialTitle string
	HasFile       bool
	LaunchURL     string
	TargetName    string
	TargetType    string
	VisibleFrom   string
	VisibleUntil  string
	Directions    template.HTML
	CreatedAt     string
	TimeZone      string // friendly label for display
}

// assignmentEditData is the view-model for editing an assignment.
type assignmentEditData struct {
	viewdata.BaseVM

	ID            string
	MaterialID    string
	MaterialTitle string
	HasFile       bool
	LaunchURL     string
	TargetName    string
	TargetType    string
	VisibleFrom   string
	VisibleUntil  string
	Directions    string
	TimeZone      string // friendly label for display
	Error         template.HTML
}

// ========================= LEADER VIEW MODELS ===============================

// leaderMaterialItem is a single row in the leader "My Materials" list.
type leaderMaterialItem struct {
	ID             string
	Title          string
	Subject        string
	Type           string
	HasFile        bool
	LaunchURL      string
	Description    string        // truncated for card display
	Directions     template.HTML // HTML content for modal display
	HasDirections  bool          // true if directions exist
	AvailableUntil string
	OrgName        string // for org-wide assignments
	IsOrgWide      bool
}

// leaderMaterialsListData is the view model for the leader materials list.
type leaderMaterialsListData struct {
	viewdata.BaseVM

	Materials []leaderMaterialItem
}

// leaderMaterialViewData is the view model for the leader material detail page.
type leaderMaterialViewData struct {
	viewdata.BaseVM

	MaterialID     string
	MaterialTitle  string
	Subject        string
	Type           string
	TypeDisplay    string
	Description    string
	Directions     template.HTML // HTML content, from assignment
	DisplayURL     string        // Original URL for display (without tracking params)
	LaunchURL      string        // URL with tracking params (id) for the Open button
	HasFile        bool
	FileName       string
	Status         string
	AvailableUntil string
	CanOpen        bool
}
