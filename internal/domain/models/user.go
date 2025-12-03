// internal/domain/models/user.go
package models

import (
	"time"

	"go.mongodb.org/mongo-driver/bson/primitive"
)

// User represents admins, leaders, and members.
//
// NOTE:
//   - Group membership is not embedded on User anymore.
//     Use the group_memberships collection to discover a user's groups.
type User struct {
	ID             primitive.ObjectID  `bson:"_id,omitempty" json:"id"`
	FullName       string              `bson:"full_name" json:"full_name"`
	FullNameCI     string              `bson:"full_name_ci" json:"full_name_ci"` // lowercase, diacritics-stripped
	Email          string              `bson:"email" json:"email"`
	AuthMethod     string              `bson:"auth_method,omitempty" json:"auth_method,omitempty"`
	Role           string              `bson:"role" json:"role"` // admin | leader | member
	Status         string              `bson:"status,omitempty" json:"status,omitempty"`
	OrganizationID *primitive.ObjectID `bson:"organization_id,omitempty" json:"organization_id,omitempty"`

	CreatedAt time.Time `bson:"created_at" json:"created_at"`
	UpdatedAt time.Time `bson:"updated_at" json:"updated_at"`
}
