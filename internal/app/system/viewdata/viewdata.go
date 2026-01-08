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
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
)

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

	// Page context
	Title       string
	BackURL     string
	CurrentPath string
}

// storageProvider is set by Init and used to generate logo URLs.
var storageProvider storage.Store

// Init sets the storage provider for generating logo URLs.
// Call this once at startup from bootstrap.
func Init(store storage.Store) {
	storageProvider = store
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

	vm := BaseVM{
		SiteName:    models.DefaultSiteName,
		IsLoggedIn:  signedIn,
		Role:        effectiveRole,
		UserName:    name,
		Title:       title,
		BackURL:     httpnav.ResolveBackURL(r, backDefault),
		CurrentPath: httpnav.CurrentPath(r),
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
