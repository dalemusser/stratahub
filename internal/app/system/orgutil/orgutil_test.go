package orgutil_test

import (
	"context"
	"testing"

	"github.com/dalemusser/stratahub/internal/app/system/orgutil"
	"github.com/dalemusser/stratahub/internal/testutil"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

func TestListActiveOrgs(t *testing.T) {
	db := testutil.SetupTestDB(t)
	fixtures := testutil.NewFixtures(t, db)
	ctx, cancel := testutil.TestContext()
	defer cancel()

	// Create active orgs
	fixtures.CreateOrganization(ctx, "Active Org 1")
	fixtures.CreateOrganization(ctx, "Active Org 2")

	// Create an inactive org directly
	_, _ = db.Collection("organizations").InsertOne(ctx, bson.M{
		"_id":    primitive.NewObjectID(),
		"name":   "Inactive Org",
		"status": "inactive",
	})

	orgs, err := orgutil.ListActiveOrgs(ctx, db)
	if err != nil {
		t.Fatalf("ListActiveOrgs failed: %v", err)
	}

	if len(orgs) != 2 {
		t.Errorf("expected 2 active orgs, got %d", len(orgs))
	}
}

func TestListActiveOrgs_Empty(t *testing.T) {
	db := testutil.SetupTestDB(t)
	ctx, cancel := testutil.TestContext()
	defer cancel()

	orgs, err := orgutil.ListActiveOrgs(ctx, db)
	if err != nil {
		t.Fatalf("ListActiveOrgs failed: %v", err)
	}

	if len(orgs) != 0 {
		t.Errorf("expected 0 orgs, got %d", len(orgs))
	}
}

func TestFetchOrgNames(t *testing.T) {
	db := testutil.SetupTestDB(t)
	fixtures := testutil.NewFixtures(t, db)
	ctx, cancel := testutil.TestContext()
	defer cancel()

	org1 := fixtures.CreateOrganization(ctx, "First Org")
	org2 := fixtures.CreateOrganization(ctx, "Second Org")
	fixtures.CreateOrganization(ctx, "Third Org") // not fetched

	names, err := orgutil.FetchOrgNames(ctx, db, []primitive.ObjectID{org1.ID, org2.ID})
	if err != nil {
		t.Fatalf("FetchOrgNames failed: %v", err)
	}

	if len(names) != 2 {
		t.Errorf("expected 2 names, got %d", len(names))
	}
	if names[org1.ID] != "First Org" {
		t.Errorf("org1 name: got %q, want %q", names[org1.ID], "First Org")
	}
	if names[org2.ID] != "Second Org" {
		t.Errorf("org2 name: got %q, want %q", names[org2.ID], "Second Org")
	}
}

func TestFetchOrgNames_Empty(t *testing.T) {
	db := testutil.SetupTestDB(t)
	ctx, cancel := testutil.TestContext()
	defer cancel()

	names, err := orgutil.FetchOrgNames(ctx, db, []primitive.ObjectID{})
	if err != nil {
		t.Fatalf("FetchOrgNames failed: %v", err)
	}

	if len(names) != 0 {
		t.Errorf("expected empty map, got %d entries", len(names))
	}
}

func TestGetOrgName(t *testing.T) {
	db := testutil.SetupTestDB(t)
	fixtures := testutil.NewFixtures(t, db)
	ctx, cancel := testutil.TestContext()
	defer cancel()

	org := fixtures.CreateOrganization(ctx, "Test Organization")

	name, err := orgutil.GetOrgName(ctx, db, org.ID)
	if err != nil {
		t.Fatalf("GetOrgName failed: %v", err)
	}

	if name != "Test Organization" {
		t.Errorf("name: got %q, want %q", name, "Test Organization")
	}
}

func TestGetOrgName_NotFound(t *testing.T) {
	db := testutil.SetupTestDB(t)
	ctx, cancel := testutil.TestContext()
	defer cancel()

	fakeID := primitive.NewObjectID()
	name, err := orgutil.GetOrgName(ctx, db, fakeID)
	if err != nil {
		t.Fatalf("GetOrgName failed: %v", err)
	}

	// Should return empty string, not error
	if name != "" {
		t.Errorf("expected empty string, got %q", name)
	}
}

func TestGetOrgName_ZeroID(t *testing.T) {
	db := testutil.SetupTestDB(t)
	ctx, cancel := testutil.TestContext()
	defer cancel()

	name, err := orgutil.GetOrgName(ctx, db, primitive.NilObjectID)
	if err != nil {
		t.Fatalf("GetOrgName failed: %v", err)
	}

	if name != "" {
		t.Errorf("expected empty string for zero ID, got %q", name)
	}
}

func TestResolveLeaderOrg(t *testing.T) {
	db := testutil.SetupTestDB(t)
	fixtures := testutil.NewFixtures(t, db)
	ctx, cancel := testutil.TestContext()
	defer cancel()

	org := fixtures.CreateOrganization(ctx, "Leader's Org")
	leader := fixtures.CreateUser(ctx, "Test Leader", "leader@example.com", "leader", &org.ID)

	orgID, orgName, err := orgutil.ResolveLeaderOrg(ctx, db, leader.ID)
	if err != nil {
		t.Fatalf("ResolveLeaderOrg failed: %v", err)
	}

	if orgID != org.ID {
		t.Errorf("orgID: got %v, want %v", orgID, org.ID)
	}
	if orgName != "Leader's Org" {
		t.Errorf("orgName: got %q, want %q", orgName, "Leader's Org")
	}
}

func TestResolveLeaderOrg_UserNotFound(t *testing.T) {
	db := testutil.SetupTestDB(t)
	ctx, cancel := testutil.TestContext()
	defer cancel()

	fakeID := primitive.NewObjectID()
	_, _, err := orgutil.ResolveLeaderOrg(ctx, db, fakeID)
	if err != orgutil.ErrUserNotFound {
		t.Errorf("expected ErrUserNotFound, got %v", err)
	}
}

func TestResolveLeaderOrg_NoOrganization(t *testing.T) {
	db := testutil.SetupTestDB(t)
	fixtures := testutil.NewFixtures(t, db)
	ctx, cancel := testutil.TestContext()
	defer cancel()

	// Create user without organization
	user := fixtures.CreateUser(ctx, "Admin User", "admin@example.com", "admin", nil)

	_, _, err := orgutil.ResolveLeaderOrg(ctx, db, user.ID)
	if err != orgutil.ErrNoOrganization {
		t.Errorf("expected ErrNoOrganization, got %v", err)
	}
}

func TestResolveOrgFromHex(t *testing.T) {
	db := testutil.SetupTestDB(t)
	fixtures := testutil.NewFixtures(t, db)
	ctx, cancel := testutil.TestContext()
	defer cancel()

	org := fixtures.CreateOrganization(ctx, "Test Organization")

	orgID, orgName, err := orgutil.ResolveOrgFromHex(ctx, db, org.ID.Hex())
	if err != nil {
		t.Fatalf("ResolveOrgFromHex failed: %v", err)
	}

	if orgID != org.ID {
		t.Errorf("orgID: got %v, want %v", orgID, org.ID)
	}
	if orgName != "Test Organization" {
		t.Errorf("orgName: got %q, want %q", orgName, "Test Organization")
	}
}

func TestResolveOrgFromHex_BadID(t *testing.T) {
	db := testutil.SetupTestDB(t)
	ctx, cancel := testutil.TestContext()
	defer cancel()

	_, _, err := orgutil.ResolveOrgFromHex(ctx, db, "invalid-hex")
	if err != orgutil.ErrBadOrgID {
		t.Errorf("expected ErrBadOrgID, got %v", err)
	}
}

func TestResolveOrgFromHex_NotFound(t *testing.T) {
	db := testutil.SetupTestDB(t)
	ctx, cancel := testutil.TestContext()
	defer cancel()

	fakeID := primitive.NewObjectID()
	_, _, err := orgutil.ResolveOrgFromHex(ctx, db, fakeID.Hex())
	if err != orgutil.ErrOrgNotFound {
		t.Errorf("expected ErrOrgNotFound, got %v", err)
	}
}

func TestResolveActiveOrgFromHex(t *testing.T) {
	db := testutil.SetupTestDB(t)
	fixtures := testutil.NewFixtures(t, db)
	ctx, cancel := testutil.TestContext()
	defer cancel()

	org := fixtures.CreateOrganization(ctx, "Active Organization")

	orgID, orgName, err := orgutil.ResolveActiveOrgFromHex(ctx, db, org.ID.Hex())
	if err != nil {
		t.Fatalf("ResolveActiveOrgFromHex failed: %v", err)
	}

	if orgID != org.ID {
		t.Errorf("orgID: got %v, want %v", orgID, org.ID)
	}
	if orgName != "Active Organization" {
		t.Errorf("orgName: got %q, want %q", orgName, "Active Organization")
	}
}

func TestResolveActiveOrgFromHex_NotActive(t *testing.T) {
	db := testutil.SetupTestDB(t)
	ctx, cancel := testutil.TestContext()
	defer cancel()

	// Create inactive org directly
	orgID := primitive.NewObjectID()
	_, _ = db.Collection("organizations").InsertOne(ctx, bson.M{
		"_id":    orgID,
		"name":   "Inactive Org",
		"status": "inactive",
	})

	_, _, err := orgutil.ResolveActiveOrgFromHex(ctx, db, orgID.Hex())
	if err != orgutil.ErrOrgNotActive {
		t.Errorf("expected ErrOrgNotActive, got %v", err)
	}
}

func TestResolveActiveOrgFromHex_BadID(t *testing.T) {
	db := testutil.SetupTestDB(t)
	ctx, cancel := testutil.TestContext()
	defer cancel()

	_, _, err := orgutil.ResolveActiveOrgFromHex(ctx, db, "invalid-hex")
	if err != orgutil.ErrBadOrgID {
		t.Errorf("expected ErrBadOrgID, got %v", err)
	}
}

func TestIsExpectedOrgError(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		expected bool
	}{
		{"ErrBadOrgID", orgutil.ErrBadOrgID, true},
		{"ErrOrgNotFound", orgutil.ErrOrgNotFound, true},
		{"ErrOrgNotActive", orgutil.ErrOrgNotActive, true},
		{"ErrUserNotFound", orgutil.ErrUserNotFound, false},
		{"ErrNoOrganization", orgutil.ErrNoOrganization, false},
		{"nil", nil, false},
		{"context.Canceled", context.Canceled, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := orgutil.IsExpectedOrgError(tt.err)
			if result != tt.expected {
				t.Errorf("IsExpectedOrgError(%v): got %v, want %v", tt.err, result, tt.expected)
			}
		})
	}
}

