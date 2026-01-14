package activity_test

import (
	"testing"
	"time"

	"github.com/dalemusser/stratahub/internal/app/store/activity"
	"github.com/dalemusser/stratahub/internal/testutil"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

func TestStore_Create(t *testing.T) {
	db := testutil.SetupTestDB(t)
	store := activity.New(db)
	ctx, cancel := testutil.TestContext()
	defer cancel()

	userID := primitive.NewObjectID()
	sessionID := primitive.NewObjectID()

	event := activity.Event{
		UserID:    userID,
		SessionID: sessionID,
		EventType: activity.EventPageView,
		PagePath:  "/dashboard",
	}

	err := store.Create(ctx, event)
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}
}

func TestStore_Create_AutoGeneratesID(t *testing.T) {
	db := testutil.SetupTestDB(t)
	store := activity.New(db)
	ctx, cancel := testutil.TestContext()
	defer cancel()

	userID := primitive.NewObjectID()
	sessionID := primitive.NewObjectID()

	event := activity.Event{
		UserID:    userID,
		SessionID: sessionID,
		EventType: activity.EventPageView,
	}

	err := store.Create(ctx, event)
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	// Verify an event was created by getting it back
	events, err := store.GetByUser(ctx, userID, 10)
	if err != nil {
		t.Fatalf("GetByUser failed: %v", err)
	}

	if len(events) != 1 {
		t.Errorf("expected 1 event, got %d", len(events))
	}
	if events[0].ID.IsZero() {
		t.Error("expected ID to be auto-generated")
	}
}

func TestStore_RecordResourceLaunch(t *testing.T) {
	db := testutil.SetupTestDB(t)
	store := activity.New(db)
	ctx, cancel := testutil.TestContext()
	defer cancel()

	userID := primitive.NewObjectID()
	sessionID := primitive.NewObjectID()
	orgID := primitive.NewObjectID()
	resourceID := primitive.NewObjectID()

	err := store.RecordResourceLaunch(ctx, userID, sessionID, &orgID, resourceID, "Math Lesson 1")
	if err != nil {
		t.Fatalf("RecordResourceLaunch failed: %v", err)
	}

	// Verify event was created
	events, err := store.GetBySession(ctx, sessionID)
	if err != nil {
		t.Fatalf("GetBySession failed: %v", err)
	}

	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}

	event := events[0]
	if event.EventType != activity.EventResourceLaunch {
		t.Errorf("EventType: got %q, want %q", event.EventType, activity.EventResourceLaunch)
	}
	if event.ResourceID == nil || *event.ResourceID != resourceID {
		t.Errorf("ResourceID: got %v, want %v", event.ResourceID, resourceID)
	}
	if event.ResourceName != "Math Lesson 1" {
		t.Errorf("ResourceName: got %q, want %q", event.ResourceName, "Math Lesson 1")
	}
}

func TestStore_RecordResourceView(t *testing.T) {
	db := testutil.SetupTestDB(t)
	store := activity.New(db)
	ctx, cancel := testutil.TestContext()
	defer cancel()

	userID := primitive.NewObjectID()
	sessionID := primitive.NewObjectID()
	resourceID := primitive.NewObjectID()

	err := store.RecordResourceView(ctx, userID, sessionID, nil, resourceID, "Science Lab")
	if err != nil {
		t.Fatalf("RecordResourceView failed: %v", err)
	}

	// Verify event was created
	events, err := store.GetBySession(ctx, sessionID)
	if err != nil {
		t.Fatalf("GetBySession failed: %v", err)
	}

	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}

	event := events[0]
	if event.EventType != activity.EventResourceView {
		t.Errorf("EventType: got %q, want %q", event.EventType, activity.EventResourceView)
	}
}

