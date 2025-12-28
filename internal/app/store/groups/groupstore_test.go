package groupstore_test

import (
	"testing"

	groupstore "github.com/dalemusser/stratahub/internal/app/store/groups"
	"github.com/dalemusser/stratahub/internal/domain/models"
	"github.com/dalemusser/stratahub/internal/testutil"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
)

func TestStore_Create(t *testing.T) {
	db := testutil.SetupTestDB(t)
	store := groupstore.New(db)
	fixtures := testutil.NewFixtures(t, db)
	ctx, cancel := testutil.TestContext()
	defer cancel()

	// Create an organization first
	org := fixtures.CreateOrganization(ctx, "Test Org")

	group := models.Group{
		Name:           "Test Group",
		Description:    "A test group description",
		OrganizationID: org.ID,
	}

	created, err := store.Create(ctx, group)
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	// Verify ID was assigned
	if created.ID == primitive.NilObjectID {
		t.Error("expected ID to be assigned")
	}

	// Verify normalized fields
	if created.NameCI == "" {
		t.Error("expected NameCI to be set")
	}

	// Verify timestamps
	if created.CreatedAt.IsZero() {
		t.Error("expected CreatedAt to be set")
	}
	if created.UpdatedAt.IsZero() {
		t.Error("expected UpdatedAt to be set")
	}

	// Verify default status
	if created.Status != "active" {
		t.Errorf("expected status 'active', got %q", created.Status)
	}

	// Verify organization ID
	if created.OrganizationID != org.ID {
		t.Errorf("OrganizationID: got %v, want %v", created.OrganizationID, org.ID)
	}
}

func TestStore_Create_DuplicateNameInSameOrg(t *testing.T) {
	db := testutil.SetupTestDB(t)
	store := groupstore.New(db)
	fixtures := testutil.NewFixtures(t, db)
	ctx, cancel := testutil.TestContext()
	defer cancel()

	org := fixtures.CreateOrganization(ctx, "Test Org")

	group1 := models.Group{
		Name:           "Duplicate Group",
		OrganizationID: org.ID,
	}

	_, err := store.Create(ctx, group1)
	if err != nil {
		t.Fatalf("first Create failed: %v", err)
	}

	// Try to create duplicate in same org
	group2 := models.Group{
		Name:           "Duplicate Group",
		OrganizationID: org.ID,
	}

	_, err = store.Create(ctx, group2)
	if err != groupstore.ErrDuplicateGroupName {
		t.Errorf("expected ErrDuplicateGroupName, got %v", err)
	}
}

func TestStore_Create_SameNameDifferentOrgs(t *testing.T) {
	db := testutil.SetupTestDB(t)
	store := groupstore.New(db)
	fixtures := testutil.NewFixtures(t, db)
	ctx, cancel := testutil.TestContext()
	defer cancel()

	org1 := fixtures.CreateOrganization(ctx, "Org One")
	org2 := fixtures.CreateOrganization(ctx, "Org Two")

	group1 := models.Group{
		Name:           "Same Name Group",
		OrganizationID: org1.ID,
	}

	_, err := store.Create(ctx, group1)
	if err != nil {
		t.Fatalf("Create in org1 failed: %v", err)
	}

	// Same name in different org should succeed
	group2 := models.Group{
		Name:           "Same Name Group",
		OrganizationID: org2.ID,
	}

	_, err = store.Create(ctx, group2)
	if err != nil {
		t.Fatalf("Create in org2 should succeed: %v", err)
	}
}

func TestStore_GetByID(t *testing.T) {
	db := testutil.SetupTestDB(t)
	store := groupstore.New(db)
	fixtures := testutil.NewFixtures(t, db)
	ctx, cancel := testutil.TestContext()
	defer cancel()

	org := fixtures.CreateOrganization(ctx, "Test Org")

	group := models.Group{
		Name:           "GetByID Test Group",
		Description:    "Test description",
		OrganizationID: org.ID,
	}

	created, err := store.Create(ctx, group)
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	found, err := store.GetByID(ctx, created.ID)
	if err != nil {
		t.Fatalf("GetByID failed: %v", err)
	}

	if found.Name != created.Name {
		t.Errorf("Name: got %q, want %q", found.Name, created.Name)
	}
	if found.Description != created.Description {
		t.Errorf("Description: got %q, want %q", found.Description, created.Description)
	}
	if found.OrganizationID != created.OrganizationID {
		t.Errorf("OrganizationID: got %v, want %v", found.OrganizationID, created.OrganizationID)
	}
}

func TestStore_GetByID_NotFound(t *testing.T) {
	db := testutil.SetupTestDB(t)
	store := groupstore.New(db)
	ctx, cancel := testutil.TestContext()
	defer cancel()

	fakeID := primitive.NewObjectID()
	_, err := store.GetByID(ctx, fakeID)
	if err != mongo.ErrNoDocuments {
		t.Errorf("expected mongo.ErrNoDocuments, got %v", err)
	}
}

func TestStore_UpdateInfo(t *testing.T) {
	db := testutil.SetupTestDB(t)
	store := groupstore.New(db)
	fixtures := testutil.NewFixtures(t, db)
	ctx, cancel := testutil.TestContext()
	defer cancel()

	org := fixtures.CreateOrganization(ctx, "Test Org")

	group := models.Group{
		Name:           "Original Name",
		Description:    "Original description",
		OrganizationID: org.ID,
	}

	created, err := store.Create(ctx, group)
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	// Update the group
	err = store.UpdateInfo(ctx, created.ID, "Updated Name", "Updated description", "active")
	if err != nil {
		t.Fatalf("UpdateInfo failed: %v", err)
	}

	// Verify the update
	found, err := store.GetByID(ctx, created.ID)
	if err != nil {
		t.Fatalf("GetByID failed: %v", err)
	}

	if found.Name != "Updated Name" {
		t.Errorf("Name: got %q, want %q", found.Name, "Updated Name")
	}
	if found.Description != "Updated description" {
		t.Errorf("Description: got %q, want %q", found.Description, "Updated description")
	}
}

func TestStore_Create_CaseInsensitiveName(t *testing.T) {
	db := testutil.SetupTestDB(t)
	store := groupstore.New(db)
	fixtures := testutil.NewFixtures(t, db)
	ctx, cancel := testutil.TestContext()
	defer cancel()

	org := fixtures.CreateOrganization(ctx, "Test Org")

	group := models.Group{
		Name:           "École Française",
		OrganizationID: org.ID,
	}

	created, err := store.Create(ctx, group)
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	// Verify case-insensitive field
	if created.NameCI != "ecole francaise" {
		t.Errorf("NameCI: got %q, want %q", created.NameCI, "ecole francaise")
	}

	// Try to create with same name different case (should fail)
	group2 := models.Group{
		Name:           "ÉCOLE FRANÇAISE",
		OrganizationID: org.ID,
	}

	_, err = store.Create(ctx, group2)
	if err != groupstore.ErrDuplicateGroupName {
		t.Errorf("expected ErrDuplicateGroupName for case-variant, got %v", err)
	}
}
