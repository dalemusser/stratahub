// internal/app/features/systemusers/types.go
package systemusers

import (
	"html/template"

	"github.com/dalemusser/stratahub/internal/app/system/viewdata"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

// Row used in the system users list.
type userRow struct {
	ID       primitive.ObjectID
	FullName string
	Email    string
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

// Form view model for New/Edit system user.
type formData struct {
	viewdata.BaseVM

	ID       string
	FullName string
	Email    string
	URole    string // legacy
	UserRole string // preferred
	Auth     string
	Status   string

	IsSelf bool

	Error template.HTML
}

// View-only model for the "View System User" page.
type viewData struct {
	viewdata.BaseVM

	ID       string
	FullName string
	Email    string
	URole    string
	UserRole string
	Auth     string
	Status   string
}

// Used by the Manage modal.
type manageModalData struct {
	ID       string
	FullName string
	Email    string
	Role     string
	Auth     string
	Status   string
	BackURL  string
}
