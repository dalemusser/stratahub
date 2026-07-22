package staffauth_test

import (
	"testing"
	"time"

	"github.com/dalemusser/stratahub/internal/app/system/staffauth"
	"github.com/dalemusser/stratahub/internal/testutil"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

func TestUnlockKey_ScopedToSession(t *testing.T) {
	wsID := primitive.NewObjectID()
	userID := primitive.NewObjectID()

	a := staffauth.UnlockKey(wsID, userID, "session-token-1")
	b := staffauth.UnlockKey(wsID, userID, "session-token-2")
	c := staffauth.UnlockKey(wsID, userID, "session-token-1")

	if a == b {
		t.Error("different session tokens should produce different keys")
	}
	if a != c {
		t.Error("same inputs should produce the same key")
	}
}

func TestUnlockStore_GrantGetRevoke(t *testing.T) {
	db := testutil.SetupTestDB(t)
	store := staffauth.NewUnlockStore(db)
	ctx, cancel := testutil.TestContext()
	defer cancel()

	wsID := primitive.NewObjectID()
	userID := primitive.NewObjectID()
	key := staffauth.UnlockKey(wsID, userID, "tok")

	// No unlock yet
	u, err := store.GetActive(ctx, key)
	if err != nil {
		t.Fatalf("GetActive failed: %v", err)
	}
	if u != nil {
		t.Fatal("expected no active unlock before Grant")
	}

	// Grant
	if err := store.Grant(ctx, key, wsID, userID, "D. Teacher", 10*time.Minute); err != nil {
		t.Fatalf("Grant failed: %v", err)
	}
	u, err = store.GetActive(ctx, key)
	if err != nil {
		t.Fatalf("GetActive failed: %v", err)
	}
	if u == nil {
		t.Fatal("expected active unlock after Grant")
	}
	if u.GrantedBy != "D. Teacher" {
		t.Errorf("GrantedBy = %q, want %q", u.GrantedBy, "D. Teacher")
	}
	if u.WorkspaceID != wsID || u.UserID != userID {
		t.Error("unlock scoped to wrong workspace/user")
	}

	// A different session's key sees nothing
	otherKey := staffauth.UnlockKey(wsID, userID, "other-session")
	if other, _ := store.GetActive(ctx, otherKey); other != nil {
		t.Error("unlock leaked to a different session key")
	}

	// Revoke
	if err := store.Revoke(ctx, key); err != nil {
		t.Fatalf("Revoke failed: %v", err)
	}
	u, err = store.GetActive(ctx, key)
	if err != nil {
		t.Fatalf("GetActive failed: %v", err)
	}
	if u != nil {
		t.Error("expected no active unlock after Revoke")
	}
}

func TestUnlockStore_ExpiryAndRefresh(t *testing.T) {
	db := testutil.SetupTestDB(t)
	store := staffauth.NewUnlockStore(db)
	ctx, cancel := testutil.TestContext()
	defer cancel()

	wsID := primitive.NewObjectID()
	userID := primitive.NewObjectID()
	key := staffauth.UnlockKey(wsID, userID, "tok")

	// Grant an already-expired unlock (negative duration)
	if err := store.Grant(ctx, key, wsID, userID, "keyword", -1*time.Minute); err != nil {
		t.Fatalf("Grant failed: %v", err)
	}
	if u, _ := store.GetActive(ctx, key); u != nil {
		t.Error("expired unlock should not be active")
	}

	// Refresh must NOT resurrect an expired unlock
	if err := store.Refresh(ctx, key, 10*time.Minute); err != nil {
		t.Fatalf("Refresh failed: %v", err)
	}
	if u, _ := store.GetActive(ctx, key); u != nil {
		t.Error("Refresh resurrected an expired unlock")
	}

	// Fresh grant, then refresh slides the expiry forward
	if err := store.Grant(ctx, key, wsID, userID, "keyword", 1*time.Minute); err != nil {
		t.Fatalf("Grant failed: %v", err)
	}
	before, _ := store.GetActive(ctx, key)
	if before == nil {
		t.Fatal("expected active unlock")
	}
	if err := store.Refresh(ctx, key, 30*time.Minute); err != nil {
		t.Fatalf("Refresh failed: %v", err)
	}
	after, _ := store.GetActive(ctx, key)
	if after == nil {
		t.Fatal("expected active unlock after refresh")
	}
	if !after.ExpiresAt.After(before.ExpiresAt) {
		t.Errorf("Refresh did not extend expiry: before=%v after=%v", before.ExpiresAt, after.ExpiresAt)
	}

	// Re-grant replaces the existing record (upsert, no duplicate-key error)
	if err := store.Grant(ctx, key, wsID, userID, "T. Other", 5*time.Minute); err != nil {
		t.Fatalf("re-Grant failed: %v", err)
	}
	regranted, _ := store.GetActive(ctx, key)
	if regranted == nil || regranted.GrantedBy != "T. Other" {
		t.Error("re-Grant should replace the unlock record")
	}
}
