// internal/domain/models/mhs_collection.go
package models

import (
	"time"

	"go.mongodb.org/mongo-driver/bson/primitive"
)

// MHSCollection is a named set of unit version references that defines
// which builds are served to players. File data (paths, sizes, key filenames)
// is looked up from mhs_builds at serving time — collections only store references.
type MHSCollection struct {
	ID          primitive.ObjectID  `bson:"_id,omitempty" json:"id,omitempty"`
	Name        string              `bson:"name" json:"name"`
	Description string              `bson:"description,omitempty" json:"description,omitempty"`
	Units       []MHSCollectionUnit `bson:"units" json:"units"`
	// Ceremony is the end-of-game ceremony version served after the last
	// unit; nil means the collection has no ceremony (the game returns to the
	// units page as before). Pinned per collection like the units.
	Ceremony      *MHSCollectionCeremony `bson:"ceremony,omitempty" json:"ceremony,omitempty"`
	CreatedAt     time.Time              `bson:"created_at" json:"created_at"`
	CreatedByID   primitive.ObjectID     `bson:"created_by_id" json:"created_by_id"`
	CreatedByName string                 `bson:"created_by_name" json:"created_by_name"`
}

// MHSCollectionCeremony is a reference to a specific ceremony version
// (an mhs_builds record with Kind ceremony and UnitID "end").
type MHSCollectionCeremony struct {
	Version         string `bson:"version" json:"version"`
	BuildIdentifier string `bson:"build_identifier,omitempty" json:"build_identifier,omitempty"`
}

// HasCeremony reports whether the collection references a ceremony version.
func (c MHSCollection) HasCeremony() bool { return c.Ceremony != nil && c.Ceremony.Version != "" }

// MHSCollectionUnit is a reference to a specific unit version within a collection.
// File data is stored in the mhs_builds collection and looked up at serving time.
type MHSCollectionUnit struct {
	UnitID          string `bson:"unit_id" json:"unit_id"`
	Title           string `bson:"title" json:"title"`
	Version         string `bson:"version" json:"version"`
	BuildIdentifier string `bson:"build_identifier" json:"build_identifier"`
}
