// internal/app/store/materialassign/materialassignstore.go
package materialassignstore

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
	return &Store{c: db.Collection("material_assignments")}
}

// Create inserts a new material assignment document.
//
// The caller is responsible for setting MaterialID and exactly one of
// OrganizationID or LeaderID. If ID is zero, a new ObjectID will be assigned.
// If CreatedAt is zero, it will be set to now (UTC).
func (s *Store) Create(ctx context.Context, a models.MaterialAssignment) (models.MaterialAssignment, error) {
	now := time.Now().UTC()

	if a.CreatedAt.IsZero() {
		a.CreatedAt = now
	}

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
func (s *Store) GetByID(ctx context.Context, id primitive.ObjectID) (models.MaterialAssignment, error) {
	var a models.MaterialAssignment
	err := s.c.FindOne(ctx, bson.M{"_id": id}).Decode(&a)
	return a, err
}

// Update replaces an existing assignment identified by its _id.
//
// The caller must provide a.ID. UpdatedAt will be set to now (UTC).
func (s *Store) Update(ctx context.Context, a models.MaterialAssignment) (models.MaterialAssignment, error) {
	if a.ID.IsZero() {
		return a, mongo.ErrNilDocument
	}

	now := time.Now().UTC()
	a.UpdatedAt = &now

	_, err := s.c.ReplaceOne(ctx, bson.M{"_id": a.ID}, a)
	return a, err
}

// ListByMaterial returns all assignments for a given material.
func (s *Store) ListByMaterial(ctx context.Context, materialID primitive.ObjectID) ([]models.MaterialAssignment, error) {
	cur, err := s.c.Find(ctx, bson.M{"material_id": materialID})
	if err != nil {
		return nil, err
	}
	defer cur.Close(ctx)
	var out []models.MaterialAssignment
	if err := cur.All(ctx, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// ListByOrg returns all org-wide assignments for a given organization.
func (s *Store) ListByOrg(ctx context.Context, orgID primitive.ObjectID) ([]models.MaterialAssignment, error) {
	cur, err := s.c.Find(ctx, bson.M{"organization_id": orgID})
	if err != nil {
		return nil, err
	}
	defer cur.Close(ctx)
	var out []models.MaterialAssignment
	if err := cur.All(ctx, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// ListByLeader returns all individual assignments for a given leader.
func (s *Store) ListByLeader(ctx context.Context, leaderID primitive.ObjectID) ([]models.MaterialAssignment, error) {
	cur, err := s.c.Find(ctx, bson.M{"leader_id": leaderID})
	if err != nil {
		return nil, err
	}
	defer cur.Close(ctx)
	var out []models.MaterialAssignment
	if err := cur.All(ctx, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// DeleteByMaterial removes all assignments for a material.
// Returns the number of documents deleted.
func (s *Store) DeleteByMaterial(ctx context.Context, materialID primitive.ObjectID) (int64, error) {
	res, err := s.c.DeleteMany(ctx, bson.M{"material_id": materialID})
	if err != nil {
		return 0, err
	}
	return res.DeletedCount, nil
}

// DeleteByOrg removes all org-wide assignments for an organization.
// Returns the number of documents deleted.
func (s *Store) DeleteByOrg(ctx context.Context, orgID primitive.ObjectID) (int64, error) {
	res, err := s.c.DeleteMany(ctx, bson.M{"organization_id": orgID})
	if err != nil {
		return 0, err
	}
	return res.DeletedCount, nil
}

// DeleteByLeader removes all individual assignments for a leader.
// Returns the number of documents deleted.
func (s *Store) DeleteByLeader(ctx context.Context, leaderID primitive.ObjectID) (int64, error) {
	res, err := s.c.DeleteMany(ctx, bson.M{"leader_id": leaderID})
	if err != nil {
		return 0, err
	}
	return res.DeletedCount, nil
}

// CountByMaterial returns the number of assignments for a given material.
func (s *Store) CountByMaterial(ctx context.Context, materialID primitive.ObjectID) (int64, error) {
	return s.c.CountDocuments(ctx, bson.M{"material_id": materialID})
}

// ListAll returns all material assignments.
func (s *Store) ListAll(ctx context.Context) ([]models.MaterialAssignment, error) {
	cur, err := s.c.Find(ctx, bson.M{})
	if err != nil {
		return nil, err
	}
	defer cur.Close(ctx)
	var out []models.MaterialAssignment
	if err := cur.All(ctx, &out); err != nil {
		return nil, err
	}
	return out, nil
}