func TestLoadActiveOrgOptions(t *testing.T) {
	db := testutil.SetupTestDB(t)
	fixtures := testutil.NewFixtures(t, db)
	ctx, cancel := testutil.TestContext()
	defer cancel()

	fixtures.CreateOrganization(ctx, "Alpha Org")
	fixtures.CreateOrganization(ctx, "Beta Org")
	fixtures.CreateOrganization(ctx, "Gamma Org")

	opts, ids, err := orgutil.LoadActiveOrgOptions(ctx, db)
	if err != nil {
		t.Fatalf("LoadActiveOrgOptions failed: %v", err)
	}

	if len(opts) != 3 {
		t.Errorf("expected 3 options, got %d", len(opts))
	}
	if len(ids) != 3 {
		t.Errorf("expected 3 IDs, got %d", len(ids))
	}

	// Verify sorted alphabetically
	if opts[0].Name != "Alpha Org" {
		t.Errorf("first org should be Alpha Org, got %q", opts[0].Name)
	}
	if opts[1].Name != "Beta Org" {
		t.Errorf("second org should be Beta Org, got %q", opts[1].Name)
	}
	if opts[2].Name != "Gamma Org" {
		t.Errorf("third org should be Gamma Org, got %q", opts[2].Name)
	}
}

func TestLoadActiveOrgOptions_SortedCaseInsensitive(t *testing.T) {
	db := testutil.SetupTestDB(t)
	fixtures := testutil.NewFixtures(t, db)
	ctx, cancel := testutil.TestContext()
	defer cancel()

	fixtures.CreateOrganization(ctx, "zebra")
	fixtures.CreateOrganization(ctx, "Apple")
	fixtures.CreateOrganization(ctx, "BANANA")

	opts, _, err := orgutil.LoadActiveOrgOptions(ctx, db)
	if err != nil {
		t.Fatalf("LoadActiveOrgOptions failed: %v", err)
	}

	// Should be sorted: Apple, BANANA, zebra (case-insensitive)
	if opts[0].Name != "Apple" {
		t.Errorf("expected Apple first, got %q", opts[0].Name)
	}
	if opts[1].Name != "BANANA" {
		t.Errorf("expected BANANA second, got %q", opts[1].Name)
	}
	if opts[2].Name != "zebra" {
		t.Errorf("expected zebra third, got %q", opts[2].Name)
	}
}

