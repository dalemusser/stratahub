package audit_test

import (
	"testing"
	"time"

	"github.com/dalemusser/stratahub/internal/app/store/audit"
	"github.com/dalemusser/stratahub/internal/testutil"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

func TestStore_Log(t *testing.T) {
	db := testutil.SetupTestDB(t)
	store := audit.New(db)
	ctx, cancel := testutil.TestContext()
	defer cancel()

	userID := primitive.NewObjectID()
	event := audit.Event{
		Category:  audit.CategoryAuth,
		EventType: audit.EventLoginSuccess,
		UserID:    &userID,
		IP:        "192.168.1.1",
		UserAgent: "TestBrowser/1.0",
		Success:   true,
	}

	err := store.Log(ctx, event)
	if err != nil {
		t.Fatalf("Log failed: %v", err)
	}

	// Verify event was logged
	events, err := store.GetByUser(ctx, userID, 10)
	if err != nil {
		t.Fatalf("GetByUser failed: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
}

func TestStore_Log_AutoGeneratesID(t *testing.T) {
	db := testutil.SetupTestDB(t)
	store := audit.New(db)
	ctx, cancel := testutil.TestContext()
	defer cancel()

	event := audit.Event{
		Category:  audit.CategoryAuth,
		EventType: audit.EventLoginSuccess,
		IP:        "192.168.1.1",
		Success:   true,
	}

	err := store.Log(ctx, event)
	if err != nil {
		t.Fatalf("Log failed: %v", err)
	}

	// Verify event was logged
	events, err := store.GetRecent(ctx, 10)
	if err != nil {
		t.Fatalf("GetRecent failed: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	if events[0].ID.IsZero() {
		t.Error("expected ID to be auto-generated")
	}
}

func TestStore_Log_AutoSetsTimestamp(t *testing.T) {
	db := testutil.SetupTestDB(t)
	store := audit.New(db)
	ctx, cancel := testutil.TestContext()
	defer cancel()

	before := time.Now().Add(-time.Second)
	event := audit.Event{
		Category:  audit.CategoryAuth,
		EventType: audit.EventLoginSuccess,
		IP:        "192.168.1.1",
		Success:   true,
	}

	err := store.Log(ctx, event)
	if err != nil {
		t.Fatalf("Log failed: %v", err)
	}
	after := time.Now().Add(time.Second)

	events, err := store.GetRecent(ctx, 10)
	if err != nil {
		t.Fatalf("GetRecent failed: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	if events[0].Timestamp.Before(before) || events[0].Timestamp.After(after) {
		t.Errorf("expected timestamp to be set to current time, got %v", events[0].Timestamp)
	}
}

func TestStore_Log_WithDetails(t *testing.T) {
	db := testutil.SetupTestDB(t)
	store := audit.New(db)
	ctx, cancel := testutil.TestContext()
	defer cancel()

	userID := primitive.NewObjectID()
	event := audit.Event{
		Category:  audit.CategoryAuth,
		EventType: audit.EventLoginSuccess,
		UserID:    &userID,
		IP:        "192.168.1.1",
		Success:   true,
		Details: map[string]string{
			"auth_method": "password",
			"login_id":    "testuser",
		},
	}

	err := store.Log(ctx, event)
	if err != nil {
		t.Fatalf("Log failed: %v", err)
	}

	events, err := store.GetByUser(ctx, userID, 10)
	if err != nil {
		t.Fatalf("GetByUser failed: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	if events[0].Details["auth_method"] != "password" {
		t.Errorf("expected auth_method=password, got %s", events[0].Details["auth_method"])
	}
}

func TestStore_GetByUser(t *testing.T) {
	db := testutil.SetupTestDB(t)
	store := audit.New(db)
	ctx, cancel := testutil.TestContext()
	defer cancel()

	user1 := primitive.NewObjectID()
	user2 := primitive.NewObjectID()

	// Log events for user1
	for i := 0; i < 3; i++ {
		err := store.Log(ctx, audit.Event{
			Category:  audit.CategoryAuth,
			EventType: audit.EventLoginSuccess,
			UserID:    &user1,
			IP:        "192.168.1.1",
			Success:   true,
		})
		if err != nil {
			t.Fatalf("Log failed: %v", err)
		}
	}

	// Log event for user2
	err := store.Log(ctx, audit.Event{
		Category:  audit.CategoryAuth,
		EventType: audit.EventLoginSuccess,
		UserID:    &user2,
		IP:        "192.168.1.2",
		Success:   true,
	})
	if err != nil {
		t.Fatalf("Log failed: %v", err)
	}

	// Get events for user1
	events, err := store.GetByUser(ctx, user1, 10)
	if err != nil {
		t.Fatalf("GetByUser failed: %v", err)
	}
	if len(events) != 3 {
		t.Errorf("expected 3 events for user1, got %d", len(events))
	}

	// Get events for user2
	events, err = store.GetByUser(ctx, user2, 10)
	if err != nil {
		t.Fatalf("GetByUser failed: %v", err)
	}
	if len(events) != 1 {
		t.Errorf("expected 1 event for user2, got %d", len(events))
	}
}

func TestStore_GetByUser_Limit(t *testing.T) {
	db := testutil.SetupTestDB(t)
	store := audit.New(db)
	ctx, cancel := testutil.TestContext()
	defer cancel()

	userID := primitive.NewObjectID()

	// Log 5 events
	for i := 0; i < 5; i++ {
		err := store.Log(ctx, audit.Event{
			Category:  audit.CategoryAuth,
			EventType: audit.EventLoginSuccess,
			UserID:    &userID,
			IP:        "192.168.1.1",
			Success:   true,
		})
		if err != nil {
			t.Fatalf("Log failed: %v", err)
		}
	}

	// Get with limit 3
	events, err := store.GetByUser(ctx, userID, 3)
	if err != nil {
		t.Fatalf("GetByUser failed: %v", err)
	}
	if len(events) != 3 {
		t.Errorf("expected 3 events, got %d", len(events))
	}
}

func TestStore_GetRecent(t *testing.T) {
	db := testutil.SetupTestDB(t)
	store := audit.New(db)
	ctx, cancel := testutil.TestContext()
	defer cancel()

	// Log 3 events
	for i := 0; i < 3; i++ {
		err := store.Log(ctx, audit.Event{
			Category:  audit.CategoryAuth,
			EventType: audit.EventLoginSuccess,
			IP:        "192.168.1.1",
			Success:   true,
		})
		if err != nil {
			t.Fatalf("Log failed: %v", err)
		}
	}

	events, err := store.GetRecent(ctx, 10)
	if err != nil {
		t.Fatalf("GetRecent failed: %v", err)
	}
	if len(events) != 3 {
		t.Errorf("expected 3 events, got %d", len(events))
	}
}

func TestStore_GetRecent_Empty(t *testing.T) {
	db := testutil.SetupTestDB(t)
	store := audit.New(db)
	ctx, cancel := testutil.TestContext()
	defer cancel()

	events, err := store.GetRecent(ctx, 10)
	if err != nil {
		t.Fatalf("GetRecent failed: %v", err)
	}
	if len(events) != 0 {
		t.Errorf("expected 0 events, got %d", len(events))
	}
}

func TestStore_Query_ByCategory(t *testing.T) {
	db := testutil.SetupTestDB(t)
	store := audit.New(db)
	ctx, cancel := testutil.TestContext()
	defer cancel()

	// Log auth event
	err := store.Log(ctx, audit.Event{
		Category:  audit.CategoryAuth,
		EventType: audit.EventLoginSuccess,
		IP:        "192.168.1.1",
		Success:   true,
	})
	if err != nil {
		t.Fatalf("Log failed: %v", err)
	}

	// Log admin event
	err = store.Log(ctx, audit.Event{
		Category:  audit.CategoryAdmin,
		EventType: audit.EventUserCreated,
		IP:        "192.168.1.1",
		Success:   true,
	})
	if err != nil {
		t.Fatalf("Log failed: %v", err)
	}

	// Query only auth events
	events, err := store.Query(ctx, audit.QueryFilter{
		Category: audit.CategoryAuth,
	})
	if err != nil {
		t.Fatalf("Query failed: %v", err)
	}
	if len(events) != 1 {
		t.Errorf("expected 1 auth event, got %d", len(events))
	}
	if events[0].Category != audit.CategoryAuth {
		t.Errorf("expected auth category, got %s", events[0].Category)
	}
}

func TestStore_Query_ByEventType(t *testing.T) {
	db := testutil.SetupTestDB(t)
	store := audit.New(db)
	ctx, cancel := testutil.TestContext()
	defer cancel()

	// Log login success
	err := store.Log(ctx, audit.Event{
		Category:  audit.CategoryAuth,
		EventType: audit.EventLoginSuccess,
		IP:        "192.168.1.1",
		Success:   true,
	})
	if err != nil {
		t.Fatalf("Log failed: %v", err)
	}

	// Log logout
	err = store.Log(ctx, audit.Event{
		Category:  audit.CategoryAuth,
		EventType: audit.EventLogout,
		IP:        "192.168.1.1",
		Success:   true,
	})
	if err != nil {
		t.Fatalf("Log failed: %v", err)
	}

	// Query only login_success events
	events, err := store.Query(ctx, audit.QueryFilter{
		EventType: audit.EventLoginSuccess,
	})
	if err != nil {
		t.Fatalf("Query failed: %v", err)
	}
	if len(events) != 1 {
		t.Errorf("expected 1 login_success event, got %d", len(events))
	}
}

func TestStore_Query_ByOrganization(t *testing.T) {
	db := testutil.SetupTestDB(t)
	store := audit.New(db)
	ctx, cancel := testutil.TestContext()
	defer cancel()

	org1 := primitive.NewObjectID()
	org2 := primitive.NewObjectID()

	// Log event for org1
	err := store.Log(ctx, audit.Event{
		Category:       audit.CategoryAuth,
		EventType:      audit.EventLoginSuccess,
		OrganizationID: &org1,
		IP:             "192.168.1.1",
		Success:        true,
	})
	if err != nil {
		t.Fatalf("Log failed: %v", err)
	}

	// Log event for org2
	err = store.Log(ctx, audit.Event{
		Category:       audit.CategoryAuth,
		EventType:      audit.EventLoginSuccess,
		OrganizationID: &org2,
		IP:             "192.168.1.2",
		Success:        true,
	})
	if err != nil {
		t.Fatalf("Log failed: %v", err)
	}

	// Query only org1 events
	events, err := store.Query(ctx, audit.QueryFilter{
		OrganizationID: &org1,
	})
	if err != nil {
		t.Fatalf("Query failed: %v", err)
	}
	if len(events) != 1 {
		t.Errorf("expected 1 event for org1, got %d", len(events))
	}
}

func TestStore_Query_ByMultipleOrganizations(t *testing.T) {
	db := testutil.SetupTestDB(t)
	store := audit.New(db)
	ctx, cancel := testutil.TestContext()
	defer cancel()

	org1 := primitive.NewObjectID()
	org2 := primitive.NewObjectID()
	org3 := primitive.NewObjectID()

	// Log event for each org
	for _, org := range []*primitive.ObjectID{&org1, &org2, &org3} {
		err := store.Log(ctx, audit.Event{
			Category:       audit.CategoryAuth,
			EventType:      audit.EventLoginSuccess,
			OrganizationID: org,
			IP:             "192.168.1.1",
			Success:        true,
		})
		if err != nil {
			t.Fatalf("Log failed: %v", err)
		}
	}

	// Query org1 and org2
	events, err := store.Query(ctx, audit.QueryFilter{
		OrganizationIDs: []primitive.ObjectID{org1, org2},
	})
	if err != nil {
		t.Fatalf("Query failed: %v", err)
	}
	if len(events) != 2 {
		t.Errorf("expected 2 events, got %d", len(events))
	}
}

func TestStore_Query_ByTimeRange(t *testing.T) {
	db := testutil.SetupTestDB(t)
	store := audit.New(db)
	ctx, cancel := testutil.TestContext()
	defer cancel()

	now := time.Now()
	oneHourAgo := now.Add(-time.Hour)
	twoHoursAgo := now.Add(-2 * time.Hour)

	// Log event with old timestamp
	oldEvent := audit.Event{
		Category:  audit.CategoryAuth,
		EventType: audit.EventLoginSuccess,
		Timestamp: twoHoursAgo,
		IP:        "192.168.1.1",
		Success:   true,
	}
	err := store.Log(ctx, oldEvent)
	if err != nil {
		t.Fatalf("Log failed: %v", err)
	}

	// Log event with recent timestamp
	recentEvent := audit.Event{
		Category:  audit.CategoryAuth,
		EventType: audit.EventLoginSuccess,
		Timestamp: now,
		IP:        "192.168.1.2",
		Success:   true,
	}
	err = store.Log(ctx, recentEvent)
	if err != nil {
		t.Fatalf("Log failed: %v", err)
	}

	// Query only events from last hour
	events, err := store.Query(ctx, audit.QueryFilter{
		StartTime: &oneHourAgo,
	})
	if err != nil {
		t.Fatalf("Query failed: %v", err)
	}
	if len(events) != 1 {
		t.Errorf("expected 1 recent event, got %d", len(events))
	}
}

func TestStore_Query_WithOffset(t *testing.T) {
	db := testutil.SetupTestDB(t)
	store := audit.New(db)
	ctx, cancel := testutil.TestContext()
	defer cancel()

	// Log 5 events
	for i := 0; i < 5; i++ {
		err := store.Log(ctx, audit.Event{
			Category:  audit.CategoryAuth,
			EventType: audit.EventLoginSuccess,
			IP:        "192.168.1.1",
			Success:   true,
		})
		if err != nil {
			t.Fatalf("Log failed: %v", err)
		}
	}

	// Query with offset 2, limit 2
	events, err := store.Query(ctx, audit.QueryFilter{
		Limit:  2,
		Offset: 2,
	})
	if err != nil {
		t.Fatalf("Query failed: %v", err)
	}
	if len(events) != 2 {
		t.Errorf("expected 2 events, got %d", len(events))
	}
}

func TestStore_CountByFilter(t *testing.T) {
	db := testutil.SetupTestDB(t)
	store := audit.New(db)
	ctx, cancel := testutil.TestContext()
	defer cancel()

	// Log 3 events
	for i := 0; i < 3; i++ {
		err := store.Log(ctx, audit.Event{
			Category:  audit.CategoryAuth,
			EventType: audit.EventLoginSuccess,
			IP:        "192.168.1.1",
			Success:   true,
		})
		if err != nil {
			t.Fatalf("Log failed: %v", err)
		}
	}

	count, err := store.CountByFilter(ctx, audit.QueryFilter{
		Category: audit.CategoryAuth,
	})
	if err != nil {
		t.Fatalf("CountByFilter failed: %v", err)
	}
	if count != 3 {
		t.Errorf("expected count 3, got %d", count)
	}
}

func TestStore_CountByFilter_Empty(t *testing.T) {
	db := testutil.SetupTestDB(t)
	store := audit.New(db)
	ctx, cancel := testutil.TestContext()
	defer cancel()

	count, err := store.CountByFilter(ctx, audit.QueryFilter{})
	if err != nil {
		t.Fatalf("CountByFilter failed: %v", err)
	}
	if count != 0 {
		t.Errorf("expected count 0, got %d", count)
	}
}

func TestStore_GetFailedLogins(t *testing.T) {
	db := testutil.SetupTestDB(t)
	store := audit.New(db)
	ctx, cancel := testutil.TestContext()
	defer cancel()

	since := time.Now().Add(-time.Hour)

	// Log failed login
	err := store.Log(ctx, audit.Event{
		Category:      audit.CategoryAuth,
		EventType:     audit.EventLoginFailedWrongPassword,
		IP:            "192.168.1.1",
		Success:       false,
		FailureReason: "wrong password",
	})
	if err != nil {
		t.Fatalf("Log failed: %v", err)
	}

	// Log successful login (should not appear)
	err = store.Log(ctx, audit.Event{
		Category:  audit.CategoryAuth,
		EventType: audit.EventLoginSuccess,
		IP:        "192.168.1.2",
		Success:   true,
	})
	if err != nil {
		t.Fatalf("Log failed: %v", err)
	}

	events, err := store.GetFailedLogins(ctx, since, 10)
	if err != nil {
		t.Fatalf("GetFailedLogins failed: %v", err)
	}
	if len(events) != 1 {
		t.Errorf("expected 1 failed login, got %d", len(events))
	}
	if events[0].Success {
		t.Error("expected success=false")
	}
}

func TestStore_GetFailedLogins_AllTypes(t *testing.T) {
	db := testutil.SetupTestDB(t)
	store := audit.New(db)
	ctx, cancel := testutil.TestContext()
	defer cancel()

	since := time.Now().Add(-time.Hour)

	failedTypes := []string{
		audit.EventLoginFailedUserNotFound,
		audit.EventLoginFailedWrongPassword,
		audit.EventLoginFailedUserDisabled,
		audit.EventLoginFailedAuthMethodDisabled,
		audit.EventLoginFailedRateLimit,
	}

	for _, eventType := range failedTypes {
		err := store.Log(ctx, audit.Event{
			Category:  audit.CategoryAuth,
			EventType: eventType,
			IP:        "192.168.1.1",
			Success:   false,
		})
		if err != nil {
			t.Fatalf("Log failed: %v", err)
		}
	}

	events, err := store.GetFailedLogins(ctx, since, 10)
	if err != nil {
		t.Fatalf("GetFailedLogins failed: %v", err)
	}
	if len(events) != 5 {
		t.Errorf("expected 5 failed logins, got %d", len(events))
	}
}

func TestStore_GetFailedLogins_Empty(t *testing.T) {
	db := testutil.SetupTestDB(t)
	store := audit.New(db)
	ctx, cancel := testutil.TestContext()
	defer cancel()

	since := time.Now().Add(-time.Hour)

	events, err := store.GetFailedLogins(ctx, since, 10)
	if err != nil {
		t.Fatalf("GetFailedLogins failed: %v", err)
	}
	if len(events) != 0 {
		t.Errorf("expected 0 failed logins, got %d", len(events))
	}
}

func TestStore_EnsureIndexes(t *testing.T) {
	db := testutil.SetupTestDB(t)
	store := audit.New(db)
	ctx, cancel := testutil.TestContext()
	defer cancel()

	// EnsureIndexes should not error
	err := store.EnsureIndexes(ctx)
	if err != nil {
		t.Fatalf("EnsureIndexes failed: %v", err)
	}

	// Calling again should be idempotent
	err = store.EnsureIndexes(ctx)
	if err != nil {
		t.Fatalf("Second EnsureIndexes failed: %v", err)
	}
}

func TestStore_Log_FailedEvent(t *testing.T) {
	db := testutil.SetupTestDB(t)
	store := audit.New(db)
	ctx, cancel := testutil.TestContext()
	defer cancel()

	event := audit.Event{
		Category:      audit.CategoryAuth,
		EventType:     audit.EventLoginFailedUserNotFound,
		IP:            "192.168.1.1",
		Success:       false,
		FailureReason: "user not found",
	}

	err := store.Log(ctx, event)
	if err != nil {
		t.Fatalf("Log failed: %v", err)
	}

	events, err := store.GetRecent(ctx, 10)
	if err != nil {
		t.Fatalf("GetRecent failed: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	if events[0].FailureReason != "user not found" {
		t.Errorf("expected failure_reason='user not found', got %q", events[0].FailureReason)
	}
}

func TestStore_Log_AdminEvent(t *testing.T) {
	db := testutil.SetupTestDB(t)
	store := audit.New(db)
	ctx, cancel := testutil.TestContext()
	defer cancel()

	actorID := primitive.NewObjectID()
	targetUserID := primitive.NewObjectID()
	orgID := primitive.NewObjectID()

	event := audit.Event{
		Category:       audit.CategoryAdmin,
		EventType:      audit.EventUserCreated,
		ActorID:        &actorID,
		UserID:         &targetUserID,
		OrganizationID: &orgID,
		IP:             "192.168.1.1",
		Success:        true,
		Details: map[string]string{
			"role":        "member",
			"auth_method": "password",
		},
	}

	err := store.Log(ctx, event)
	if err != nil {
		t.Fatalf("Log failed: %v", err)
	}

	events, err := store.GetByUser(ctx, targetUserID, 10)
	if err != nil {
		t.Fatalf("GetByUser failed: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	if events[0].ActorID == nil || *events[0].ActorID != actorID {
		t.Error("expected ActorID to be preserved")
	}
}
