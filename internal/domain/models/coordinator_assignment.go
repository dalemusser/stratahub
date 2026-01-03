// internal/domain/models/coordinator_assignment.go
package models

import (
	"time"

	"go.mongodb.org/mongo-driver/bson/primitive"
)

// CoordinatorAssignment links a coordinator (user) to an organization they can manage.
// Coordinators can be assigned to multiple organizations via multiple assignment records.
type CoordinatorAssignment struct {
	ID             primitive.ObjectID `bson:"_id,omitempty" json:"id"`
	UserID         primitive.ObjectID `bson:"user_id" json:"user_id"`
	OrganizationID primitive.ObjectID `bson:"organization_id" json:"organization_id"`

	// Audit fields
	CreatedAt     time.Time          `bson:"created_at" json:"created_at"`
	CreatedByID   primitive.ObjectID `bson:"created_by_id" json:"created_by_id"`
	CreatedByName string             `bson:"created_by_name" json:"created_by_name"`
}
