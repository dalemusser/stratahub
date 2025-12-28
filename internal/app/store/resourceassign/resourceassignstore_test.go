package resourceassignstore_test

import (
	"testing"
	"time"

	resourceassignstore "github.com/dalemusser/stratahub/internal/app/store/resourceassign"
	"github.com/dalemusser/stratahub/internal/domain/models"
	"github.com/dalemusser/stratahub/internal/testutil"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
)

func TestStore_Create(t *testing.T) {
	db := testutil.SetupTestDB(t)
	store := resourceassignstore.New(db)
	fixtures := testutil.NewFixtures(t, db)
	ctx, cancel := testutil.TestContext()
	defer cancel()

	org := fixtures.CreateOrganization(ctx, "Test Org")
	group := fixtures.CreateGroup(ctx, "Test Group", org.ID)
	resource := fixtures.CreateResource(ctx, "Test Resource", "https://example.com/resource")

	assignment := models.GroupResourceAssignment{
		GroupID:        group.ID,
		OrganizationID: org.ID,
		ResourceID:     resource.ID,
		Instructions:   "Complete all exercises",
	}

	created, err := store.Create(ctx, assignment)
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	// Verify ID was assigned
	if created.ID == primitive.NilObjectID {
		t.Error("expected ID to be assigned")
	}

	// Verify timestamps
	if created.CreatedAt.IsZero() {
		t.Error("expected CreatedAt to be set")
	}

	// Verify fields
	if created.GroupID != group.ID {
		t.Errorf("GroupID: got %v, want %v", created.GroupID, group.ID)
	}
	if created.OrganizationID != org.ID {
		t.Errorf("OrganizationID: got %v, want %v", created.OrganizationID, org.ID)
	}
	if created.ResourceID != resource.ID {
		t.Errorf("ResourceID: got %v, want %v", created.ResourceID, resource.ID)
	}
	if created.Instructions != "Complete all exercises" {
		t.Errorf("Instructions: got %q, want %q", created.Instructions, "Complete all exercises")
	}
}

func TestStore_Create_WithScheduling(t *testing.T) {
	db := testutil.SetupTestDB(t)
	store := resourceassignstore.New(db)
	fixtures := testutil.NewFixtures(t, db)
	ctx, cancel := testutil.TestContext()
	defer cancel()

	org := fixtures.CreateOrganization(ctx, "Test Org")
	group := fixtures.CreateGroup(ctx, "Test Group", org.ID)
	resource := fixtures.CreateResource(ctx, "Test Resource", "https://example.com/resource")

	visibleFrom := time.Date(2024, 6, 1, 0, 0, 0, 0, time.UTC)
	visibleUntil := time.Date(2024, 6, 30, 23, 59, 59, 0, time.UTC)

	assignment := models.GroupResourceAssignment{
		GroupID:        group.ID,
		OrganizationID: org.ID,
		ResourceID:     resource.ID,
		VisibleFrom:    &visibleFrom,
		VisibleUntil:   &visibleUntil,
	}

	created, err := store.Create(ctx, assignment)
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	if created.VisibleFrom == nil || !created.VisibleFrom.Equal(visibleFrom) {
		t.Errorf("VisibleFrom: got %v, want %v", created.VisibleFrom, visibleFrom)
	}
	if created.VisibleUntil == nil || !created.VisibleUntil.Equal(visibleUntil) {
		t.Errorf("VisibleUntil: got %v, want %v", created.VisibleUntil, visibleUntil)
	}
}

