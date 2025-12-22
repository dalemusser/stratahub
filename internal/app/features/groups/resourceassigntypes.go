// internal/app/features/groups/resourceassigntypes.go
package groups

import "html/template"

// assignedResourceItem represents a single assigned resource in the list view.
type assignedResourceItem struct {
	AssignmentID      string
	ResourceID        string
	Title             string
	Subject           string
	Type              string
	Status            string
	Availability      string
	AvailabilityTitle string
}

// availableResourceItem represents a single available resource that can be assigned.
type availableResourceItem struct {
	ResourceID string
	Title      string
	Subject    string
	Type       string
	Status     string
}

// assignmentListData is the view model for the Assign Resources page.
type assignmentListData struct {
	Title      string
	IsLoggedIn bool
	Role       string
	UserName   string

	GroupID   string
	GroupName string

	Assigned  []assignedResourceItem
	Available []availableResourceItem

	AvailableShown int
	AvailableTotal int64

	Query       string
	TypeFilter  string
	TypeOptions []string

	CurrentAfter  string
	CurrentBefore string
	NextCursor    string
	PrevCursor    string
	HasNext       bool
	HasPrev       bool

	Flash       template.HTML
	BackURL     string
	CurrentPath string
}

// manageAssignmentModalVM is used to render the modal for managing an existing assignment.
type manageAssignmentModalVM struct {
	AssignmentID  string
	GroupID       string
	GroupName     string
	ResourceID    string
	ResourceTitle string
	ResourceType  string

	BackURL string
}
