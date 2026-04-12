// internal/app/store/globalsettings/store.go
package globalsettings

import (
	"context"
	"sync"
	"time"

	"github.com/dalemusser/stratahub/internal/domain/models"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// Store provides access to the global_settings collection.
// It uses an in-memory cache with a short TTL to avoid hitting
// MongoDB on every request through the maintenance middleware.
type Store struct {
	c *mongo.Collection

	mu     sync.RWMutex
	cached *models.GlobalSettings
	expiry time.Time
}

// cacheTTL is how long the cached settings are valid.
const cacheTTL = 30 * time.Second

// New creates a new global settings store.
func New(db *mongo.Database) *Store {
	return &Store{c: db.Collection("global_settings")}
}

// Get returns the global settings, using the in-memory cache when possible.
// If no document exists, returns defaults (maintenance mode off).
func (s *Store) Get(ctx context.Context) (models.GlobalSettings, error) {
	s.mu.RLock()
	if s.cached != nil && time.Now().Before(s.expiry) {
		gs := *s.cached
		s.mu.RUnlock()
		return gs, nil
	}
	s.mu.RUnlock()

	// Cache miss — fetch from DB
	var gs models.GlobalSettings
	err := s.c.FindOne(ctx, bson.M{"_id": models.GlobalSettingsID}).Decode(&gs)
	if err == mongo.ErrNoDocuments {
		gs = models.GlobalSettings{ID: models.GlobalSettingsID}
		s.setCache(&gs)
		return gs, nil
	}
	if err != nil {
		return gs, err
	}

	s.setCache(&gs)
	return gs, nil
}

// Save updates the global settings (upsert).
// Invalidates the cache so the next Get picks up the change immediately.
func (s *Store) Save(ctx context.Context, gs models.GlobalSettings) error {
	gs.ID = models.GlobalSettingsID
	gs.UpdatedAt = time.Now().UTC()

	opts := options.Update().SetUpsert(true)
	_, err := s.c.UpdateOne(ctx, bson.M{"_id": models.GlobalSettingsID}, bson.M{
		"$set": bson.M{
			"maintenance_mode":    gs.MaintenanceMode,
			"maintenance_message": gs.MaintenanceMessage,
			"updated_at":          gs.UpdatedAt,
			"updated_by_name":     gs.UpdatedByName,
		},
	}, opts)

	if err == nil {
		s.setCache(&gs)
	}
	return err
}

func (s *Store) setCache(gs *models.GlobalSettings) {
	s.mu.Lock()
	defer s.mu.Unlock()
	copy := *gs
	s.cached = &copy
	s.expiry = time.Now().Add(cacheTTL)
}

// InvalidateCache forces the next Get to read from the database.
func (s *Store) InvalidateCache() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.cached = nil
}
