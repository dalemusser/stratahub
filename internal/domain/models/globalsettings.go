// internal/domain/models/globalsettings.go
package models

import (
	"encoding/hex"
	"time"

	"go.mongodb.org/mongo-driver/bson/primitive"
)

// GlobalSettingsID is the fixed ObjectID for the singleton global settings document.
// This must be deterministic across restarts so the document is always found.
var GlobalSettingsID = mustParseObjectID("000000000000000000000001")

func mustParseObjectID(s string) primitive.ObjectID {
	b, err := hex.DecodeString(s)
	if err != nil {
		panic("invalid global settings ID: " + err.Error())
	}
	var oid primitive.ObjectID
	copy(oid[:], b)
	return oid
}

// GlobalSettings holds system-wide settings that apply across all workspaces.
// Stored as a singleton document in the global_settings collection.
type GlobalSettings struct {
	ID                 primitive.ObjectID `bson:"_id"`
	MaintenanceMode    bool               `bson:"maintenance_mode"`
	MaintenanceMessage string             `bson:"maintenance_message,omitempty"` // e.g., "Expected back by Monday morning"
	UpdatedAt          time.Time          `bson:"updated_at,omitempty"`
	UpdatedByName      string             `bson:"updated_by_name,omitempty"`
}
