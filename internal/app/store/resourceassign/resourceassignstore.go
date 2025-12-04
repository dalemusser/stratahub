package resourceassignstore

import (
	"context"
	"time"

	"github.com/dalemusser/stratahub/internal/domain/models"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
)

type Store struct {
	c *mongo.Collection
}

func New(db *mongo.Database) *Store {
	return &Store{c: db.Collection("group_resource_assignments")}
}

// Create inserts a new group-resource assignment document.
//
// The caller is responsible for setting GroupID, ResourceID, scheduling and
// instruction fields. If ID is zero, a new ObjectID will be assigned by MongoDB.
// If CreatedAt is zero, it will be set to now (UTC).
func (s *Store) Create(ctx context.Context, a models.GroupResourceAssignment) (models.GroupResourceAssignment, error) {
	now := time.Now().UTC()

	if a.CreatedAt.IsZero() {
		a.CreatedAt = now
	}
	// UpdatedAt is left as provided; typically nil on initial create.

	res, err := s.c.InsertOne(ctx, a)
	if err != nil {
		return a, err
	}
	if oid, ok := res.InsertedID.(primitive.ObjectID); ok {
		a.ID = oid
	}
	return a, nil
}

// Delete removes the assignment with the given _id.
func (s *Store) Delete(ctx context.Context, id primitive.ObjectID) error {
	_, err := s.c.DeleteOne(ctx, bson.M{"_id": id})
	return err
}

// GetByID returns a single assignment by its _id.
func (s *Store) GetByID(ctx context.Context, id primitive.ObjectID) (models.GroupResourceAssignment, error) {
	var a models.GroupResourceAssignment
	err := s.c.FindOne(ctx, bson.M{"_id": id}).Decode(&a)
	return a, err
}

// Update replaces an existing assignment identified by its _id.
//
// The caller must provide a.ID. UpdatedAt will be set to now (UTC).
func (s *Store) Update(ctx context.Context, a models.GroupResourceAssignment) (models.GroupResourceAssignment, error) {
	if a.ID.IsZero() {
		return a, mongo.ErrNilDocument
	}

	now := time.Now().UTC()
	a.UpdatedAt = &now

	_, err := s.c.ReplaceOne(ctx, bson.M{"_id": a.ID}, a)
	return a, err
}

func (s *Store) ListByGroup(ctx context.Context, groupID primitive.ObjectID) ([]models.GroupResourceAssignment, error) {
	cur, err := s.c.Find(ctx, bson.M{"group_id": groupID})
	if err != nil {
		return nil, err
	}
	defer cur.Close(ctx)
	var out []models.GroupResourceAssignment
	if err := cur.All(ctx, &out); err != nil {
		return nil, err
	}
	return out, nil
}
