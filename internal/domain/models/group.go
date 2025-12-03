// internal/domain/models/group.go
package models

import (
	"time"

	"go.mongodb.org/mongo-driver/bson/primitive"
)

// Group represents a cohort (class) inside an organization.
//
// NOTE:
//   - Member/leader lists are no longer embedded on Group.
//     All membership is stored in the group_memberships collection.
//   - Status is a first-class field on the group document
//     (e.g., "active").
type Group struct {
	ID             primitive.ObjectID `bson:"_id" json:"id"`
	Name           string             `bson:"name" json:"name"`
	NameCI         string             `bson:"name_ci" json:"name_ci"`
	Description    string             `bson:"description" json:"description"`
	OrganizationID primitive.ObjectID `bson:"organization_id" json:"organization_id"`

	Status string `bson:"status" json:"status"`

	CreatedAt time.Time `bson:"created_at" json:"created_at"`
	UpdatedAt time.Time `bson:"updated_at" json:"updated_at"`
}
