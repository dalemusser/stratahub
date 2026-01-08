// internal/app/store/settings/settingsstore.go
package settingsstore

import (
	"context"
	"time"

	"github.com/dalemusser/stratahub/internal/domain/models"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// Store provides access to the site_settings collection.
// Each workspace has its own settings document (one document per workspace_id).
type Store struct {
	c *mongo.Collection
}

// New creates a new settings store.
func New(db *mongo.Database) *Store {
	return &Store{c: db.Collection("site_settings")}
}

// Get returns the site settings for a specific workspace.
// If no settings exist for the workspace, returns default settings.
func (s *Store) Get(ctx context.Context, workspaceID primitive.ObjectID) (models.SiteSettings, error) {
	var settings models.SiteSettings
	filter := bson.M{"workspace_id": workspaceID}
	err := s.c.FindOne(ctx, filter).Decode(&settings)
	if err == mongo.ErrNoDocuments {
		// Return default settings for this workspace
		return models.SiteSettings{
			WorkspaceID: workspaceID,
			SiteName:    models.DefaultSiteName,
		}, nil
	}
	if err != nil {
		return models.SiteSettings{}, err
	}
	return settings, nil
}

// Save updates the site settings for a specific workspace.
// Uses upsert so it works whether settings exist or not.
func (s *Store) Save(ctx context.Context, workspaceID primitive.ObjectID, settings models.SiteSettings) error {
	now := time.Now().UTC()
	settings.UpdatedAt = &now
	settings.WorkspaceID = workspaceID

	// Use upsert with workspace_id filter
	filter := bson.M{"workspace_id": workspaceID}
	update := bson.M{
		"$set": bson.M{
			"workspace_id":    workspaceID,
			"site_name":       settings.SiteName,
			"logo_path":       settings.LogoPath,
			"logo_name":       settings.LogoName,
			"footer_html":     settings.FooterHTML,
			"updated_at":      settings.UpdatedAt,
			"updated_by_id":   settings.UpdatedByID,
			"updated_by_name": settings.UpdatedByName,
		},
		"$setOnInsert": bson.M{
			"_id": primitive.NewObjectID(),
		},
	}

	opts := options.Update().SetUpsert(true)
	_, err := s.c.UpdateOne(ctx, filter, update, opts)
	return err
}

// Exists checks if settings have been saved for a specific workspace.
func (s *Store) Exists(ctx context.Context, workspaceID primitive.ObjectID) (bool, error) {
	filter := bson.M{"workspace_id": workspaceID}
	count, err := s.c.CountDocuments(ctx, filter)
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

// Delete removes settings for a specific workspace.
// Used when deleting a workspace.
func (s *Store) Delete(ctx context.Context, workspaceID primitive.ObjectID) error {
	filter := bson.M{"workspace_id": workspaceID}
	_, err := s.c.DeleteOne(ctx, filter)
	return err
}