func TestStore_RecordPageView(t *testing.T) {
	db := testutil.SetupTestDB(t)
	store := activity.New(db)
	ctx, cancel := testutil.TestContext()
	defer cancel()

	userID := primitive.NewObjectID()
	sessionID := primitive.NewObjectID()
	orgID := primitive.NewObjectID()

	err := store.RecordPageView(ctx, userID, sessionID, &orgID, "/members")
	if err != nil {
		t.Fatalf("RecordPageView failed: %v", err)
	}

	// Verify event was created
	events, err := store.GetBySession(ctx, sessionID)
	if err != nil {
		t.Fatalf("GetBySession failed: %v", err)
	}

	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}

	event := events[0]
	if event.EventType != activity.EventPageView {
		t.Errorf("EventType: got %q, want %q", event.EventType, activity.EventPageView)
	}
	if event.PagePath != "/members" {
		t.Errorf("PagePath: got %q, want %q", event.PagePath, "/members")
	}
}

func TestStore_GetBySession(t *testing.T) {
	db := testutil.SetupTestDB(t)
	store := activity.New(db)
	ctx, cancel := testutil.TestContext()
	defer cancel()

	userID := primitive.NewObjectID()
	session1 := primitive.NewObjectID()
	session2 := primitive.NewObjectID()
	resourceID := primitive.NewObjectID()

	// Create events for session1
	_ = store.RecordPageView(ctx, userID, session1, nil, "/page1")
	_ = store.RecordResourceLaunch(ctx, userID, session1, nil, resourceID, "Resource")

	// Create event for session2
	_ = store.RecordPageView(ctx, userID, session2, nil, "/page2")

	// Get events for session1
	events, err := store.GetBySession(ctx, session1)
	if err != nil {
		t.Fatalf("GetBySession failed: %v", err)
	}

	if len(events) != 2 {
		t.Errorf("expected 2 events for session1, got %d", len(events))
	}

	// Verify all events belong to session1
	for _, e := range events {
		if e.SessionID != session1 {
			t.Errorf("expected SessionID %v, got %v", session1, e.SessionID)
		}
	}
}

func TestStore_GetBySession_SortedByTimestamp(t *testing.T) {
	db := testutil.SetupTestDB(t)
	store := activity.New(db)
	ctx, cancel := testutil.TestContext()
	defer cancel()

	userID := primitive.NewObjectID()
	sessionID := primitive.NewObjectID()
	resourceID := primitive.NewObjectID()

	// Create multiple events
	_ = store.RecordPageView(ctx, userID, sessionID, nil, "/page1")
	time.Sleep(10 * time.Millisecond)
	_ = store.RecordPageView(ctx, userID, sessionID, nil, "/page2")
	time.Sleep(10 * time.Millisecond)
	_ = store.RecordResourceLaunch(ctx, userID, sessionID, nil, resourceID, "Resource")

	events, err := store.GetBySession(ctx, sessionID)
	if err != nil {
		t.Fatalf("GetBySession failed: %v", err)
	}

	// Verify sorted by timestamp ascending (oldest first)
	for i := 1; i < len(events); i++ {
		if events[i].Timestamp.Before(events[i-1].Timestamp) {
			t.Error("expected events sorted by timestamp ascending")
		}
	}
}

func TestStore_GetByUser(t *testing.T) {
	db := testutil.SetupTestDB(t)
	store := activity.New(db)
	ctx, cancel := testutil.TestContext()
	defer cancel()

	user1 := primitive.NewObjectID()
	user2 := primitive.NewObjectID()
	sessionID := primitive.NewObjectID()

	// Create events for user1
	_ = store.RecordPageView(ctx, user1, sessionID, nil, "/page1")
	_ = store.RecordPageView(ctx, user1, sessionID, nil, "/page2")
	_ = store.RecordPageView(ctx, user1, sessionID, nil, "/page3")

	// Create event for user2
	_ = store.RecordPageView(ctx, user2, sessionID, nil, "/page4")

	// Get events for user1 with limit
	events, err := store.GetByUser(ctx, user1, 2)
	if err != nil {
		t.Fatalf("GetByUser failed: %v", err)
	}

	if len(events) != 2 {
		t.Errorf("expected 2 events (limit), got %d", len(events))
	}

	for _, e := range events {
		if e.UserID != user1 {
			t.Errorf("expected UserID %v, got %v", user1, e.UserID)
		}
	}
}

