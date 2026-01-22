package coordinatorassign_test

import (
	"testing"

	"github.com/dalemusser/stratahub/internal/app/store/coordinatorassign"
	"github.com/dalemusser/stratahub/internal/domain/models"
	"github.com/dalemusser/stratahub/internal/testutil"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
)

func TestStore_Create(t *testing.T) {
	db := testutil.SetupTestDB(t)
	store := coordinatorassign.New(db)
	fixtures := testutil.NewFixtures(t, db)
	ctx, cancel := testutil.TestContext()
	defer cancel()

	coordinator := fixtures.CreateUser(ctx, "Test Coordinator", "coordinator@example.com", "coordinator", nil)
	org := fixtures.CreateOrganization(ctx, "Test Org")

	a := models.CoordinatorAssignment{
		UserID:         coordinator.ID,
		OrganizationID: org.ID,
	}

	created, err := store.Create(ctx, a)
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	if created.ID == primitive.NilObjectID {
		t.Error("expected ID to be assigned")
	}
	if created.CreatedAt.IsZero() {
		t.Error("expected CreatedAt to be set")
	}
	if created.UserID != coordinator.ID {
		t.Errorf("UserID: got %v, want %v", created.UserID, coordinator.ID)
	}
	if created.OrganizationID != org.ID {
		t.Errorf("OrganizationID: got %v, want %v", created.OrganizationID, org.ID)
	}
}

func TestStore_GetByID(t *testing.T) {
	db := testutil.SetupTestDB(t)
	store := coordinatorassign.New(db)
	fixtures := testutil.NewFixtures(t, db)
	ctx, cancel := testutil.TestContext()
	defer cancel()

	coordinator := fixtures.CreateUser(ctx, "Test Coordinator", "coordinator@example.com", "coordinator", nil)
	org := fixtures.CreateOrganization(ctx, "Test Org")

	created, err := store.Create(ctx, models.CoordinatorAssignment{
		UserID:         coordinator.ID,
		OrganizationID: org.ID,
	})
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
	if found.UserID != coordinator.ID {
		t.Errorf("UserID: got %v, want %v", found.UserID, coordinator.ID)
	}
}

func TestStore_GetByID_NotFound(t *testing.T) {
	db := testutil.SetupTestDB(t)
	store := coordinatorassign.New(db)
	ctx, cancel := testutil.TestContext()
	defer cancel()

	fakeID := primitive.NewObjectID()
	_, err := store.GetByID(ctx, fakeID)
	if err != mongo.ErrNoDocuments {
		t.Errorf("expected mongo.ErrNoDocuments, got %v", err)
	}
}

func TestStore_Delete(t *testing.T) {
	db := testutil.SetupTestDB(t)
	store := coordinatorassign.New(db)
	fixtures := testutil.NewFixtures(t, db)
	ctx, cancel := testutil.TestContext()
	defer cancel()

	coordinator := fixtures.CreateUser(ctx, "Test Coordinator", "coordinator@example.com", "coordinator", nil)
	org := fixtures.CreateOrganization(ctx, "Test Org")

	created, err := store.Create(ctx, models.CoordinatorAssignment{
		UserID:         coordinator.ID,
		OrganizationID: org.ID,
	})
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}

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