func TestLoadActiveLeaders(t *testing.T) {
	db := testutil.SetupTestDB(t)
	fixtures := testutil.NewFixtures(t, db)
	ctx, cancel := testutil.TestContext()
	defer cancel()

	org := fixtures.CreateOrganization(ctx, "Test Org")
	fixtures.CreateUser(ctx, "Alpha Leader", "alpha@example.com", "leader", &org.ID)
	fixtures.CreateUser(ctx, "Beta Leader", "beta@example.com", "leader", &org.ID)
	fixtures.CreateUser(ctx, "Member User", "member@example.com", "member", &org.ID)

	leaders, err := orgutil.LoadActiveLeaders(ctx, db, nil)
	if err != nil {
		t.Fatalf("LoadActiveLeaders failed: %v", err)
	}

	if len(leaders) != 2 {
		t.Errorf("expected 2 leaders, got %d", len(leaders))
	}

	// Verify sorted by name
	if leaders[0].FullName != "Alpha Leader" {
		t.Errorf("expected Alpha Leader first, got %q", leaders[0].FullName)
	}
}

func TestLoadActiveLeaders_FilterByOrgs(t *testing.T) {
	db := testutil.SetupTestDB(t)
	fixtures := testutil.NewFixtures(t, db)
	ctx, cancel := testutil.TestContext()
	defer cancel()

	org1 := fixtures.CreateOrganization(ctx, "Org One")
	org2 := fixtures.CreateOrganization(ctx, "Org Two")
	fixtures.CreateUser(ctx, "Leader One", "leader1@example.com", "leader", &org1.ID)
	fixtures.CreateUser(ctx, "Leader Two", "leader2@example.com", "leader", &org2.ID)

	// Only get leaders from org1
	leaders, err := orgutil.LoadActiveLeaders(ctx, db, []primitive.ObjectID{org1.ID})
	if err != nil {
		t.Fatalf("LoadActiveLeaders failed: %v", err)
	}

	if len(leaders) != 1 {
		t.Errorf("expected 1 leader, got %d", len(leaders))
	}
	if leaders[0].FullName != "Leader One" {
		t.Errorf("expected Leader One, got %q", leaders[0].FullName)
	}
}

