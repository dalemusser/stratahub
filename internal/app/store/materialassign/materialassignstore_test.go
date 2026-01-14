package materialassignstore_test

import (
	"testing"
	"time"

	materialassignstore "github.com/dalemusser/stratahub/internal/app/store/materialassign"
	"github.com/dalemusser/stratahub/internal/domain/models"
	"github.com/dalemusser/stratahub/internal/testutil"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
)

func TestStore_Create(t *testing.T) {
	db := testutil.SetupTestDB(t)
	store := materialassignstore.New(db)
	fixtures := testutil.NewFixtures(t, db)
	ctx, cancel := testutil.TestContext()
	defer cancel()

	org := fixtures.CreateOrganization(ctx, "Test Org")
	material := fixtures.CreateMaterial(ctx, "Test Material", "document")

	assignment := models.MaterialAssignment{
		MaterialID:     material.ID,
		OrganizationID: &org.ID,
		Directions:     "Complete this material",
	}

	created, err := store.Create(ctx, assignment)
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	// Verify ID was assigned
	if created.ID == primitive.NilObjectID {
		t.Error("expected ID to be assigned")
	}

	// Verify CreatedAt was set
	if created.CreatedAt.IsZero() {
		t.Error("expected CreatedAt to be set")
	}

	// Verify fields
	if created.MaterialID != material.ID {
		t.Errorf("MaterialID: got %v, want %v", created.MaterialID, material.ID)
	}
	if created.OrganizationID == nil || *created.OrganizationID != org.ID {
		t.Errorf("OrganizationID: got %v, want %v", created.OrganizationID, org.ID)
	}
	if created.Directions != "Complete this material" {
		t.Errorf("Directions: got %q, want %q", created.Directions, "Complete this material")
	}
}

func TestStore_Create_WithLeader(t *testing.T) {
	db := testutil.SetupTestDB(t)
	store := materialassignstore.New(db)
	fixtures := testutil.NewFixtures(t, db)
	ctx, cancel := testutil.TestContext()
	defer cancel()

	org := fixtures.CreateOrganization(ctx, "Test Org")
	leader := fixtures.CreateLeader(ctx, "Test Leader", "leader@example.com", org.ID)
	material := fixtures.CreateMaterial(ctx, "Test Material", "document")

	assignment := models.MaterialAssignment{
		MaterialID: material.ID,
		LeaderID:   &leader.ID,
		Directions: "Individual assignment",
	}

	created, err := store.Create(ctx, assignment)
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	if created.LeaderID == nil || *created.LeaderID != leader.ID {
		t.Errorf("LeaderID: got %v, want %v", created.LeaderID, leader.ID)
	}
}

func TestStore_Create_PreservesExistingCreatedAt(t *testing.T) {
	db := testutil.SetupTestDB(t)
	store := materialassignstore.New(db)
	fixtures := testutil.NewFixtures(t, db)
	ctx, cancel := testutil.TestContext()
	defer cancel()

	org := fixtures.CreateOrganization(ctx, "Test Org")
	material := fixtures.CreateMaterial(ctx, "Test Material", "document")

	existingTime := time.Date(2024, 1, 15, 10, 30, 0, 0, time.UTC)
	assignment := models.MaterialAssignment{
		MaterialID:     material.ID,
		OrganizationID: &org.ID,
		CreatedAt:      existingTime,
	}

	created, err := store.Create(ctx, assignment)
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	if !created.CreatedAt.Equal(existingTime) {
		t.Errorf("CreatedAt should be preserved: got %v, want %v", created.CreatedAt, existingTime)
	}
}

