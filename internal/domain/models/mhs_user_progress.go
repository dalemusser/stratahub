// internal/domain/models/mhs_user_progress.go
package models

// Terminology: User Identifiers
//   - UserID / userID / user_id: The MongoDB ObjectID (_id) that uniquely identifies a user record
//   - LoginID / loginID / login_id: The human-readable string users type to log in

import (
	"time"

	"go.mongodb.org/mongo-driver/bson/primitive"
)

// MHSUserProgress tracks a student's progress through the Mission HydroSci units.
// One document per (workspace, user) pair.
type MHSUserProgress struct {
	ID             primitive.ObjectID `bson:"_id,omitempty" json:"id"`
	WorkspaceID    primitive.ObjectID `bson:"workspace_id" json:"workspace_id"`
	UserID         primitive.ObjectID `bson:"user_id" json:"user_id"`
	LoginID        string             `bson:"login_id" json:"login_id"`
	CurrentUnit    string             `bson:"current_unit" json:"current_unit"`
	CompletedUnits []string           `bson:"completed_units" json:"completed_units"`
	CreatedAt      time.Time          `bson:"created_at" json:"created_at"`
	UpdatedAt      time.Time          `bson:"updated_at" json:"updated_at"`
}
