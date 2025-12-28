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
	"go.mongodb.org/mongo-driver/mongo/options"
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
	// ShowInLibrary is left as passed in (defaults to false if not set)
	r.CreatedAt = now
	r.UpdatedAt = &now

	if strings.TrimSpace(r.Title) == "" {
		return models.Resource{}, mongo.CommandError{Message: "title is required"}
	}
	// Must have either LaunchURL or FilePath
	hasURL := strings.TrimSpace(r.LaunchURL) != ""
	hasFile := strings.TrimSpace(r.FilePath) != ""
	if !hasURL && !hasFile {
		return models.Resource{}, mongo.CommandError{Message: "either launch_url or file is required"}
	}
	if hasURL && !urlutil.IsValidAbsHTTPURL(r.LaunchURL) {
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
	// Subject and Description can be cleared (set to empty)
	mut.SubjectCI = text.Fold(mut.Subject)
	set["subject"] = mut.Subject
	set["subject_ci"] = mut.SubjectCI
	set["description"] = mut.Description
	// LaunchURL can be empty if using file upload; validate only if non-empty
	set["launch_url"] = mut.LaunchURL
	if strings.TrimSpace(mut.LaunchURL) != "" {
		if !urlutil.IsValidAbsHTTPURL(mut.LaunchURL) {
			return mongo.CommandError{Message: "launch_url must be a valid http(s) URL"}
		}
	}
	// File fields
	set["file_path"] = mut.FilePath
	set["file_name"] = mut.FileName
	set["file_size"] = mut.FileSize
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
	set["default_instructions"] = mut.DefaultInstructions
	now := time.Now().UTC()
	mut.UpdatedAt = &now
	set["updated_at"] = mut.UpdatedAt
	if len(set) == 0 {
		// Nothing to update; treat as no-op
		return nil
	}
	_, err := s.c.UpdateByID(ctx, id, bson.M{"$set": set})
	if err != nil {
		if wafflemongo.IsDup(err) {
			return ErrDuplicateTitle
		}
		return err
	}
	return nil
}

// GetByID returns a resource by its ID.
func (s *Store) GetByID(ctx context.Context, id primitive.ObjectID) (models.Resource, error) {
	var r models.Resource
	err := s.c.FindOne(ctx, bson.M{"_id": id}).Decode(&r)
	if err != nil {
		return models.Resource{}, err
	}
	return r, nil
}

// Delete removes a resource by ID. Returns the number of documents deleted (0 or 1).
func (s *Store) Delete(ctx context.Context, id primitive.ObjectID) (int64, error) {
	res, err := s.c.DeleteOne(ctx, bson.M{"_id": id})
	if err != nil {
		return 0, err
	}
	return res.DeletedCount, nil
}

// GetByIDs returns multiple resources by their IDs.
func (s *Store) GetByIDs(ctx context.Context, ids []primitive.ObjectID) ([]models.Resource, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	cur, err := s.c.Find(ctx, bson.M{"_id": bson.M{"$in": ids}})
	if err != nil {
		return nil, err
	}
	defer cur.Close(ctx)
	var out []models.Resource
	if err := cur.All(ctx, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// Find returns resources matching the given filter with optional find options.
// The caller is responsible for building the filter and options (pagination, sorting, projection).
func (s *Store) Find(ctx context.Context, filter bson.M, opts ...*options.FindOptions) ([]models.Resource, error) {
	cur, err := s.c.Find(ctx, filter, opts...)
	if err != nil {
		return nil, err
	}
	defer cur.Close(ctx)

	var resources []models.Resource
	if err := cur.All(ctx, &resources); err != nil {
		return nil, err
	}
	return resources, nil
}

// Count returns the number of resources matching the given filter.
func (s *Store) Count(ctx context.Context, filter bson.M) (int64, error) {
	return s.c.CountDocuments(ctx, filter)
}
