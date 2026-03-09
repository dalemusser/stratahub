// internal/domain/models/mhs_device_status.go
package models

// Terminology: User Identifiers
//   - UserID / userID / user_id: The MongoDB ObjectID (_id) that uniquely identifies a user record
//   - LoginID / loginID / login_id: The human-readable string users type to log in

import (
	"time"

	"go.mongodb.org/mongo-driver/bson/primitive"
)

// MHSDeviceStatus tracks per-device readiness for Mission HydroSci.
// One document per (workspace, user, device) triple.
type MHSDeviceStatus struct {
	ID                   primitive.ObjectID `bson:"_id,omitempty" json:"id"`
	WorkspaceID          primitive.ObjectID `bson:"workspace_id" json:"workspace_id"`
	UserID               primitive.ObjectID `bson:"user_id" json:"user_id"`
	LoginID              string             `bson:"login_id" json:"login_id"`
	DeviceID             string             `bson:"device_id" json:"device_id"`
	DeviceType           string             `bson:"device_type" json:"device_type"`
	PWAInstalled         bool               `bson:"pwa_installed" json:"pwa_installed"`
	SWRegistered         bool               `bson:"sw_registered" json:"sw_registered"`
	UnitStatus           map[string]string  `bson:"unit_status" json:"unit_status"`
	StorageQuota         int64              `bson:"storage_quota" json:"storage_quota"`
	StorageUsage         int64              `bson:"storage_usage" json:"storage_usage"`
	StorageBaselineUsage int64              `bson:"storage_baseline_usage" json:"storage_baseline_usage"`
	LastSeen             time.Time          `bson:"last_seen" json:"last_seen"`
	CreatedAt            time.Time          `bson:"created_at" json:"created_at"`
	UpdatedAt            time.Time          `bson:"updated_at" json:"updated_at"`
}
