package reports

import (
	"github.com/dalemusser/stratahub/internal/app/system/orgutil"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

// groupRow represents a single group row in the middle pane of the
// members report. Count is the number of members in that group (after
// filters are applied).
type groupRow struct {
	ID    primitive.ObjectID
	Name  string
	Count int64
}

// orgPaneResult holds the data for the org pane in the members report.
type orgPaneResult struct {
	Rows       []orgutil.OrgRow
	Total      int64
	HasPrev    bool
	HasNext    bool
	PrevCursor string
	NextCursor string
	AllCount   int64 // total members across all orgs (respecting filters)
}

// groupsPaneResult holds the data for the groups pane in the members report.
type groupsPaneResult struct {
	Rows            []groupRow
	OrgMembersCount int64 // total members in selected org
	OrgName         string
}

// exportCountsResult holds the calculated export counts.
type exportCountsResult struct {
	ExportRecordCount    int64
	MembersInGroupsCount int64
}

// pageData is the view model for the HTML Members Report page. It
// mirrors the original strata_hub implementation, but is factored
// into this types file so the handler logic files stay focused on
// query / CSV logic.
type pageData struct {
	Title      string
	IsLoggedIn bool
	Role       string
	UserName   string

	// Optional back link
	ShowBack bool
	BackURL  string
	// URL-encoded return query-string fragment (e.g. "&return=/foo")
	ReturnQS string

	// Left org pane
	OrgQuery        string
	OrgShown        int
	OrgTotal        int64
	OrgHasPrev      bool
	OrgHasNext      bool
	OrgPrevCur      string
	OrgNextCur      string
	SelectedOrg     string // "all" or hex string
	SelectedOrgName string
	OrgRows         []orgutil.OrgRow
	AllCount        int64 // total members across all orgs (respecting filters)

	// Middle groups pane (only when an org is selected)
	SelectedGroup        string
	GroupRows            []groupRow
	OrgMembersCount      int64 // total members in selected org (respecting member_status)
	MembersInGroupsCount int64 // members that belong to at least one group in scope
	ExportRecordCount    int64 // number of CSV rows in export

	// Right controls (filters / filename)
	GroupStatus      string // kept for UI parity with template
	MemberStatus     string
	DownloadFilename string

	// Used by templates to build links and preserve filters
	CurrentPath string
}
