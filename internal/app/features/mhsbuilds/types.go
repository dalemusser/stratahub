// internal/app/features/mhsbuilds/types.go
package mhsbuilds

import (
	"time"

	"github.com/dalemusser/stratahub/internal/app/system/viewdata"
)

// UploadData is the view model for the upload form page.
type UploadData struct {
	viewdata.BaseVM
	Error string
}

// DetectedUnit holds analysis results for a single unit found in a zip.
type DetectedUnit struct {
	UnitID        string // "unit1"
	FileCount     int
	TotalSize     int64
	SizeLabel     string // "104.3 MB"
	LatestVersion string // Current latest version in DB (for suggestion)
	SuggestedNext string // Auto-incremented version suggestion
}

// ReviewData is the view model for the upload review page.
type ReviewData struct {
	viewdata.BaseVM
	DetectedUnits   []DetectedUnit
	BuildIdentifier string
	CollectionName  string
	TempFilePath    string // Hidden field for the temp zip path
	Error           string
}

// ManualUnitRow is a row in the manual collection creation form.
type ManualUnitRow struct {
	UnitID            string
	Version           string                // Pre-filled from latest collection
	BuildIdentifier   string                // Pre-filled from latest collection
	AvailableVersions []ManualVersionOption // Versions available in S3/database
}

// ManualVersionOption is a version available for selection in the manual form.
type ManualVersionOption struct {
	Version         string
	BuildIdentifier string
	Selected        bool
}

// ManualData is the view model for the manual collection creation page.
type ManualData struct {
	viewdata.BaseVM
	Units          []ManualUnitRow
	CollectionName string
	Error          string
}

// CollectionVM is a view model for a collection in the list.
type CollectionVM struct {
	ID            string
	Name          string
	Description   string
	UnitsSummary  string // "unit1:v2.2.2, unit2:v2.2.2, ..."
	CreatedAt     time.Time
	CreatedByName string
	IsActive      bool // True if this is the workspace active collection
}

// CollectionsData is the view model for the collections list page.
type CollectionsData struct {
	viewdata.BaseVM
	Collections      []CollectionVM
	ActiveID         string // Current workspace active collection ID
}

// CollectionUnitVM is a unit row in the collection detail view.
type CollectionUnitVM struct {
	UnitID          string
	Title           string
	Version         string
	BuildIdentifier string
	FileCount       int
	TotalSize       int64
	SizeLabel       string
}

// ManageModalData is the view model for the collection manage modal (snippet).
type ManageModalData struct {
	ID        string
	Name      string
	IsActive  bool
	CanDelete bool
	CSRFToken string
}

// EditCollectionData is the view model for the collection edit page.
type EditCollectionData struct {
	viewdata.BaseVM
	ID          string
	Name        string
	Description string
	Units       []EditCollectionUnitRow
	IsActive    bool
	Error       string
}

// EditCollectionUnitRow is a row in the edit collection form.
type EditCollectionUnitRow struct {
	UnitID            string
	Title             string
	Version           string
	BuildIdentifier   string
	AvailableVersions []ManualVersionOption // Reuses the same option type as manual creation
}

// AssignmentWorkspace is a workspace that has this collection active.
type AssignmentWorkspace struct {
	Name      string
	Subdomain string
}

// AssignmentGroup is a group pinned to this collection.
type AssignmentGroup struct {
	GroupName     string
	WorkspaceName string
}

// MHSEnabledGroup is a group with Mission HydroSci enabled.
type MHSEnabledGroup struct {
	GroupName      string
	WorkspaceID    string
	WorkspaceName  string
	CollectionUsed string // "Pinned: <name>" or "Workspace active" or "None"
}

// WorkspaceOption is a workspace for the filter dropdown.
type WorkspaceOption struct {
	ID        string
	Name      string
	Subdomain string
	Selected  bool
}

// AssignmentUser is a user with this collection as their override.
type AssignmentUser struct {
	UserName      string
	LoginID       string
	WorkspaceName string
}

// AssignmentsData is the view model for the collection assignments page.
type AssignmentsData struct {
	viewdata.BaseVM
	CollectionID       string
	CollectionName     string
	Workspaces         []AssignmentWorkspace
	Groups             []AssignmentGroup
	Users              []AssignmentUser
	EnabledGroups      []MHSEnabledGroup
	WorkspaceOptions   []WorkspaceOption
	SelectedWorkspace  string // "all" or workspace ID hex
	IsUnused           bool
}

// StorageBuildVM represents a unit version in the S3 storage page.
type StorageBuildVM struct {
	ID              string
	UnitID          string
	Version         string
	BuildIdentifier string
	FileCount       int
	TotalSize       int64
	SizeLabel       string
	Collections     []string // Names of collections that reference this version
	CollectionCount int      // Number of collections referencing this version
	CanDelete       bool     // True if not referenced by any collection
}

// StorageData is the view model for the S3 storage page.
type StorageData struct {
	viewdata.BaseVM
	Builds      []StorageBuildVM
	SyncMessage string // Result message after a sync operation
	Error       string
}

// CollectionDetailData is the view model for the collection detail page.
type CollectionDetailData struct {
	viewdata.BaseVM
	ID            string
	Name          string
	Description   string
	Units         []CollectionUnitVM
	CreatedAt     time.Time
	CreatedByName string
	IsActive      bool   // True if this is the workspace active collection
	ActiveID      string // Current workspace active collection ID
}
