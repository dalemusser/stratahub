package membershipstore_test

// Terminology: User Identifiers
//   - UserID / userID / user_id: The MongoDB ObjectID (_id) that uniquely identifies a user record
//   - LoginID / loginID / login_id: The human-readable string users type to log in

import (
	"testing"

	membershipstore "github.com/dalemusser/stratahub/internal/app/store/memberships"
	"github.com/dalemusser/stratahub/internal/testutil"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
)

func TestStore_Add(t *testing.T) {
	db := testutil.SetupTestDB(t)
	store := membershipstore.New(db)
	fixtures := testutil.NewFixtures(t, db)
	ctx, cancel := testutil.TestContext()
	defer cancel()

	org := fixtures.CreateOrganization(ctx, "Test Org")
	group := fixtures.CreateGroup(ctx, "Test Group", org.ID)
	member := fixtures.CreateMember(ctx, "Test Member", "member@example.com", org.ID)

	err := store.Add(ctx, group.ID, member.ID, "member")
	if err != nil {
		t.Fatalf("Add failed: %v", err)
	}

	// Verify the membership was created
	count, err := db.Collection("group_memberships").CountDocuments(ctx, bson.M{
		"group_id": group.ID,
		"user_id":  member.ID,
	})
	if err != nil {
		t.Fatalf("CountDocuments failed: %v", err)
	}
	if count != 1 {
		t.Errorf("expected 1 membership, got %d", count)
	}
}

func TestStore_Add_LeaderRole(t *testing.T) {
	db := testutil.SetupTestDB(t)
	store := membershipstore.New(db)
	fixtures := testutil.NewFixtures(t, db)
	ctx, cancel := testutil.TestContext()
	defer cancel()

	org := fixtures.CreateOrganization(ctx, "Test Org")
	group := fixtures.CreateGroup(ctx, "Test Group", org.ID)
	leader := fixtures.CreateLeader(ctx, "Test Leader", "leader@example.com", org.ID)

	err := store.Add(ctx, group.ID, leader.ID, "leader")
	if err != nil {
		t.Fatalf("Add failed: %v", err)
	}

	// Verify role was set correctly
	var membership struct {
		Role string `bson:"role"`
	}
	err = db.Collection("group_memberships").FindOne(ctx, bson.M{
		"group_id": group.ID,
		"user_id":  leader.ID,
	}).Decode(&membership)
	if err != nil {
		t.Fatalf("FindOne failed: %v", err)
	}
	if membership.Role != "leader" {
		t.Errorf("Role: got %q, want %q", membership.Role, "leader")
	}
}

func TestStore_Add_InvalidRole(t *testing.T) {
	db := testutil.SetupTestDB(t)
	store := membershipstore.New(db)
	fixtures := testutil.NewFixtures(t, db)
	ctx, cancel := testutil.TestContext()
	defer cancel()

	org := fixtures.CreateOrganization(ctx, "Test Org")
	group := fixtures.CreateGroup(ctx, "Test Group", org.ID)
	member := fixtures.CreateMember(ctx, "Test Member", "member@example.com", org.ID)

	err := store.Add(ctx, group.ID, member.ID, "invalid_role")
	if err == nil {
		t.Fatal("expected error for invalid role")
	}
}

func TestStore_Add_OrgMismatch(t *testing.T) {
	db := testutil.SetupTestDB(t)
	store := membershipstore.New(db)
	fixtures := testutil.NewFixtures(t, db)
	ctx, cancel := testutil.TestContext()
	defer cancel()

	org1 := fixtures.CreateOrganization(ctx, "Org One")
	org2 := fixtures.CreateOrganization(ctx, "Org Two")

	group := fixtures.CreateGroup(ctx, "Group in Org1", org1.ID)
	member := fixtures.CreateMember(ctx, "Member in Org2", "member@example.com", org2.ID)

	err := store.Add(ctx, group.ID, member.ID, "member")
	if err == nil {
		t.Fatal("expected error when user and group belong to different orgs")
	}
}

