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
	ID            primitive.ObjectID  `bson:"_id,omitempty" json:"id,omitempty"`
	Name          string              `bson:"name" json:"name"`
	Description   string              `bson:"description,omitempty" json:"description,omitempty"`
	Units         []MHSCollectionUnit `bson:"units" json:"units"`
	CreatedAt     time.Time           `bson:"created_at" json:"created_at"`
	CreatedByID   primitive.ObjectID  `bson:"created_by_id" json:"created_by_id"`
	CreatedByName string              `bson:"created_by_name" json:"created_by_name"`
}

// MHSCollectionUnit is a reference to a specific unit version within a collection.
// File data is stored in the mhs_builds collection and looked up at serving time.
type MHSCollectionUnit struct {
	UnitID          string `bson:"unit_id" json:"unit_id"`
	Title           string `bson:"title" json:"title"`
	Version         string `bson:"version" json:"version"`
	BuildIdentifier string `bson:"build_identifier" json:"build_identifier"`
}
