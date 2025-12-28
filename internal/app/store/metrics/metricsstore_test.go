package metricsstore_test

import (
	"testing"

	metricsstore "github.com/dalemusser/stratahub/internal/app/store/metrics"
	"github.com/dalemusser/stratahub/internal/testutil"
)

func TestFetchDashboardCounts_Empty(t *testing.T) {
	db := testutil.SetupTestDB(t)
	ctx, cancel := testutil.TestContext()
	defer cancel()

	counts := metricsstore.FetchDashboardCounts(ctx, db)

	if counts.Organizations != 0 {
		t.Errorf("Organizations: got %d, want 0", counts.Organizations)
	}
	if counts.Leaders != 0 {
		t.Errorf("Leaders: got %d, want 0", counts.Leaders)
	}
	if counts.Groups != 0 {
		t.Errorf("Groups: got %d, want 0", counts.Groups)
	}
	if counts.Members != 0 {
		t.Errorf("Members: got %d, want 0", counts.Members)
	}
	if counts.Resources != 0 {
		t.Errorf("Resources: got %d, want 0", counts.Resources)
	}
}

func TestFetchDashboardCounts_WithData(t *testing.T) {
	db := testutil.SetupTestDB(t)
	fixtures := testutil.NewFixtures(t, db)
	ctx, cancel := testutil.TestContext()
	defer cancel()

	// Create test data
	org1 := fixtures.CreateOrganization(ctx, "Org One")
	org2 := fixtures.CreateOrganization(ctx, "Org Two")

	// Create leaders (2)
	fixtures.CreateLeader(ctx, "Leader One", "leader1@example.com", org1.ID)
	fixtures.CreateLeader(ctx, "Leader Two", "leader2@example.com", org2.ID)

	// Create members (3)
	fixtures.CreateMember(ctx, "Member One", "member1@example.com", org1.ID)
	fixtures.CreateMember(ctx, "Member Two", "member2@example.com", org1.ID)
	fixtures.CreateMember(ctx, "Member Three", "member3@example.com", org2.ID)

	// Create groups (2)
	fixtures.CreateGroup(ctx, "Group One", org1.ID)
	fixtures.CreateGroup(ctx, "Group Two", org2.ID)

	// Create resources (4)
	fixtures.CreateResource(ctx, "Resource One", "https://example.com/1")
	fixtures.CreateResource(ctx, "Resource Two", "https://example.com/2")
	fixtures.CreateResource(ctx, "Resource Three", "https://example.com/3")
	fixtures.CreateResource(ctx, "Resource Four", "https://example.com/4")

	// Fetch counts
	counts := metricsstore.FetchDashboardCounts(ctx, db)

	if counts.Organizations != 2 {
		t.Errorf("Organizations: got %d, want 2", counts.Organizations)
	}
	if counts.Leaders != 2 {
		t.Errorf("Leaders: got %d, want 2", counts.Leaders)
	}
	if counts.Members != 3 {
		t.Errorf("Members: got %d, want 3", counts.Members)
	}
	if counts.Groups != 2 {
		t.Errorf("Groups: got %d, want 2", counts.Groups)
	}
	if counts.Resources != 4 {
		t.Errorf("Resources: got %d, want 4", counts.Resources)
	}
}

func TestFetchDashboardCounts_IgnoresAdminsAndAnalysts(t *testing.T) {
	db := testutil.SetupTestDB(t)
	fixtures := testutil.NewFixtures(t, db)
	ctx, cancel := testutil.TestContext()
	defer cancel()

	org := fixtures.CreateOrganization(ctx, "Test Org")

	// Create various user types
	fixtures.CreateAdmin(ctx, "Admin User", "admin@example.com")
	fixtures.CreateAnalyst(ctx, "Analyst User", "analyst@example.com")
	fixtures.CreateLeader(ctx, "Leader User", "leader@example.com", org.ID)
	fixtures.CreateMember(ctx, "Member User", "member@example.com", org.ID)

	counts := metricsstore.FetchDashboardCounts(ctx, db)

	// Only leaders and members should be counted (admins and analysts are excluded)
	if counts.Leaders != 1 {
		t.Errorf("Leaders: got %d, want 1", counts.Leaders)
	}
	if counts.Members != 1 {
		t.Errorf("Members: got %d, want 1", counts.Members)
	}
}
