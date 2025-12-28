// internal/app/store/materials/materialstore.go
package materialstore

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

var ErrDuplicateTitle = errors.New("a material with this title already exists")

func New(db *mongo.Database) *Store {
	return &Store{c: db.Collection("materials")}
}

// Create inserts a new Material, setting TitleCI/SubjectCI and timestamps.
// Materials must have either LaunchURL or FilePath (mutually exclusive).
func (s *Store) Create(ctx context.Context, m models.Material) (models.Material, error) {
	now := time.Now().UTC()

	m.ID = primitive.NewObjectID()
	m.TitleCI = text.Fold(m.Title)
	if m.Subject != "" {
		m.SubjectCI = text.Fold(m.Subject)
	}
	if m.Status == "" {
		m.Status = status.Active
	}
	if m.Type == "" {
		m.Type = models.DefaultMaterialType
	}
	m.CreatedAt = now
	m.UpdatedAt = &now

	// Validation
	if strings.TrimSpace(m.Title) == "" {
		return models.Material{}, mongo.CommandError{Message: "title is required"}
	}
	if !status.IsValid(m.Status) {
		return models.Material{}, mongo.CommandError{Message: "status must be 'active' or 'disabled'"}
	}
	if strings.TrimSpace(m.Type) == "" {
		return models.Material{}, mongo.CommandError{Message: "type is required"}
	}

	// Must have either URL or file, but not both
	hasURL := strings.TrimSpace(m.LaunchURL) != ""
	hasFile := strings.TrimSpace(m.FilePath) != ""
	if !hasURL && !hasFile {
		return models.Material{}, mongo.CommandError{Message: "either launch_url or file is required"}
	}
	if hasURL && hasFile {
		return models.Material{}, mongo.CommandError{Message: "cannot have both launch_url and file"}
	}
	if hasURL && !urlutil.IsValidAbsHTTPURL(m.LaunchURL) {
		return models.Material{}, mongo.CommandError{Message: "launch_url must be a valid http(s) URL"}
	}

	_, err := s.c.InsertOne(ctx, m)
	if err != nil {
		if wafflemongo.IsDup(err) {
			return models.Material{}, ErrDuplicateTitle
		}
		return models.Material{}, err
	}
	return m, nil
}

// Update modifies mutable fields and refreshes UpdatedAt.
func (s *Store) Update(ctx context.Context, id primitive.ObjectID, mut models.Material) error {
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
	set["default_instructions"] = mut.DefaultInstructions

	if mut.Status != "" {
		if !status.IsValid(mut.Status) {
			return mongo.CommandError{Message: "status must be 'active' or 'disabled'"}
		}
		set["status"] = mut.Status
	}
	if strings.TrimSpace(mut.Type) != "" {
		set["type"] = mut.Type
	}

	// Handle URL/file updates - allow clearing or changing
	// LaunchURL
	if mut.LaunchURL != "" {
		if !urlutil.IsValidAbsHTTPURL(mut.LaunchURL) {
			return mongo.CommandError{Message: "launch_url must be a valid http(s) URL"}
		}
		set["launch_url"] = mut.LaunchURL
		// Clear file fields when switching to URL
		set["file_path"] = ""
		set["file_name"] = ""
		set["file_size"] = int64(0)
	}

	// File fields (set by caller when uploading new file)
	if mut.FilePath != "" {
		set["file_path"] = mut.FilePath
		set["file_name"] = mut.FileName
		set["file_size"] = mut.FileSize
		// Clear URL when switching to file
		set["launch_url"] = ""
	}

	// Audit fields
	if mut.UpdatedByID != nil {
		set["updated_by_id"] = mut.UpdatedByID
		set["updated_by_name"] = mut.UpdatedByName
	}

	now := time.Now().UTC()
	set["updated_at"] = now

	if len(set) == 0 {
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

// GetByID returns a material by its ID.
func (s *Store) GetByID(ctx context.Context, id primitive.ObjectID) (models.Material, error) {
	var m models.Material
	err := s.c.FindOne(ctx, bson.M{"_id": id}).Decode(&m)
	if err != nil {
		return models.Material{}, err
	}
	return m, nil
}

// Delete removes a material by ID. Returns the number of documents deleted (0 or 1).
func (s *Store) Delete(ctx context.Context, id primitive.ObjectID) (int64, error) {
	res, err := s.c.DeleteOne(ctx, bson.M{"_id": id})
	if err != nil {
		return 0, err
	}
	return res.DeletedCount, nil
}

// GetByIDs returns multiple materials by their IDs.
func (s *Store) GetByIDs(ctx context.Context, ids []primitive.ObjectID) ([]models.Material, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	cur, err := s.c.Find(ctx, bson.M{"_id": bson.M{"$in": ids}})
	if err != nil {
		return nil, err
	}
	defer cur.Close(ctx)
	var out []models.Material
	if err := cur.All(ctx, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// Find returns materials matching the given filter with optional find options.
func (s *Store) Find(ctx context.Context, filter bson.M, opts ...*options.FindOptions) ([]models.Material, error) {
	cur, err := s.c.Find(ctx, filter, opts...)
	if err != nil {
		return nil, err
	}
	defer cur.Close(ctx)

	var materials []models.Material
	if err := cur.All(ctx, &materials); err != nil {
		return nil, err
	}
	return materials, nil
}

// Count returns the number of materials matching the given filter.
func (s *Store) Count(ctx context.Context, filter bson.M) (int64, error) {
	return s.c.CountDocuments(ctx, filter)
}
