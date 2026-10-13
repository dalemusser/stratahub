// Package logdata provides read-only access to stratalog's logdata collection.
package logdata

import (
	"context"
	"encoding/binary"
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

// HasEntrySince reports whether the user has at least one entry created at
// or after since. Entry ids are time-ordered ObjectIDs assigned on insert,
// so with the {game, user_id, _id} index this is a single index seek.
func (s *Store) HasEntrySince(ctx context.Context, game, userID string, since time.Time) (bool, error) {
	filter := bson.M{
		"game":    game,
		"user_id": userID,
		"_id":     bson.M{"$gte": primitive.NewObjectIDFromTimestamp(since.UTC())},
	}
	opts := options.FindOne().SetProjection(bson.M{"_id": 1})
	err := s.c.FindOne(ctx, filter, opts).Err()
	if err == mongo.ErrNoDocuments {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
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

// UserActivity is one user's game log over a window.
type UserActivity struct {
	Events  int
	First   time.Time
	Last    time.Time
	Crashes int // entries with eventType "crash"
}

// activityChunk bounds the $in list of one aggregate.
const activityChunk = 200

// ActivityByUsers counts each user's entries received in [from, to), with
// the first and last receive times and the crash events among them. Entry
// ids are time-ordered ObjectIDs assigned on insert, so the range is an
// _id range per user on the {game, user_id, _id} index. Users without
// entries are absent from the result.
func (s *Store) ActivityByUsers(ctx context.Context, game string, userIDs []string, from, to time.Time) (map[string]UserActivity, error) {
	out := make(map[string]UserActivity, len(userIDs))
	lo, hi := ObjectIDAt(from), ObjectIDAt(to)
	for start := 0; start < len(userIDs); start += activityChunk {
		end := start + activityChunk
		if end > len(userIDs) {
			end = len(userIDs)
		}
		pipeline := mongo.Pipeline{
			{{Key: "$match", Value: bson.M{
				"game":    game,
				"user_id": bson.M{"$in": userIDs[start:end]},
				"_id":     bson.M{"$gte": lo, "$lt": hi},
			}}},
			{{Key: "$group", Value: bson.M{
				"_id":     "$user_id",
				"n":       bson.M{"$sum": 1},
				"first":   bson.M{"$min": "$serverTimestamp"},
				"last":    bson.M{"$max": "$serverTimestamp"},
				"crashes": bson.M{"$sum": bson.M{"$cond": bson.A{bson.M{"$eq": bson.A{"$eventType", "crash"}}, 1, 0}}},
			}}},
		}
		cur, err := s.c.Aggregate(ctx, pipeline)
		if err != nil {
			return nil, err
		}
		var rows []struct {
			ID      string    `bson:"_id"`
			N       int       `bson:"n"`
			First   time.Time `bson:"first"`
			Last    time.Time `bson:"last"`
			Crashes int       `bson:"crashes"`
		}
		err = cur.All(ctx, &rows)
		cur.Close(ctx)
		if err != nil {
			return nil, err
		}
		for _, r := range rows {
			out[r.ID] = UserActivity{Events: r.N, First: r.First, Last: r.Last, Crashes: r.Crashes}
		}
	}
	return out, nil
}

// ObjectIDAt is the smallest ObjectID whose embedded time is t (seconds),
// for exact time-range bounds on _id.
func ObjectIDAt(t time.Time) primitive.ObjectID {
	var id primitive.ObjectID
	binary.BigEndian.PutUint32(id[0:4], uint32(t.Unix()))
	return id
}
