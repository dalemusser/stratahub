// internal/app/store/mhsdevicestatus/store.go
package mhsdevicestatus

import (
	"context"
	"time"

	"github.com/dalemusser/stratahub/internal/domain/models"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// Store provides access to the mhs_device_status collection.
type Store struct {
	c *mongo.Collection
}

// New creates a new device status store.
func New(db *mongo.Database) *Store {
	return &Store{c: db.Collection("mhs_device_status")}
}

// Upsert creates or updates a device status record.
// Matches on (workspace_id, user_id, device_id).
// Sets storage_baseline_usage only on insert (first report for this device).
func (s *Store) Upsert(ctx context.Context, status models.MHSDeviceStatus) error {
	now := time.Now().UTC()
	filter := bson.M{
		"workspace_id": status.WorkspaceID,
		"user_id":      status.UserID,
		"device_id":    status.DeviceID,
	}

	update := bson.M{
		"$setOnInsert": bson.M{
			"_id":                    primitive.NewObjectID(),
			"created_at":             now,
			"storage_baseline_usage": status.StorageUsage,
		},
		"$set": bson.M{
			"workspace_id":  status.WorkspaceID,
			"user_id":       status.UserID,
			"login_id":      status.LoginID,
			"device_id":     status.DeviceID,
			"device_type":   status.DeviceType,
			"pwa_installed": status.PWAInstalled,
			"sw_registered": status.SWRegistered,
			"unit_status":   status.UnitStatus,
			"storage_quota": status.StorageQuota,
			"storage_usage": status.StorageUsage,
			"last_seen":     now,
			"updated_at":    now,
		},
	}

	opts := options.Update().SetUpsert(true)
	_, err := s.c.UpdateOne(ctx, filter, update, opts)
	return err
}

// ListByUserIDs returns all device status records for the given user IDs within a workspace.
func (s *Store) ListByUserIDs(ctx context.Context, workspaceID primitive.ObjectID, userIDs []primitive.ObjectID) ([]models.MHSDeviceStatus, error) {
	if len(userIDs) == 0 {
		return nil, nil
	}

	filter := bson.M{
		"workspace_id": workspaceID,
		"user_id":      bson.M{"$in": userIDs},
	}

	cur, err := s.c.Find(ctx, filter)
	if err != nil {
		return nil, err
	}
	defer cur.Close(ctx)

	var results []models.MHSDeviceStatus
	if err := cur.All(ctx, &results); err != nil {
		return nil, err
	}
	return results, nil
}

// DeleteByUser removes all device status records for a user in a workspace.
func (s *Store) DeleteByUser(ctx context.Context, workspaceID, userID primitive.ObjectID) error {
	filter := bson.M{"workspace_id": workspaceID, "user_id": userID}
	_, err := s.c.DeleteMany(ctx, filter)
	return err
}
