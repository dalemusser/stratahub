package sessions_test

import (
	"testing"
	"time"

	"github.com/dalemusser/stratahub/internal/app/store/sessions"
	"github.com/dalemusser/stratahub/internal/testutil"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
)

func TestStore_Create(t *testing.T) {
	db := testutil.SetupTestDB(t)
	store := sessions.New(db)
	ctx, cancel := testutil.TestContext()
	defer cancel()

	userID := primitive.NewObjectID()
	orgID := primitive.NewObjectID()

	sess, err := store.Create(ctx, userID, &orgID, "192.168.1.1", "Mozilla/5.0", sessions.CreatedByLogin)
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	if sess.ID == primitive.NilObjectID {
		t.Error("expected ID to be assigned")
	}
	if sess.UserID != userID {
		t.Errorf("UserID: got %v, want %v", sess.UserID, userID)
	}
	if sess.OrganizationID == nil || *sess.OrganizationID != orgID {
		t.Errorf("OrganizationID: got %v, want %v", sess.OrganizationID, orgID)
	}
	if sess.IP != "192.168.1.1" {
		t.Errorf("IP: got %q, want %q", sess.IP, "192.168.1.1")
	}
	if sess.CreatedBy != sessions.CreatedByLogin {
		t.Errorf("CreatedBy: got %q, want %q", sess.CreatedBy, sessions.CreatedByLogin)
	}
	if sess.LoginAt.IsZero() {
		t.Error("expected LoginAt to be set")
	}
	if sess.LastActiveAt.IsZero() {
		t.Error("expected LastActiveAt to be set")
	}
	if sess.LogoutAt != nil {
		t.Error("expected LogoutAt to be nil for new session")
	}
}

func TestStore_Create_WithoutOrgID(t *testing.T) {
	db := testutil.SetupTestDB(t)
	store := sessions.New(db)
	ctx, cancel := testutil.TestContext()
	defer cancel()

	userID := primitive.NewObjectID()

	sess, err := store.Create(ctx, userID, nil, "192.168.1.1", "", sessions.CreatedByLogin)
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	if sess.OrganizationID != nil {
		t.Error("expected OrganizationID to be nil")
	}
}

func TestStore_Create_ClosesExistingSessions(t *testing.T) {
	db := testutil.SetupTestDB(t)
	store := sessions.New(db)
	ctx, cancel := testutil.TestContext()
	defer cancel()

	userID := primitive.NewObjectID()

	// Create first session
	sess1, err := store.Create(ctx, userID, nil, "192.168.1.1", "", sessions.CreatedByLogin)
	if err != nil {
		t.Fatalf("Create first session failed: %v", err)
	}

	// Create second session - should close the first
	_, err = store.Create(ctx, userID, nil, "192.168.1.2", "", sessions.CreatedByHeartbeat)
	if err != nil {
		t.Fatalf("Create second session failed: %v", err)
	}

	// Verify first session is now closed
	oldSess, err := store.GetByID(ctx, sess1.ID)
	if err != nil {
		t.Fatalf("GetByID failed: %v", err)
	}

	if oldSess.LogoutAt == nil {
		t.Error("expected first session to be closed")
	}
	if oldSess.EndReason != "inactive" {
		t.Errorf("EndReason: got %q, want %q", oldSess.EndReason, "inactive")
	}
}

func TestStore_GetByID(t *testing.T) {
	db := testutil.SetupTestDB(t)
	store := sessions.New(db)
	ctx, cancel := testutil.TestContext()
	defer cancel()

	userID := primitive.NewObjectID()

	created, err := store.Create(ctx, userID, nil, "192.168.1.1", "Test Agent", sessions.CreatedByLogin)
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	found, err := store.GetByID(ctx, created.ID)
	if err != nil {
		t.Fatalf("GetByID failed: %v", err)
	}

	if found.ID != created.ID {
		t.Errorf("ID: got %v, want %v", found.ID, created.ID)
	}
	if found.UserAgent != "Test Agent" {
		t.Errorf("UserAgent: got %q, want %q", found.UserAgent, "Test Agent")
	}
}

func TestStore_GetByID_NotFound(t *testing.T) {
	db := testutil.SetupTestDB(t)
	store := sessions.New(db)
	ctx, cancel := testutil.TestContext()
	defer cancel()

	fakeID := primitive.NewObjectID()
	_, err := store.GetByID(ctx, fakeID)
	if err != mongo.ErrNoDocuments {
		t.Errorf("expected mongo.ErrNoDocuments, got %v", err)
	}
}

