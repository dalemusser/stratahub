// Package logdata provides read-only access to stratalog's logdata collection.
package logdata

import (
	"context"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// LogEntry represents a single log entry from stratalog.
type LogEntry struct {
	ID              primitive.ObjectID     `bson:"_id"`
	Game            string                 `bson:"game"`
	UserID          string                 `bson:"user_id"`
	EventType       string                 `bson:"eventType"`
	EventKey        string                 `bson:"eventKey,omitempty"`
	SceneName       string                 `bson:"sceneName,omitempty"`
	ServerTimestamp time.Time              `bson:"serverTimestamp"`
	Version         string                 `bson:"version,omitempty"`
	Data            map[string]interface{} `bson:"data,omitempty"`
	Device          map[string]interface{} `bson:"device,omitempty"`
}

// Store provides read-only queries against stratalog's logdata collection.
type Store struct {
	c *mongo.Collection
}

// New creates a new logdata store.
func New(db *mongo.Database) *Store {
	return &Store{c: db.Collection("logdata")}
}

// ListForUser returns all log entries for a user in a game, ordered by _id ascending.
// userID is the 24-char hex string of stratahub.users._id.
func (s *Store) ListForUser(ctx context.Context, game, userID string) ([]LogEntry, error) {
	filter := bson.M{
		"game":    game,
		"user_id": userID,
	}
	opts := options.Find().SetSort(bson.D{{Key: "_id", Value: 1}})

	cur, err := s.c.Find(ctx, filter, opts)
	if err != nil {
		return nil, err
	}
	defer cur.Close(ctx)

	var entries []LogEntry
	if err := cur.All(ctx, &entries); err != nil {
		return nil, err
	}
	return entries, nil
}

// ListForUserByScenes returns log entries for a user filtered by scene names.
// userID is the 24-char hex string of stratahub.users._id.
func (s *Store) ListForUserByScenes(ctx context.Context, game, userID string, sceneNames []string) ([]LogEntry, error) {
	filter := bson.M{
		"game":      game,
		"user_id":   userID,
		"sceneName": bson.M{"$in": sceneNames},
	}
	opts := options.Find().SetSort(bson.D{{Key: "_id", Value: 1}})

	cur, err := s.c.Find(ctx, filter, opts)
	if err != nil {
		return nil, err
	}
	defer cur.Close(ctx)

	var entries []LogEntry
	if err := cur.All(ctx, &entries); err != nil {
		return nil, err
	}
	return entries, nil
}

// CountForUser returns the total number of log entries for a user in a game.
// userID is the 24-char hex string of stratahub.users._id.
func (s *Store) CountForUser(ctx context.Context, game, userID string) (int64, error) {
	filter := bson.M{
		"game":    game,
		"user_id": userID,
	}
	return s.c.CountDocuments(ctx, filter)
}
