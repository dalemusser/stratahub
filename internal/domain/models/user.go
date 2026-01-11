// internal/domain/models/user.go
package models

// Terminology: User Identifiers
//   - UserID / userID / user_id: The MongoDB ObjectID (_id) that uniquely identifies a user record
//   - LoginID / loginID / login_id: The human-readable string users type to log in

import (
	"time"

	"go.mongodb.org/mongo-driver/bson/primitive"
)

// User represents admins, analysts, coordinators, leaders, and members.
//
// NOTE:
//   - Group membership is not embedded on User anymore.
//     Use the group_memberships collection to discover a user's groups.
//   - Coordinator organization assignments are stored in coordinator_assignments collection.
//
// Auth fields:
//   - LoginID: What the user types to identify themselves (stored lowercase)
//   - LoginIDCI: Case/diacritic-insensitive version for matching (folded)
//   - AuthReturnID: What the auth provider returns after authentication
//   - Email: Contact email (optional, stored lowercase)
//   - AuthMethod: google, microsoft, classlink, clever, email, password, trust
type User struct {
	ID             primitive.ObjectID  `bson:"_id,omitempty" json:"id"`
	WorkspaceID    *primitive.ObjectID `bson:"workspace_id,omitempty" json:"workspace_id,omitempty"` // Parent workspace (nil for super-admins)
	FullName       string              `bson:"full_name" json:"full_name"`
	FullNameCI     string              `bson:"full_name_ci" json:"full_name_ci"` // lowercase, diacritics-stripped

	// Authentication fields
	LoginID      *string `bson:"login_id" json:"login_id"`             // User identifier (lowercase)
	LoginIDCI    *string `bson:"login_id_ci" json:"login_id_ci"`       // Folded for case/diacritic-insensitive matching
	AuthReturnID *string `bson:"auth_return_id" json:"auth_return_id"` // Provider's return identifier
	Email        *string `bson:"email" json:"email"`                   // Contact email (lowercase, optional)
	AuthMethod   string  `bson:"auth_method" json:"auth_method"`       // google, microsoft, classlink, clever, email, password, trust

	// Password auth fields
	PasswordHash *string `bson:"password_hash,omitempty" json:"-"`    // bcrypt hash (never in JSON)
	PasswordTemp *bool   `bson:"password_temp,omitempty" json:"-"`    // true if must change on next login

	Role           string              `bson:"role" json:"role"` // admin | analyst | coordinator | leader | member
	Status         string              `bson:"status,omitempty" json:"status,omitempty"`
	OrganizationID *primitive.ObjectID `bson:"organization_id,omitempty" json:"organization_id,omitempty"` // For leaders/members (single org)

	// Coordinator-specific permissions (only relevant when Role == "coordinator")
	CanManageMaterials bool `bson:"can_manage_materials,omitempty" json:"can_manage_materials,omitempty"`
	CanManageResources bool `bson:"can_manage_resources,omitempty" json:"can_manage_resources,omitempty"`

	// User preferences
	ThemePreference string `bson:"theme_preference,omitempty" json:"theme_preference,omitempty"` // light, dark, system (empty = system)

	CreatedAt time.Time `bson:"created_at" json:"created_at"`
	UpdatedAt time.Time `bson:"updated_at" json:"updated_at"`
}
