// internal/app/features/workspaces/types.go
package workspaces

import (
	"html/template"
	"time"

	"github.com/dalemusser/stratahub/internal/app/system/viewdata"
)

// listData is the view model for the workspace list page.
type listData struct {
	viewdata.BaseVM
	Rows       []workspaceRow
	Total      int64
	Domain     string // Primary domain for building workspace URLs
	Error      template.HTML
	HasPrev    bool
	HasNext    bool
	PrevCursor string
	NextCursor string
}

// workspaceRow represents a single workspace in the list.
type workspaceRow struct {
	ID         string
	Name       string
	Subdomain  string
	Status     string
	UserCount  int64
	OrgCount   int64
	CreatedAt  time.Time
	StatusBadge string
}

// newData is the view model for the new workspace form.
type newData struct {
	viewdata.BaseVM
	Name      string
	Subdomain string
	Domain    string // Primary domain for subdomain suffix display
	Error     template.HTML
}

// statsData is the view model for workspace statistics.
type statsData struct {
	viewdata.BaseVM
	WorkspaceID   string
	WorkspaceName string
	Subdomain     string
	Status        string
	CreatedAt     time.Time

	// Counts
	UserCount     int64
	AdminCount    int64
	AnalystCount  int64
	CoordCount    int64
	LeaderCount   int64
	MemberCount   int64
	OrgCount      int64
	GroupCount    int64
	ResourceCount int64
	MaterialCount int64
}

// deleteConfirmData is the view model for the delete confirmation modal.
type deleteConfirmData struct {
	viewdata.BaseVM
	WorkspaceID   string
	WorkspaceName string
	Subdomain     string
	UserCount     int64
	OrgCount      int64
	Error         template.HTML
}

// manageModalData is the view model for the workspace manage modal snippet.
type manageModalData struct {
	ID        string
	Name      string
	Subdomain string
	Status    string
	Domain    string // Primary domain for building workspace URLs
	CSRFToken string
}