func TestStore_GetByUserInTimeRange(t *testing.T) {
	db := testutil.SetupTestDB(t)
	store := activity.New(db)
	ctx, cancel := testutil.TestContext()
	defer cancel()

	userID := primitive.NewObjectID()
	sessionID := primitive.NewObjectID()

	// Record time before creating events
	start := time.Now().UTC()

	// Create some events
	_ = store.RecordPageView(ctx, userID, sessionID, nil, "/page1")
	time.Sleep(10 * time.Millisecond)
	_ = store.RecordPageView(ctx, userID, sessionID, nil, "/page2")

	end := time.Now().UTC()

	// Query in time range
	events, err := store.GetByUserInTimeRange(ctx, userID, start, end)
	if err != nil {
		t.Fatalf("GetByUserInTimeRange failed: %v", err)
	}

	if len(events) != 2 {
		t.Errorf("expected 2 events in range, got %d", len(events))
	}
}

func TestStore_GetByUserInTimeRange_Empty(t *testing.T) {
	db := testutil.SetupTestDB(t)
	store := activity.New(db)
	ctx, cancel := testutil.TestContext()
	defer cancel()

	userID := primitive.NewObjectID()

	// Query time range in the past (no events)
	start := time.Now().UTC().Add(-2 * time.Hour)
	end := time.Now().UTC().Add(-1 * time.Hour)

	events, err := store.GetByUserInTimeRange(ctx, userID, start, end)
	if err != nil {
		t.Fatalf("GetByUserInTimeRange failed: %v", err)
	}

	if len(events) != 0 {
		t.Errorf("expected 0 events, got %d", len(events))
	}
}

func TestStore_GetByOrganization(t *testing.T) {
	db := testutil.SetupTestDB(t)
	store := activity.New(db)
	ctx, cancel := testutil.TestContext()
	defer cancel()

	org1 := primitive.NewObjectID()
	org2 := primitive.NewObjectID()
	user1 := primitive.NewObjectID()
	user2 := primitive.NewObjectID()
	sessionID := primitive.NewObjectID()

	// Create events for org1
	_ = store.RecordPageView(ctx, user1, sessionID, &org1, "/page1")
	_ = store.RecordPageView(ctx, user2, sessionID, &org1, "/page2")

	// Create event for org2
	_ = store.RecordPageView(ctx, user1, sessionID, &org2, "/page3")

	// Get events for org1
	events, err := store.GetByOrganization(ctx, org1, 10)
	if err != nil {
		t.Fatalf("GetByOrganization failed: %v", err)
	}

	if len(events) != 2 {
		t.Errorf("expected 2 events for org1, got %d", len(events))
	}

	for _, e := range events {
		if e.OrganizationID == nil || *e.OrganizationID != org1 {
			t.Errorf("expected OrganizationID %v, got %v", org1, e.OrganizationID)
		}
	}
}

func TestStore_GetLastResourceLaunch(t *testing.T) {
	db := testutil.SetupTestDB(t)
	store := activity.New(db)
	ctx, cancel := testutil.TestContext()
	defer cancel()

	userID := primitive.NewObjectID()
	sessionID := primitive.NewObjectID()
	resource1 := primitive.NewObjectID()
	resource2 := primitive.NewObjectID()

	// Create multiple resource launches
	_ = store.RecordResourceLaunch(ctx, userID, sessionID, nil, resource1, "First Resource")
	time.Sleep(10 * time.Millisecond)
	_ = store.RecordResourceLaunch(ctx, userID, sessionID, nil, resource2, "Second Resource")

	// Get last resource launch
	event, err := store.GetLastResourceLaunch(ctx, userID, sessionID)
	if err != nil {
		t.Fatalf("GetLastResourceLaunch failed: %v", err)
	}

	if event == nil {
		t.Fatal("expected event, got nil")
	}
	if event.ResourceName != "Second Resource" {
		t.Errorf("ResourceName: got %q, want %q", event.ResourceName, "Second Resource")
	}
	if event.ResourceID == nil || *event.ResourceID != resource2 {
		t.Errorf("ResourceID: got %v, want %v", event.ResourceID, resource2)
	}
}

