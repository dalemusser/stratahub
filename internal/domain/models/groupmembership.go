// internal/domain/models/groupmembership.go
package models

// Terminology: User Identifiers
//   - UserID / userID / user_id: The MongoDB ObjectID (_id) that uniquely identifies a user record
//   - LoginID / loginID / login_id: The human-readable string users type to log in

import (
	"time"

	"go.mongodb.org/mongo-driver/bson/primitive"
)

// GroupMembership is the authoritative join between users and groups.
// Exactly one document per (user_id, group_id); role is a scalar ("leader"|"member").
type GroupMembership struct {
	ID          primitive.ObjectID `bson:"_id,omitempty" json:"id"`
	WorkspaceID primitive.ObjectID `bson:"workspace_id" json:"workspace_id"` // Parent workspace
	GroupID     primitive.ObjectID `bson:"group_id" json:"group_id"`
	UserID      primitive.ObjectID `bson:"user_id" json:"user_id"`
	OrgID       primitive.ObjectID `bson:"org_id" json:"org_id"`
	Role        string             `bson:"role" json:"role"` // "leader" | "member"
	CreatedAt   time.Time          `bson:"created_at" json:"created_at"`
}
