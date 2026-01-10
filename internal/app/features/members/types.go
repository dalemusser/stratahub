// internal/app/features/members/types.go
package members

import (
	"html/template"

	userstore "github.com/dalemusser/stratahub/internal/app/store/users"
	"github.com/dalemusser/stratahub/internal/app/system/authutil"
	"github.com/dalemusser/stratahub/internal/app/system/formutil"
	"github.com/dalemusser/stratahub/internal/app/system/orgutil"
	"github.com/dalemusser/stratahub/internal/app/system/viewdata"
	"github.com/dalemusser/stratahub/internal/domain/models"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

// memberInput defines validation rules for creating or editing a member.
// Note: LoginID is validated conditionally in handlers based on auth method.
type memberInput struct {
	FullName string `validate:"required,max=200" label:"Full name"`
}

// Table row for members list
type memberRow struct {
	ID       primitive.ObjectID
	FullName string
	LoginID  string
	OrgName  string
	Status   string
}

// List page VM
type listData struct {
	viewdata.BaseVM

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
}

// Common aux types for forms
type orgOption struct {
	ID   primitive.ObjectID
	Name string
}

type newData struct {
	formutil.Base

	AuthMethods []models.AuthMethod

	// org picker state
	OrgLocked bool
	OrgHex    string
	OrgName   string // display name when org is locked
	Orgs      []orgOption

	// form fields
	FullName     string
	LoginID      string
	Email        string
	AuthReturnID string
	Auth         string
	TempPassword string
	Status       string

	IsEdit bool // false for new forms
}

// Template helper methods for auth field visibility
func (d newData) EmailIsLoginMethod() bool       { return authutil.EmailIsLogin(d.Auth) }
func (d newData) RequiresAuthReturnIDMethod() bool { return authutil.RequiresAuthReturnID(d.Auth) }
func (d newData) IsPasswordMethod() bool         { return d.Auth == "password" }

type editData struct {
	formutil.Base

	AuthMethods []models.AuthMethod

	ID       string
	FullName string
	LoginID  string
	Email    string

	AuthReturnID string
	Auth         string
	TempPassword string

	OrgID   string
	OrgName string
	Status  string

	IsEdit bool // true for edit forms

	// Legacy method warning: shown when user's current auth method is disabled for workspace
	AuthMethodDisabled      bool
	AuthMethodDisabledLabel string
}

// Template helper methods for auth field visibility
func (d editData) EmailIsLoginMethod() bool       { return authutil.EmailIsLogin(d.Auth) }
func (d editData) RequiresAuthReturnIDMethod() bool { return authutil.RequiresAuthReturnID(d.Auth) }
func (d editData) IsPasswordMethod() bool         { return d.Auth == "password" }

type viewData struct {
	viewdata.BaseVM
	ID, FullName, LoginID string
	OrgName, Status, Auth string
}

type uploadData struct {
	formutil.Base
	OrgLocked bool
	OrgHex    string
	OrgName   string

	// ReturnURL is the original return destination (e.g., dashboard).
	// Used for cancel links on preview and Done button on summary.
	ReturnURL string

	// Enabled auth methods for CSV format display
	CSVAuthMethods models.EnabledAuthMethodsForCSV

	// Preview mode: show what will happen before confirm
	ShowPreview   bool
	PreviewRows   []uploadPreviewRow
	PreviewJSON   string // JSON-encoded preview data for confirmation form
	TotalToCreate int
	TotalToUpdate int

	// Summary mode: show results after confirm
	ShowSummary    bool
	Created        int
	Updated        int
	SkippedCount   int
	CreatedMembers []userstore.MemberSummary
	UpdatedMembers []userstore.MemberSummary
	SkippedMembers []userstore.SkippedMember
	Success        template.HTML
}

// uploadPreviewRow represents a single member row for preview display.
type uploadPreviewRow struct {
	FullName     string
	LoginID      string
	AuthMethod   string
	Email        string // display only (may be empty)
	AuthReturnID string // display only (may be empty)
	IsNew        bool   // true if will be created, false if will be updated
}

// memberManageModalData is used to render the Manage Member modal.
type memberManageModalData struct {
	MemberID string
	FullName string
	LoginID  string
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
