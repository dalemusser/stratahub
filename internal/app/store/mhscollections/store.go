// internal/app/store/mhscollections/store.go
package mhscollections

import (
	"context"
	"errors"
	"time"

	"github.com/dalemusser/stratahub/internal/domain/models"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// ErrNotFound is returned when a collection record does not exist.
var ErrNotFound = errors.New("mhs collection not found")

// Store provides access to the mhs_collections collection.
type Store struct {
	c *mongo.Collection
}

// New creates a new MHS collections store.
func New(db *mongo.Database) *Store {
	return &Store{c: db.Collection("mhs_collections")}
}

// Create inserts a new collection record.
func (s *Store) Create(ctx context.Context, coll models.MHSCollection) (primitive.ObjectID, error) {
	if coll.ID.IsZero() {
		coll.ID = primitive.NewObjectID()
	}
	if coll.CreatedAt.IsZero() {
		coll.CreatedAt = time.Now().UTC()
	}
	_, err := s.c.InsertOne(ctx, coll)
	return coll.ID, err
}

// GetByID returns a collection by its ID.
func (s *Store) GetByID(ctx context.Context, id primitive.ObjectID) (models.MHSCollection, error) {
	var c models.MHSCollection
	err := s.c.FindOne(ctx, bson.M{"_id": id}).Decode(&c)
	if err == mongo.ErrNoDocuments {
		return c, ErrNotFound
	}
	return c, err
}

// List returns collections ordered by created_at descending, up to limit.
func (s *Store) List(ctx context.Context, limit int) ([]models.MHSCollection, error) {
	opts := options.Find().
		SetSort(bson.D{{Key: "created_at", Value: -1}}).
		SetLimit(int64(limit))
	cursor, err := s.c.Find(ctx, bson.M{}, opts)
	if err != nil {
		return nil, err
	}
	var collections []models.MHSCollection
	if err := cursor.All(ctx, &collections); err != nil {
		return nil, err
	}
	return collections, nil
}

// Latest returns the most recently created collection.
func (s *Store) Latest(ctx context.Context) (models.MHSCollection, error) {
	var c models.MHSCollection
	opts := options.FindOne().SetSort(bson.D{{Key: "created_at", Value: -1}})
	err := s.c.FindOne(ctx, bson.M{}, opts).Decode(&c)
	if err == mongo.ErrNoDocuments {
		return c, ErrNotFound
	}
	return c, err
}

// Update updates a collection's name, description, and units.
func (s *Store) Update(ctx context.Context, id primitive.ObjectID, coll models.MHSCollection) error {
	_, err := s.c.UpdateOne(ctx, bson.M{"_id": id}, bson.M{
		"$set": bson.M{
			"name":        coll.Name,
			"description": coll.Description,
			"units":       coll.Units,
		},
	})
	if err != nil {
		return err
	}
	return nil
}

// Delete removes a collection by ID.
func (s *Store) Delete(ctx context.Context, id primitive.ObjectID) error {
	_, err := s.c.DeleteOne(ctx, bson.M{"_id": id})
	return err
}

// Count returns the total number of collections.
func (s *Store) Count(ctx context.Context) (int64, error) {
	return s.c.CountDocuments(ctx, bson.M{})
}
