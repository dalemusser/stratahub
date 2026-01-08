// internal/app/store/workspaces/workspacestore.go
package workspacestore

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

var (
	ErrDuplicateName      = errors.New("a workspace with this name already exists")
	ErrDuplicateSubdomain = errors.New("a workspace with this subdomain already exists")
	ErrNotFound           = errors.New("workspace not found")
)

func New(db *mongo.Database) *Store {
	return &Store{c: db.Collection("workspaces")}
}

// Create inserts a new workspace.
func (s *Store) Create(ctx context.Context, ws models.Workspace) (models.Workspace, error) {
	now := time.Now().UTC()
	ws.ID = primitive.NewObjectID()
	ws.NameCI = text.Fold(ws.Name)
	if ws.Status == "" {
		ws.Status = status.Active
	}
	ws.CreatedAt = now
	ws.UpdatedAt = now
	_, err := s.c.InsertOne(ctx, ws)
	if err != nil {
		if wafflemongo.IsDup(err) {
			// Could be either name or subdomain duplicate
			return models.Workspace{}, ErrDuplicateName
		}
		return models.Workspace{}, err
	}
	return ws, nil
}

// GetByID retrieves a workspace by its ID.
func (s *Store) GetByID(ctx context.Context, id primitive.ObjectID) (models.Workspace, error) {
	var ws models.Workspace
	err := s.c.FindOne(ctx, bson.M{"_id": id}).Decode(&ws)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			return models.Workspace{}, ErrNotFound
		}
		return models.Workspace{}, err
	}
	return ws, nil
}

// GetBySubdomain retrieves a workspace by its subdomain.
func (s *Store) GetBySubdomain(ctx context.Context, subdomain string) (models.Workspace, error) {
	var ws models.Workspace
	err := s.c.FindOne(ctx, bson.M{"subdomain": subdomain}).Decode(&ws)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			return models.Workspace{}, ErrNotFound
		}
		return models.Workspace{}, err
	}
	return ws, nil
}

// Update modifies a workspace's mutable fields.
func (s *Store) Update(ctx context.Context, id primitive.ObjectID, ws models.Workspace) error {
	set := bson.M{
		"updated_at": time.Now().UTC(),
	}
	if ws.Name != "" {
		set["name"] = ws.Name
		set["name_ci"] = text.Fold(ws.Name)
	}
	if ws.Subdomain != "" {
		set["subdomain"] = ws.Subdomain
	}
	if ws.Status != "" {
		set["status"] = ws.Status
	}
	// Logo fields - allow empty string to clear
	set["logo_path"] = ws.LogoPath
	set["logo_name"] = ws.LogoName

	_, err := s.c.UpdateByID(ctx, id, bson.M{"$set": set})
	if err != nil {
		if wafflemongo.IsDup(err) {
			return ErrDuplicateName
		}
		return err
	}
	return nil
}

// Delete removes a workspace by ID.
func (s *Store) Delete(ctx context.Context, id primitive.ObjectID) (int64, error) {
	res, err := s.c.DeleteOne(ctx, bson.M{"_id": id})
	if err != nil {
		return 0, err
	}
	return res.DeletedCount, nil
}

// Find returns workspaces matching the given filter.
func (s *Store) Find(ctx context.Context, filter bson.M, opts ...*options.FindOptions) ([]models.Workspace, error) {
	cur, err := s.c.Find(ctx, filter, opts...)
	if err != nil {
		return nil, err
	}
	defer cur.Close(ctx)

	var workspaces []models.Workspace
	if err := cur.All(ctx, &workspaces); err != nil {
		return nil, err
	}
	return workspaces, nil
}

// Count returns the number of workspaces matching the given filter.
func (s *Store) Count(ctx context.Context, filter bson.M) (int64, error) {
	return s.c.CountDocuments(ctx, filter)
}

// GetFirst returns the first workspace (for single-workspace deployments).
// Returns ErrNotFound if no workspaces exist.
func (s *Store) GetFirst(ctx context.Context) (models.Workspace, error) {
	var ws models.Workspace
	opts := options.FindOne().SetSort(bson.D{{Key: "created_at", Value: 1}})
	err := s.c.FindOne(ctx, bson.M{}, opts).Decode(&ws)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			return models.Workspace{}, ErrNotFound
		}
		return models.Workspace{}, err
	}
	return ws, nil
}

// EnsureIndexes creates indexes for the workspaces collection.
func (s *Store) EnsureIndexes(ctx context.Context) error {
	indexes := []mongo.IndexModel{
		// Unique subdomain for URL routing
		{
			Keys:    bson.D{{Key: "subdomain", Value: 1}},
			Options: options.Index().SetUnique(true).SetName("idx_workspace_subdomain"),
		},
		// Case-insensitive name for sorting
		{
			Keys:    bson.D{{Key: "name_ci", Value: 1}},
			Options: options.Index().SetName("idx_workspace_name_ci"),
		},
		// Status filter
		{
			Keys:    bson.D{{Key: "status", Value: 1}},
			Options: options.Index().SetName("idx_workspace_status"),
		},
	}
	_, err := s.c.Indexes().CreateMany(ctx, indexes)
	return err
}

// EnsureDefault creates a default workspace if none exists.
// Returns the existing or newly created workspace.
func (s *Store) EnsureDefault(ctx context.Context, name, subdomain string) (models.Workspace, error) {
	// Check if any workspace exists
	count, err := s.c.CountDocuments(ctx, bson.M{})
	if err != nil {
		return models.Workspace{}, err
	}
	if count > 0 {
		// Return the first workspace
		return s.GetFirst(ctx)
	}

	// Create default workspace
	ws := models.Workspace{
		Name:      name,
		Subdomain: subdomain,
		Status:    status.Active,
	}
	return s.Create(ctx, ws)
}
