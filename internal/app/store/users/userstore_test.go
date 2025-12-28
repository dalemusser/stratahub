package userstore_test

import (
	"testing"

	userstore "github.com/dalemusser/stratahub/internal/app/store/users"
	"github.com/dalemusser/stratahub/internal/domain/models"
	"github.com/dalemusser/stratahub/internal/testutil"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
)

func TestStore_Create_Admin(t *testing.T) {
	db := testutil.SetupTestDB(t)
	store := userstore.New(db)
	ctx, cancel := testutil.TestContext()
	defer cancel()

	user := models.User{
		FullName: "Admin User",
		Email:    "admin@example.com",
		Role:     "admin",
	}

	created, err := store.Create(ctx, user)
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	// Verify ID was assigned
	if created.ID == primitive.NilObjectID {
		t.Error("expected ID to be assigned")
	}

	// Verify normalized fields
	if created.FullNameCI == "" {
		t.Error("expected FullNameCI to be set")
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

	// Verify admin doesn't require org
	if created.OrganizationID != nil {
		t.Error("admin should not have organization_id")
	}
}

func TestStore_Create_Member(t *testing.T) {
	db := testutil.SetupTestDB(t)
	store := userstore.New(db)
	fixtures := testutil.NewFixtures(t, db)
	ctx, cancel := testutil.TestContext()
	defer cancel()

	// Create an organization first
	org := fixtures.CreateOrganization(ctx, "Test Org")

	user := models.User{
		FullName:       "Member User",
		Email:          "member@example.com",
		Role:           "member",
		OrganizationID: &org.ID,
	}

	created, err := store.Create(ctx, user)
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	if created.OrganizationID == nil {
		t.Error("expected OrganizationID to be set")
	}
	if *created.OrganizationID != org.ID {
		t.Errorf("OrganizationID: got %v, want %v", *created.OrganizationID, org.ID)
	}
}

func TestStore_Create_MemberWithoutOrg(t *testing.T) {
	db := testutil.SetupTestDB(t)
	store := userstore.New(db)
	ctx, cancel := testutil.TestContext()
	defer cancel()

	user := models.User{
		FullName: "Member User",
		Email:    "member@example.com",
		Role:     "member",
		// No OrganizationID
	}

	_, err := store.Create(ctx, user)
	if err == nil {
		t.Fatal("expected error when creating member without org")
	}
}

func TestStore_Create_LeaderWithoutOrg(t *testing.T) {
	db := testutil.SetupTestDB(t)
	store := userstore.New(db)
	ctx, cancel := testutil.TestContext()
	defer cancel()

	user := models.User{
		FullName: "Leader User",
		Email:    "leader@example.com",
		Role:     "leader",
		// No OrganizationID
	}

	_, err := store.Create(ctx, user)
	if err == nil {
		t.Fatal("expected error when creating leader without org")
	}
}

func TestStore_Create_InvalidRole(t *testing.T) {
	db := testutil.SetupTestDB(t)
	store := userstore.New(db)
	ctx, cancel := testutil.TestContext()
	defer cancel()

	user := models.User{
		FullName: "Test User",
		Email:    "test@example.com",
		Role:     "invalid_role",
	}

	_, err := store.Create(ctx, user)
	if err == nil {
		t.Fatal("expected error for invalid role")
	}
}

func TestStore_Create_DuplicateEmail(t *testing.T) {
	db := testutil.SetupTestDB(t)
	store := userstore.New(db)
	ctx, cancel := testutil.TestContext()
	defer cancel()

	user1 := models.User{
		FullName: "User One",
		Email:    "duplicate@example.com",
		Role:     "admin",
	}

	_, err := store.Create(ctx, user1)
	if err != nil {
		t.Fatalf("first Create failed: %v", err)
	}

	user2 := models.User{
		FullName: "User Two",
		Email:    "duplicate@example.com",
		Role:     "admin",
	}

	_, err = store.Create(ctx, user2)
	if err != userstore.ErrDuplicateEmail {
		t.Errorf("expected ErrDuplicateEmail, got %v", err)
	}
}

func TestStore_GetByID(t *testing.T) {
	db := testutil.SetupTestDB(t)
	store := userstore.New(db)
	ctx, cancel := testutil.TestContext()
	defer cancel()

	user := models.User{
		FullName:   "Test User",
		Email:      "getbyid@example.com",
		Role:       "admin",
		AuthMethod: "internal",
	}

	created, err := store.Create(ctx, user)
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	found, err := store.GetByID(ctx, created.ID)
	if err != nil {
		t.Fatalf("GetByID failed: %v", err)
	}

	if found.FullName != created.FullName {
		t.Errorf("FullName: got %q, want %q", found.FullName, created.FullName)
	}
	if found.Email != created.Email {
		t.Errorf("Email: got %q, want %q", found.Email, created.Email)
	}
}

func TestStore_GetByID_NotFound(t *testing.T) {
	db := testutil.SetupTestDB(t)
	store := userstore.New(db)
	ctx, cancel := testutil.TestContext()
	defer cancel()

	fakeID := primitive.NewObjectID()
	_, err := store.GetByID(ctx, fakeID)
	if err != mongo.ErrNoDocuments {
		t.Errorf("expected mongo.ErrNoDocuments, got %v", err)
	}
}

func TestStore_GetByEmail(t *testing.T) {
	db := testutil.SetupTestDB(t)
	store := userstore.New(db)
	ctx, cancel := testutil.TestContext()
	defer cancel()

	user := models.User{
		FullName: "Email Test User",
		Email:    "FindMe@Example.COM",
		Role:     "admin",
	}

	created, err := store.Create(ctx, user)
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	// Search with different case
	found, err := store.GetByEmail(ctx, "findme@example.com")
	if err != nil {
		t.Fatalf("GetByEmail failed: %v", err)
	}

	if found.ID != created.ID {
		t.Errorf("ID: got %v, want %v", found.ID, created.ID)
	}
}

func TestStore_GetMemberByID(t *testing.T) {
	db := testutil.SetupTestDB(t)
	store := userstore.New(db)
	fixtures := testutil.NewFixtures(t, db)
	ctx, cancel := testutil.TestContext()
	defer cancel()

	org := fixtures.CreateOrganization(ctx, "Test Org")

	// Create a member
	memberUser := models.User{
		FullName:       "Member User",
		Email:          "member@example.com",
		Role:           "member",
		OrganizationID: &org.ID,
	}
	member, err := store.Create(ctx, memberUser)
	if err != nil {
		t.Fatalf("Create member failed: %v", err)
	}

	// Create an admin
	adminUser := models.User{
		FullName: "Admin User",
		Email:    "admin@example.com",
		Role:     "admin",
	}
	admin, err := store.Create(ctx, adminUser)
	if err != nil {
		t.Fatalf("Create admin failed: %v", err)
	}

	// GetMemberByID should find the member
	found, err := store.GetMemberByID(ctx, member.ID)
	if err != nil {
		t.Fatalf("GetMemberByID failed: %v", err)
	}
	if found.ID != member.ID {
		t.Errorf("ID: got %v, want %v", found.ID, member.ID)
	}

	// GetMemberByID should NOT find the admin
	_, err = store.GetMemberByID(ctx, admin.ID)
	if err != mongo.ErrNoDocuments {
		t.Errorf("expected mongo.ErrNoDocuments for admin, got %v", err)
	}
}

func TestStore_EmailExistsForOther(t *testing.T) {
	db := testutil.SetupTestDB(t)
	store := userstore.New(db)
	ctx, cancel := testutil.TestContext()
	defer cancel()

	user1 := models.User{
		FullName: "User One",
		Email:    "user1@example.com",
		Role:     "admin",
	}
	created1, err := store.Create(ctx, user1)
	if err != nil {
		t.Fatalf("Create user1 failed: %v", err)
	}

	user2 := models.User{
		FullName: "User Two",
		Email:    "user2@example.com",
		Role:     "admin",
	}
	created2, err := store.Create(ctx, user2)
	if err != nil {
		t.Fatalf("Create user2 failed: %v", err)
	}

	// Check if user1's email exists for someone other than user1 (should be false)
	exists, err := store.EmailExistsForOther(ctx, "user1@example.com", created1.ID)
	if err != nil {
		t.Fatalf("EmailExistsForOther failed: %v", err)
	}
	if exists {
		t.Error("expected false when checking own email")
	}

	// Check if user1's email exists for someone other than user2 (should be true)
	exists, err = store.EmailExistsForOther(ctx, "user1@example.com", created2.ID)
	if err != nil {
		t.Fatalf("EmailExistsForOther failed: %v", err)
	}
	if !exists {
		t.Error("expected true when checking another user's email")
	}

	// Check non-existent email
	exists, err = store.EmailExistsForOther(ctx, "nonexistent@example.com", created1.ID)
	if err != nil {
		t.Fatalf("EmailExistsForOther failed: %v", err)
	}
	if exists {
		t.Error("expected false for non-existent email")
	}
}

func TestStore_UpdateMember(t *testing.T) {
	db := testutil.SetupTestDB(t)
	store := userstore.New(db)
	fixtures := testutil.NewFixtures(t, db)
	ctx, cancel := testutil.TestContext()
	defer cancel()

	org := fixtures.CreateOrganization(ctx, "Test Org")

	memberUser := models.User{
		FullName:       "Original Name",
		Email:          "original@example.com",
		Role:           "member",
		OrganizationID: &org.ID,
		AuthMethod:     "internal",
	}
	member, err := store.Create(ctx, memberUser)
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	// Update the member
	upd := userstore.MemberUpdate{
		FullName:       "Updated Name",
		Email:          "updated@example.com",
		AuthMethod:     "google",
		Status:         "disabled",
		OrganizationID: org.ID,
	}

	err = store.UpdateMember(ctx, member.ID, upd)
	if err != nil {
		t.Fatalf("UpdateMember failed: %v", err)
	}

	// Verify the update
	found, err := store.GetByID(ctx, member.ID)
	if err != nil {
		t.Fatalf("GetByID failed: %v", err)
	}

	if found.FullName != "Updated Name" {
		t.Errorf("FullName: got %q, want %q", found.FullName, "Updated Name")
	}
	if found.Email != "updated@example.com" {
		t.Errorf("Email: got %q, want %q", found.Email, "updated@example.com")
	}
	if found.AuthMethod != "google" {
		t.Errorf("AuthMethod: got %q, want %q", found.AuthMethod, "google")
	}
	if found.Status != "disabled" {
		t.Errorf("Status: got %q, want %q", found.Status, "disabled")
	}
}

func TestStore_DeleteMember(t *testing.T) {
	db := testutil.SetupTestDB(t)
	store := userstore.New(db)
	fixtures := testutil.NewFixtures(t, db)
	ctx, cancel := testutil.TestContext()
	defer cancel()

	org := fixtures.CreateOrganization(ctx, "Test Org")

	memberUser := models.User{
		FullName:       "Delete Me",
		Email:          "delete@example.com",
		Role:           "member",
		OrganizationID: &org.ID,
	}
	member, err := store.Create(ctx, memberUser)
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	// Delete the member
	count, err := store.DeleteMember(ctx, member.ID)
	if err != nil {
		t.Fatalf("DeleteMember failed: %v", err)
	}
	if count != 1 {
		t.Errorf("expected 1 deleted, got %d", count)
	}

	// Verify deletion
	_, err = store.GetByID(ctx, member.ID)
	if err != mongo.ErrNoDocuments {
		t.Errorf("expected mongo.ErrNoDocuments after delete, got %v", err)
	}
}

func TestStore_DeleteMember_WrongRole(t *testing.T) {
	db := testutil.SetupTestDB(t)
	store := userstore.New(db)
	ctx, cancel := testutil.TestContext()
	defer cancel()

	// Create an admin
	adminUser := models.User{
		FullName: "Admin User",
		Email:    "admin@example.com",
		Role:     "admin",
	}
	admin, err := store.Create(ctx, adminUser)
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	// Try to delete admin using DeleteMember (should not delete)
	count, err := store.DeleteMember(ctx, admin.ID)
	if err != nil {
		t.Fatalf("DeleteMember failed: %v", err)
	}
	if count != 0 {
		t.Errorf("expected 0 deleted (admin not a member), got %d", count)
	}

	// Verify admin still exists
	_, err = store.GetByID(ctx, admin.ID)
	if err != nil {
		t.Errorf("admin should still exist: %v", err)
	}
}
