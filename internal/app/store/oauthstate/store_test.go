package oauthstate_test

import (
	"testing"
	"time"

	"github.com/dalemusser/stratahub/internal/app/store/oauthstate"
	"github.com/dalemusser/stratahub/internal/testutil"
)

func TestStore_Save(t *testing.T) {
	db := testutil.SetupTestDB(t)
	store := oauthstate.New(db)
	ctx, cancel := testutil.TestContext()
	defer cancel()

	state := "test-state-123"
	returnURL := "/dashboard"
	workspace := "myworkspace"
	expiresAt := time.Now().Add(10 * time.Minute)

	err := store.Save(ctx, state, returnURL, workspace, expiresAt)
	if err != nil {
		t.Fatalf("Save failed: %v", err)
	}
}

func TestStore_Save_EmptyOptionalFields(t *testing.T) {
	db := testutil.SetupTestDB(t)
	store := oauthstate.New(db)
	ctx, cancel := testutil.TestContext()
	defer cancel()

	state := "test-state-456"
	expiresAt := time.Now().Add(10 * time.Minute)

	// Empty returnURL and workspace
	err := store.Save(ctx, state, "", "", expiresAt)
	if err != nil {
		t.Fatalf("Save with empty optional fields failed: %v", err)
	}

	// Validate should still work
	returnURL, workspace, valid, err := store.Validate(ctx, state)
	if err != nil {
		t.Fatalf("Validate failed: %v", err)
	}
	if !valid {
		t.Error("expected state to be valid")
	}
	if returnURL != "" {
		t.Errorf("expected empty returnURL, got %q", returnURL)
	}
	if workspace != "" {
		t.Errorf("expected empty workspace, got %q", workspace)
	}
}

func TestStore_Validate(t *testing.T) {
	db := testutil.SetupTestDB(t)
	store := oauthstate.New(db)
	ctx, cancel := testutil.TestContext()
	defer cancel()

	state := "test-state-789"
	returnURL := "/settings"
	workspace := "acme"
	expiresAt := time.Now().Add(10 * time.Minute)

	err := store.Save(ctx, state, returnURL, workspace, expiresAt)
	if err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	// Validate
	gotReturnURL, gotWorkspace, valid, err := store.Validate(ctx, state)
	if err != nil {
		t.Fatalf("Validate failed: %v", err)
	}

	if !valid {
		t.Error("expected state to be valid")
	}
	if gotReturnURL != returnURL {
		t.Errorf("expected returnURL %q, got %q", returnURL, gotReturnURL)
	}
	if gotWorkspace != workspace {
		t.Errorf("expected workspace %q, got %q", workspace, gotWorkspace)
	}
}

func TestStore_Validate_InvalidState(t *testing.T) {
	db := testutil.SetupTestDB(t)
	store := oauthstate.New(db)
	ctx, cancel := testutil.TestContext()
	defer cancel()

	// Validate non-existent state
	returnURL, workspace, valid, err := store.Validate(ctx, "non-existent-state")
	if err != nil {
		t.Fatalf("Validate error: %v", err)
	}

	if valid {
		t.Error("expected invalid state to return valid=false")
	}
	if returnURL != "" {
		t.Errorf("expected empty returnURL, got %q", returnURL)
	}
	if workspace != "" {
		t.Errorf("expected empty workspace, got %q", workspace)
	}
}

func TestStore_Validate_SingleUse(t *testing.T) {
	db := testutil.SetupTestDB(t)
	store := oauthstate.New(db)
	ctx, cancel := testutil.TestContext()
	defer cancel()

	state := "single-use-state"
	expiresAt := time.Now().Add(10 * time.Minute)

	err := store.Save(ctx, state, "", "", expiresAt)
	if err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	// First validation should succeed
	_, _, valid, err := store.Validate(ctx, state)
	if err != nil {
		t.Fatalf("First Validate failed: %v", err)
	}
	if !valid {
		t.Error("expected first validation to succeed")
	}

	// Second validation should fail (state deleted)
	_, _, valid, err = store.Validate(ctx, state)
	if err != nil {
		t.Fatalf("Second Validate error: %v", err)
	}
	if valid {
		t.Error("expected second validation to fail (single use)")
	}
}

