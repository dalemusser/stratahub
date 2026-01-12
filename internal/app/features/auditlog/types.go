// internal/app/features/auditlog/types.go
package auditlog

// Terminology: User Identifiers
//   - UserID / userID / user_id: The MongoDB ObjectID (_id) that uniquely identifies a user record
//   - LoginID / loginID / login_id: The human-readable string users type to log in

import (
	"time"

	"github.com/dalemusser/stratahub/internal/app/store/audit"
	"github.com/dalemusser/stratahub/internal/app/system/timezones"
	"github.com/dalemusser/stratahub/internal/app/system/viewdata"
)

// listItem represents a single audit event row for display.
type listItem struct {
	ID        string
	Timestamp time.Time
	Category  string
	EventType string
	ActorName string // Resolved from ActorID
	TargetName string // Resolved from UserID
	OrgName   string // Resolved from OrganizationID
	IP        string
	Success   bool
	Details   map[string]string
}

// listData is the view model for the audit log list page.
type listData struct {
	viewdata.BaseVM

	Items []listItem

	// Filters
	Category  string
	EventType string
	StartDate string
	EndDate   string

	// Filter options
	Categories []categoryOption
	EventTypes []string

	// Timezone selector
	TimezoneGroups []timezones.ZoneGroup

	// Pagination
	Page       int
	TotalPages int
	Total      int64
	Shown      int
	RangeStart int
	RangeEnd   int
	HasPrev    bool
	HasNext    bool
	PrevPage   int
	NextPage   int
}

// categoryOption represents a category for the filter dropdown.
type categoryOption struct {
	Value string
	Label string
}

// allCategories returns the available categories for filtering.
func allCategories() []categoryOption {
	return []categoryOption{
		{Value: audit.CategoryAuth, Label: "Authentication"},
		{Value: audit.CategoryAdmin, Label: "Administration"},
		// Security category has no events yet - add back when implemented
	}
}

// eventTypesForCategory returns the event types for a given category.
// If category is empty, returns all event types.
func eventTypesForCategory(category string) []string {
	authEvents := []string{
		audit.EventLoginSuccess,
		audit.EventLoginFailedUserNotFound,
		audit.EventLoginFailedWrongPassword,
		audit.EventLoginFailedUserDisabled,
		audit.EventLoginFailedRateLimit,
		audit.EventLogout,
		audit.EventPasswordChanged,
		audit.EventVerificationCodeSent,
		audit.EventVerificationCodeResent,
		audit.EventVerificationCodeFailed,
		audit.EventMagicLinkUsed,
	}

	adminEvents := []string{
		audit.EventUserCreated,
		audit.EventUserUpdated,
		audit.EventUserDisabled,
		audit.EventUserEnabled,
		audit.EventUserDeleted,
		audit.EventGroupCreated,
		audit.EventGroupUpdated,
		audit.EventGroupDeleted,
		audit.EventMemberAddedToGroup,
		audit.EventMemberRemovedFromGroup,
		audit.EventOrgCreated,
		audit.EventOrgUpdated,
		audit.EventOrgDeleted,
		audit.EventResourceCreated,
		audit.EventResourceUpdated,
		audit.EventResourceDeleted,
		audit.EventMaterialCreated,
		audit.EventMaterialUpdated,
		audit.EventMaterialDeleted,
		audit.EventResourceAssignedToGroup,
		audit.EventResourceAssignmentUpdated,
		audit.EventResourceUnassignedFromGroup,
		audit.EventCoordinatorAssignedToOrg,
		audit.EventCoordinatorUnassignedFromOrg,
		audit.EventMaterialAssigned,
		audit.EventMaterialAssignmentUpdated,
		audit.EventMaterialUnassigned,
	}

	switch category {
	case audit.CategoryAuth:
		return authEvents
	case audit.CategoryAdmin:
		return adminEvents
	case "":
		// Return all event types when no category selected
		all := make([]string, 0, len(authEvents)+len(adminEvents))
		all = append(all, authEvents...)
		all = append(all, adminEvents...)
		return all
	default:
		return nil
	}
}
