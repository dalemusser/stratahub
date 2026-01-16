// internal/app/system/viewdata/viewdata.go
package viewdata

import (
	"context"
	"html/template"
	"net/http"

	settingsstore "github.com/dalemusser/stratahub/internal/app/store/settings"
	"github.com/dalemusser/stratahub/internal/app/system/auth"
	"github.com/dalemusser/stratahub/internal/app/system/authz"
	"github.com/dalemusser/stratahub/internal/app/system/timeouts"
	"github.com/dalemusser/stratahub/internal/app/system/workspace"
	"github.com/dalemusser/stratahub/internal/domain/models"
	"github.com/dalemusser/waffle/pantry/httpnav"
	"github.com/dalemusser/waffle/pantry/storage"
	"github.com/gorilla/csrf"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
)

// AnnouncementVM represents an announcement for display in templates.
type AnnouncementVM struct {
	ID          string
	Title       string
	Content     string
	Type        string // info, warning, critical
	Dismissible bool
}

// BaseVM contains common fields for all view models.
// Embed this struct in your feature-specific view models.
//
// Usage:
//
//	type myPageData struct {
//	    viewdata.BaseVM
//	    // page-specific fields...
//	}
//
//	data := myPageData{
//	    BaseVM: viewdata.NewBaseVM(r, db, "Page Title", "/default-back"),
//	    // page-specific fields...
//	}
type BaseVM struct {
	// Site settings (from database)
	SiteName   string
	LogoURL    string
	FooterHTML template.HTML

	// User context (from auth middleware)
	IsLoggedIn bool
	Role       string
	UserName   string
	UserOrg    string // Organization name for leaders/members

	// Workspace context
	IsApex bool // true if on apex domain (no workspace)

	// Page context
	Title       string
	BackURL     string
	CurrentPath string

	// CSRF protection
	CSRFToken string // Token for form submission

	// Announcements for banner display
	Announcements []AnnouncementVM
}

// storageProvider is set by Init and used to generate logo URLs.
var storageProvider storage.Store

// AnnouncementLoader is a function that loads active announcements.
// This is set by bootstrap to avoid circular dependencies.
type AnnouncementLoader func(ctx context.Context) []AnnouncementVM

var announcementLoader AnnouncementLoader

// Init sets the storage provider for generating logo URLs.
// Call this once at startup from bootstrap.
func Init(store storage.Store) {
	storageProvider = store
}

// SetAnnouncementLoader sets the function used to load active announcements.
// Call this once at startup from bootstrap after the announcement store is available.
func SetAnnouncementLoader(loader AnnouncementLoader) {
	announcementLoader = loader
}

// NewBaseVM creates a fully populated BaseVM for a page.
// This is the preferred way to create a BaseVM for embedding in view models.
//
// Parameters:
//   - r: the HTTP request
//   - db: database for loading site settings (can be nil for defaults)
//   - title: the page title
//   - backDefault: default URL for the back button if none in request
func NewBaseVM(r *http.Request, db *mongo.Database, title, backDefault string) BaseVM {
	role, name, _, signedIn := authz.UserCtx(r)

	// Compute effective role for UI purposes
	// When superadmin is on a workspace subdomain (not apex), show admin UI
	effectiveRole := role
	ws := workspace.FromRequest(r)
	if role == "superadmin" && ws != nil && !ws.IsApex && !ws.ID.IsZero() {
		effectiveRole = "admin"
	}

	// Determine if we're on apex domain
	isApex := ws != nil && ws.IsApex

	vm := BaseVM{
		SiteName:    models.DefaultSiteName,
		IsLoggedIn:  signedIn,
		Role:        effectiveRole,
		UserName:    name,
		IsApex:      isApex,
		Title:       title,
		BackURL:     httpnav.ResolveBackURL(r, backDefault),
		CurrentPath: httpnav.CurrentPath(r),
		CSRFToken:   csrf.Token(r),
	}

	// Get organization name for leaders/members
	if user, ok := auth.CurrentUser(r); ok && user.OrganizationName != "" {
		vm.UserOrg = user.OrganizationName
	}

	if db != nil {
		ctx, cancel := context.WithTimeout(r.Context(), timeouts.Short())
		defer cancel()

		// Get workspace ID from context for workspace-scoped settings
		wsID := workspace.IDFromRequest(r)
		if wsID != primitive.NilObjectID {
			store := settingsstore.New(db)
			settings, err := store.Get(ctx, wsID)
			if err == nil {
				vm.SiteName = settings.SiteName
				vm.FooterHTML = template.HTML(settings.FooterHTML)
				if settings.HasLogo() && storageProvider != nil {
					vm.LogoURL = storageProvider.URL(settings.LogoPath)
				}
			}
		}
	}

	// Load active announcements if loader is configured
	if announcementLoader != nil {
		vm.Announcements = announcementLoader(r.Context())
	}

	return vm
}

// LoadBase populates a BaseVM with site settings and user info from the request context.
// Pass db=nil if you don't need site settings (will use defaults).
//
// Deprecated: Use NewBaseVM instead, which also sets Title, BackURL, and CurrentPath.
func LoadBase(r *http.Request, db *mongo.Database) BaseVM {
	return NewBaseVM(r, db, "", "")
}

// GetSiteName returns the site name from settings, or the default if not available.
func GetSiteName(ctx context.Context, db *mongo.Database, wsID primitive.ObjectID) string {
	if db == nil || wsID == primitive.NilObjectID {
		return models.DefaultSiteName
	}

	store := settingsstore.New(db)
	settings, err := store.Get(ctx, wsID)
	if err != nil {
		return models.DefaultSiteName
	}
	return settings.SiteName
}

// GetSettings returns the full site settings, or defaults if not available.
func GetSettings(ctx context.Context, db *mongo.Database, wsID primitive.ObjectID) models.SiteSettings {
	if db == nil || wsID == primitive.NilObjectID {
		return models.SiteSettings{SiteName: models.DefaultSiteName}
	}

	store := settingsstore.New(db)
	settings, err := store.Get(ctx, wsID)
	if err != nil {
		return models.SiteSettings{SiteName: models.DefaultSiteName}
	}
	return settings
}
