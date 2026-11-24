package mhscollections_test

import (
	"testing"
	"time"

	"github.com/dalemusser/stratahub/internal/app/store/mhscollections"
	"github.com/dalemusser/stratahub/internal/domain/models"
	"github.com/dalemusser/stratahub/internal/testutil"
)

// An upload fills the units it leaves out from LatestBase: the newest collection that is
// not a test collection, so a test build never passes into someone else's upload.
func TestLatestBase_SkipsTestCollections(t *testing.T) {
	db := testutil.SetupTestDB(t)
	store := mhscollections.New(db)
	ctx, cancel := testutil.TestContext()
	defer cancel()

	mk := func(name string, test bool) models.MHSCollection {
		c := models.MHSCollection{Name: name, TestOnly: test, Units: []models.MHSCollectionUnit{{UnitID: "unit1", Version: name}}}
		id, err := store.Create(ctx, c)
		if err != nil {
			t.Fatalf("create %s: %v", name, err)
		}
		c.ID = id
		time.Sleep(5 * time.Millisecond) // distinct created_at
		return c
	}

	if _, err := store.LatestBase(ctx); err != mhscollections.ErrNotFound {
		t.Fatalf("empty store: got %v, want ErrNotFound", err)
	}
	mk("team-1", false)
	mk("team-2", false)
	test := mk("our-test", true)

	latest, err := store.Latest(ctx)
	if err != nil || latest.Name != "our-test" {
		t.Fatalf("Latest = %q, %v; want our-test", latest.Name, err)
	}
	base, err := store.LatestBase(ctx)
	if err != nil || base.Name != "team-2" {
		t.Fatalf("LatestBase = %q, %v; want team-2 (the test collection is skipped)", base.Name, err)
	}

	// Clearing the flag (the edit form) makes it a base again.
	test.TestOnly = false
	if err := store.Update(ctx, test.ID, test); err != nil {
		t.Fatalf("update: %v", err)
	}
	if base, _ := store.LatestBase(ctx); base.Name != "our-test" {
		t.Fatalf("after clearing the flag LatestBase = %q, want our-test", base.Name)
	}
	got, _ := store.GetByID(ctx, test.ID)
	if got.TestOnly {
		t.Fatal("TestOnly still set after Update cleared it")
	}
}
