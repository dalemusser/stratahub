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
	CurrentUnit    string             `bson:"current_unit" json:"current_unit"`
	CompletedUnits []string           `bson:"completed_units" json:"completed_units"`

	// CollectionOverrideID is the per-user collection override.
	// When set, this user plays from this collection instead of the group/workspace default.
	CollectionOverrideID *primitive.ObjectID `bson:"collection_override_id,omitempty" json:"collection_override_id,omitempty"`

	// End-of-game ceremony marks (docs/mission-hydrosci/mhs-end-ceremony-plan.md
	// D8), written by the ceremony host page and never by unit progress:
	// when the student first pressed Begin, when a showing last reached its end,
	// the ceremony version last shown, and how many times Begin was pressed.
	CeremonyStartedAt  *time.Time `bson:"ceremony_started_at,omitempty" json:"ceremony_started_at,omitempty"`
	CeremonyFinishedAt *time.Time `bson:"ceremony_finished_at,omitempty" json:"ceremony_finished_at,omitempty"`
	CeremonyVersion    string     `bson:"ceremony_version,omitempty" json:"ceremony_version,omitempty"`
	CeremonyViewCount  int        `bson:"ceremony_view_count,omitempty" json:"ceremony_view_count,omitempty"`
	CreatedAt          time.Time  `bson:"created_at" json:"created_at"`
	UpdatedAt          time.Time  `bson:"updated_at" json:"updated_at"`
}
