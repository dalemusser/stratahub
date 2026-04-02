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
	PlayerID        string                 `bson:"playerId"`
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

// ListForPlayer returns all log entries for a player in a game, ordered by _id ascending.
func (s *Store) ListForPlayer(ctx context.Context, game, playerID string) ([]LogEntry, error) {
	filter := bson.M{
		"game":     game,
		"playerId": playerID,
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

// ListForPlayerByScenes returns log entries for a player filtered by scene names.
func (s *Store) ListForPlayerByScenes(ctx context.Context, game, playerID string, sceneNames []string) ([]LogEntry, error) {
	filter := bson.M{
		"game":      game,
		"playerId":  playerID,
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

// CountForPlayer returns the total number of log entries for a player in a game.
func (s *Store) CountForPlayer(ctx context.Context, game, playerID string) (int64, error) {
	filter := bson.M{
		"game":     game,
		"playerId": playerID,
	}
	return s.c.CountDocuments(ctx, filter)
}
