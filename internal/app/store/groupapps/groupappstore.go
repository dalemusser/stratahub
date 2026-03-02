package groupappstore

import (
	"context"
	"time"

	"github.com/dalemusser/stratahub/internal/domain/models"
	wafflemongo "github.com/dalemusser/waffle/pantry/mongo"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
)

// Store manages group_app_settings documents.
type Store struct {
	c *mongo.Collection
}

// New creates a new group app settings Store.
func New(db *mongo.Database) *Store {
	return &Store{c: db.Collection("group_app_settings")}
}

// Enable inserts a group_app_settings document (app is enabled for the group).
// No-op on duplicate (idempotent).
func (s *Store) Enable(ctx context.Context, workspaceID, groupID, enabledByID primitive.ObjectID, appID, enabledByName string) error {
	doc := models.GroupAppSetting{
		WorkspaceID:   workspaceID,
		GroupID:       groupID,
		AppID:         appID,
		EnabledAt:     time.Now().UTC(),
		EnabledByID:   enabledByID,
		EnabledByName: enabledByName,
	}
	_, err := s.c.InsertOne(ctx, doc)
	if err != nil && wafflemongo.IsDup(err) {
		return nil // already enabled — no-op
	}
	return err
}

// Disable removes the group_app_settings document (app is disabled for the group).
func (s *Store) Disable(ctx context.Context, groupID primitive.ObjectID, appID string) error {
	_, err := s.c.DeleteOne(ctx, bson.M{"group_id": groupID, "app_id": appID})
	return err
}

// ListByGroup returns all enabled app settings for a group.
func (s *Store) ListByGroup(ctx context.Context, groupID primitive.ObjectID) ([]models.GroupAppSetting, error) {
	cur, err := s.c.Find(ctx, bson.M{"group_id": groupID})
	if err != nil {
		return nil, err
	}
	defer cur.Close(ctx)
	var out []models.GroupAppSetting
	if err := cur.All(ctx, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// IsEnabled checks whether an app is enabled for a group.
func (s *Store) IsEnabled(ctx context.Context, groupID primitive.ObjectID, appID string) (bool, error) {
	count, err := s.c.CountDocuments(ctx, bson.M{"group_id": groupID, "app_id": appID})
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

// EnabledAppIDsForUser returns distinct app IDs enabled across all groups
// where the given user is a member. Uses an aggregation pipeline across
// group_memberships → group_app_settings.
func EnabledAppIDsForUser(ctx context.Context, db *mongo.Database, userID primitive.ObjectID) ([]string, error) {
	coll := db.Collection("group_memberships")
	pipeline := mongo.Pipeline{
		// 1. Find all groups where user is a member
		{{Key: "$match", Value: bson.M{
			"user_id": userID,
			"role":    "member",
		}}},
		// 2. Lookup enabled apps for each group
		{{Key: "$lookup", Value: bson.M{
			"from":         "group_app_settings",
			"localField":   "group_id",
			"foreignField": "group_id",
			"as":           "apps",
		}}},
		// 3. Unwind joined apps (skip groups with no enabled apps)
		{{Key: "$unwind", Value: "$apps"}},
		// 4. Group by app_id to get distinct IDs
		{{Key: "$group", Value: bson.M{
			"_id": "$apps.app_id",
		}}},
	}

	cur, err := coll.Aggregate(ctx, pipeline)
	if err != nil {
		return nil, err
	}
	defer cur.Close(ctx)

	var results []struct {
		ID string `bson:"_id"`
	}
	if err := cur.All(ctx, &results); err != nil {
		return nil, err
	}

	ids := make([]string, len(results))
	for i, r := range results {
		ids[i] = r.ID
	}
	return ids, nil
}

// DeleteByGroup removes all app settings for a group (cascade delete).
// Returns the number of documents deleted.
func (s *Store) DeleteByGroup(ctx context.Context, groupID primitive.ObjectID) (int64, error) {
	res, err := s.c.DeleteMany(ctx, bson.M{"group_id": groupID})
	if err != nil {
		return 0, err
	}
	return res.DeletedCount, nil
}