func TestStore_ListByUser(t *testing.T) {
	db := testutil.SetupTestDB(t)
	store := coordinatorassign.New(db)
	fixtures := testutil.NewFixtures(t, db)
	ctx, cancel := testutil.TestContext()
	defer cancel()

	coordinator1 := fixtures.CreateUser(ctx, "Coordinator One", "coord1@example.com", "coordinator", nil)
	coordinator2 := fixtures.CreateUser(ctx, "Coordinator Two", "coord2@example.com", "coordinator", nil)
	org1 := fixtures.CreateOrganization(ctx, "Org One")
	org2 := fixtures.CreateOrganization(ctx, "Org Two")
	org3 := fixtures.CreateOrganization(ctx, "Org Three")

	// Assign coordinator1 to 2 orgs
	_, _ = store.Create(ctx, models.CoordinatorAssignment{UserID: coordinator1.ID, OrganizationID: org1.ID})
	_, _ = store.Create(ctx, models.CoordinatorAssignment{UserID: coordinator1.ID, OrganizationID: org2.ID})

	// Assign coordinator2 to 1 org
	_, _ = store.Create(ctx, models.CoordinatorAssignment{UserID: coordinator2.ID, OrganizationID: org3.ID})

	// List for coordinator1
	list, err := store.ListByUser(ctx, coordinator1.ID)
	if err != nil {
		t.Fatalf("ListByUser failed: %v", err)
	}

	if len(list) != 2 {
		t.Errorf("expected 2 assignments for coordinator1, got %d", len(list))
	}

	for _, a := range list {
		if a.UserID != coordinator1.ID {
			t.Errorf("expected UserID %v, got %v", coordinator1.ID, a.UserID)
		}
	}
}

func TestStore_ListByOrg(t *testing.T) {
	db := testutil.SetupTestDB(t)
	store := coordinatorassign.New(db)
	fixtures := testutil.NewFixtures(t, db)
	ctx, cancel := testutil.TestContext()
	defer cancel()

	coordinator1 := fixtures.CreateUser(ctx, "Coordinator One", "coord1@example.com", "coordinator", nil)
	coordinator2 := fixtures.CreateUser(ctx, "Coordinator Two", "coord2@example.com", "coordinator", nil)
	org1 := fixtures.CreateOrganization(ctx, "Org One")
	org2 := fixtures.CreateOrganization(ctx, "Org Two")

	// Assign both coordinators to org1
	_, _ = store.Create(ctx, models.CoordinatorAssignment{UserID: coordinator1.ID, OrganizationID: org1.ID})
	_, _ = store.Create(ctx, models.CoordinatorAssignment{UserID: coordinator2.ID, OrganizationID: org1.ID})

	// Assign only coordinator1 to org2
	_, _ = store.Create(ctx, models.CoordinatorAssignment{UserID: coordinator1.ID, OrganizationID: org2.ID})

	// List for org1
	list, err := store.ListByOrg(ctx, org1.ID)
	if err != nil {
		t.Fatalf("ListByOrg failed: %v", err)
	}

	if len(list) != 2 {
		t.Errorf("expected 2 coordinators for org1, got %d", len(list))
	}

	for _, a := range list {
		if a.OrganizationID != org1.ID {
			t.Errorf("expected OrganizationID %v, got %v", org1.ID, a.OrganizationID)
		}
	}
}

func TestStore_OrgIDsByUser(t *testing.T) {
	db := testutil.SetupTestDB(t)
	store := coordinatorassign.New(db)
	fixtures := testutil.NewFixtures(t, db)
	ctx, cancel := testutil.TestContext()
	defer cancel()

	coordinator := fixtures.CreateUser(ctx, "Test Coordinator", "coordinator@example.com", "coordinator", nil)
	org1 := fixtures.CreateOrganization(ctx, "Org One")
	org2 := fixtures.CreateOrganization(ctx, "Org Two")
	org3 := fixtures.CreateOrganization(ctx, "Org Three")
	_ = org3 // org3 exists but is not assigned to coordinator

	// Assign coordinator to 2 orgs
	_, _ = store.Create(ctx, models.CoordinatorAssignment{UserID: coordinator.ID, OrganizationID: org1.ID})
	_, _ = store.Create(ctx, models.CoordinatorAssignment{UserID: coordinator.ID, OrganizationID: org2.ID})

	// org3 is not assigned

	orgIDs, err := store.OrgIDsByUser(ctx, coordinator.ID)
	if err != nil {
		t.Fatalf("OrgIDsByUser failed: %v", err)
	}

	if len(orgIDs) != 2 {
		t.Errorf("expected 2 org IDs, got %d", len(orgIDs))
	}

	// Verify both org1 and org2 are in the list
	foundOrg1, foundOrg2 := false, false
	for _, id := range orgIDs {
		if id == org1.ID {
			foundOrg1 = true
		}
		if id == org2.ID {
			foundOrg2 = true
		}
	}
	if !foundOrg1 {
		t.Error("expected org1 in orgIDs")
	}
	if !foundOrg2 {
		t.Error("expected org2 in orgIDs")
	}
}