func TestStore_GetLastResourceLaunch_NoLaunches(t *testing.T) {
	db := testutil.SetupTestDB(t)
	store := activity.New(db)
	ctx, cancel := testutil.TestContext()
	defer cancel()

	userID := primitive.NewObjectID()
	sessionID := primitive.NewObjectID()

	// Only create page views, no resource launches
	_ = store.RecordPageView(ctx, userID, sessionID, nil, "/page1")

	event, err := store.GetLastResourceLaunch(ctx, userID, sessionID)
	if err != nil {
		t.Fatalf("GetLastResourceLaunch failed: %v", err)
	}

	if event != nil {
		t.Error("expected nil for no resource launches")
	}
}

func TestStore_CountByUserInTimeRange(t *testing.T) {
	db := testutil.SetupTestDB(t)
	store := activity.New(db)
	ctx, cancel := testutil.TestContext()
	defer cancel()

	userID := primitive.NewObjectID()
	sessionID := primitive.NewObjectID()
	resourceID := primitive.NewObjectID()

	start := time.Now().UTC()

	// Create mixed events
	_ = store.RecordPageView(ctx, userID, sessionID, nil, "/page1")
	_ = store.RecordResourceLaunch(ctx, userID, sessionID, nil, resourceID, "Resource 1")
	_ = store.RecordResourceLaunch(ctx, userID, sessionID, nil, resourceID, "Resource 2")
	_ = store.RecordPageView(ctx, userID, sessionID, nil, "/page2")

	end := time.Now().UTC()

	// Count resource launches
	count, err := store.CountByUserInTimeRange(ctx, userID, activity.EventResourceLaunch, start, end)
	if err != nil {
		t.Fatalf("CountByUserInTimeRange failed: %v", err)
	}

	if count != 2 {
		t.Errorf("expected 2 resource launches, got %d", count)
	}

	// Count page views
	count, err = store.CountByUserInTimeRange(ctx, userID, activity.EventPageView, start, end)
	if err != nil {
		t.Fatalf("CountByUserInTimeRange failed: %v", err)
	}

	if count != 2 {
		t.Errorf("expected 2 page views, got %d", count)
	}
}

func TestStore_CountByUserInTimeRange_Zero(t *testing.T) {
	db := testutil.SetupTestDB(t)
	store := activity.New(db)
	ctx, cancel := testutil.TestContext()
	defer cancel()

	userID := primitive.NewObjectID()
	start := time.Now().UTC().Add(-2 * time.Hour)
	end := time.Now().UTC().Add(-1 * time.Hour)

	count, err := store.CountByUserInTimeRange(ctx, userID, activity.EventResourceLaunch, start, end)
	if err != nil {
		t.Fatalf("CountByUserInTimeRange failed: %v", err)
	}

	if count != 0 {
		t.Errorf("expected 0, got %d", count)
	}
}

func TestStore_EnsureIndexes(t *testing.T) {
	db := testutil.SetupTestDB(t)
	store := activity.New(db)
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

// Test event type constants

func TestEventTypeConstants(t *testing.T) {
	if activity.EventResourceLaunch != "resource_launch" {
		t.Errorf("EventResourceLaunch: got %q, want %q", activity.EventResourceLaunch, "resource_launch")
	}
	if activity.EventResourceView != "resource_view" {
		t.Errorf("EventResourceView: got %q, want %q", activity.EventResourceView, "resource_view")
	}
	if activity.EventPageView != "page_view" {
		t.Errorf("EventPageView: got %q, want %q", activity.EventPageView, "page_view")
	}
}
