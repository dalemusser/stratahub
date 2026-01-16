// internal/app/features/systemusers/types.go
package systemusers

// Terminology: User Identifiers
//   - UserID / userID / user_id: The MongoDB ObjectID (_id) that uniquely identifies a user record
//   - LoginID / loginID / login_id: The human-readable string users type to log in

import (
	"html/template"

	"github.com/dalemusser/stratahub/internal/app/system/authutil"
	"github.com/dalemusser/stratahub/internal/app/system/viewdata"
	"github.com/dalemusser/stratahub/internal/domain/models"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

// Row used in the system users list.
type userRow struct {
	ID       primitive.ObjectID
	FullName string
	LoginID  string
	Role     string
	Auth     string
	Status   string
}

// View model for the system users list page.
type listData struct {
	viewdata.BaseVM

	SearchQuery string
	Status      string // "", active, disabled
	URole       string // legacy
	UserRole    string // preferred

	Shown      int
	Total      int64
	RangeStart int
	RangeEnd   int
	PrevStart  int
	NextStart  int

	HasPrev    bool
	HasNext    bool
	PrevCursor string
	NextCursor string

	Rows []userRow

	Flash template.HTML
}

// CoordinatorOrg represents an organization assigned to a coordinator.
type CoordinatorOrg struct {
	ID   string
	Name string
}

// Form view model for New/Edit system user.
type formData struct {
	viewdata.BaseVM

	AuthMethods []models.AuthMethod

	ID       string
	FullName string
	LoginID  string
	Email    string
	URole    string // legacy
	UserRole string // preferred
	Auth     string
	Status   string

	// Auth fields for conditional display
	AuthReturnID string
	TempPassword string
	IsEdit       bool

	IsSelf bool

	// Coordinator organization assignments (for edit form)
	CoordinatorOrgs []CoordinatorOrg

	// Coordinator-specific permissions
	CanManageMaterials bool
	CanManageResources bool

	// Previous role (for detecting role changes in edit)
	PreviousRole string

	Error template.HTML
}

// Template helper methods for auth field visibility
func (d formData) EmailIsLoginMethod() bool       { return authutil.EmailIsLogin(d.Auth) }
func (d formData) RequiresAuthReturnIDMethod() bool { return authutil.RequiresAuthReturnID(d.Auth) }
func (d formData) IsPasswordMethod() bool         { return d.Auth == "password" }

// View-only model for the "View System User" page.
type viewData struct {
	viewdata.BaseVM

	ID       string
	FullName string
	LoginID  string
	URole    string
	UserRole string
	Auth     string
	Status   string
}

// Used by the Manage modal.
type manageModalData struct {
	ID        string
	FullName  string
	LoginID   string
	Role      string
	Auth      string
	Status    string
	BackURL   string
	CSRFToken string
	IsSelf    bool
}
