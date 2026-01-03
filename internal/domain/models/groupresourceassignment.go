// internal/domain/models/groupresourceassignment.go
package models

import (
	"time"

	"go.mongodb.org/mongo-driver/bson/primitive"
)

// GroupResourceAssignment represents a single resource assigned to a group.
//
// It models a document in the `group_resource_assignments` collection and
// includes scheduling, instructions, and basic audit fields.
type GroupResourceAssignment struct {
	ID             primitive.ObjectID `bson:"_id,omitempty" json:"id"`
	WorkspaceID    primitive.ObjectID `bson:"workspace_id" json:"workspace_id"` // Parent workspace
	GroupID        primitive.ObjectID `bson:"group_id" json:"group_id"`
	OrganizationID primitive.ObjectID `bson:"organization_id" json:"organization_id"`
	ResourceID     primitive.ObjectID `bson:"resource_id" json:"resource_id"`

	VisibleFrom  *time.Time `bson:"visible_from,omitempty" json:"visible_from,omitempty"`
	VisibleUntil *time.Time `bson:"visible_until,omitempty" json:"visible_until,omitempty"`

	Instructions string `bson:"instructions" json:"instructions"`

	CreatedAt time.Time  `bson:"created_at" json:"created_at"`
	UpdatedAt *time.Time `bson:"updated_at,omitempty" json:"updated_at,omitempty"`

	CreatedByID   *primitive.ObjectID `bson:"created_by_id,omitempty" json:"created_by_id,omitempty"`
	CreatedByName string              `bson:"created_by_name,omitempty" json:"created_by_name,omitempty"`
	UpdatedByID   *primitive.ObjectID `bson:"updated_by_id,omitempty" json:"updated_by_id,omitempty"`
	UpdatedByName string              `bson:"updated_by_name,omitempty" json:"updated_by_name,omitempty"`
}
