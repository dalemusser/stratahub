// internal/app/store/coordinatorassign/coordinatorassignstore.go
package coordinatorassign

// Terminology: User Identifiers
//   - UserID / userID / user_id: The MongoDB ObjectID (_id) that uniquely identifies a user record
//   - LoginID / loginID / login_id: The human-readable string users type to log in

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
	return &Store{c: db.Collection("coordinator_assignments")}
}

// Create inserts a new coordinator-organization assignment.
// If CreatedAt is zero, it will be set to now (UTC).
func (s *Store) Create(ctx context.Context, a models.CoordinatorAssignment) (models.CoordinatorAssignment, error) {
	if a.CreatedAt.IsZero() {
		a.CreatedAt = time.Now().UTC()
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
func (s *Store) GetByID(ctx context.Context, id primitive.ObjectID) (models.CoordinatorAssignment, error) {
	var a models.CoordinatorAssignment
	err := s.c.FindOne(ctx, bson.M{"_id": id}).Decode(&a)
	return a, err
}

// ListByUser returns all organization assignments for a coordinator.
func (s *Store) ListByUser(ctx context.Context, userID primitive.ObjectID) ([]models.CoordinatorAssignment, error) {
	cur, err := s.c.Find(ctx, bson.M{"user_id": userID})
	if err != nil {
		return nil, err
	}
	defer cur.Close(ctx)
	var out []models.CoordinatorAssignment
	if err := cur.All(ctx, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// ListByOrg returns all coordinator assignments for an organization.
func (s *Store) ListByOrg(ctx context.Context, orgID primitive.ObjectID) ([]models.CoordinatorAssignment, error) {
	cur, err := s.c.Find(ctx, bson.M{"organization_id": orgID})
	if err != nil {
		return nil, err
	}
	defer cur.Close(ctx)
	var out []models.CoordinatorAssignment
	if err := cur.All(ctx, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// OrgIDsByUser returns just the organization IDs for a coordinator.
// This is useful for authorization checks.
func (s *Store) OrgIDsByUser(ctx context.Context, userID primitive.ObjectID) ([]primitive.ObjectID, error) {
	cur, err := s.c.Find(ctx, bson.M{"user_id": userID})
	if err != nil {
		return nil, err
	}
	defer cur.Close(ctx)

	var orgIDs []primitive.ObjectID
	for cur.Next(ctx) {
		var a models.CoordinatorAssignment
		if err := cur.Decode(&a); err != nil {
			return nil, err
		}
		orgIDs = append(orgIDs, a.OrganizationID)
	}
	return orgIDs, cur.Err()
}

// DeleteByUser removes all organization assignments for a coordinator.
// Used when a coordinator's role is changed to something else.
// Returns the number of documents deleted.
func (s *Store) DeleteByUser(ctx context.Context, userID primitive.ObjectID) (int64, error) {
	res, err := s.c.DeleteMany(ctx, bson.M{"user_id": userID})
	if err != nil {
		return 0, err
	}
	return res.DeletedCount, nil
}

// DeleteByOrg removes all coordinator assignments for an organization.
// Used when an organization is deleted.
// Returns the number of documents deleted.
func (s *Store) DeleteByOrg(ctx context.Context, orgID primitive.ObjectID) (int64, error) {
	res, err := s.c.DeleteMany(ctx, bson.M{"organization_id": orgID})
	if err != nil {
		return 0, err
	}
	return res.DeletedCount, nil
}

// Exists checks if a coordinator-organization assignment already exists.
func (s *Store) Exists(ctx context.Context, userID, orgID primitive.ObjectID) (bool, error) {
	err := s.c.FindOne(ctx, bson.M{
		"user_id":         userID,
		"organization_id": orgID,
	}).Err()
	if err == nil {
		return true, nil
	}
	if err == mongo.ErrNoDocuments {
		return false, nil
	}
	return false, err
}

// CountByUser returns the number of organizations assigned to a coordinator.
func (s *Store) CountByUser(ctx context.Context, userID primitive.ObjectID) (int64, error) {
	return s.c.CountDocuments(ctx, bson.M{"user_id": userID})
}