func TestStore_Close(t *testing.T) {
	db := testutil.SetupTestDB(t)
	store := sessions.New(db)
	ctx, cancel := testutil.TestContext()
	defer cancel()

	userID := primitive.NewObjectID()

	sess, err := store.Create(ctx, userID, nil, "192.168.1.1", "", sessions.CreatedByLogin)
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	// Wait briefly so duration > 0
	time.Sleep(10 * time.Millisecond)

	err = store.Close(ctx, sess.ID, "logout")
	if err != nil {
		t.Fatalf("Close failed: %v", err)
	}

	// Verify session is closed
	closed, err := store.GetByID(ctx, sess.ID)
	if err != nil {
		t.Fatalf("GetByID failed: %v", err)
	}

	if closed.LogoutAt == nil {
		t.Error("expected LogoutAt to be set")
	}
	if closed.EndReason != "logout" {
		t.Errorf("EndReason: got %q, want %q", closed.EndReason, "logout")
	}
	if closed.DurationSecs < 0 {
		t.Errorf("DurationSecs: got %d, expected >= 0", closed.DurationSecs)
	}
}

func TestStore_Close_NotFound(t *testing.T) {
	db := testutil.SetupTestDB(t)
	store := sessions.New(db)
	ctx, cancel := testutil.TestContext()
	defer cancel()

	fakeID := primitive.NewObjectID()
	err := store.Close(ctx, fakeID, "logout")
	if err != mongo.ErrNoDocuments {
		t.Errorf("expected mongo.ErrNoDocuments, got %v", err)
	}
}

func TestStore_UpdateLastActive(t *testing.T) {
	db := testutil.SetupTestDB(t)
	store := sessions.New(db)
	ctx, cancel := testutil.TestContext()
	defer cancel()

	userID := primitive.NewObjectID()

	sess, err := store.Create(ctx, userID, nil, "192.168.1.1", "", sessions.CreatedByLogin)
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	result, err := store.UpdateLastActive(ctx, sess.ID, "/dashboard")
	if err != nil {
		t.Fatalf("UpdateLastActive failed: %v", err)
	}

	if !result.Updated {
		t.Error("expected Updated to be true")
	}
	// First update, previous page should be empty
	if result.PreviousPage != "" {
		t.Errorf("PreviousPage: got %q, want empty", result.PreviousPage)
	}

	// Second update should return previous page
	result2, err := store.UpdateLastActive(ctx, sess.ID, "/settings")
	if err != nil {
		t.Fatalf("UpdateLastActive (2) failed: %v", err)
	}

	if result2.PreviousPage != "/dashboard" {
		t.Errorf("PreviousPage: got %q, want %q", result2.PreviousPage, "/dashboard")
	}
}

func TestStore_UpdateLastActive_ClosedSession(t *testing.T) {
	db := testutil.SetupTestDB(t)
	store := sessions.New(db)
	ctx, cancel := testutil.TestContext()
	defer cancel()

	userID := primitive.NewObjectID()

	sess, err := store.Create(ctx, userID, nil, "192.168.1.1", "", sessions.CreatedByLogin)
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	// Close the session
	err = store.Close(ctx, sess.ID, "logout")
	if err != nil {
		t.Fatalf("Close failed: %v", err)
	}

	// Try to update closed session
	result, err := store.UpdateLastActive(ctx, sess.ID, "/dashboard")
	if err != nil {
		t.Fatalf("UpdateLastActive failed: %v", err)
	}

	if result.Updated {
		t.Error("expected Updated to be false for closed session")
	}
}

func TestStore_GetActiveByUser(t *testing.T) {
	db := testutil.SetupTestDB(t)
	store := sessions.New(db)
	ctx, cancel := testutil.TestContext()
	defer cancel()

	user1 := primitive.NewObjectID()
	user2 := primitive.NewObjectID()

	// Create session for user1
	_, err := store.Create(ctx, user1, nil, "192.168.1.1", "", sessions.CreatedByLogin)
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	// Create session for user2
	_, err = store.Create(ctx, user2, nil, "192.168.1.2", "", sessions.CreatedByLogin)
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	// Get active sessions for user1
	sessions1, err := store.GetActiveByUser(ctx, user1)
	if err != nil {
		t.Fatalf("GetActiveByUser failed: %v", err)
	}

	if len(sessions1) != 1 {
		t.Errorf("expected 1 active session for user1, got %d", len(sessions1))
	}
}

func TestStore_GetActiveByUser_NoActiveSessions(t *testing.T) {
	db := testutil.SetupTestDB(t)
	store := sessions.New(db)
	ctx, cancel := testutil.TestContext()
	defer cancel()

	userID := primitive.NewObjectID()

	// Create and close a session
	sess, err := store.Create(ctx, userID, nil, "192.168.1.1", "", sessions.CreatedByLogin)
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	err = store.Close(ctx, sess.ID, "logout")
	if err != nil {
		t.Fatalf("Close failed: %v", err)
	}

	// Should return empty list, not error
	activeSessions, err := store.GetActiveByUser(ctx, userID)
	if err != nil {
		t.Fatalf("GetActiveByUser failed: %v", err)
	}

	if len(activeSessions) != 0 {
		t.Errorf("expected 0 active sessions, got %d", len(activeSessions))
	}
}

