// internal/app/features/members/types.go
package members

import (
	"html/template"

	"github.com/dalemusser/stratahub/internal/app/system/formutil"
	"github.com/dalemusser/stratahub/internal/app/system/orgutil"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

// memberInput defines validation rules for creating or editing a member.
type memberInput struct {
	FullName string `validate:"required,max=200" label:"Full name"`
	Email    string `validate:"required,email,max=254" label:"Email"`
}

// Table row for members list
type memberRow struct {
	ID       primitive.ObjectID
	FullName string
	Email    string
	OrgName  string
	Status   string
}

// List page VM
type listData struct {
	Title, Role, UserName string
	IsLoggedIn            bool

	// org pane (admin only)
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

	// members list
	SearchQuery string
	Status      string
	Shown       int
	Total       int64
	HasPrev     bool
	HasNext     bool
	PrevCursor  string
	NextCursor  string
	RangeStart  int
	RangeEnd    int
	PrevStart   int
	NextStart   int
	MemberRows  []memberRow

	AllowUpload bool
	AllowAdd    bool

	BackURL     string
	CurrentPath string
}

// Common aux types for forms
type orgOption struct {
	ID   primitive.ObjectID
	Name string
}

type newData struct {
	formutil.Base

	// org picker state
	OrgLocked bool
	OrgHex    string
	OrgName   string // display name when org is locked
	Orgs      []orgOption

	// form echo-on-error
	FullName string
	Email    string
	Auth     string
	Status   string
}

type editData struct {
	formutil.Base

	ID, FullName, Email          string
	OrgID, OrgName, Status, Auth string
	Orgs                         []orgOption
}

type viewData struct {
	Title, Role, UserName string
	IsLoggedIn            bool
	ID, FullName, Email   string
	OrgName, Status, Auth string
	BackURL               string
}

type uploadData struct {
	formutil.Base
	OrgLocked     bool
	OrgHex        string
	OrgName       string
	Success       template.HTML
	Created       int
	Previously    int
	SkippedCount  int
	SkippedEmails []string
}

// memberManageModalData is used to render the Manage Member modal.
type memberManageModalData struct {
	MemberID string
	FullName string
	Email    string
	OrgName  string
	BackURL  string
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
