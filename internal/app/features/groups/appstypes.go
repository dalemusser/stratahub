package groups

import "github.com/dalemusser/stratahub/internal/app/system/viewdata"

// appToggleItem represents a single app row in the Manage Apps page.
type appToggleItem struct {
	ID          string
	Name        string
	Description string
	Enabled     bool
}

// groupAppsData is the view model for the Manage Apps page.
type groupAppsData struct {
	viewdata.BaseVM
	GroupID   string
	GroupName string
	Apps      []appToggleItem
}