func TestStore_Add_Duplicate(t *testing.T) {
	db := testutil.SetupTestDB(t)
	store := membershipstore.New(db)
	fixtures := testutil.NewFixtures(t, db)
	ctx, cancel := testutil.TestContext()
	defer cancel()

	org := fixtures.CreateOrganization(ctx, "Test Org")
	group := fixtures.CreateGroup(ctx, "Test Group", org.ID)
	member := fixtures.CreateMember(ctx, "Test Member", "member@example.com", org.ID)

	// First add should succeed
	err := store.Add(ctx, group.ID, member.ID, "member")
	if err != nil {
		t.Fatalf("first Add failed: %v", err)
	}

	// Second add should fail with duplicate error
	err = store.Add(ctx, group.ID, member.ID, "member")
	if err != membershipstore.ErrDuplicateMembership {
		t.Errorf("expected ErrDuplicateMembership, got %v", err)
	}
}

func TestStore_Add_GroupNotFound(t *testing.T) {
	db := testutil.SetupTestDB(t)
	store := membershipstore.New(db)
	fixtures := testutil.NewFixtures(t, db)
	ctx, cancel := testutil.TestContext()
	defer cancel()

	org := fixtures.CreateOrganization(ctx, "Test Org")
	member := fixtures.CreateMember(ctx, "Test Member", "member@example.com", org.ID)
	fakeGroupID := primitive.NewObjectID()

	err := store.Add(ctx, fakeGroupID, member.ID, "member")
	if err != mongo.ErrNoDocuments {
		t.Errorf("expected mongo.ErrNoDocuments, got %v", err)
	}
}

func TestStore_Add_UserNotFound(t *testing.T) {
	db := testutil.SetupTestDB(t)
	store := membershipstore.New(db)
	fixtures := testutil.NewFixtures(t, db)
	ctx, cancel := testutil.TestContext()
	defer cancel()

	org := fixtures.CreateOrganization(ctx, "Test Org")
	group := fixtures.CreateGroup(ctx, "Test Group", org.ID)
	fakeUserID := primitive.NewObjectID()

	err := store.Add(ctx, group.ID, fakeUserID, "member")
	if err != mongo.ErrNoDocuments {
		t.Errorf("expected mongo.ErrNoDocuments, got %v", err)
	}
}

func TestStore_Remove(t *testing.T) {
	db := testutil.SetupTestDB(t)
	store := membershipstore.New(db)
	fixtures := testutil.NewFixtures(t, db)
	ctx, cancel := testutil.TestContext()
	defer cancel()

	org := fixtures.CreateOrganization(ctx, "Test Org")
	group := fixtures.CreateGroup(ctx, "Test Group", org.ID)
	member := fixtures.CreateMember(ctx, "Test Member", "member@example.com", org.ID)

	// Add membership first
	err := store.Add(ctx, group.ID, member.ID, "member")
	if err != nil {
		t.Fatalf("Add failed: %v", err)
	}

	// Verify it exists
	count, _ := db.Collection("group_memberships").CountDocuments(ctx, bson.M{
		"group_id": group.ID,
		"user_id":  member.ID,
	})
	if count != 1 {
		t.Fatalf("expected 1 membership before remove, got %d", count)
	}

	// Remove it
	err = store.Remove(ctx, group.ID, member.ID)
	if err != nil {
		t.Fatalf("Remove failed: %v", err)
	}

	// Verify it's gone
	count, _ = db.Collection("group_memberships").CountDocuments(ctx, bson.M{
		"group_id": group.ID,
		"user_id":  member.ID,
	})
	if count != 0 {
		t.Errorf("expected 0 memberships after remove, got %d", count)
	}
}

func TestStore_Remove_NonExistent(t *testing.T) {
	db := testutil.SetupTestDB(t)
	store := membershipstore.New(db)
	ctx, cancel := testutil.TestContext()
	defer cancel()

	// Remove should not error even if membership doesn't exist
	err := store.Remove(ctx, primitive.NewObjectID(), primitive.NewObjectID())
	if err != nil {
		t.Errorf("Remove should not error for non-existent membership: %v", err)
	}
}

