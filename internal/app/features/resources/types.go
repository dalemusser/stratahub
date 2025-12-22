// internal/app/features/resources/types.go
package resources

import (
	"html/template"

	"github.com/dalemusser/stratahub/internal/app/system/timezones"
	"github.com/dalemusser/stratahub/internal/domain/models"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

// ========================= ADMIN (LIBRARY) VIEW MODELS ======================

// AdminResourcesHandler is the top-level handler for admin Resource (Library) views.
// The concrete handler struct itself lives in handler.go; these are just the
// shared view-model types used by the admin templates.

// listItem is a summary row for display in the admin resources list.
type listItem struct {
	ID            primitive.ObjectID
	Title         string
	TitleCI       string // case-insensitive title for cursor building
	Subject       string
	Type          string
	Status        string
	ShowInLibrary bool
	Description   string
}

// listData provides template data for the admin resources list.
type listData struct {
	Title      string
	IsLoggedIn bool
	Role       string
	UserName   string

	Q           string
	Items       []listItem
	CurrentPath string

	// Pagination
	Shown      int
	Total      int64
	HasPrev    bool
	HasNext    bool
	PrevCursor string
	NextCursor string
	RangeStart int
	RangeEnd   int
	PrevStart  int
	NextStart  int
}

// manageModalData is used to render the admin "Manage Resource" modal.
type manageModalData struct {
	ID            string
	Title         string
	Subject       string
	Type          string
	Status        string
	ShowInLibrary bool
	Description   string
	BackURL       string
}

// ResourceTypeOption is used to populate the resource type select menu.
type ResourceTypeOption struct {
	ID    string
	Label string
}

// resourceFormVM is the unified form view-model used by the New and Edit
// admin flows. New and Edit handlers populate this and then render the
// corresponding templates.
type resourceFormVM struct {
	Title       string
	IsLoggedIn  bool
	Role        string
	UserName    string
	BackURL     string
	CurrentPath string

	ID            string
	ResourceTitle string
	Subject       string
	Description   string
	LaunchURL     string
	Type          string

	Status              string
	ShowInLibrary       bool
	DefaultInstructions string

	// Navigation / redirects
	SubmitReturn string
	DeleteReturn string

	// Error message to show above the form
	Error template.HTML

	// Populated with models.ResourceTypes as ID + label
	TypeOptions []ResourceTypeOption
}

// viewData is the view-only model for the admin resource detail page.
type viewData struct {
	Title               string
	IsLoggedIn          bool
	Role                string
	UserName            string
	ID                  string
	ResourceTitle       string
	Subject             string
	Description         string
	LaunchURL           string
	Type                string
	Status              string
	ShowInLibrary       bool
	DefaultInstructions string
	BackURL             string
}

// ========================= MEMBER VIEW MODELS ===============================

// MemberResourcesHandler is the handler for member-facing resource pages
// ("My Resources" list and individual resource view). The concrete handler
// struct is defined in handler.go; these are the shared view-model types.

// common holds fields shared by the member list and detail pages.
type common struct {
	Title      string
	IsLoggedIn bool
	Role       string
	UserName   string
	UserID     string // login id used by the member (email)
}

// resourceListItem is a single row in the member "My Resources" list.
type resourceListItem struct {
	ID             string
	Title          string
	Subject        string
	Type           string
	LaunchURL      string
	AvailableUntil string
}

// resourceListData is the view model for the member resources list.
type resourceListData struct {
	common
	Resources []resourceListItem
	TimeZone  string // human-readable label (e.g., "America/Chicago")
}

// viewResourceData is the view model for the member resource detail page.
type viewResourceData struct {
	common

	ResourceID          string
	ResourceTitle       string
	Subject             string
	Type                string
	TypeDisplay         string
	Description         string
	DefaultInstructions string
	LaunchURL           string
	Status              string
	AvailableUntil      string
	BackURL             string
	CanOpen             bool
	TimeZone            string
}

// Organization is a convenience alias; not strictly necessary but mirrors the
// original strata_hub code where Organization lived in the domain models.
type Organization = models.Organization

// Time zone groups are provided by the shared timezones package and used by
// other parts of the app (e.g., organizations and groups) when rendering
// select menus. They are imported here for completeness if needed by
// resource-related templates in the future.
var _ = timezones.ZoneGroup{}
