// internal/app/features/groups/resourceassigntypes.go
package groups

import (
	"html/template"

	"github.com/dalemusser/stratahub/internal/app/system/viewdata"
)

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
	viewdata.BaseVM

	GroupID   string
	GroupName string

	Assigned  []assignedResourceItem
	Available []availableResourceItem

	AvailableShown      int
	AvailableTotal      int64
	AvailableRangeStart int
	AvailableRangeEnd   int
	NextStart           int
	PrevStart           int

	Query       string
	TypeFilter  string
	TypeOptions []string

	CurrentAfter  string
	CurrentBefore string
	NextCursor    string
	PrevCursor    string
	HasNext       bool
	HasPrev       bool

	Flash template.HTML
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
