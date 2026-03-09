// internal/app/store/mhsuserprogress/store.go
package mhsuserprogress

import (
	"context"
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

// unitOrder is the fixed sequence of units.
var unitOrder = []string{"unit1", "unit2", "unit3", "unit4", "unit5"}

// unitNumber returns the 1-based position of a unit ID (0 if invalid).
func unitNumber(id string) int {
	if len(id) == 5 && id[:4] == "unit" {
		n, err := strconv.Atoi(id[4:])
		if err == nil && n >= 1 && n <= 5 {
			return n
		}
	}
	return 0
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

// CompleteUnit marks a unit as completed and advances current_unit.
// Idempotent: if the unit is already completed or before current_unit, returns current state.
func (s *Store) CompleteUnit(ctx context.Context, workspaceID, userID primitive.ObjectID, unitID string) (models.MHSUserProgress, error) {
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
	if num >= 5 {
		nextUnit = "complete"
	} else {
		nextUnit = unitOrder[num] // num is 1-based, so unitOrder[num] gives next
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
