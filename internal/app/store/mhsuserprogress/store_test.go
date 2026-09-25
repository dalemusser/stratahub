package mhsuserprogress_test

import (
	"testing"

	"github.com/dalemusser/stratahub/internal/app/store/mhsuserprogress"
	"github.com/dalemusser/stratahub/internal/testutil"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

var fiveUnits = []string{"unit1", "unit2", "unit3", "unit4", "unit5"}

// The end-of-game mark: set by the game's EndGame (first wins), set and
// overwritten by the staff jump, cleared by any set-to-unit.
func TestEndOfGameMark(t *testing.T) {
	db := testutil.SetupTestDB(t)
	store := mhsuserprogress.New(db)
	ctx, cancel := testutil.TestContext()
	defer cancel()
	ws, user := primitive.NewObjectID(), primitive.NewObjectID()

	// EndGame on a student with no record yet: the record is created with the mark.
	if err := store.MarkGameEnded(ctx, ws, user, mhsuserprogress.GameEndedByGame, ""); err != nil {
		t.Fatalf("MarkGameEnded: %v", err)
	}
	p, err := store.GetOrCreate(ctx, ws, user)
	if err != nil {
		t.Fatalf("GetOrCreate: %v", err)
	}
	if !p.GameEnded() || p.GameEndedBy != "game" || p.CurrentUnit != "unit1" {
		t.Fatalf("after EndGame: ended=%v by=%q current=%q", p.GameEnded(), p.GameEndedBy, p.CurrentUnit)
	}
	first := *p.GameEndedAt

	// A second EndGame (a replay) keeps the first timestamp.
	if err := store.MarkGameEnded(ctx, ws, user, mhsuserprogress.GameEndedByGame, ""); err != nil {
		t.Fatalf("MarkGameEnded again: %v", err)
	}
	if p, _ = store.GetOrCreate(ctx, ws, user); !p.GameEndedAt.Equal(first) {
		t.Fatalf("second EndGame moved the mark: %v → %v", first, p.GameEndedAt)
	}

	// Setting a unit puts the student back in the game: the mark is cleared.
	if err := store.SetToUnit(ctx, ws, user, "unit3"); err != nil {
		t.Fatalf("SetToUnit: %v", err)
	}
	if p, _ = store.GetOrCreate(ctx, ws, user); p.GameEnded() || p.GameEndedBy != "" || p.GameEndedName != "" || p.CurrentUnit != "unit3" {
		t.Fatalf("after SetToUnit: ended=%v by=%q name=%q current=%q", p.GameEnded(), p.GameEndedBy, p.GameEndedName, p.CurrentUnit)
	}

	// The staff jump: every unit complete, current "complete", mark set with the staff name.
	if err := store.JumpToEndOfGame(ctx, ws, user, fiveUnits, "Ms. Rivera"); err != nil {
		t.Fatalf("JumpToEndOfGame: %v", err)
	}
	p, _ = store.GetOrCreate(ctx, ws, user)
	if !p.GameEnded() || p.GameEndedBy != "staff" || p.GameEndedName != "Ms. Rivera" || p.CurrentUnit != "complete" || len(p.CompletedUnits) != 5 {
		t.Fatalf("after jump: ended=%v by=%q name=%q current=%q completed=%v", p.GameEnded(), p.GameEndedBy, p.GameEndedName, p.CurrentUnit, p.CompletedUnits)
	}

	// The jump works for a student with no record at all (upsert).
	other := primitive.NewObjectID()
	if err := store.JumpToEndOfGame(ctx, ws, other, fiveUnits, "Ms. Rivera"); err != nil {
		t.Fatalf("JumpToEndOfGame (new): %v", err)
	}
	if p, _ = store.GetOrCreate(ctx, ws, other); !p.GameEnded() || p.CurrentUnit != "complete" {
		t.Fatalf("after jump (new): ended=%v current=%q", p.GameEnded(), p.CurrentUnit)
	}
}
