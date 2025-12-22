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
