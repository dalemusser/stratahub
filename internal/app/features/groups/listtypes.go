// internal/app/features/groups/listtypes.go
package groups

import (
	"github.com/dalemusser/stratahub/internal/app/system/orgutil"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

// groupManageModalData is the view model for the group manage modal snippet.
type groupManageModalData struct {
	GroupID          string
	GroupName        string
	OrganizationName string
	BackURL          string
}

// groupListItem represents a single group row in the list.
type groupListItem struct {
	ID                     primitive.ObjectID
	Name                   string
	OrganizationName       string
	LeadersCount           int
	MembersCount           int
	AssignedResourcesCount int
}

// groupListData is the view model for the groups list page.
type groupListData struct {
	Title, SearchQuery string
	IsLoggedIn         bool
	Role, UserName     string

	// Left org pane (admins only)
	ShowOrgPane   bool
	OrgQuery      string
	OrgShown      int
	OrgTotal      int64
	OrgHasPrev    bool
	OrgHasNext    bool
	OrgPrevCur    string
	OrgNextCur    string
	OrgRangeStart int
	OrgRangeEnd   int
	SelectedOrg   string // "all" or hex
	OrgRows       []orgutil.OrgRow
	AllCount      int64

	// Right table
	Shown      int
	Total      int64
	HasPrev    bool
	HasNext    bool
	PrevCursor string
	NextCursor string
	Groups     []groupListItem

	// groups range + page-index starts
	RangeStart int
	RangeEnd   int
	PrevStart  int
	NextStart  int

	CurrentPath string
}

// orgPaneData holds all the data needed to render the org pane.
type orgPaneData struct {
	Rows       []orgutil.OrgRow
	Total      int64
	HasPrev    bool
	HasNext    bool
	PrevCursor string
	NextCursor string
	RangeStart int
	RangeEnd   int
	AllCount   int64
}
