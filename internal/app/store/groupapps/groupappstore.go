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

// GetMHSCollectionForUser returns the pinned MHS collection ID for a user's MHS-enabled group.
// Returns nil if no group is pinned or the user has no MHS-enabled group.
// Uses an aggregation: group_memberships → group_app_settings where app_id=missionhydrosci and mhs_collection_id is set.
func GetMHSCollectionForUser(ctx context.Context, db *mongo.Database, userID primitive.ObjectID) *primitive.ObjectID {
	coll := db.Collection("group_memberships")
	pipeline := mongo.Pipeline{
		// Find groups where user is a member
		{{Key: "$match", Value: bson.M{
			"user_id": userID,
			"role":    "member",
		}}},
		// Lookup MHS app settings for each group
		{{Key: "$lookup", Value: bson.M{
			"from": "group_app_settings",
			"let":  bson.M{"gid": "$group_id"},
			"pipeline": mongo.Pipeline{
				{{Key: "$match", Value: bson.M{
					"$expr": bson.M{"$eq": []string{"$group_id", "$$gid"}},
					"app_id":           "missionhydrosci",
					"mhs_collection_id": bson.M{"$exists": true, "$ne": nil},
				}}},
			},
			"as": "mhs_app",
		}}},
		// Only keep groups that have a pinned collection
		{{Key: "$unwind", Value: "$mhs_app"}},
		// Return just the collection ID (take first match)
		{{Key: "$limit", Value: 1}},
		{{Key: "$project", Value: bson.M{
			"_id":               0,
			"mhs_collection_id": "$mhs_app.mhs_collection_id",
		}}},
	}

	cur, err := coll.Aggregate(ctx, pipeline)
	if err != nil {
		return nil
	}
	defer cur.Close(ctx)

	var result struct {
		MHSCollectionID *primitive.ObjectID `bson:"mhs_collection_id"`
	}
	if cur.Next(ctx) {
		if err := cur.Decode(&result); err == nil && result.MHSCollectionID != nil {
			return result.MHSCollectionID
		}
	}
	return nil
}

// SetMHSCollection sets or clears the pinned MHS collection for a group's app setting.
// Pass nil to clear the pin (group follows workspace active).
func (s *Store) SetMHSCollection(ctx context.Context, groupID primitive.ObjectID, collectionID *primitive.ObjectID) error {
	filter := bson.M{"group_id": groupID, "app_id": "missionhydrosci"}
	update := bson.M{}
	if collectionID != nil && !collectionID.IsZero() {
		update["$set"] = bson.M{"mhs_collection_id": collectionID}
	} else {
		update["$unset"] = bson.M{"mhs_collection_id": ""}
	}
	_, err := s.c.UpdateOne(ctx, filter, update)
	return err
}

// GetByGroupAndApp returns the app setting for a specific group and app.
func (s *Store) GetByGroupAndApp(ctx context.Context, groupID primitive.ObjectID, appID string) (models.GroupAppSetting, error) {
	var setting models.GroupAppSetting
	err := s.c.FindOne(ctx, bson.M{"group_id": groupID, "app_id": appID}).Decode(&setting)
	if err == mongo.ErrNoDocuments {
		return setting, nil
	}
	return setting, err
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