func TestStore_Validate_Expired(t *testing.T) {
	db := testutil.SetupTestDB(t)
	store := oauthstate.New(db)
	ctx, cancel := testutil.TestContext()
	defer cancel()

	state := "expired-state"
	// Already expired
	expiresAt := time.Now().Add(-1 * time.Minute)

	err := store.Save(ctx, state, "", "", expiresAt)
	if err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	// Validate should fail (expired)
	_, _, valid, err := store.Validate(ctx, state)
	if err != nil {
		t.Fatalf("Validate error: %v", err)
	}
	if valid {
		t.Error("expected expired state to be invalid")
	}
}

func TestStore_CleanupExpired(t *testing.T) {
	db := testutil.SetupTestDB(t)
	store := oauthstate.New(db)
	ctx, cancel := testutil.TestContext()
	defer cancel()

	// Save some expired states
	for i := 0; i < 3; i++ {
		err := store.Save(ctx, "expired-"+string(rune('a'+i)), "", "", time.Now().Add(-1*time.Minute))
		if err != nil {
			t.Fatalf("Save expired state failed: %v", err)
		}
	}

	// Save some valid states
	for i := 0; i < 2; i++ {
		err := store.Save(ctx, "valid-"+string(rune('a'+i)), "", "", time.Now().Add(10*time.Minute))
		if err != nil {
			t.Fatalf("Save valid state failed: %v", err)
		}
	}

	// Cleanup expired
	deleted, err := store.CleanupExpired(ctx)
	if err != nil {
		t.Fatalf("CleanupExpired failed: %v", err)
	}
	if deleted != 3 {
		t.Errorf("expected 3 deleted, got %d", deleted)
	}

	// Valid states should still be valid
	_, _, valid, _ := store.Validate(ctx, "valid-a")
	if !valid {
		t.Error("expected valid-a to still be valid")
	}
}

func TestStore_CleanupExpired_NoExpired(t *testing.T) {
	db := testutil.SetupTestDB(t)
	store := oauthstate.New(db)
	ctx, cancel := testutil.TestContext()
	defer cancel()

	// No expired states
	deleted, err := store.CleanupExpired(ctx)
	if err != nil {
		t.Fatalf("CleanupExpired failed: %v", err)
	}
	if deleted != 0 {
		t.Errorf("expected 0 deleted, got %d", deleted)
	}
}

func TestStore_EnsureIndexes(t *testing.T) {
	db := testutil.SetupTestDB(t)
	ctx, cancel := testutil.TestContext()
	defer cancel()

	// Indexes are already created by SetupTestDB via indexes.EnsureAll
	// Verify the oauth_states collection has indexes by listing them
	cur, err := db.Collection("oauth_states").Indexes().List(ctx)
	if err != nil {
		t.Fatalf("List indexes failed: %v", err)
	}
	defer cur.Close(ctx)

	indexCount := 0
	for cur.Next(ctx) {
		indexCount++
	}

	// Should have at least 2 indexes (state unique, expires_at TTL) plus _id
	if indexCount < 3 {
		t.Errorf("expected at least 3 indexes, got %d", indexCount)
	}
}

func TestStore_Save_DuplicateState(t *testing.T) {
	db := testutil.SetupTestDB(t)
	store := oauthstate.New(db)
	ctx, cancel := testutil.TestContext()
	defer cancel()

	// Indexes are already created by SetupTestDB via indexes.EnsureAll

	state := "duplicate-state"
	expiresAt := time.Now().Add(10 * time.Minute)

	// First save
	err := store.Save(ctx, state, "", "", expiresAt)
	if err != nil {
		t.Fatalf("First Save failed: %v", err)
	}

	// Second save with same state should fail (unique constraint)
	err = store.Save(ctx, state, "", "", expiresAt)
	if err == nil {
		t.Error("expected duplicate state to fail")
	}
}

func TestStore_Validate_ReturnsCorrectData(t *testing.T) {
	db := testutil.SetupTestDB(t)
	store := oauthstate.New(db)
	ctx, cancel := testutil.TestContext()
	defer cancel()

	state := "state-with-data"
	returnURL := "/after-login"
	workspace := "tenant1"
	expiresAt := time.Now().Add(10 * time.Minute)

	err := store.Save(ctx, state, returnURL, workspace, expiresAt)
	if err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	gotReturnURL, gotWorkspace, valid, err := store.Validate(ctx, state)
	if err != nil {
		t.Fatalf("Validate failed: %v", err)
	}

	if !valid {
		t.Fatal("expected valid=true")
	}
	if gotReturnURL != returnURL {
		t.Errorf("returnURL: expected %q, got %q", returnURL, gotReturnURL)
	}
	if gotWorkspace != workspace {
		t.Errorf("workspace: expected %q, got %q", workspace, gotWorkspace)
	}
}
