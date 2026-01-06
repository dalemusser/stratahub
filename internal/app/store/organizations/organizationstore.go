// internal/app/store/organizations/organizationstore.go
package organizationstore

import (
	"context"
	"errors"
	"time"

	"github.com/dalemusser/stratahub/internal/app/system/status"
	"github.com/dalemusser/stratahub/internal/domain/models"
	wafflemongo "github.com/dalemusser/waffle/pantry/mongo"
	"github.com/dalemusser/waffle/pantry/text"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

type Store struct {
	c *mongo.Collection
}

var ErrDuplicateOrganization = errors.New("an organization with this name already exists")

func New(db *mongo.Database) *Store {
	return &Store{c: db.Collection("organizations")}
}

func (s *Store) Create(ctx context.Context, org models.Organization) (models.Organization, error) {
	now := time.Now().UTC()
	org.ID = primitive.NewObjectID()
	org.NameCI = text.Fold(org.Name)
	org.CityCI = text.Fold(org.City)
	org.StateCI = text.Fold(org.State)
	if org.Status == "" {
		org.Status = status.Active
	}
	org.CreatedAt = now
	org.UpdatedAt = now
	_, err := s.c.InsertOne(ctx, org)
	if err != nil {
		if wafflemongo.IsDup(err) {
			return models.Organization{}, ErrDuplicateOrganization
		}
		return models.Organization{}, err
	}
	return org, nil
}

func (s *Store) GetByID(ctx context.Context, id primitive.ObjectID) (models.Organization, error) {
	var org models.Organization
	err := s.c.FindOne(ctx, bson.M{"_id": id}).Decode(&org)
	if err != nil {
		return models.Organization{}, err
	}
	return org, nil
}

// GetByIDs loads multiple organizations by their ObjectIDs.
func (s *Store) GetByIDs(ctx context.Context, ids []primitive.ObjectID) ([]models.Organization, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	cur, err := s.c.Find(ctx, bson.M{"_id": bson.M{"$in": ids}})
	if err != nil {
		return nil, err
	}
	defer cur.Close(ctx)
	var orgs []models.Organization
	if err := cur.All(ctx, &orgs); err != nil {
		return nil, err
	}
	return orgs, nil
}

// Update modifies an organization's mutable fields and refreshes UpdatedAt.
func (s *Store) Update(ctx context.Context, id primitive.ObjectID, org models.Organization) error {
	set := bson.M{
		"updated_at": time.Now().UTC(),
	}
	if org.Name != "" {
		set["name"] = org.Name
		set["name_ci"] = text.Fold(org.Name)
	}
	if org.City != "" {
		set["city"] = org.City
		set["city_ci"] = text.Fold(org.City)
	}
	if org.State != "" {
		set["state"] = org.State
		set["state_ci"] = text.Fold(org.State)
	}
	if org.Status != "" {
		set["status"] = org.Status
	}
	if org.TimeZone != "" {
		set["time_zone"] = org.TimeZone
	}
	if org.ContactInfo != "" {
		set["contact_info"] = org.ContactInfo
	}
	_, err := s.c.UpdateByID(ctx, id, bson.M{"$set": set})
	if err != nil {
		if wafflemongo.IsDup(err) {
			return ErrDuplicateOrganization
		}
		return err
	}
	return nil
}

// Delete removes an organization by ID. Returns the number of documents deleted (0 or 1).
func (s *Store) Delete(ctx context.Context, id primitive.ObjectID) (int64, error) {
	res, err := s.c.DeleteOne(ctx, bson.M{"_id": id})
	if err != nil {
		return 0, err
	}
	return res.DeletedCount, nil
}

// ExistsByNameCI checks if an organization with the given case-insensitive name exists.
func (s *Store) ExistsByNameCI(ctx context.Context, nameCI string) (bool, error) {
	err := s.c.FindOne(ctx, bson.M{"name_ci": nameCI}).Err()
	if err == mongo.ErrNoDocuments {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}

// NameExistsForOther checks if an organization with the given name exists, excluding the specified ID.
// This is useful for update validation to ensure uniqueness while allowing the current record to keep its name.
func (s *Store) NameExistsForOther(ctx context.Context, nameCI string, excludeID primitive.ObjectID) (bool, error) {
	err := s.c.FindOne(ctx, bson.M{
		"name_ci": nameCI,
		"_id":     bson.M{"$ne": excludeID},
	}).Err()
	if err == mongo.ErrNoDocuments {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}

// Find returns organizations matching the given filter with optional find options.
// The caller is responsible for building the filter and options (pagination, sorting, projection).
func (s *Store) Find(ctx context.Context, filter bson.M, opts ...*options.FindOptions) ([]models.Organization, error) {
	cur, err := s.c.Find(ctx, filter, opts...)
	if err != nil {
		return nil, err
	}
	defer cur.Close(ctx)

	var orgs []models.Organization
	if err := cur.All(ctx, &orgs); err != nil {
		return nil, err
	}
	return orgs, nil
}

// Count returns the number of organizations matching the given filter.
func (s *Store) Count(ctx context.Context, filter bson.M) (int64, error) {
	return s.c.CountDocuments(ctx, filter)
}