func TestStore_GetByID(t *testing.T) {
	db := testutil.SetupTestDB(t)
	store := materialassignstore.New(db)
	fixtures := testutil.NewFixtures(t, db)
	ctx, cancel := testutil.TestContext()
	defer cancel()

	org := fixtures.CreateOrganization(ctx, "Test Org")
	material := fixtures.CreateMaterial(ctx, "Test Material", "document")

	assignment := models.MaterialAssignment{
		MaterialID:     material.ID,
		OrganizationID: &org.ID,
		Directions:     "Test directions",
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
	if found.MaterialID != created.MaterialID {
		t.Errorf("MaterialID: got %v, want %v", found.MaterialID, created.MaterialID)
	}
	if found.Directions != created.Directions {
		t.Errorf("Directions: got %q, want %q", found.Directions, created.Directions)
	}
}

func TestStore_GetByID_NotFound(t *testing.T) {
	db := testutil.SetupTestDB(t)
	store := materialassignstore.New(db)
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
	store := materialassignstore.New(db)
	fixtures := testutil.NewFixtures(t, db)
	ctx, cancel := testutil.TestContext()
	defer cancel()

	org := fixtures.CreateOrganization(ctx, "Test Org")
	material := fixtures.CreateMaterial(ctx, "Test Material", "document")

	assignment := models.MaterialAssignment{
		MaterialID:     material.ID,
		OrganizationID: &org.ID,
		Directions:     "Original directions",
	}

	created, err := store.Create(ctx, assignment)
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	// Update the assignment
	created.Directions = "Updated directions"
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

	if found.Directions != "Updated directions" {
		t.Errorf("Directions: got %q, want %q", found.Directions, "Updated directions")
	}
	if found.VisibleFrom == nil || !found.VisibleFrom.Equal(visibleFrom) {
		t.Errorf("VisibleFrom: got %v, want %v", found.VisibleFrom, visibleFrom)
	}
}

func TestStore_Update_ZeroID(t *testing.T) {
	db := testutil.SetupTestDB(t)
	store := materialassignstore.New(db)
	ctx, cancel := testutil.TestContext()
	defer cancel()

	assignment := models.MaterialAssignment{
		// ID is zero
		Directions: "Test directions",
	}

	_, err := store.Update(ctx, assignment)
	if err != mongo.ErrNilDocument {
		t.Errorf("expected mongo.ErrNilDocument, got %v", err)
	}
}

func TestStore_Delete(t *testing.T) {
	db := testutil.SetupTestDB(t)
	store := materialassignstore.New(db)
	fixtures := testutil.NewFixtures(t, db)
	ctx, cancel := testutil.TestContext()
	defer cancel()

	org := fixtures.CreateOrganization(ctx, "Test Org")
	material := fixtures.CreateMaterial(ctx, "Test Material", "document")

	assignment := models.MaterialAssignment{
		MaterialID:     material.ID,
		OrganizationID: &org.ID,
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
	store := materialassignstore.New(db)
	ctx, cancel := testutil.TestContext()
	defer cancel()

	// Delete should not error even if assignment doesn't exist
	fakeID := primitive.NewObjectID()
	err := store.Delete(ctx, fakeID)
	if err != nil {
		t.Errorf("Delete should not error for non-existent assignment: %v", err)
	}
}

func TestStore_ListByMaterial(t *testing.T) {
	db := testutil.SetupTestDB(t)
	store := materialassignstore.New(db)
	fixtures := testutil.NewFixtures(t, db)
	ctx, cancel := testutil.TestContext()
	defer cancel()

	org1 := fixtures.CreateOrganization(ctx, "Org One")
	org2 := fixtures.CreateOrganization(ctx, "Org Two")
	material1 := fixtures.CreateMaterial(ctx, "Material One", "document")
	material2 := fixtures.CreateMaterial(ctx, "Material Two", "survey")

	// Create 2 assignments for material1
	a1 := models.MaterialAssignment{
		MaterialID:     material1.ID,
		OrganizationID: &org1.ID,
	}
	_, err := store.Create(ctx, a1)
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	a2 := models.MaterialAssignment{
		MaterialID:     material1.ID,
		OrganizationID: &org2.ID,
	}
	_, err = store.Create(ctx, a2)
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	// Create 1 assignment for material2
	a3 := models.MaterialAssignment{
		MaterialID:     material2.ID,
		OrganizationID: &org1.ID,
	}
	_, err = store.Create(ctx, a3)
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	// List assignments for material1
	list, err := store.ListByMaterial(ctx, material1.ID)
	if err != nil {
		t.Fatalf("ListByMaterial failed: %v", err)
	}

	if len(list) != 2 {
		t.Errorf("expected 2 assignments for material1, got %d", len(list))
	}

	// Verify all returned assignments belong to material1
	for _, a := range list {
		if a.MaterialID != material1.ID {
			t.Errorf("expected MaterialID %v, got %v", material1.ID, a.MaterialID)
		}
	}
}

func TestStore_ListByMaterial_Empty(t *testing.T) {
	db := testutil.SetupTestDB(t)
	store := materialassignstore.New(db)
	fixtures := testutil.NewFixtures(t, db)
	ctx, cancel := testutil.TestContext()
	defer cancel()

	material := fixtures.CreateMaterial(ctx, "Unassigned Material", "document")

	list, err := store.ListByMaterial(ctx, material.ID)
	if err != nil {
		t.Fatalf("ListByMaterial failed: %v", err)
	}

	if len(list) != 0 {
		t.Errorf("expected 0 assignments, got %d", len(list))
	}
}

func TestStore_ListByOrg(t *testing.T) {
	db := testutil.SetupTestDB(t)
	store := materialassignstore.New(db)
	fixtures := testutil.NewFixtures(t, db)
	ctx, cancel := testutil.TestContext()
	defer cancel()

	org1 := fixtures.CreateOrganization(ctx, "Org One")
	org2 := fixtures.CreateOrganization(ctx, "Org Two")
	material1 := fixtures.CreateMaterial(ctx, "Material One", "document")
	material2 := fixtures.CreateMaterial(ctx, "Material Two", "survey")

	// Create 2 assignments for org1
	a1 := models.MaterialAssignment{
		MaterialID:     material1.ID,
		OrganizationID: &org1.ID,
	}
	_, err := store.Create(ctx, a1)
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	a2 := models.MaterialAssignment{
		MaterialID:     material2.ID,
		OrganizationID: &org1.ID,
	}
	_, err = store.Create(ctx, a2)
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	// Create 1 assignment for org2
	a3 := models.MaterialAssignment{
		MaterialID:     material1.ID,
		OrganizationID: &org2.ID,
	}
	_, err = store.Create(ctx, a3)
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	// List assignments for org1
	list, err := store.ListByOrg(ctx, org1.ID)
	if err != nil {
		t.Fatalf("ListByOrg failed: %v", err)
	}

	if len(list) != 2 {
		t.Errorf("expected 2 assignments for org1, got %d", len(list))
	}

	// Verify all returned assignments belong to org1
	for _, a := range list {
		if a.OrganizationID == nil || *a.OrganizationID != org1.ID {
			t.Errorf("expected OrganizationID %v, got %v", org1.ID, a.OrganizationID)
		}
	}
}

func TestStore_ListByLeader(t *testing.T) {
	db := testutil.SetupTestDB(t)
	store := materialassignstore.New(db)
	fixtures := testutil.NewFixtures(t, db)
	ctx, cancel := testutil.TestContext()
	defer cancel()

	org := fixtures.CreateOrganization(ctx, "Test Org")
	leader1 := fixtures.CreateLeader(ctx, "Leader One", "leader1@example.com", org.ID)
	leader2 := fixtures.CreateLeader(ctx, "Leader Two", "leader2@example.com", org.ID)
	material1 := fixtures.CreateMaterial(ctx, "Material One", "document")
	material2 := fixtures.CreateMaterial(ctx, "Material Two", "survey")

	// Create 2 assignments for leader1
	a1 := models.MaterialAssignment{
		MaterialID: material1.ID,
		LeaderID:   &leader1.ID,
	}
	_, err := store.Create(ctx, a1)
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	a2 := models.MaterialAssignment{
		MaterialID: material2.ID,
		LeaderID:   &leader1.ID,
	}
	_, err = store.Create(ctx, a2)
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	// Create 1 assignment for leader2
	a3 := models.MaterialAssignment{
		MaterialID: material1.ID,
		LeaderID:   &leader2.ID,
	}
	_, err = store.Create(ctx, a3)
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	// List assignments for leader1
	list, err := store.ListByLeader(ctx, leader1.ID)
	if err != nil {
		t.Fatalf("ListByLeader failed: %v", err)
	}

	if len(list) != 2 {
		t.Errorf("expected 2 assignments for leader1, got %d", len(list))
	}

	// Verify all returned assignments belong to leader1
	for _, a := range list {
		if a.LeaderID == nil || *a.LeaderID != leader1.ID {
			t.Errorf("expected LeaderID %v, got %v", leader1.ID, a.LeaderID)
		}
	}
}

func TestStore_DeleteByMaterial(t *testing.T) {
	db := testutil.SetupTestDB(t)
	store := materialassignstore.New(db)
	fixtures := testutil.NewFixtures(t, db)
	ctx, cancel := testutil.TestContext()
	defer cancel()

	org1 := fixtures.CreateOrganization(ctx, "Org One")
	org2 := fixtures.CreateOrganization(ctx, "Org Two")
	material1 := fixtures.CreateMaterial(ctx, "Material One", "document")
	material2 := fixtures.CreateMaterial(ctx, "Material Two", "survey")

	// Create 2 assignments for material1
	a1 := models.MaterialAssignment{MaterialID: material1.ID, OrganizationID: &org1.ID}
	_, _ = store.Create(ctx, a1)
	a2 := models.MaterialAssignment{MaterialID: material1.ID, OrganizationID: &org2.ID}
	_, _ = store.Create(ctx, a2)

	// Create 1 assignment for material2
	a3 := models.MaterialAssignment{MaterialID: material2.ID, OrganizationID: &org1.ID}
	_, _ = store.Create(ctx, a3)

	// Delete all assignments for material1
	count, err := store.DeleteByMaterial(ctx, material1.ID)
	if err != nil {
		t.Fatalf("DeleteByMaterial failed: %v", err)
	}

	if count != 2 {
		t.Errorf("expected 2 deleted, got %d", count)
	}

	// Verify material1 has no assignments
	list, _ := store.ListByMaterial(ctx, material1.ID)
	if len(list) != 0 {
		t.Errorf("expected 0 assignments for material1 after delete, got %d", len(list))
	}

	// Verify material2 still has its assignment
	list, _ = store.ListByMaterial(ctx, material2.ID)
	if len(list) != 1 {
		t.Errorf("expected 1 assignment for material2, got %d", len(list))
	}
}

func TestStore_DeleteByMaterial_None(t *testing.T) {
	db := testutil.SetupTestDB(t)
	store := materialassignstore.New(db)
	fixtures := testutil.NewFixtures(t, db)
	ctx, cancel := testutil.TestContext()
	defer cancel()

	material := fixtures.CreateMaterial(ctx, "Unassigned Material", "document")

	count, err := store.DeleteByMaterial(ctx, material.ID)
	if err != nil {
		t.Fatalf("DeleteByMaterial failed: %v", err)
	}

	if count != 0 {
		t.Errorf("expected 0 deleted, got %d", count)
	}
}

func TestStore_DeleteByOrg(t *testing.T) {
	db := testutil.SetupTestDB(t)
	store := materialassignstore.New(db)
	fixtures := testutil.NewFixtures(t, db)
	ctx, cancel := testutil.TestContext()
	defer cancel()

	org1 := fixtures.CreateOrganization(ctx, "Org One")
	org2 := fixtures.CreateOrganization(ctx, "Org Two")
	material1 := fixtures.CreateMaterial(ctx, "Material One", "document")
	material2 := fixtures.CreateMaterial(ctx, "Material Two", "survey")

	// Create 2 assignments for org1
	a1 := models.MaterialAssignment{MaterialID: material1.ID, OrganizationID: &org1.ID}
	_, _ = store.Create(ctx, a1)
	a2 := models.MaterialAssignment{MaterialID: material2.ID, OrganizationID: &org1.ID}
	_, _ = store.Create(ctx, a2)

	// Create 1 assignment for org2
	a3 := models.MaterialAssignment{MaterialID: material1.ID, OrganizationID: &org2.ID}
	_, _ = store.Create(ctx, a3)

	// Delete all assignments for org1
	count, err := store.DeleteByOrg(ctx, org1.ID)
	if err != nil {
		t.Fatalf("DeleteByOrg failed: %v", err)
	}

	if count != 2 {
		t.Errorf("expected 2 deleted, got %d", count)
	}

	// Verify org1 has no assignments
	list, _ := store.ListByOrg(ctx, org1.ID)
	if len(list) != 0 {
		t.Errorf("expected 0 assignments for org1 after delete, got %d", len(list))
	}

	// Verify org2 still has its assignment
	list, _ = store.ListByOrg(ctx, org2.ID)
	if len(list) != 1 {
		t.Errorf("expected 1 assignment for org2, got %d", len(list))
	}
}

func TestStore_DeleteByLeader(t *testing.T) {
	db := testutil.SetupTestDB(t)
	store := materialassignstore.New(db)
	fixtures := testutil.NewFixtures(t, db)
	ctx, cancel := testutil.TestContext()
	defer cancel()

	org := fixtures.CreateOrganization(ctx, "Test Org")
	leader1 := fixtures.CreateLeader(ctx, "Leader One", "leader1@example.com", org.ID)
	leader2 := fixtures.CreateLeader(ctx, "Leader Two", "leader2@example.com", org.ID)
	material1 := fixtures.CreateMaterial(ctx, "Material One", "document")
	material2 := fixtures.CreateMaterial(ctx, "Material Two", "survey")

	// Create 2 assignments for leader1
	a1 := models.MaterialAssignment{MaterialID: material1.ID, LeaderID: &leader1.ID}
	_, _ = store.Create(ctx, a1)
	a2 := models.MaterialAssignment{MaterialID: material2.ID, LeaderID: &leader1.ID}
	_, _ = store.Create(ctx, a2)

	// Create 1 assignment for leader2
	a3 := models.MaterialAssignment{MaterialID: material1.ID, LeaderID: &leader2.ID}
	_, _ = store.Create(ctx, a3)

	// Delete all assignments for leader1
	count, err := store.DeleteByLeader(ctx, leader1.ID)
	if err != nil {
		t.Fatalf("DeleteByLeader failed: %v", err)
	}

	if count != 2 {
		t.Errorf("expected 2 deleted, got %d", count)
	}

	// Verify leader1 has no assignments
	list, _ := store.ListByLeader(ctx, leader1.ID)
	if len(list) != 0 {
		t.Errorf("expected 0 assignments for leader1 after delete, got %d", len(list))
	}

	// Verify leader2 still has its assignment
	list, _ = store.ListByLeader(ctx, leader2.ID)
	if len(list) != 1 {
		t.Errorf("expected 1 assignment for leader2, got %d", len(list))
	}
}

func TestStore_CountByMaterial(t *testing.T) {
	db := testutil.SetupTestDB(t)
	store := materialassignstore.New(db)
	fixtures := testutil.NewFixtures(t, db)
	ctx, cancel := testutil.TestContext()
	defer cancel()

	org1 := fixtures.CreateOrganization(ctx, "Org One")
	org2 := fixtures.CreateOrganization(ctx, "Org Two")
	material := fixtures.CreateMaterial(ctx, "Test Material", "document")

	// Create 3 assignments for material
	a1 := models.MaterialAssignment{MaterialID: material.ID, OrganizationID: &org1.ID}
	_, _ = store.Create(ctx, a1)
	a2 := models.MaterialAssignment{MaterialID: material.ID, OrganizationID: &org2.ID}
	_, _ = store.Create(ctx, a2)

	leader := fixtures.CreateLeader(ctx, "Test Leader", "leader@example.com", org1.ID)
	a3 := models.MaterialAssignment{MaterialID: material.ID, LeaderID: &leader.ID}
	_, _ = store.Create(ctx, a3)

	count, err := store.CountByMaterial(ctx, material.ID)
	if err != nil {
		t.Fatalf("CountByMaterial failed: %v", err)
	}

	if count != 3 {
		t.Errorf("expected count 3, got %d", count)
	}
}

func TestStore_CountByMaterial_Zero(t *testing.T) {
	db := testutil.SetupTestDB(t)
	store := materialassignstore.New(db)
	fixtures := testutil.NewFixtures(t, db)
	ctx, cancel := testutil.TestContext()
	defer cancel()

	material := fixtures.CreateMaterial(ctx, "Unassigned Material", "document")

	count, err := store.CountByMaterial(ctx, material.ID)
	if err != nil {
		t.Fatalf("CountByMaterial failed: %v", err)
	}

	if count != 0 {
		t.Errorf("expected count 0, got %d", count)
	}
}

func TestStore_ListAll(t *testing.T) {
	db := testutil.SetupTestDB(t)
	store := materialassignstore.New(db)
	fixtures := testutil.NewFixtures(t, db)
	ctx, cancel := testutil.TestContext()
	defer cancel()

	org := fixtures.CreateOrganization(ctx, "Test Org")
	leader := fixtures.CreateLeader(ctx, "Test Leader", "leader@example.com", org.ID)
	material1 := fixtures.CreateMaterial(ctx, "Material One", "document")
	material2 := fixtures.CreateMaterial(ctx, "Material Two", "survey")

	// Create various assignments
	a1 := models.MaterialAssignment{MaterialID: material1.ID, OrganizationID: &org.ID}
	_, _ = store.Create(ctx, a1)
	a2 := models.MaterialAssignment{MaterialID: material2.ID, OrganizationID: &org.ID}
	_, _ = store.Create(ctx, a2)
	a3 := models.MaterialAssignment{MaterialID: material1.ID, LeaderID: &leader.ID}
	_, _ = store.Create(ctx, a3)

	list, err := store.ListAll(ctx)
	if err != nil {
		t.Fatalf("ListAll failed: %v", err)
	}

	if len(list) != 3 {
		t.Errorf("expected 3 assignments, got %d", len(list))
	}
}

func TestStore_ListAll_Empty(t *testing.T) {
	db := testutil.SetupTestDB(t)
	store := materialassignstore.New(db)
	ctx, cancel := testutil.TestContext()
	defer cancel()

	list, err := store.ListAll(ctx)
	if err != nil {
		t.Fatalf("ListAll failed: %v", err)
	}

	if len(list) != 0 {
		t.Errorf("expected 0 assignments, got %d", len(list))
	}
}