func TestStore_GetByID(t *testing.T) {
	db := testutil.SetupTestDB(t)
	store := resourceassignstore.New(db)
	fixtures := testutil.NewFixtures(t, db)
	ctx, cancel := testutil.TestContext()
	defer cancel()

	org := fixtures.CreateOrganization(ctx, "Test Org")
	group := fixtures.CreateGroup(ctx, "Test Group", org.ID)
	resource := fixtures.CreateResource(ctx, "Test Resource", "https://example.com/resource")

	assignment := models.GroupResourceAssignment{
		GroupID:        group.ID,
		OrganizationID: org.ID,
		ResourceID:     resource.ID,
		Instructions:   "Test instructions",
	}

	created, err := store.Create(ctx, assignment)
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
	if found.GroupID != created.GroupID {
		t.Errorf("GroupID: got %v, want %v", found.GroupID, created.GroupID)
	}
	if found.Instructions != created.Instructions {
		t.Errorf("Instructions: got %q, want %q", found.Instructions, created.Instructions)
	}
}

func TestStore_GetByID_NotFound(t *testing.T) {
	db := testutil.SetupTestDB(t)
	store := resourceassignstore.New(db)
	ctx, cancel := testutil.TestContext()
	defer cancel()

	fakeID := primitive.NewObjectID()
	_, err := store.GetByID(ctx, fakeID)
	if err != mongo.ErrNoDocuments {
		t.Errorf("expected mongo.ErrNoDocuments, got %v", err)
	}
}

func TestStore_Update(t *testing.T) {
	db := testutil.SetupTestDB(t)
	store := resourceassignstore.New(db)
	fixtures := testutil.NewFixtures(t, db)
	ctx, cancel := testutil.TestContext()
	defer cancel()

	org := fixtures.CreateOrganization(ctx, "Test Org")
	group := fixtures.CreateGroup(ctx, "Test Group", org.ID)
	resource := fixtures.CreateResource(ctx, "Test Resource", "https://example.com/resource")

	assignment := models.GroupResourceAssignment{
		GroupID:        group.ID,
		OrganizationID: org.ID,
		ResourceID:     resource.ID,
		Instructions:   "Original instructions",
	}

	created, err := store.Create(ctx, assignment)
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	// Update the assignment
	created.Instructions = "Updated instructions"
	visibleFrom := time.Date(2024, 7, 1, 0, 0, 0, 0, time.UTC)
	created.VisibleFrom = &visibleFrom

	updated, err := store.Update(ctx, created)
	if err != nil {
		t.Fatalf("Update failed: %v", err)
	}

	// Verify UpdatedAt was set
	if updated.UpdatedAt == nil || updated.UpdatedAt.IsZero() {
		t.Error("expected UpdatedAt to be set")
	}

	// Verify the changes were persisted
	found, err := store.GetByID(ctx, created.ID)
	if err != nil {
		t.Fatalf("GetByID failed: %v", err)
	}

	if found.Instructions != "Updated instructions" {
		t.Errorf("Instructions: got %q, want %q", found.Instructions, "Updated instructions")
	}
	if found.VisibleFrom == nil || !found.VisibleFrom.Equal(visibleFrom) {
		t.Errorf("VisibleFrom: got %v, want %v", found.VisibleFrom, visibleFrom)
	}
}

func TestStore_Update_MissingID(t *testing.T) {
	db := testutil.SetupTestDB(t)
	store := resourceassignstore.New(db)
	ctx, cancel := testutil.TestContext()
	defer cancel()

	assignment := models.GroupResourceAssignment{
		// ID is zero
		Instructions: "Test instructions",
	}

	_, err := store.Update(ctx, assignment)
	if err != mongo.ErrNilDocument {
		t.Errorf("expected mongo.ErrNilDocument, got %v", err)
	}
}

func TestStore_Delete(t *testing.T) {
	db := testutil.SetupTestDB(t)
	store := resourceassignstore.New(db)
	fixtures := testutil.NewFixtures(t, db)
	ctx, cancel := testutil.TestContext()
	defer cancel()

	org := fixtures.CreateOrganization(ctx, "Test Org")
	group := fixtures.CreateGroup(ctx, "Test Group", org.ID)
	resource := fixtures.CreateResource(ctx, "Test Resource", "https://example.com/resource")

	assignment := models.GroupResourceAssignment{
		GroupID:        group.ID,
		OrganizationID: org.ID,
		ResourceID:     resource.ID,
	}

	created, err := store.Create(ctx, assignment)
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	// Verify it exists
	_, err = store.GetByID(ctx, created.ID)
	if err != nil {
		t.Fatalf("assignment should exist before delete: %v", err)
	}

	// Delete it
	err = store.Delete(ctx, created.ID)
	if err != nil {
		t.Fatalf("Delete failed: %v", err)
	}

	// Verify it's gone
	_, err = store.GetByID(ctx, created.ID)
	if err != mongo.ErrNoDocuments {
		t.Errorf("expected mongo.ErrNoDocuments after delete, got %v", err)
	}
}

