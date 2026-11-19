package mhsactivity

import (
	"testing"
	"time"

	"github.com/dalemusser/stratahub/internal/testutil"
	"go.mongodb.org/mongo-driver/bson"
)

// The save service's answer for a day: a save or a settings write in the day
// is arrived; none, with an older save kept or no save at all, is none; none
// while every kept save is newer than the day (older ones dropped) is unknown.
func TestReadSaves(t *testing.T) {
	db := testutil.SetupTestDB(t)
	ctx, cancel := testutil.TestContext()
	defer cancel()
	start := time.Date(2026, 11, 18, 6, 0, 0, 0, time.UTC)
	end := start.Add(24 * time.Hour)
	state := func(user string, at time.Time) bson.M {
		return bson.M{"user_id": user, "game": Game, "timestamp": at, "save_data": bson.M{}}
	}
	docs := []interface{}{
		state("saved", start.Add(3*time.Hour)),
		state("older", start.Add(-48*time.Hour)),
		state("pruned", end.Add(2*time.Hour)),
		state("settings", end.Add(2*time.Hour)),
		state("othergame", start.Add(time.Hour)),
	}
	docs[4].(bson.M)["game"] = "other"
	if _, err := db.Collection(saveStatesCollection).InsertMany(ctx, docs); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Collection(saveSettingsCollection).InsertOne(ctx, bson.M{"user_id": "settings", "game": Game, "timestamp": start.Add(time.Hour)}); err != nil {
		t.Fatal(err)
	}
	got, err := readSaves(ctx, db, []string{"saved", "older", "pruned", "settings", "never", "othergame"}, start, end)
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]saveState{"saved": saveArrived, "older": saveNone, "pruned": saveUnknown, "settings": saveArrived, "never": saveNone, "othergame": saveNone}
	for id, w := range want {
		if got[id] != w {
			t.Errorf("%s: %v, want %v", id, got[id], w)
		}
	}
}
