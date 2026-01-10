// internal/domain/models/sitesettings.go
package models

import (
	"time"

	"go.mongodb.org/mongo-driver/bson/primitive"
)

// SiteSettings holds workspace-specific configuration that can be edited by admins.
// Each workspace has its own settings document (one document per workspace_id).
type SiteSettings struct {
	ID primitive.ObjectID `bson:"_id,omitempty" json:"id,omitempty"`

	// Workspace scoping - each workspace has its own settings
	WorkspaceID primitive.ObjectID `bson:"workspace_id" json:"workspace_id"`

	// Display settings
	SiteName string `bson:"site_name" json:"site_name"` // Name shown in menu header

	// Logo (file upload)
	LogoPath string `bson:"logo_path,omitempty" json:"logo_path,omitempty"` // Storage path for uploaded logo
	LogoName string `bson:"logo_name,omitempty" json:"logo_name,omitempty"` // Original filename

	// Footer
	FooterHTML string `bson:"footer_html,omitempty" json:"footer_html,omitempty"` // Custom HTML for footer

	// Authentication
	// EnabledAuthMethods is the list of auth methods enabled for this workspace.
	// If empty/nil, all methods from AllAuthMethods are enabled (default).
	EnabledAuthMethods []string `bson:"enabled_auth_methods,omitempty" json:"enabled_auth_methods,omitempty"`

	// Audit fields
	UpdatedAt     *time.Time          `bson:"updated_at,omitempty" json:"updated_at,omitempty"`
	UpdatedByID   *primitive.ObjectID `bson:"updated_by_id,omitempty" json:"updated_by_id,omitempty"`
	UpdatedByName string              `bson:"updated_by_name,omitempty" json:"updated_by_name,omitempty"`
}

// HasLogo returns true if a logo has been uploaded.
func (s *SiteSettings) HasLogo() bool {
	return s.LogoPath != ""
}

// GetEnabledAuthMethods returns the enabled auth methods for this workspace.
// If none are configured, returns all methods from AllAuthMethods (default behavior).
func (s *SiteSettings) GetEnabledAuthMethods() []AuthMethod {
	if len(s.EnabledAuthMethods) == 0 {
		return AllAuthMethods
	}
	// Filter AllAuthMethods to only those in EnabledAuthMethods
	enabledSet := make(map[string]bool)
	for _, m := range s.EnabledAuthMethods {
		enabledSet[m] = true
	}
	var result []AuthMethod
	for _, m := range AllAuthMethods {
		if enabledSet[m.Value] {
			result = append(result, m)
		}
	}
	return result
}

// IsAuthMethodEnabled checks if a specific auth method is enabled for this workspace.
// If no methods are configured, all valid methods are considered enabled (default).
func (s *SiteSettings) IsAuthMethodEnabled(method string) bool {
	if len(s.EnabledAuthMethods) == 0 {
		return IsValidAuthMethod(method)
	}
	for _, m := range s.EnabledAuthMethods {
		if m == method {
			return true
		}
	}
	return false
}

// DefaultSiteName is the default site name used when settings don't exist.
const DefaultSiteName = "StrataHub"