func TestStore_GetByUser(t *testing.T) {
	db := testutil.SetupTestDB(t)
	store := sessions.New(db)
	ctx, cancel := testutil.TestContext()
	defer cancel()

	userID := primitive.NewObjectID()

	// Create multiple sessions (Create closes previous, but we're testing GetByUser returns history)
	for i := 0; i < 5; i++ {
		_, err := store.Create(ctx, userID, nil, "192.168.1.1", "", sessions.CreatedByLogin)
		if err != nil {
			t.Fatalf("Create failed: %v", err)
		}
	}

	// Get user's session history with limit
	history, err := store.GetByUser(ctx, userID, 3)
	if err != nil {
		t.Fatalf("GetByUser failed: %v", err)
	}

	if len(history) != 3 {
		t.Errorf("expected 3 sessions (limit), got %d", len(history))
	}

	// Verify sorted by login_at descending (most recent first)
	for i := 1; i < len(history); i++ {
		if history[i].LoginAt.After(history[i-1].LoginAt) {
			t.Error("expected sessions sorted by login_at descending")
		}
	}
}

func TestStore_GetByOrganization(t *testing.T) {
	db := testutil.SetupTestDB(t)
	store := sessions.New(db)
	ctx, cancel := testutil.TestContext()
	defer cancel()

	org1 := primitive.NewObjectID()
	org2 := primitive.NewObjectID()
	user1 := primitive.NewObjectID()
	user2 := primitive.NewObjectID()

	// Create sessions for org1
	_, _ = store.Create(ctx, user1, &org1, "192.168.1.1", "", sessions.CreatedByLogin)
	_, _ = store.Create(ctx, user2, &org1, "192.168.1.2", "", sessions.CreatedByLogin)

	// Create session for org2
	_, _ = store.Create(ctx, user1, &org2, "192.168.1.3", "", sessions.CreatedByLogin)

	// Get sessions for org1
	orgSessions, err := store.GetByOrganization(ctx, org1, 10)
	if err != nil {
		t.Fatalf("GetByOrganization failed: %v", err)
	}

	if len(orgSessions) != 2 {
		t.Errorf("expected 2 sessions for org1, got %d", len(orgSessions))
	}

	for _, sess := range orgSessions {
		if sess.OrganizationID == nil || *sess.OrganizationID != org1 {
			t.Errorf("expected OrganizationID %v, got %v", org1, sess.OrganizationID)
		}
	}
}

func TestStore_CountActiveInOrg(t *testing.T) {
	db := testutil.SetupTestDB(t)
	store := sessions.New(db)
	ctx, cancel := testutil.TestContext()
	defer cancel()

	orgID := primitive.NewObjectID()
	user1 := primitive.NewObjectID()
	user2 := primitive.NewObjectID()

	// Create active sessions for org
	_, _ = store.Create(ctx, user1, &orgID, "192.168.1.1", "", sessions.CreatedByLogin)
	_, _ = store.Create(ctx, user2, &orgID, "192.168.1.2", "", sessions.CreatedByLogin)

	// Count with a generous threshold
	count, err := store.CountActiveInOrg(ctx, orgID, 5*time.Minute)
	if err != nil {
		t.Fatalf("CountActiveInOrg failed: %v", err)
	}

	if count != 2 {
		t.Errorf("expected count 2, got %d", count)
	}
}

func TestStore_CountActiveInOrg_Empty(t *testing.T) {
	db := testutil.SetupTestDB(t)
	store := sessions.New(db)
	ctx, cancel := testutil.TestContext()
	defer cancel()

	orgID := primitive.NewObjectID()

	count, err := store.CountActiveInOrg(ctx, orgID, 5*time.Minute)
	if err != nil {
		t.Fatalf("CountActiveInOrg failed: %v", err)
	}

	if count != 0 {
		t.Errorf("expected count 0 for empty org, got %d", count)
	}
}

func TestStore_EnsureIndexes(t *testing.T) {
	db := testutil.SetupTestDB(t)
	store := sessions.New(db)
	ctx, cancel := testutil.TestContext()
	defer cancel()

	// Should not error
	err := store.EnsureIndexes(ctx)
	if err != nil {
		t.Fatalf("EnsureIndexes failed: %v", err)
	}

	// Can call multiple times without error
	err = store.EnsureIndexes(ctx)
	if err != nil {
		t.Fatalf("EnsureIndexes (second call) failed: %v", err)
	}
}

// Test constant values

func TestCreationSourceConstants(t *testing.T) {
	if sessions.CreatedByLogin != "login" {
		t.Errorf("CreatedByLogin: got %q, want %q", sessions.CreatedByLogin, "login")
	}
	if sessions.CreatedByHeartbeat != "heartbeat" {
		t.Errorf("CreatedByHeartbeat: got %q, want %q", sessions.CreatedByHeartbeat, "heartbeat")
	}
}
