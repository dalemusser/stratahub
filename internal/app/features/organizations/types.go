// internal/app/features/organizations/types.go
package organizations

import (
	"github.com/dalemusser/stratahub/internal/app/system/formutil"
	"github.com/dalemusser/stratahub/internal/app/system/timezones"
	"github.com/dalemusser/stratahub/internal/domain/models"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

// listItem is a single row in the organizations list.
type listItem struct {
	ID           primitive.ObjectID
	Name         string
	NameCI       string // case-insensitive name for cursor building
	City         string
	State        string
	LeadersCount int64
	GroupsCount  int64
}

// listData is the view model for the organizations list page.
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

// orgManageModalData is used for the HTMX “Manage Organization” modal.
type orgManageModalData struct {
	ID      string
	Name    string
	BackURL string
}

// newData is the view model for the "New Organization" page.
type newData struct {
	formutil.Base

	Name     string
	City     string
	State    string
	TimeZone string
	Contact  string

	TimeZoneGroups []timezones.ZoneGroup
}

// viewData is the view model for the “View Organization” page.
type viewData struct {
	Title      string
	IsLoggedIn bool
	Role       string
	UserName   string

	ID       string
	Name     string
	City     string
	State    string
	TimeZone string
	Contact  string
	BackURL  string

	TimeZoneGroups []timezones.ZoneGroup
}

// editData is the view model for the "Edit Organization" page.
type editData struct {
	formutil.Base

	ID       string
	Name     string
	City     string
	State    string
	TimeZone string
	Contact  string

	TimeZoneGroups []timezones.ZoneGroup
}

// Organization convenience aliases if you want them:
type Organization = models.Organization