func TestStore_AddBatch(t *testing.T) {
	db := testutil.SetupTestDB(t)
	store := membershipstore.New(db)
	fixtures := testutil.NewFixtures(t, db)
	ctx, cancel := testutil.TestContext()
	defer cancel()

	org := fixtures.CreateOrganization(ctx, "Test Org")
	group := fixtures.CreateGroup(ctx, "Test Group", org.ID)

	member1 := fixtures.CreateMember(ctx, "Member One", "member1@example.com", org.ID)
	member2 := fixtures.CreateMember(ctx, "Member Two", "member2@example.com", org.ID)
	leader := fixtures.CreateLeader(ctx, "Leader", "leader@example.com", org.ID)

	entries := []membershipstore.MembershipEntry{
		{UserID: member1.ID, Role: "member"},
		{UserID: member2.ID, Role: "member"},
		{UserID: leader.ID, Role: "leader"},
	}

	result, err := store.AddBatch(ctx, group.ID, org.ID, entries)
	if err != nil {
		t.Fatalf("AddBatch failed: %v", err)
	}

	if result.Added != 3 {
		t.Errorf("Added: got %d, want 3", result.Added)
	}
	if result.Duplicates != 0 {
		t.Errorf("Duplicates: got %d, want 0", result.Duplicates)
	}

	// Verify all memberships were created
	count, _ := db.Collection("group_memberships").CountDocuments(ctx, bson.M{"group_id": group.ID})
	if count != 3 {
		t.Errorf("expected 3 memberships, got %d", count)
	}
}

func TestStore_AddBatch_Empty(t *testing.T) {
	db := testutil.SetupTestDB(t)
	store := membershipstore.New(db)
	fixtures := testutil.NewFixtures(t, db)
	ctx, cancel := testutil.TestContext()
	defer cancel()

	org := fixtures.CreateOrganization(ctx, "Test Org")
	group := fixtures.CreateGroup(ctx, "Test Group", org.ID)

	result, err := store.AddBatch(ctx, group.ID, org.ID, nil)
	if err != nil {
		t.Fatalf("AddBatch failed: %v", err)
	}

	if result.Added != 0 {
		t.Errorf("Added: got %d, want 0", result.Added)
	}
}

func TestStore_AddBatch_WithDuplicates(t *testing.T) {
	db := testutil.SetupTestDB(t)
	store := membershipstore.New(db)
	fixtures := testutil.NewFixtures(t, db)
	ctx, cancel := testutil.TestContext()
	defer cancel()

	org := fixtures.CreateOrganization(ctx, "Test Org")
	group := fixtures.CreateGroup(ctx, "Test Group", org.ID)

	member1 := fixtures.CreateMember(ctx, "Member One", "member1@example.com", org.ID)
	member2 := fixtures.CreateMember(ctx, "Member Two", "member2@example.com", org.ID)

	// Add first member directly
	err := store.Add(ctx, group.ID, member1.ID, "member")
	if err != nil {
		t.Fatalf("Add failed: %v", err)
	}

	// Now batch add including the duplicate
	entries := []membershipstore.MembershipEntry{
		{UserID: member1.ID, Role: "member"}, // duplicate
		{UserID: member2.ID, Role: "member"}, // new
	}

	result, err := store.AddBatch(ctx, group.ID, org.ID, entries)
	if err != nil {
		t.Fatalf("AddBatch failed: %v", err)
	}

	if result.Added != 1 {
		t.Errorf("Added: got %d, want 1", result.Added)
	}
	if result.Duplicates != 1 {
		t.Errorf("Duplicates: got %d, want 1", result.Duplicates)
	}
}

func TestStore_AddBatch_InvalidRole(t *testing.T) {
	db := testutil.SetupTestDB(t)
	store := membershipstore.New(db)
	fixtures := testutil.NewFixtures(t, db)
	ctx, cancel := testutil.TestContext()
	defer cancel()

	org := fixtures.CreateOrganization(ctx, "Test Org")
	group := fixtures.CreateGroup(ctx, "Test Group", org.ID)
	member := fixtures.CreateMember(ctx, "Member", "member@example.com", org.ID)

	entries := []membershipstore.MembershipEntry{
		{UserID: member.ID, Role: "invalid"},
	}

	_, err := store.AddBatch(ctx, group.ID, org.ID, entries)
	if err == nil {
		t.Fatal("expected error for invalid role")
	}
}
