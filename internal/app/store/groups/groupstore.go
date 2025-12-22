// internal/app/store/groups/groupstore.go
package groupstore

import (
	"context"
	"errors"
	"strings"
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

var ErrDuplicateGroupName = errors.New("a group with this name already exists in the organization")

func New(db *mongo.Database) *Store {
	return &Store{c: db.Collection("groups")}
}

func (s *Store) GetByID(ctx context.Context, id primitive.ObjectID) (models.Group, error) {
	var g models.Group
	if err := s.c.FindOne(ctx, bson.M{"_id": id}).Decode(&g); err != nil {
		return models.Group{}, err
	}
	return g, nil
}

func (s *Store) Create(ctx context.Context, g models.Group) (models.Group, error) {
	now := time.Now().UTC()
	g.ID = primitive.NewObjectID()
	g.NameCI = text.Fold(g.Name)
	if g.Status == "" {
		g.Status = status.Active
	}
	g.CreatedAt = now
	g.UpdatedAt = now
	_, err := s.c.InsertOne(ctx, g)
	if err != nil {
		if wafflemongo.IsDup(err) {
			return models.Group{}, ErrDuplicateGroupName
		}
		return models.Group{}, err
	}
	return g, nil
}

func (s *Store) UpdateInfo(ctx context.Context, id primitive.ObjectID, name, desc, stat string) error {
	set := bson.M{
		"updated_at": time.Now().UTC(),
	}
	if strings.TrimSpace(name) != "" {
		set["name"] = name
		set["name_ci"] = text.Fold(name)
	}
	if desc != "" {
		set["description"] = desc
	}
	if stat != "" {
		if stat != status.Active {
			return mongo.CommandError{Message: "status must be active"}
		}
		set["status"] = stat
	}
	_, err := s.c.UpdateByID(ctx, id, bson.M{"$set": set})
	return err
}
