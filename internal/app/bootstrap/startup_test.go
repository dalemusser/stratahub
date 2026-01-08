package bootstrap

import (
	"testing"
	"time"

	"github.com/dalemusser/stratahub/internal/domain/models"
	"github.com/dalemusser/stratahub/internal/testutil"
	"github.com/dalemusser/waffle/pantry/text"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.uber.org/zap"
)

func testLogger() *zap.Logger {
	return zap.NewNop()
}

func TestEnsureSuperAdmin_CreatesNew(t *testing.T) {
	db := testutil.SetupTestDB(t)
	ctx, cancel := testutil.TestContext()
	defer cancel()

	deps := DBDeps{StrataHubMongoDatabase: db}

	err := ensureSuperAdmin(ctx, deps, "superadmin@test.com", testLogger())
	if err != nil {
		t.Fatalf("ensureSuperAdmin failed: %v", err)
	}

	// Verify user was created
	var user models.User
	err = db.Collection("users").FindOne(ctx, bson.M{"email": "superadmin@test.com"}).Decode(&user)
	if err != nil {
		t.Fatalf("failed to find created user: %v", err)
	}

	if user.Role != "superadmin" {
		t.Errorf("expected role 'superadmin', got %q", user.Role)
	}
	if user.WorkspaceID != nil {
		t.Error("expected superadmin to have nil workspace_id")
	}
	if user.Status != "active" {
		t.Errorf("expected status 'active', got %q", user.Status)
	}
}