func TestStore_OrgIDsByUser_Empty(t *testing.T) {
	db := testutil.SetupTestDB(t)
	store := coordinatorassign.New(db)
	fixtures := testutil.NewFixtures(t, db)
	ctx, cancel := testutil.TestContext()
	defer cancel()

	coordinator := fixtures.CreateUser(ctx, "Test Coordinator", "coordinator@example.com", "coordinator", nil)

	// No assignments
	orgIDs, err := store.OrgIDsByUser(ctx, coordinator.ID)
	if err != nil {
		t.Fatalf("OrgIDsByUser failed: %v", err)
	}

	if len(orgIDs) != 0 {
		t.Errorf("expected 0 org IDs, got %d", len(orgIDs))
	}
}

func TestStore_DeleteByUser(t *testing.T) {
	db := testutil.SetupTestDB(t)
	store := coordinatorassign.New(db)
	fixtures := testutil.NewFixtures(t, db)
	ctx, cancel := testutil.TestContext()
	defer cancel()

	coordinator1 := fixtures.CreateUser(ctx, "Coordinator One", "coord1@example.com", "coordinator", nil)
	coordinator2 := fixtures.CreateUser(ctx, "Coordinator Two", "coord2@example.com", "coordinator", nil)
	org1 := fixtures.CreateOrganization(ctx, "Org One")
	org2 := fixtures.CreateOrganization(ctx, "Org Two")

	// Assign coordinator1 to 2 orgs
	_, _ = store.Create(ctx, models.CoordinatorAssignment{UserID: coordinator1.ID, OrganizationID: org1.ID})
	_, _ = store.Create(ctx, models.CoordinatorAssignment{UserID: coordinator1.ID, OrganizationID: org2.ID})

	// Assign coordinator2 to 1 org
	_, _ = store.Create(ctx, models.CoordinatorAssignment{UserID: coordinator2.ID, OrganizationID: org1.ID})

	// Delete all for coordinator1
	count, err := store.DeleteByUser(ctx, coordinator1.ID)
	if err != nil {
		t.Fatalf("DeleteByUser failed: %v", err)
	}

	if count != 2 {
		t.Errorf("expected 2 deleted, got %d", count)
	}

	// Verify coordinator1 has no assignments
	list, _ := store.ListByUser(ctx, coordinator1.ID)
	if len(list) != 0 {
		t.Errorf("expected 0 assignments for coordinator1, got %d", len(list))
	}

	// Verify coordinator2 still has assignment
	list, _ = store.ListByUser(ctx, coordinator2.ID)
	if len(list) != 1 {
		t.Errorf("expected 1 assignment for coordinator2, got %d", len(list))
	}
}

func TestStore_DeleteByOrg(t *testing.T) {
	db := testutil.SetupTestDB(t)
	store := coordinatorassign.New(db)
	fixtures := testutil.NewFixtures(t, db)
	ctx, cancel := testutil.TestContext()
	defer cancel()

	coordinator1 := fixtures.CreateUser(ctx, "Coordinator One", "coord1@example.com", "coordinator", nil)
	coordinator2 := fixtures.CreateUser(ctx, "Coordinator Two", "coord2@example.com", "coordinator", nil)
	org1 := fixtures.CreateOrganization(ctx, "Org One")
	org2 := fixtures.CreateOrganization(ctx, "Org Two")
	_ = coordinator2 // used in assignment below

	// Assign both coordinators to org1
	_, _ = store.Create(ctx, models.CoordinatorAssignment{UserID: coordinator1.ID, OrganizationID: org1.ID})
	_, _ = store.Create(ctx, models.CoordinatorAssignment{UserID: coordinator2.ID, OrganizationID: org1.ID})

	// Assign coordinator1 to org2
	_, _ = store.Create(ctx, models.CoordinatorAssignment{UserID: coordinator1.ID, OrganizationID: org2.ID})

	// Delete all for org1
	count, err := store.DeleteByOrg(ctx, org1.ID)
	if err != nil {
		t.Fatalf("DeleteByOrg failed: %v", err)
	}

	if count != 2 {
		t.Errorf("expected 2 deleted, got %d", count)
	}

	// Verify org1 has no coordinators
	list, _ := store.ListByOrg(ctx, org1.ID)
	if len(list) != 0 {
		t.Errorf("expected 0 assignments for org1, got %d", len(list))
	}

	// Verify org2 still has coordinator1
	list, _ = store.ListByOrg(ctx, org2.ID)
	if len(list) != 1 {
		t.Errorf("expected 1 assignment for org2, got %d", len(list))
	}
}

