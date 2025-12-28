// internal/domain/models/materialassignment.go
package models

import (
	"time"

	"go.mongodb.org/mongo-driver/bson/primitive"
)

// MaterialAssignment represents a material assigned to either an Organization
// (all Leaders in that org) or an individual Leader.
//
// Exactly one of OrganizationID or LeaderID must be set:
// - OrganizationID set → all Leaders in the org see this material
// - LeaderID set → only that specific Leader sees this material
type MaterialAssignment struct {
	ID         primitive.ObjectID `bson:"_id,omitempty" json:"id"`
	MaterialID primitive.ObjectID `bson:"material_id" json:"material_id"`

	// Exactly one is set - determines assignment scope
	OrganizationID *primitive.ObjectID `bson:"organization_id,omitempty" json:"organization_id,omitempty"` // Org-wide
	LeaderID       *primitive.ObjectID `bson:"leader_id,omitempty" json:"leader_id,omitempty"`             // Individual

	VisibleFrom  *time.Time `bson:"visible_from,omitempty" json:"visible_from,omitempty"`
	VisibleUntil *time.Time `bson:"visible_until,omitempty" json:"visible_until,omitempty"`

	// Directions is copied from Material.DefaultInstructions when assigned,
	// but can be customized per-assignment.
	Directions string `bson:"directions" json:"directions"`

	CreatedAt time.Time  `bson:"created_at" json:"created_at"`
	UpdatedAt *time.Time `bson:"updated_at,omitempty" json:"updated_at,omitempty"`

	CreatedByID   *primitive.ObjectID `bson:"created_by_id,omitempty" json:"created_by_id,omitempty"`
	CreatedByName string              `bson:"created_by_name,omitempty" json:"created_by_name,omitempty"`
	UpdatedByID   *primitive.ObjectID `bson:"updated_by_id,omitempty" json:"updated_by_id,omitempty"`
	UpdatedByName string              `bson:"updated_by_name,omitempty" json:"updated_by_name,omitempty"`
}

// IsOrgWide returns true if this assignment is to an organization (all leaders).
func (a *MaterialAssignment) IsOrgWide() bool {
	return a.OrganizationID != nil && !a.OrganizationID.IsZero()
}

// IsIndividual returns true if this assignment is to a specific leader.
func (a *MaterialAssignment) IsIndividual() bool {
	return a.LeaderID != nil && !a.LeaderID.IsZero()
}
