// internal/app/features/resources/types.go
package resources

// Terminology: User Identifiers
//   - UserID / userID / user_id: The MongoDB ObjectID (_id) that uniquely identifies a user record
//   - LoginID / loginID / login_id: The human-readable string users type to log in

import (
	"html/template"

	"github.com/dalemusser/stratahub/internal/app/system/timezones"
	"github.com/dalemusser/stratahub/internal/app/system/viewdata"
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
	viewdata.BaseVM

	Q     string
	Items []listItem

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

	// Permission flags
	CanCreate bool // True for admin; false for coordinator (view-only)
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
	CanEdit       bool   // True for admin; false for coordinator (view-only)
	CSRFToken     string
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
	viewdata.BaseVM

	ID            string
	ResourceTitle string
	Subject       string
	Description   string
	LaunchURL     string
	Type          string

	Status              string
	ShowInLibrary       bool
	DefaultInstructions string

	// File upload fields
	HasFile  bool
	FileName string
	FileSize int64

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
	viewdata.BaseVM

	ID                  string
	ResourceTitle       string
	Subject             string
	Description         string
	LaunchURL           string
	Type                string
	Status              string
	ShowInLibrary       bool
	DefaultInstructions template.HTML // HTML content, sanitized for safe rendering

	// File fields
	HasFile  bool
	FileName string
	FileSize int64

	// Permission flags
	CanEdit bool // True for admin; false for coordinator (view-only)
}

// ========================= MEMBER VIEW MODELS ===============================

// MemberResourcesHandler is the handler for member-facing resource pages
// ("My Resources" list and individual resource view). The concrete handler
// struct is defined in handler.go; these are the shared view-model types.

// common holds fields shared by the member list and detail pages.
type common struct {
	viewdata.BaseVM
	UserID string // login id used by the member
}

// resourceListItem is a single row in the member "My Resources" list.
type resourceListItem struct {
	ID             string
	Title          string
	Subject        string
	Type           string
	LaunchURL      string
	HasFile        bool
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
	DefaultInstructions template.HTML // HTML content, sanitized for safe rendering
	DisplayURL          string        // Original URL for display (without tracking params)
	LaunchURL           string        // URL with tracking params (group, id, org) for the Open button
	HasFile             bool
	FileName            string
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
