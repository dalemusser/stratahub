package groups

import "github.com/dalemusser/stratahub/internal/app/system/viewdata"

// appToggleItem represents a single app row in the Manage Apps page.
type appToggleItem struct {
	ID          string
	Name        string
	Description string
	Enabled     bool
}

// mhsCollectionOption is a collection available for pinning a group.
type mhsCollectionOption struct {
	ID   string
	Name string
}

// groupAppsData is the view model for the Manage Apps page.
type groupAppsData struct {
	viewdata.BaseVM
	GroupID   string
	GroupName string
	Apps      []appToggleItem

	// MHS collection pin (only relevant when MHS is enabled)
	MHSCollectionID      string                // Current pinned collection ID (empty = use workspace active)
	MHSCollectionName    string                // Name of the pinned collection (for display)
	MHSCollectionOptions []mhsCollectionOption // Available collections for the dropdown
}
