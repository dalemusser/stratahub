// internal/app/store/mhsbuilds/store.go
package mhsbuilds

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

// ErrNotFound is returned when a build record does not exist.
var ErrNotFound = errors.New("mhs build not found")

// ErrDuplicate is returned when a build with the same unit+version already exists.
var ErrDuplicate = errors.New("mhs build already exists for this unit and version")

// Store provides access to the mhs_builds collection.
type Store struct {
	c *mongo.Collection
}

// New creates a new MHS builds store.
func New(db *mongo.Database) *Store {
	return &Store{c: db.Collection("mhs_builds")}
}

// Create inserts a new build record.
func (s *Store) Create(ctx context.Context, build models.MHSBuild) (primitive.ObjectID, error) {
	if build.ID.IsZero() {
		build.ID = primitive.NewObjectID()
	}
	if build.CreatedAt.IsZero() {
		build.CreatedAt = time.Now().UTC()
	}
	_, err := s.c.InsertOne(ctx, build)
	if mongo.IsDuplicateKeyError(err) {
		return primitive.NilObjectID, ErrDuplicate
	}
	return build.ID, err
}

// GetByID returns a build by its ID.
func (s *Store) GetByID(ctx context.Context, id primitive.ObjectID) (models.MHSBuild, error) {
	var b models.MHSBuild
	err := s.c.FindOne(ctx, bson.M{"_id": id}).Decode(&b)
	if err == mongo.ErrNoDocuments {
		return b, ErrNotFound
	}
	return b, err
}

// GetByUnitVersion returns the build for a specific unit and version.
func (s *Store) GetByUnitVersion(ctx context.Context, unitID, version string) (models.MHSBuild, error) {
	var b models.MHSBuild
	filter := bson.M{"unit_id": unitID, "version": version}
	err := s.c.FindOne(ctx, filter).Decode(&b)
	if err == mongo.ErrNoDocuments {
		return b, ErrNotFound
	}
	return b, err
}

// ListByUnit returns all builds for a unit, ordered by created_at descending.
func (s *Store) ListByUnit(ctx context.Context, unitID string) ([]models.MHSBuild, error) {
	filter := bson.M{"unit_id": unitID}
	opts := options.Find().SetSort(bson.D{{Key: "created_at", Value: -1}})
	cursor, err := s.c.Find(ctx, filter, opts)
	if err != nil {
		return nil, err
	}
	var builds []models.MHSBuild
	if err := cursor.All(ctx, &builds); err != nil {
		return nil, err
	}
	return builds, nil
}

// LatestByUnit returns the most recently created build for a unit.
func (s *Store) LatestByUnit(ctx context.Context, unitID string) (models.MHSBuild, error) {
	var b models.MHSBuild
	filter := bson.M{"unit_id": unitID}
	opts := options.FindOne().SetSort(bson.D{{Key: "created_at", Value: -1}})
	err := s.c.FindOne(ctx, filter, opts).Decode(&b)
	if err == mongo.ErrNoDocuments {
		return b, ErrNotFound
	}
	return b, err
}

// ListAll returns all builds, ordered by unit_id then version.
func (s *Store) ListAll(ctx context.Context) ([]models.MHSBuild, error) {
	opts := options.Find().SetSort(bson.D{{Key: "unit_id", Value: 1}, {Key: "version", Value: 1}})
	cursor, err := s.c.Find(ctx, bson.M{}, opts)
	if err != nil {
		return nil, err
	}
	var builds []models.MHSBuild
	if err := cursor.All(ctx, &builds); err != nil {
		return nil, err
	}
	return builds, nil
}

// Delete removes a build record by ID.
func (s *Store) Delete(ctx context.Context, id primitive.ObjectID) error {
	_, err := s.c.DeleteOne(ctx, bson.M{"_id": id})
	return err
}

// DeleteByUnitVersion removes a build record by unit and version.
func (s *Store) DeleteByUnitVersion(ctx context.Context, unitID, version string) error {
	_, err := s.c.DeleteOne(ctx, bson.M{"unit_id": unitID, "version": version})
	return err
}

// UpdateFiles updates the file list, sizes, and key files for an existing build record.
// Preserves the build identifier and other metadata.
func (s *Store) UpdateFiles(ctx context.Context, unitID, version string, files []models.MHSBuildFile, totalSize int64, dataFile, frameworkFile, codeFile string) error {
	filter := bson.M{"unit_id": unitID, "version": version}
	update := bson.M{"$set": bson.M{
		"files":          files,
		"total_size":     totalSize,
		"data_file":      dataFile,
		"framework_file": frameworkFile,
		"code_file":      codeFile,
	}}
	_, err := s.c.UpdateOne(ctx, filter, update)
	return err
}

// GetByUnitVersionBatch looks up multiple unit+version pairs in a single query.
// Returns a map keyed by "unitID:version".
func (s *Store) GetByUnitVersionBatch(ctx context.Context, pairs []UnitVersionPair) (map[string]models.MHSBuild, error) {
	if len(pairs) == 0 {
		return make(map[string]models.MHSBuild), nil
	}

	// Build $or query for all pairs
	orConditions := make([]bson.M, len(pairs))
	for i, p := range pairs {
		orConditions[i] = bson.M{"unit_id": p.UnitID, "version": p.Version}
	}

	cursor, err := s.c.Find(ctx, bson.M{"$or": orConditions})
	if err != nil {
		return nil, err
	}

	result := make(map[string]models.MHSBuild, len(pairs))
	var builds []models.MHSBuild
	if err := cursor.All(ctx, &builds); err != nil {
		return nil, err
	}
	for _, b := range builds {
		result[b.UnitID+":"+b.Version] = b
	}
	return result, nil
}

// UnitVersionPair is a unit ID + version for batch lookup.
type UnitVersionPair struct {
	UnitID  string
	Version string
}

// Exists checks if a build exists for the given unit and version.
func (s *Store) Exists(ctx context.Context, unitID, version string) (bool, error) {
	filter := bson.M{"unit_id": unitID, "version": version}
	count, err := s.c.CountDocuments(ctx, filter)
	if err != nil {
		return false, err
	}
	return count > 0, nil
}
