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

	// Audit fields
	UpdatedAt     *time.Time          `bson:"updated_at,omitempty" json:"updated_at,omitempty"`
	UpdatedByID   *primitive.ObjectID `bson:"updated_by_id,omitempty" json:"updated_by_id,omitempty"`
	UpdatedByName string              `bson:"updated_by_name,omitempty" json:"updated_by_name,omitempty"`
}

// HasLogo returns true if a logo has been uploaded.
func (s *SiteSettings) HasLogo() bool {
	return s.LogoPath != ""
}

// DefaultSiteName is the default site name used when settings don't exist.
const DefaultSiteName = "StrataHub"
