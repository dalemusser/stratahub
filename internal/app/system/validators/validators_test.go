package validators_test

import (
	"testing"
	"time"

	"github.com/dalemusser/stratahub/internal/app/system/validators"
	"github.com/dalemusser/stratahub/internal/testutil"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

func TestEnsureAll(t *testing.T) {
	db := testutil.SetupTestDB(t)
	ctx, cancel := testutil.TestContext()
	defer cancel()

	// EnsureAll should succeed on a clean database
	err := validators.EnsureAll(ctx, db)
	if err != nil {
		t.Fatalf("EnsureAll failed: %v", err)
	}
}

func TestEnsureAll_Idempotent(t *testing.T) {
	db := testutil.SetupTestDB(t)
	ctx, cancel := testutil.TestContext()
	defer cancel()

	// First call
	err := validators.EnsureAll(ctx, db)
	if err != nil {
		t.Fatalf("First EnsureAll failed: %v", err)
	}

	// Second call should also succeed (idempotent)
	err = validators.EnsureAll(ctx, db)
	if err != nil {
		t.Fatalf("Second EnsureAll failed: %v", err)
	}
}

func TestEnsureAll_CreatesCollections(t *testing.T) {
	db := testutil.SetupTestDB(t)
	ctx, cancel := testutil.TestContext()
	defer cancel()

	err := validators.EnsureAll(ctx, db)
	if err != nil {
		t.Fatalf("EnsureAll failed: %v", err)
	}

	// Verify collections exist
	expectedCollections := []string{
		"users",
		"organizations",
		"groups",
		"resources",
		"group_memberships",
		"group_resource_assignments",
		"coordinator_assignments",
		"login_records",
	}

	names, err := db.ListCollectionNames(ctx, bson.M{})
	if err != nil {
		t.Fatalf("ListCollectionNames failed: %v", err)
	}

	collMap := make(map[string]bool)
	for _, name := range names {
		collMap[name] = true
	}

	for _, expected := range expectedCollections {
		if !collMap[expected] {
			t.Errorf("expected collection %q to exist", expected)
		}
	}
}

func TestUsersValidator_RequiredFields(t *testing.T) {
	db := testutil.SetupTestDB(t)
	ctx, cancel := testutil.TestContext()
	defer cancel()

	err := validators.EnsureAll(ctx, db)
	if err != nil {
		t.Fatalf("EnsureAll failed: %v", err)
	}

	// Try to insert user without required fields - should fail
	_, err = db.Collection("users").InsertOne(ctx, bson.M{
		"login_id": "test",
	})
	if err == nil {
		t.Error("expected validation error when inserting user without required fields")
	}
}

func TestUsersValidator_ValidUser(t *testing.T) {
	db := testutil.SetupTestDB(t)
	ctx, cancel := testutil.TestContext()
	defer cancel()

	err := validators.EnsureAll(ctx, db)
	if err != nil {
		t.Fatalf("EnsureAll failed: %v", err)
	}

	// Insert valid user - should succeed
	_, err = db.Collection("users").InsertOne(ctx, bson.M{
		"full_name":    "Test User",
		"full_name_ci": "test user",
		"login_id":     "testuser",
		"login_id_ci":  "testuser",
		"role":         "member",
		"status":       "active",
		"auth_method":  "password",
	})
	if err != nil {
		t.Errorf("Insert valid user failed: %v", err)
	}
}

func TestUsersValidator_InvalidRole(t *testing.T) {
	db := testutil.SetupTestDB(t)
	ctx, cancel := testutil.TestContext()
	defer cancel()

	err := validators.EnsureAll(ctx, db)
	if err != nil {
		t.Fatalf("EnsureAll failed: %v", err)
	}

	// Try to insert user with invalid role - should fail
	_, err = db.Collection("users").InsertOne(ctx, bson.M{
		"full_name":    "Test User",
		"full_name_ci": "test user",
		"role":         "invalid_role",
		"status":       "active",
		"auth_method":  "password",
	})
	if err == nil {
		t.Error("expected validation error when inserting user with invalid role")
	}
}

func TestUsersValidator_InvalidStatus(t *testing.T) {
	db := testutil.SetupTestDB(t)
	ctx, cancel := testutil.TestContext()
	defer cancel()

	err := validators.EnsureAll(ctx, db)
	if err != nil {
		t.Fatalf("EnsureAll failed: %v", err)
	}

	// Try to insert user with invalid status - should fail
	_, err = db.Collection("users").InsertOne(ctx, bson.M{
		"full_name":    "Test User",
		"full_name_ci": "test user",
		"role":         "member",
		"status":       "invalid_status",
		"auth_method":  "password",
	})
	if err == nil {
		t.Error("expected validation error when inserting user with invalid status")
	}
}

func TestUsersValidator_InvalidAuthMethod(t *testing.T) {
	db := testutil.SetupTestDB(t)
	ctx, cancel := testutil.TestContext()
	defer cancel()

	err := validators.EnsureAll(ctx, db)
	if err != nil {
		t.Fatalf("EnsureAll failed: %v", err)
	}

	// Try to insert user with invalid auth method - should fail
	_, err = db.Collection("users").InsertOne(ctx, bson.M{
		"full_name":    "Test User",
		"full_name_ci": "test user",
		"role":         "member",
		"status":       "active",
		"auth_method":  "invalid_auth",
	})
	if err == nil {
		t.Error("expected validation error when inserting user with invalid auth_method")
	}
}

func TestOrganizationsValidator_RequiredFields(t *testing.T) {
	db := testutil.SetupTestDB(t)
	ctx, cancel := testutil.TestContext()
	defer cancel()

	err := validators.EnsureAll(ctx, db)
	if err != nil {
		t.Fatalf("EnsureAll failed: %v", err)
	}

	// Try to insert org without required fields - should fail
	_, err = db.Collection("organizations").InsertOne(ctx, bson.M{
		"city": "Test City",
	})
	if err == nil {
		t.Error("expected validation error when inserting organization without required fields")
	}
}

func TestOrganizationsValidator_ValidOrg(t *testing.T) {
	db := testutil.SetupTestDB(t)
	ctx, cancel := testutil.TestContext()
	defer cancel()

	err := validators.EnsureAll(ctx, db)
	if err != nil {
		t.Fatalf("EnsureAll failed: %v", err)
	}

	// Insert valid org - should succeed
	_, err = db.Collection("organizations").InsertOne(ctx, bson.M{
		"name":      "Test Org",
		"name_ci":   "test org",
		"status":    "active",
		"time_zone": "America/New_York",
	})
	if err != nil {
		t.Errorf("Insert valid organization failed: %v", err)
	}
}

func TestGroupsValidator_RequiredFields(t *testing.T) {
	db := testutil.SetupTestDB(t)
	ctx, cancel := testutil.TestContext()
	defer cancel()

	err := validators.EnsureAll(ctx, db)
	if err != nil {
		t.Fatalf("EnsureAll failed: %v", err)
	}

	// Try to insert group without required fields - should fail
	_, err = db.Collection("groups").InsertOne(ctx, bson.M{
		"description": "Test Description",
	})
	if err == nil {
		t.Error("expected validation error when inserting group without required fields")
	}
}

func TestGroupsValidator_ValidGroup(t *testing.T) {
	db := testutil.SetupTestDB(t)
	ctx, cancel := testutil.TestContext()
	defer cancel()

	err := validators.EnsureAll(ctx, db)
	if err != nil {
		t.Fatalf("EnsureAll failed: %v", err)
	}

	orgID := primitive.NewObjectID()

	// Insert valid group - should succeed
	_, err = db.Collection("groups").InsertOne(ctx, bson.M{
		"organization_id": orgID,
		"name":            "Test Group",
		"name_ci":         "test group",
		"status":          "active",
	})
	if err != nil {
		t.Errorf("Insert valid group failed: %v", err)
	}
}

func TestResourcesValidator_RequiredFields(t *testing.T) {
	db := testutil.SetupTestDB(t)
	ctx, cancel := testutil.TestContext()
	defer cancel()

	err := validators.EnsureAll(ctx, db)
	if err != nil {
		t.Fatalf("EnsureAll failed: %v", err)
	}

	// Try to insert resource without required fields - should fail
	_, err = db.Collection("resources").InsertOne(ctx, bson.M{
		"launch_url": "https://example.com",
	})
	if err == nil {
		t.Error("expected validation error when inserting resource without required fields")
	}
}

func TestResourcesValidator_ValidResource(t *testing.T) {
	db := testutil.SetupTestDB(t)
	ctx, cancel := testutil.TestContext()
	defer cancel()

	err := validators.EnsureAll(ctx, db)
	if err != nil {
		t.Fatalf("EnsureAll failed: %v", err)
	}

	// Insert valid resource - should succeed
	_, err = db.Collection("resources").InsertOne(ctx, bson.M{
		"title":    "Test Resource",
		"title_ci": "test resource",
		"status":   "active",
		"type":     "website",
	})
	if err != nil {
		t.Errorf("Insert valid resource failed: %v", err)
	}
}

func TestGroupMembershipsValidator_RequiredFields(t *testing.T) {
	db := testutil.SetupTestDB(t)
	ctx, cancel := testutil.TestContext()
	defer cancel()

	err := validators.EnsureAll(ctx, db)
	if err != nil {
		t.Fatalf("EnsureAll failed: %v", err)
	}

	// Try to insert membership without required fields - should fail
	_, err = db.Collection("group_memberships").InsertOne(ctx, bson.M{
		"created_at": time.Now(),
	})
	if err == nil {
		t.Error("expected validation error when inserting group_membership without required fields")
	}
}

func TestGroupMembershipsValidator_ValidMembership(t *testing.T) {
	db := testutil.SetupTestDB(t)
	ctx, cancel := testutil.TestContext()
	defer cancel()

	err := validators.EnsureAll(ctx, db)
	if err != nil {
		t.Fatalf("EnsureAll failed: %v", err)
	}

	userID := primitive.NewObjectID()
	groupID := primitive.NewObjectID()
	orgID := primitive.NewObjectID()

	// Insert valid membership - should succeed
	_, err = db.Collection("group_memberships").InsertOne(ctx, bson.M{
		"user_id":    userID,
		"group_id":   groupID,
		"org_id":     orgID,
		"role":       "member",
		"created_at": time.Now(),
	})
	if err != nil {
		t.Errorf("Insert valid group_membership failed: %v", err)
	}
}

func TestGroupMembershipsValidator_InvalidRole(t *testing.T) {
	db := testutil.SetupTestDB(t)
	ctx, cancel := testutil.TestContext()
	defer cancel()

	err := validators.EnsureAll(ctx, db)
	if err != nil {
		t.Fatalf("EnsureAll failed: %v", err)
	}

	userID := primitive.NewObjectID()
	groupID := primitive.NewObjectID()
	orgID := primitive.NewObjectID()

	// Try to insert membership with invalid role - should fail
	_, err = db.Collection("group_memberships").InsertOne(ctx, bson.M{
		"user_id":    userID,
		"group_id":   groupID,
		"org_id":     orgID,
		"role":       "invalid_role",
		"created_at": time.Now(),
	})
	if err == nil {
		t.Error("expected validation error when inserting group_membership with invalid role")
	}
}

func TestGroupResourceAssignmentsValidator_RequiredFields(t *testing.T) {
	db := testutil.SetupTestDB(t)
	ctx, cancel := testutil.TestContext()
	defer cancel()

	err := validators.EnsureAll(ctx, db)
	if err != nil {
		t.Fatalf("EnsureAll failed: %v", err)
	}

	// Try to insert assignment without required fields - should fail
	_, err = db.Collection("group_resource_assignments").InsertOne(ctx, bson.M{
		"instructions": "Test instructions",
	})
	if err == nil {
		t.Error("expected validation error when inserting group_resource_assignment without required fields")
	}
}

func TestGroupResourceAssignmentsValidator_ValidAssignment(t *testing.T) {
	db := testutil.SetupTestDB(t)
	ctx, cancel := testutil.TestContext()
	defer cancel()

	err := validators.EnsureAll(ctx, db)
	if err != nil {
		t.Fatalf("EnsureAll failed: %v", err)
	}

	groupID := primitive.NewObjectID()
	resourceID := primitive.NewObjectID()
	orgID := primitive.NewObjectID()

	// Insert valid assignment - should succeed
	_, err = db.Collection("group_resource_assignments").InsertOne(ctx, bson.M{
		"group_id":        groupID,
		"resource_id":     resourceID,
		"organization_id": orgID,
		"created_at":      time.Now(),
	})
	if err != nil {
		t.Errorf("Insert valid group_resource_assignment failed: %v", err)
	}
}

func TestCoordinatorAssignmentsValidator_RequiredFields(t *testing.T) {
	db := testutil.SetupTestDB(t)
	ctx, cancel := testutil.TestContext()
	defer cancel()

	err := validators.EnsureAll(ctx, db)
	if err != nil {
		t.Fatalf("EnsureAll failed: %v", err)
	}

	// Try to insert coordinator assignment without required fields - should fail
	_, err = db.Collection("coordinator_assignments").InsertOne(ctx, bson.M{
		"created_by_name": "Admin",
	})
	if err == nil {
		t.Error("expected validation error when inserting coordinator_assignment without required fields")
	}
}

func TestCoordinatorAssignmentsValidator_ValidAssignment(t *testing.T) {
	db := testutil.SetupTestDB(t)
	ctx, cancel := testutil.TestContext()
	defer cancel()

	err := validators.EnsureAll(ctx, db)
	if err != nil {
		t.Fatalf("EnsureAll failed: %v", err)
	}

	userID := primitive.NewObjectID()
	orgID := primitive.NewObjectID()

	// Insert valid coordinator assignment - should succeed
	_, err = db.Collection("coordinator_assignments").InsertOne(ctx, bson.M{
		"user_id":         userID,
		"organization_id": orgID,
		"created_at":      time.Now(),
	})
	if err != nil {
		t.Errorf("Insert valid coordinator_assignment failed: %v", err)
	}
}

func TestLoginRecords_NoValidator(t *testing.T) {
	db := testutil.SetupTestDB(t)
	ctx, cancel := testutil.TestContext()
	defer cancel()

	err := validators.EnsureAll(ctx, db)
	if err != nil {
		t.Fatalf("EnsureAll failed: %v", err)
	}

	// login_records has no validator, so any document should be accepted
	_, err = db.Collection("login_records").InsertOne(ctx, bson.M{
		"any_field": "any_value",
	})
	if err != nil {
		t.Errorf("Insert to login_records should succeed (no validator): %v", err)
	}
}

func TestUsersValidator_AllValidRoles(t *testing.T) {
	db := testutil.SetupTestDB(t)
	ctx, cancel := testutil.TestContext()
	defer cancel()

	err := validators.EnsureAll(ctx, db)
	if err != nil {
		t.Fatalf("EnsureAll failed: %v", err)
	}

	validRoles := []string{"superadmin", "admin", "analyst", "coordinator", "leader", "member", "guest"}

	for _, role := range validRoles {
		// Include unique login_id to avoid duplicate key error on unique index
		loginID := "user_" + role
		_, err = db.Collection("users").InsertOne(ctx, bson.M{
			"full_name":    "Test " + role,
			"full_name_ci": "test " + role,
			"login_id":     loginID,
			"login_id_ci":  loginID,
			"role":         role,
			"status":       "active",
			"auth_method":  "password",
		})
		if err != nil {
			t.Errorf("Insert user with role %q failed: %v", role, err)
		}
	}
}

func TestUsersValidator_AllValidAuthMethods(t *testing.T) {
	db := testutil.SetupTestDB(t)
	ctx, cancel := testutil.TestContext()
	defer cancel()

	err := validators.EnsureAll(ctx, db)
	if err != nil {
		t.Fatalf("EnsureAll failed: %v", err)
	}

	validAuthMethods := []string{"internal", "google", "classlink", "clever", "microsoft", "schoology", "email", "password", "trust"}

	for i, method := range validAuthMethods {
		_, err = db.Collection("users").InsertOne(ctx, bson.M{
			"full_name":    "Test User " + method,
			"full_name_ci": "test user " + method,
			"login_id":     "user" + string(rune('a'+i)),
			"login_id_ci":  "user" + string(rune('a'+i)),
			"role":         "member",
			"status":       "active",
			"auth_method":  method,
		})
		if err != nil {
			t.Errorf("Insert user with auth_method %q failed: %v", method, err)
		}
	}
}
