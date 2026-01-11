// internal/app/features/leaders/types.go
package leaders

// Terminology: User Identifiers
//   - UserID / userID / user_id: The MongoDB ObjectID (_id) that uniquely identifies a user record
//   - LoginID / loginID / login_id: The human-readable string users type to log in

import (
	"github.com/dalemusser/stratahub/internal/app/system/orgutil"
	"github.com/dalemusser/stratahub/internal/app/system/viewdata"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

// leaderRow represents a single leader in the list.
type leaderRow struct {
	ID          primitive.ObjectID
	FullName    string
	LoginID     string
	OrgName     string
	GroupsCount int
	Auth        string
	Status      string
}

// listData is the view model for the leaders list page.
type listData struct {
	viewdata.BaseVM

	// left pane
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
	AllCount      int64 // total leaders across all orgs

	// leaders table
	SearchQuery string
	Status      string
	Shown       int
	Total       int64
	RangeStart  int
	RangeEnd    int
	HasPrev     bool
	HasNext     bool
	PrevCursor  string
	NextCursor  string
	PrevStart   int
	NextStart   int
	Rows        []leaderRow
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

// leaderListResult holds the result of fetching a paginated leaders list.
type leaderListResult struct {
	Rows       []leaderRow
	Total      int64
	Shown      int
	HasPrev    bool
	HasNext    bool
	PrevCursor string
	NextCursor string
	RangeStart int
	RangeEnd   int
	PrevStart  int
	NextStart  int
}