func TestStore_Delete_NonExistent(t *testing.T) {
	db := testutil.SetupTestDB(t)
	store := resourceassignstore.New(db)
	ctx, cancel := testutil.TestContext()
	defer cancel()

	// Delete should not error even if assignment doesn't exist
	fakeID := primitive.NewObjectID()
	err := store.Delete(ctx, fakeID)
	if err != nil {
		t.Errorf("Delete should not error for non-existent assignment: %v", err)
	}
}

func TestStore_ListByGroup(t *testing.T) {
	db := testutil.SetupTestDB(t)
	store := resourceassignstore.New(db)
	fixtures := testutil.NewFixtures(t, db)
	ctx, cancel := testutil.TestContext()
	defer cancel()

	org := fixtures.CreateOrganization(ctx, "Test Org")
	group1 := fixtures.CreateGroup(ctx, "Group One", org.ID)
	group2 := fixtures.CreateGroup(ctx, "Group Two", org.ID)

	resource1 := fixtures.CreateResource(ctx, "Resource One", "https://example.com/r1")
	resource2 := fixtures.CreateResource(ctx, "Resource Two", "https://example.com/r2")
	resource3 := fixtures.CreateResource(ctx, "Resource Three", "https://example.com/r3")

	// Create 2 assignments for group1
	a1 := models.GroupResourceAssignment{
		GroupID:        group1.ID,
		OrganizationID: org.ID,
		ResourceID:     resource1.ID,
	}
	_, err := store.Create(ctx, a1)
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	a2 := models.GroupResourceAssignment{
		GroupID:        group1.ID,
		OrganizationID: org.ID,
		ResourceID:     resource2.ID,
	}
	_, err = store.Create(ctx, a2)
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	// Create 1 assignment for group2
	a3 := models.GroupResourceAssignment{
		GroupID:        group2.ID,
		OrganizationID: org.ID,
		ResourceID:     resource3.ID,
	}
	_, err = store.Create(ctx, a3)
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	// List assignments for group1
	list, err := store.ListByGroup(ctx, group1.ID)
	if err != nil {
		t.Fatalf("ListByGroup failed: %v", err)
	}

	if len(list) != 2 {
		t.Errorf("expected 2 assignments for group1, got %d", len(list))
	}

	// Verify all returned assignments belong to group1
	for _, a := range list {
		if a.GroupID != group1.ID {
			t.Errorf("expected GroupID %v, got %v", group1.ID, a.GroupID)
		}
	}

	// List assignments for group2
	list, err = store.ListByGroup(ctx, group2.ID)
	if err != nil {
		t.Fatalf("ListByGroup failed: %v", err)
	}

	if len(list) != 1 {
		t.Errorf("expected 1 assignment for group2, got %d", len(list))
	}
}

func TestStore_ListByGroup_Empty(t *testing.T) {
	db := testutil.SetupTestDB(t)
	store := resourceassignstore.New(db)
	fixtures := testutil.NewFixtures(t, db)
	ctx, cancel := testutil.TestContext()
	defer cancel()

	org := fixtures.CreateOrganization(ctx, "Test Org")
	group := fixtures.CreateGroup(ctx, "Empty Group", org.ID)

	list, err := store.ListByGroup(ctx, group.ID)
	if err != nil {
		t.Fatalf("ListByGroup failed: %v", err)
	}

	if len(list) != 0 {
		t.Errorf("expected 0 assignments, got %d", len(list))
	}
}
