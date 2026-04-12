// internal/app/store/mhsuserprogress/store.go
package mhsuserprogress

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/dalemusser/stratahub/internal/domain/models"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// Store provides access to the mhs_user_progress collection.
type Store struct {
	c *mongo.Collection
}

// New creates a new progress store.
func New(db *mongo.Database) *Store {
	return &Store{c: db.Collection("mhs_user_progress")}
}

// unitNumber returns the 1-based position of a unit ID (0 if invalid).
// Accepts any "unitN" format where N is a positive integer.
func unitNumber(id string) int {
	if len(id) > 4 && id[:4] == "unit" {
		n, err := strconv.Atoi(id[4:])
		if err == nil && n >= 1 {
			return n
		}
	}
	return 0
}

// unitsUpTo returns a slice of unit IDs from "unit1" to "unitN".
func unitsUpTo(n int) []string {
	units := make([]string, n)
	for i := 0; i < n; i++ {
		units[i] = fmt.Sprintf("unit%d", i+1)
	}
	return units
}

// GetOrCreate finds the progress record for (workspace, user).
// If none exists, it inserts a default record starting at unit1.
func (s *Store) GetOrCreate(ctx context.Context, workspaceID, userID primitive.ObjectID, loginID string) (models.MHSUserProgress, error) {
	now := time.Now().UTC()
	filter := bson.M{"workspace_id": workspaceID, "user_id": userID}

	update := bson.M{
		"$setOnInsert": bson.M{
			"_id":             primitive.NewObjectID(),
			"workspace_id":    workspaceID,
			"user_id":         userID,
			"login_id":        loginID,
			"current_unit":    "unit1",
			"completed_units": []string{},
			"created_at":      now,
			"updated_at":      now,
		},
	}

	opts := options.FindOneAndUpdate().
		SetUpsert(true).
		SetReturnDocument(options.After)

	var progress models.MHSUserProgress
	err := s.c.FindOneAndUpdate(ctx, filter, update, opts).Decode(&progress)
	if err != nil {
		return models.MHSUserProgress{}, err
	}
	return progress, nil
}

// Delete removes the progress record for (workspace, user), resetting them to defaults.
func (s *Store) Delete(ctx context.Context, workspaceID, userID primitive.ObjectID) error {
	filter := bson.M{"workspace_id": workspaceID, "user_id": userID}
	_, err := s.c.DeleteOne(ctx, filter)
	return err
}

// SetToUnit sets the user's progress so that targetUnit is their current unit.
// All units before targetUnit are marked completed. If targetUnit is "unit1",
// completed_units is cleared (equivalent to a reset without deleting the record).
func (s *Store) SetToUnit(ctx context.Context, workspaceID, userID primitive.ObjectID, targetUnit string) error {
	num := unitNumber(targetUnit)
	if num == 0 {
		return fmt.Errorf("invalid unit: %s", targetUnit)
	}

	completed := unitsUpTo(num - 1) // units before targetUnit
	now := time.Now().UTC()

	filter := bson.M{"workspace_id": workspaceID, "user_id": userID}
	update := bson.M{
		"$set": bson.M{
			"current_unit":    targetUnit,
			"completed_units": completed,
			"updated_at":      now,
		},
	}

	_, err := s.c.UpdateOne(ctx, filter, update)
	return err
}

// ListByUserIDs returns progress records for multiple users in a workspace, keyed by user ID hex.
func (s *Store) ListByUserIDs(ctx context.Context, workspaceID primitive.ObjectID, userIDs []primitive.ObjectID) (map[string]models.MHSUserProgress, error) {
	result := make(map[string]models.MHSUserProgress, len(userIDs))
	if len(userIDs) == 0 {
		return result, nil
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

	for cur.Next(ctx) {
		var p models.MHSUserProgress
		if err := cur.Decode(&p); err != nil {
			continue
		}
		result[p.UserID.Hex()] = p
	}
	return result, nil
}

// SetCollectionOverride sets or clears the per-user collection override.
// Pass nil to clear the override (return to group/workspace default).
// Uses upsert to ensure the document exists even if the user hasn't played yet.
func (s *Store) SetCollectionOverride(ctx context.Context, workspaceID, userID primitive.ObjectID, collectionID *primitive.ObjectID) error {
	now := time.Now().UTC()
	filter := bson.M{"workspace_id": workspaceID, "user_id": userID}
	set := bson.M{"updated_at": now}
	if collectionID != nil && !collectionID.IsZero() {
		set["collection_override_id"] = collectionID
	}

	update := bson.M{
		"$set": set,
		"$setOnInsert": bson.M{
			"_id":             primitive.NewObjectID(),
			"workspace_id":    workspaceID,
			"user_id":         userID,
			"current_unit":    "unit1",
			"completed_units": []string{},
			"created_at":      now,
		},
	}
	if collectionID == nil || collectionID.IsZero() {
		update["$unset"] = bson.M{"collection_override_id": ""}
	}

	opts := options.Update().SetUpsert(true)
	_, err := s.c.UpdateOne(ctx, filter, update, opts)
	return err
}

// CompleteUnit marks a unit as completed and advances current_unit.
// totalUnits is the number of units in the active collection (e.g., 5 or 6).
// Idempotent: if the unit is already completed or before current_unit, returns current state.
func (s *Store) CompleteUnit(ctx context.Context, workspaceID, userID primitive.ObjectID, unitID string, totalUnits int) (models.MHSUserProgress, error) {
	num := unitNumber(unitID)
	if num == 0 {
		// Invalid unit ID — return current state
		var progress models.MHSUserProgress
		filter := bson.M{"workspace_id": workspaceID, "user_id": userID}
		err := s.c.FindOne(ctx, filter).Decode(&progress)
		return progress, err
	}

	// Read current state
	filter := bson.M{"workspace_id": workspaceID, "user_id": userID}
	var progress models.MHSUserProgress
	err := s.c.FindOne(ctx, filter).Decode(&progress)
	if err != nil {
		return models.MHSUserProgress{}, err
	}

	// Check if already completed (idempotent)
	for _, cu := range progress.CompletedUnits {
		if cu == unitID {
			return progress, nil
		}
	}

	// Only complete the current unit (can't skip ahead)
	if progress.CurrentUnit != unitID {
		return progress, nil
	}

	// Determine next unit
	var nextUnit string
	if num >= totalUnits {
		nextUnit = "complete"
	} else {
		nextUnit = fmt.Sprintf("unit%d", num+1)
	}

	now := time.Now().UTC()
	update := bson.M{
		"$set": bson.M{
			"current_unit": nextUnit,
			"updated_at":   now,
		},
		"$addToSet": bson.M{
			"completed_units": unitID,
		},
	}

	opts := options.FindOneAndUpdate().SetReturnDocument(options.After)
	err = s.c.FindOneAndUpdate(ctx, filter, update, opts).Decode(&progress)
	if err != nil {
		return models.MHSUserProgress{}, err
	}
	return progress, nil
}
