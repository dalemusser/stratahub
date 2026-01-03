package models

import (
	"time"

	"go.mongodb.org/mongo-driver/bson/primitive"
)

// Workspace represents a top-level tenant container in StrataHub.
// Each workspace is isolated from others and can have its own:
// - Organizations, groups, leaders, members
// - Resources and materials
// - Branding (name, logo, theme)
// - Subdomain (e.g., mhs.adroit.games)
//
// All major entities (orgs, groups, users, resources, materials, assignments)
// belong to exactly one workspace via their workspace_id field.
type Workspace struct {
	ID primitive.ObjectID `bson:"_id,omitempty" json:"id"`

	// Display name for the workspace
	Name   string `bson:"name" json:"name"`
	NameCI string `bson:"name_ci" json:"name_ci"` // Case-insensitive for search

	// Subdomain for this workspace (e.g., "mhs" for mhs.adroit.games)
	// Must be unique across all workspaces
	Subdomain string `bson:"subdomain" json:"subdomain"`

	// Branding
	LogoPath string `bson:"logo_path,omitempty" json:"logo_path,omitempty"` // Storage path for logo
	LogoName string `bson:"logo_name,omitempty" json:"logo_name,omitempty"` // Original filename

	// Status: "active" or "disabled"
	Status string `bson:"status" json:"status"`

	// Audit fields
	CreatedAt time.Time `bson:"created_at" json:"created_at"`
	UpdatedAt time.Time `bson:"updated_at" json:"updated_at"`
}

// HasLogo returns true if the workspace has a logo uploaded.
func (w Workspace) HasLogo() bool {
	return w.LogoPath != ""
}
