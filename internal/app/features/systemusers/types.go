// internal/app/features/systemusers/types.go
package systemusers

import (
	"html/template"

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
	Title      string
	IsLoggedIn bool
	Role       string
	UserName   string

	SearchQuery string
	Status      string // "", active, disabled
	URole       string // legacy
	UserRole    string // preferred

	Shown int
	Total int64

	HasPrev    bool
	HasNext    bool
	PrevCursor string
	NextCursor string

	Rows []userRow

	CurrentPath string
	Flash       template.HTML
}

// Form view model for New/Edit system user.
type formData struct {
	Title, Role, UserName string
	IsLoggedIn            bool

	ID       string
	FullName string
	Email    string
	URole    string // legacy
	UserRole string // preferred
	Auth     string
	Status   string

	IsSelf bool

	Error       template.HTML
	BackURL     string
	CurrentPath string
}

// View-only model for the “View System User” page.
type viewData struct {
	Title, Role, UserName string
	IsLoggedIn            bool

	ID       string
	FullName string
	Email    string
	URole    string
	UserRole string
	Auth     string
	Status   string

	BackURL     string
	CurrentPath string
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
