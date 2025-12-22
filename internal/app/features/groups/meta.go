// internal/app/features/groups/meta.go
package groups

import (
	"strings"
	"time"

	"github.com/dalemusser/stratahub/internal/app/system/formutil"
	"github.com/dalemusser/stratahub/internal/app/system/orgutil"
)

// Type aliases for shared org/leader option types used in templates.
type orgOption = orgutil.OrgOption
type leaderOption = orgutil.LeaderOption

// newGroupData is the view model for the "Add Group" page.
type newGroupData struct {
	formutil.Base

	// Admin: list of orgs; Leader: their org shown read-only
	Organizations []orgOption
	LeaderOrgID   string
	LeaderOrgName string

	// Admin only: possible leaders across active orgs (filtered on the page)
	Leaders []leaderOption

	// Echo-on-error fields
	Name           string
	Description    string
	OrgHex         string
	SelectedLeader map[string]bool
}

// editGroupData is the view model for the Edit Group page.
type editGroupData struct {
	formutil.Base

	GroupID          string
	Name             string
	Description      string
	OrganizationID   string
	OrganizationName string
}

// assignedResourceViewItem is used on the read-only group view for
// listing resources assigned to a group.
type assignedResourceViewItem struct {
	ResourceID, ResourceTitle string
}

// groupViewData is the view model for the View Group page.
type groupViewData struct {
	Title      string
	IsLoggedIn bool
	Role       string
	UserName   string

	GroupID          string
	Name             string
	Description      string
	OrganizationName string
	LeadersCount     int
	MembersCount     int
	CreatedAt        time.Time
	UpdatedAt        time.Time

	AssignedResources []assignedResourceViewItem

	BackURL     string
	CurrentPath string
}

// groupResourceViewData is the view model for viewing a single resource
// in the context of a group.
type groupResourceViewData struct {
	Title      string
	IsLoggedIn bool
	Role       string
	UserName   string

	GroupID   string
	GroupName string

	ResourceID    string
	ResourceTitle string
	Subject       string
	Description   string
	Status        string
	LaunchURL     string

	BackURL     string
	CurrentPath string
}

// toSet converts a slice of strings into a set (map[string]bool) with
// whitespace trimmed and empties removed.
func toSet(vals []string) map[string]bool {
	m := make(map[string]bool, len(vals))
	for _, v := range vals {
		v = strings.TrimSpace(v)
		if v != "" {
			m[v] = true
		}
	}
	return m
}
