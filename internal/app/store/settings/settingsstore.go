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
// There is only one document in this collection.
type Store struct {
	c *mongo.Collection
}

// New creates a new settings store.
func New(db *mongo.Database) *Store {
	return &Store{c: db.Collection("site_settings")}
}

// Get returns the site settings.
// If no settings exist, returns default settings with an empty ID.
func (s *Store) Get(ctx context.Context) (models.SiteSettings, error) {
	var settings models.SiteSettings
	err := s.c.FindOne(ctx, bson.M{}).Decode(&settings)
	if err == mongo.ErrNoDocuments {
		// Return default settings
		return models.SiteSettings{
			SiteName: models.DefaultSiteName,
		}, nil
	}
	if err != nil {
		return models.SiteSettings{}, err
	}
	return settings, nil
}

// Save updates the site settings.
// Uses upsert so it works whether settings exist or not.
func (s *Store) Save(ctx context.Context, settings models.SiteSettings) error {
	now := time.Now().UTC()
	settings.UpdatedAt = &now

	// Use upsert with an empty filter to update the single document or create it
	filter := bson.M{}
	update := bson.M{
		"$set": bson.M{
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

// Exists checks if settings have been saved.
func (s *Store) Exists(ctx context.Context) (bool, error) {
	count, err := s.c.CountDocuments(ctx, bson.M{})
	if err != nil {
		return false, err
	}
	return count > 0, nil
}
