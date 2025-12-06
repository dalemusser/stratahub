// internal/app/features/members/types.go
package members

import (
	"html/template"
	"time"

	"go.mongodb.org/mongo-driver/bson/primitive"
)

// Timeouts for member handlers.
const (
	membersShortTimeout = 5 * time.Second
	membersMedTimeout   = 10 * time.Second
	membersLongTimeout  = 60 * time.Second
)

// Left-pane organization row
type orgRow struct {
	ID    primitive.ObjectID
	Name  string
	Count int64
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
	OrgRows       []orgRow
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
	Title, Role, UserName string
	IsLoggedIn            bool
	BackURL, CurrentPath  string

	// org picker state
	OrgLocked bool
	OrgHex    string
	OrgName   string // NEW: display name when org is locked
	Orgs      []orgOption

	// form echo-on-error
	FullName string
	Email    string
	Auth     string
	Status   string
	Error    template.HTML
}

type editData struct {
	Title, Role, UserName string
	IsLoggedIn            bool

	ID, FullName, Email          string
	OrgID, OrgName, Status, Auth string

	BackURL, CurrentPath string
	Orgs                 []orgOption
	Error                template.HTML
}

type viewData struct {
	Title, Role, UserName string
	IsLoggedIn            bool
	ID, FullName, Email   string
	OrgName, Status, Auth string
	BackURL               string
}

type uploadData struct {
	Title, Role, UserName string
	IsLoggedIn            bool
	BackURL, CurrentPath  string
	OrgLocked             bool
	OrgHex                string
	OrgName               string
	Error                 template.HTML
	Success               template.HTML
	Created               int
	Previously            int
	SkippedCount          int
	SkippedEmails         []string
}

// memberManageModalData is used to render the Manage Member modal.
type memberManageModalData struct {
	MemberID string
	FullName string
	Email    string
	OrgName  string
	BackURL  string
}
