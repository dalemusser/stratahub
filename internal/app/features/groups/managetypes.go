// internal/app/features/groups/managetypes.go
package groups

// Terminology: User Identifiers
//   - UserID / userID / user_id: The MongoDB ObjectID (_id) that uniquely identifies a user record
//   - LoginID / loginID / login_id: The human-readable string users type to log in

import "github.com/dalemusser/stratahub/internal/app/system/viewdata"

// ManagePageData holds the full view model for the Manage Group page.
type ManagePageData struct {
	viewdata.BaseVM

	GroupID          string
	GroupName        string
	GroupDescription string
	OrganizationName string

	// For leader removal confirmation dialogs
	CurrentUserID string // Hex ID of the logged-in user
	LeaderCount   int    // Number of leaders in the group

	CurrentLeaders  []UserItem
	CurrentMembers  []UserItem
	PossibleLeaders []UserItem

	AvailableMembers []UserItem
	AvailableShown   int
	AvailableTotal   int64

	Query         string
	CurrentAfter  string
	CurrentBefore string
	NextCursor    string
	PrevCursor    string
	HasNext       bool
	HasPrev       bool
}

// UserItem is a simple view-model for a user row.
type UserItem struct {
	ID       string
	FullName string
	LoginID  string
}
