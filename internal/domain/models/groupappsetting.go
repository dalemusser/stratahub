package models

import (
	"time"

	"go.mongodb.org/mongo-driver/bson/primitive"
)

// GroupAppSetting records that an app is enabled for a specific group.
// Presence of a document = enabled; absence = disabled (default).
type GroupAppSetting struct {
	ID            primitive.ObjectID `bson:"_id,omitempty"`
	WorkspaceID   primitive.ObjectID `bson:"workspace_id"`
	GroupID       primitive.ObjectID `bson:"group_id"`
	AppID         string             `bson:"app_id"`
	EnabledAt     time.Time          `bson:"enabled_at"`
	EnabledByID   primitive.ObjectID `bson:"enabled_by_id"`
	EnabledByName string             `bson:"enabled_by_name"`

	// MHSCollectionID pins this group to a specific MHS collection.
	// When set, members of this group play from this collection instead of the workspace active.
	// Only relevant when AppID is "missionhydrosci".
	MHSCollectionID *primitive.ObjectID `bson:"mhs_collection_id,omitempty"`
}