func TestEnsureSuperAdmin_PromotesExisting(t *testing.T) {
	db := testutil.SetupTestDB(t)
	ctx, cancel := testutil.TestContext()
	defer cancel()

	// Create existing user with different role
	now := time.Now().UTC()
	email := "existing@test.com"
	emailCI := text.Fold(email)
	workspaceID := primitive.NewObjectID()
	existingUser := models.User{
		ID:          primitive.NewObjectID(),
		WorkspaceID: &workspaceID,
		FullName:    "Existing User",
		FullNameCI:  text.Fold("Existing User"),
		Email:       &email,
		LoginID:     &email,
		LoginIDCI:   &emailCI,
		Role:        "admin",
		Status:      "active",
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	_, err := db.Collection("users").InsertOne(ctx, existingUser)
	if err != nil {
		t.Fatalf("failed to create existing user: %v", err)
	}

	deps := DBDeps{StrataHubMongoDatabase: db}

	err = ensureSuperAdmin(ctx, deps, email, testLogger())
	if err != nil {
		t.Fatalf("ensureSuperAdmin failed: %v", err)
	}

	// Verify user was promoted
	var user models.User
	err = db.Collection("users").FindOne(ctx, bson.M{"_id": existingUser.ID}).Decode(&user)
	if err != nil {
		t.Fatalf("failed to find user: %v", err)
	}

	if user.Role != "superadmin" {
		t.Errorf("expected role 'superadmin', got %q", user.Role)
	}
	if user.WorkspaceID != nil {
		t.Error("expected superadmin to have nil workspace_id after promotion")
	}
}

func TestEnsureSuperAdmin_AlreadySuperAdmin(t *testing.T) {
	db := testutil.SetupTestDB(t)
	ctx, cancel := testutil.TestContext()
	defer cancel()

	// Create existing superadmin
	now := time.Now().UTC()
	email := "superadmin@test.com"
	emailCI := text.Fold(email)
	existingUser := models.User{
		ID:          primitive.NewObjectID(),
		WorkspaceID: nil,
		FullName:    "Super Admin",
		FullNameCI:  text.Fold("Super Admin"),
		Email:       &email,
		LoginID:     &email,
		LoginIDCI:   &emailCI,
		Role:        "superadmin",
		Status:      "active",
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	_, err := db.Collection("users").InsertOne(ctx, existingUser)
	if err != nil {
		t.Fatalf("failed to create existing user: %v", err)
	}

	deps := DBDeps{StrataHubMongoDatabase: db}

	// Should succeed without error
	err = ensureSuperAdmin(ctx, deps, email, testLogger())
	if err != nil {
		t.Fatalf("ensureSuperAdmin failed: %v", err)
	}

	// Verify user is unchanged
	var user models.User
	err = db.Collection("users").FindOne(ctx, bson.M{"_id": existingUser.ID}).Decode(&user)
	if err != nil {
		t.Fatalf("failed to find user: %v", err)
	}

	if user.Role != "superadmin" {
		t.Errorf("expected role 'superadmin', got %q", user.Role)
	}
}

func TestMigrateDataToWorkspace_MigratesDocuments(t *testing.T) {
	db := testutil.SetupTestDB(t)
	ctx, cancel := testutil.TestContext()
	defer cancel()

	workspaceID := primitive.NewObjectID()

	// Create documents without workspace_id
	now := time.Now().UTC()
	email := "user@test.com"
	emailCI := text.Fold(email)
	user := models.User{
		ID:          primitive.NewObjectID(),
		WorkspaceID: nil, // No workspace_id
		FullName:    "Test User",
		FullNameCI:  text.Fold("Test User"),
		Email:       &email,
		LoginID:     &email,
		LoginIDCI:   &emailCI,
		Role:        "member",
		Status:      "active",
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	_, err := db.Collection("users").InsertOne(ctx, user)
	if err != nil {
		t.Fatalf("failed to create test user: %v", err)
	}

	org := models.Organization{
		ID:        primitive.NewObjectID(),
		Name:      "Test Org",
		NameCI:    text.Fold("Test Org"),
		City:      "Test City",
		CityCI:    text.Fold("Test City"),
		State:     "TS",
		StateCI:   text.Fold("TS"),
		TimeZone:  "America/New_York",
		Status:    "active",
		CreatedAt: now,
		UpdatedAt: now,
	}
	_, err = db.Collection("organizations").InsertOne(ctx, org)
	if err != nil {
		t.Fatalf("failed to create test org: %v", err)
	}

	deps := DBDeps{StrataHubMongoDatabase: db}

	// Run migration
	err = migrateDataToWorkspace(ctx, deps, workspaceID, testLogger())
	if err != nil {
		t.Fatalf("migrateDataToWorkspace failed: %v", err)
	}

	// Verify user was migrated
	var migratedUser models.User
	err = db.Collection("users").FindOne(ctx, bson.M{"_id": user.ID}).Decode(&migratedUser)
	if err != nil {
		t.Fatalf("failed to find user: %v", err)
	}
	if migratedUser.WorkspaceID == nil || *migratedUser.WorkspaceID != workspaceID {
		t.Error("expected user to have workspace_id set")
	}

	// Verify org was migrated
	var migratedOrg models.Organization
	err = db.Collection("organizations").FindOne(ctx, bson.M{"_id": org.ID}).Decode(&migratedOrg)
	if err != nil {
		t.Fatalf("failed to find org: %v", err)
	}
	if migratedOrg.WorkspaceID != workspaceID {
		t.Error("expected org to have workspace_id set")
	}
}

func TestMigrateDataToWorkspace_SkipsExisting(t *testing.T) {
	db := testutil.SetupTestDB(t)
	ctx, cancel := testutil.TestContext()
	defer cancel()

	targetWorkspaceID := primitive.NewObjectID()
	existingWorkspaceID := primitive.NewObjectID()

	// Create document with existing workspace_id
	now := time.Now().UTC()
	org := models.Organization{
		ID:          primitive.NewObjectID(),
		WorkspaceID: existingWorkspaceID, // Already has workspace_id
		Name:        "Test Org",
		NameCI:      text.Fold("Test Org"),
		City:        "Test City",
		CityCI:      text.Fold("Test City"),
		State:       "TS",
		StateCI:     text.Fold("TS"),
		TimeZone:    "America/New_York",
		Status:      "active",
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	_, err := db.Collection("organizations").InsertOne(ctx, org)
	if err != nil {
		t.Fatalf("failed to create test org: %v", err)
	}

	deps := DBDeps{StrataHubMongoDatabase: db}

	// Run migration
	err = migrateDataToWorkspace(ctx, deps, targetWorkspaceID, testLogger())
	if err != nil {
		t.Fatalf("migrateDataToWorkspace failed: %v", err)
	}

	// Verify org was NOT migrated (kept existing workspace_id)
	var migratedOrg models.Organization
	err = db.Collection("organizations").FindOne(ctx, bson.M{"_id": org.ID}).Decode(&migratedOrg)
	if err != nil {
		t.Fatalf("failed to find org: %v", err)
	}
	if migratedOrg.WorkspaceID != existingWorkspaceID {
		t.Errorf("expected org to keep workspace_id %s, got %s",
			existingWorkspaceID.Hex(), migratedOrg.WorkspaceID.Hex())
	}
}
