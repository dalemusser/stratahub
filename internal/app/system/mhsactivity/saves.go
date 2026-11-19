// internal/app/system/mhsactivity/saves.go
package mhsactivity

import (
	"context"
	"errors"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// Whether a student's game saves reached the save service (stratasave) on a
// day. The page's probe of the save service can miss on a slow network while
// the game's own saves get through, so a block found while the game ran that
// names the save service is checked against the saves themselves.
//
// The save service keeps only the newest few saves per student (its
// max_saves_per_user, 5 in 2026-11), so a day's saves can be gone once the
// student has played on later days. A day without saves is known only when
// the student's oldest kept save is older than the day (nothing of the day
// was dropped) or the student has none; otherwise it is unknown. A day is
// stored when it closes, a few hours after it ends, before the next class
// day, so the stored result nearly always knows.

// saveState is what the save service says about one student's day.
type saveState int

const (
	saveUnknown saveState = iota // not read, unreadable, or the day's saves may have been dropped
	saveNone                     // no save that day, and none could have been dropped
	saveArrived                  // a save (or a settings write) reached the save service that day
)

// Collections of the save service (stratasave saveapi and settingsapi).
const (
	saveStatesCollection   = "player_states"
	saveSettingsCollection = "player_settings"
)

// readSaves returns the save state of each student's day [start, end).
func readSaves(ctx context.Context, db *mongo.Database, hexIDs []string, start, end time.Time) (map[string]saveState, error) {
	states := db.Collection(saveStatesCollection)
	settings := db.Collection(saveSettingsCollection)
	out := make(map[string]saveState, len(hexIDs))
	for _, id := range hexIDs {
		n, err := states.CountDocuments(ctx, bson.M{"game": Game, "user_id": id, "timestamp": bson.M{"$gte": start, "$lt": end}}, options.Count().SetLimit(1))
		if err != nil {
			return nil, err
		}
		if n > 0 {
			out[id] = saveArrived
			continue
		}
		// The settings document is one per student, rewritten on each write.
		var set struct {
			Timestamp time.Time `bson:"timestamp"`
		}
		err = settings.FindOne(ctx, bson.M{"game": Game, "user_id": id}, options.FindOne().SetProjection(bson.M{"timestamp": 1})).Decode(&set)
		if err == nil && !set.Timestamp.Before(start) && set.Timestamp.Before(end) {
			out[id] = saveArrived
			continue
		}
		if err != nil && !errors.Is(err, mongo.ErrNoDocuments) {
			return nil, err
		}
		var oldest struct {
			Timestamp time.Time `bson:"timestamp"`
		}
		err = states.FindOne(ctx, bson.M{"game": Game, "user_id": id},
			options.FindOne().SetSort(bson.D{{Key: "timestamp", Value: 1}}).SetProjection(bson.M{"timestamp": 1})).Decode(&oldest)
		switch {
		case errors.Is(err, mongo.ErrNoDocuments):
			out[id] = saveNone
		case err != nil:
			return nil, err
		case oldest.Timestamp.Before(start):
			out[id] = saveNone
		default:
			out[id] = saveUnknown
		}
	}
	return out, nil
}
