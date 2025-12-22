// internal/app/features/groups/managetypes.go
package groups

// ManagePageData holds the full view model for the Manage Group page.
type ManagePageData struct {
	// layout header
	Title      string
	IsLoggedIn bool
	Role       string
	UserName   string

	GroupID          string
	GroupName        string
	GroupDescription string
	OrganizationName string

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

	// Navigation niceties
	BackURL     string // where "Back" should go
	CurrentPath string // this page's path + query (used to propagate ?return=)
}

// UserItem is a simple view-model for a user row.
type UserItem struct {
	ID       string
	FullName string
	Email    string
}
