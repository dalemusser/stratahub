// internal/app/features/uploadcsv/types.go
package uploadcsv

import (
	"html/template"

	userstore "github.com/dalemusser/stratahub/internal/app/store/users"
	"github.com/dalemusser/stratahub/internal/app/system/viewdata"
	"github.com/dalemusser/stratahub/internal/domain/models"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

// UploadData is the view model for the CSV upload page.
// It supports both org-only mode (members upload) and group mode (groups upload).
type UploadData struct {
	viewdata.BaseVM

	// Organization selection
	OrgHex    string
	OrgName   string
	OrgLocked bool // true for leaders (org is fixed)

	// Group selection (only used in GroupMode)
	GroupMode   bool   // true = show group UI and add to group after import
	GroupID     string
	GroupName   string
	GroupLocked bool // true when group is pre-selected and can't change

	// ReturnURL is the original return destination.
	// Used for cancel links on preview and Done button on summary.
	ReturnURL string

	// Form state
	Error          template.HTML
	CSVAuthMethods models.EnabledAuthMethodsForCSV

	// Preview mode: show what will happen before confirm
	ShowPreview   bool
	PreviewRows   []PreviewRow
	PreviewJSON   string // JSON-encoded preview data for confirmation form
	TotalToCreate int
	TotalToUpdate int

	// Summary mode: show results after confirm
	ShowSummary    bool
	Created        int // new users created in org
	Updated        int // existing users updated
	SkippedCount   int // skipped (in different org)
	AddedToGroup   int // added to group (group mode only)
	AlreadyInGroup int // already in group (group mode only)
	CreatedMembers []userstore.MemberSummary
	UpdatedMembers []userstore.MemberSummary
	SkippedMembers []userstore.SkippedMember
}

// PreviewRow represents a single member row for preview display.
type PreviewRow struct {
	FullName     string
	LoginID      string
	AuthMethod   string
	Email        string // display only (may be empty)
	AuthReturnID string // display only (may be empty)
	IsNew        bool   // true if will be created, false if will be updated
}

// previewMember is the JSON-serializable representation of a parsed member
// for the preview/confirm flow. Stored in hidden form field.
type previewMember struct {
	FullName     string  `json:"fn"`
	LoginID      string  `json:"li"`
	AuthMethod   string  `json:"am"`
	Email        *string `json:"em,omitempty"`
	AuthReturnID *string `json:"ar,omitempty"`
	TempPassword *string `json:"tp,omitempty"`
}

// UploadContext holds validated org/group context for handlers.
type UploadContext struct {
	Role      string
	UserID    primitive.ObjectID
	OrgID     primitive.ObjectID
	OrgName   string
	OrgLocked bool // Leaders have org locked

	GroupMode   bool
	GroupID     primitive.ObjectID
	GroupName   string
	GroupLocked bool
}
