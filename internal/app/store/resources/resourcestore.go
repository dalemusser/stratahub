// internal/store/resources/resourcestore.go
package resourcestore

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/dalemusser/stratahub/internal/app/system/status"
	"github.com/dalemusser/stratahub/internal/domain/models"
	wafflemongo "github.com/dalemusser/waffle/pantry/mongo"
	"github.com/dalemusser/waffle/pantry/text"
	"github.com/dalemusser/waffle/pantry/urlutil"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
)

type Store struct {
	c *mongo.Collection
}

var ErrDuplicateTitle = errors.New("a resource with this title already exists")

func New(db *mongo.Database) *Store {
	return &Store{c: db.Collection("resources")}
}

// Create inserts a new Resource, setting TitleCI/SubjectCI and timestamps.
// It lightly validates LaunchURL, Status, and Type.
func (s *Store) Create(ctx context.Context, r models.Resource) (models.Resource, error) {
	now := time.Now().UTC()

	r.ID = primitive.NewObjectID()
	r.TitleCI = text.Fold(r.Title)
	if r.Subject != "" {
		r.SubjectCI = text.Fold(r.Subject)
	}
	if r.Status == "" {
		r.Status = status.Active
	}
	if r.Type == "" {
		r.Type = "game"
	}
	r.ShowInLibrary = true
	r.CreatedAt = now
	r.UpdatedAt = &now

	if strings.TrimSpace(r.Title) == "" {
		return models.Resource{}, mongo.CommandError{Message: "title is required"}
	}
	if strings.TrimSpace(r.LaunchURL) == "" {
		return models.Resource{}, mongo.CommandError{Message: "launch_url is required"}
	}
	if !urlutil.IsValidAbsHTTPURL(r.LaunchURL) {
		return models.Resource{}, mongo.CommandError{Message: "launch_url must be a valid http(s) URL"}
	}
	if !status.IsValid(r.Status) {
		return models.Resource{}, mongo.CommandError{Message: "status must be 'active' or 'disabled'"}
	}
	if strings.TrimSpace(r.Type) == "" {
		return models.Resource{}, mongo.CommandError{Message: "type is required"}
	}

	_, err := s.c.InsertOne(ctx, r)
	if err != nil {
		if wafflemongo.IsDup(err) {
			return models.Resource{}, ErrDuplicateTitle
		}
		return models.Resource{}, err
	}
	return r, nil
}

// Update modifies mutable fields and refreshes UpdatedAt.
func (s *Store) Update(ctx context.Context, id primitive.ObjectID, mut models.Resource) error {
	// Build a selective $set so we don't clobber unset fields.
	set := bson.M{}
	if strings.TrimSpace(mut.Title) != "" {
		mut.TitleCI = text.Fold(mut.Title)
		set["title"] = mut.Title
		set["title_ci"] = mut.TitleCI
	}
	if strings.TrimSpace(mut.Subject) != "" {
		mut.SubjectCI = text.Fold(mut.Subject)
		set["subject"] = mut.Subject
		set["subject_ci"] = mut.SubjectCI
	}
	if mut.Description != "" {
		set["description"] = mut.Description
	}
	if strings.TrimSpace(mut.LaunchURL) != "" {
		if !urlutil.IsValidAbsHTTPURL(mut.LaunchURL) {
			return mongo.CommandError{Message: "launch_url must be a valid http(s) URL"}
		}
		set["launch_url"] = mut.LaunchURL
	}
	if mut.Status != "" {
		if !status.IsValid(mut.Status) {
			return mongo.CommandError{Message: "status must be 'active' or 'disabled'"}
		}
		set["status"] = mut.Status
	}
	if strings.TrimSpace(mut.Type) != "" {
		set["type"] = mut.Type
	}
	set["show_in_library"] = mut.ShowInLibrary
	if mut.DefaultInstructions != "" {
		set["default_instructions"] = mut.DefaultInstructions
	}
	now := time.Now().UTC()
	mut.UpdatedAt = &now
	set["updated_at"] = mut.UpdatedAt
	if len(set) == 0 {
		// Nothing to update; treat as no-op
		return nil
	}
	_, err := s.c.UpdateByID(ctx, id, bson.M{"$set": set})
	return err
}
