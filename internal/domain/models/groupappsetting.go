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
}