func TestAggregateCountByField(t *testing.T) {
	db := testutil.SetupTestDB(t)
	fixtures := testutil.NewFixtures(t, db)
	ctx, cancel := testutil.TestContext()
	defer cancel()

	org1 := fixtures.CreateOrganization(ctx, "Org One")
	org2 := fixtures.CreateOrganization(ctx, "Org Two")

	// Create members in each org
	fixtures.CreateUser(ctx, "Member 1", "m1@example.com", "member", &org1.ID)
	fixtures.CreateUser(ctx, "Member 2", "m2@example.com", "member", &org1.ID)
	fixtures.CreateUser(ctx, "Member 3", "m3@example.com", "member", &org2.ID)

	counts, err := orgutil.AggregateCountByField(ctx, db, "users",
		bson.M{"role": "member"},
		"organization_id")
	if err != nil {
		t.Fatalf("AggregateCountByField failed: %v", err)
	}

	if counts[org1.ID] != 2 {
		t.Errorf("org1 count: got %d, want 2", counts[org1.ID])
	}
	if counts[org2.ID] != 1 {
		t.Errorf("org2 count: got %d, want 1", counts[org2.ID])
	}
}

func TestAggregateCountByField_NoMatch(t *testing.T) {
	db := testutil.SetupTestDB(t)
	ctx, cancel := testutil.TestContext()
	defer cancel()

	counts, err := orgutil.AggregateCountByField(ctx, db, "users",
		bson.M{"role": "nonexistent"},
		"organization_id")
	if err != nil {
		t.Fatalf("AggregateCountByField failed: %v", err)
	}

	if len(counts) != 0 {
		t.Errorf("expected empty map, got %d entries", len(counts))
	}
}

// Test error variables are properly defined

func TestErrorVariables(t *testing.T) {
	if orgutil.ErrUserNotFound == nil {
		t.Error("ErrUserNotFound should not be nil")
	}
	if orgutil.ErrNoOrganization == nil {
		t.Error("ErrNoOrganization should not be nil")
	}
	if orgutil.ErrBadOrgID == nil {
		t.Error("ErrBadOrgID should not be nil")
	}
	if orgutil.ErrOrgNotFound == nil {
		t.Error("ErrOrgNotFound should not be nil")
	}
	if orgutil.ErrOrgNotActive == nil {
		t.Error("ErrOrgNotActive should not be nil")
	}
}