func TestStore_Exists(t *testing.T) {
	db := testutil.SetupTestDB(t)
	store := coordinatorassign.New(db)
	fixtures := testutil.NewFixtures(t, db)
	ctx, cancel := testutil.TestContext()
	defer cancel()

	coordinator := fixtures.CreateUser(ctx, "Test Coordinator", "coordinator@example.com", "coordinator", nil)
	org1 := fixtures.CreateOrganization(ctx, "Org One")
	org2 := fixtures.CreateOrganization(ctx, "Org Two")

	// Assign to org1 only
	_, _ = store.Create(ctx, models.CoordinatorAssignment{UserID: coordinator.ID, OrganizationID: org1.ID})

	// Check exists for org1
	exists, err := store.Exists(ctx, coordinator.ID, org1.ID)
	if err != nil {
		t.Fatalf("Exists failed: %v", err)
	}
	if !exists {
		t.Error("expected assignment to exist for org1")
	}

	// Check exists for org2
	exists, err = store.Exists(ctx, coordinator.ID, org2.ID)
	if err != nil {
		t.Fatalf("Exists failed: %v", err)
	}
	if exists {
		t.Error("expected assignment to NOT exist for org2")
	}
}

func TestStore_CountByUser(t *testing.T) {
	db := testutil.SetupTestDB(t)
	store := coordinatorassign.New(db)
	fixtures := testutil.NewFixtures(t, db)
	ctx, cancel := testutil.TestContext()
	defer cancel()

	coordinator := fixtures.CreateUser(ctx, "Test Coordinator", "coordinator@example.com", "coordinator", nil)
	org1 := fixtures.CreateOrganization(ctx, "Org One")
	org2 := fixtures.CreateOrganization(ctx, "Org Two")
	org3 := fixtures.CreateOrganization(ctx, "Org Three")

	// Assign to 3 orgs
	_, _ = store.Create(ctx, models.CoordinatorAssignment{UserID: coordinator.ID, OrganizationID: org1.ID})
	_, _ = store.Create(ctx, models.CoordinatorAssignment{UserID: coordinator.ID, OrganizationID: org2.ID})
	_, _ = store.Create(ctx, models.CoordinatorAssignment{UserID: coordinator.ID, OrganizationID: org3.ID})

	count, err := store.CountByUser(ctx, coordinator.ID)
	if err != nil {
		t.Fatalf("CountByUser failed: %v", err)
	}

	if count != 3 {
		t.Errorf("expected count 3, got %d", count)
	}
}

func TestStore_CountByUser_Zero(t *testing.T) {
	db := testutil.SetupTestDB(t)
	store := coordinatorassign.New(db)
	fixtures := testutil.NewFixtures(t, db)
	ctx, cancel := testutil.TestContext()
	defer cancel()

	coordinator := fixtures.CreateUser(ctx, "Test Coordinator", "coordinator@example.com", "coordinator", nil)

	count, err := store.CountByUser(ctx, coordinator.ID)
	if err != nil {
		t.Fatalf("CountByUser failed: %v", err)
	}

	if count != 0 {
		t.Errorf("expected count 0, got %d", count)
	}
}
