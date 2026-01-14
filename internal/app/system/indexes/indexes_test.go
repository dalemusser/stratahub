package indexes_test

import (
	"testing"

	"github.com/dalemusser/stratahub/internal/app/system/indexes"
	"github.com/dalemusser/stratahub/internal/testutil"
	"go.mongodb.org/mongo-driver/bson"
)

func TestEnsureAll(t *testing.T) {
	db := testutil.SetupTestDB(t)
	ctx, cancel := testutil.TestContext()
	defer cancel()

	// EnsureAll should succeed on a clean database
	err := indexes.EnsureAll(ctx, db)
	if err != nil {
		t.Fatalf("EnsureAll failed: %v", err)
	}
}

func TestEnsureAll_Idempotent(t *testing.T) {
	db := testutil.SetupTestDB(t)
	ctx, cancel := testutil.TestContext()
	defer cancel()

	// First call
	err := indexes.EnsureAll(ctx, db)
	if err != nil {
		t.Fatalf("First EnsureAll failed: %v", err)
	}

	// Second call should also succeed (idempotent)
	err = indexes.EnsureAll(ctx, db)
	if err != nil {
		t.Fatalf("Second EnsureAll failed: %v", err)
	}
}

func TestEnsureAll_CreatesUserIndexes(t *testing.T) {
	db := testutil.SetupTestDB(t)
	ctx, cancel := testutil.TestContext()
	defer cancel()

	err := indexes.EnsureAll(ctx, db)
	if err != nil {
		t.Fatalf("EnsureAll failed: %v", err)
	}

	// Verify user indexes exist
	cur, err := db.Collection("users").Indexes().List(ctx)
	if err != nil {
		t.Fatalf("List indexes failed: %v", err)
	}
	defer cur.Close(ctx)

	indexNames := make(map[string]bool)
	for cur.Next(ctx) {
		var idx bson.M
		if err := cur.Decode(&idx); err != nil {
			continue
		}
		if name, ok := idx["name"].(string); ok {
			indexNames[name] = true
		}
	}

	// Check key indexes exist
	expectedIndexes := []string{
		"uniq_users_workspace_login_auth",
		"idx_users_login_auth",
		"idx_users_role_org_status_fullnameci_id",
		"idx_users_role_status_fullnameci_id",
		"idx_users_role_org_status_loginidci_id",
		"idx_users_org",
		"idx_users_role_org",
		"idx_users_workspace_role_status",
	}

	for _, name := range expectedIndexes {
		if !indexNames[name] {
			t.Errorf("expected index %q to exist on users collection", name)
		}
	}
}

func TestEnsureAll_CreatesOrganizationIndexes(t *testing.T) {
	db := testutil.SetupTestDB(t)
	ctx, cancel := testutil.TestContext()
	defer cancel()

	err := indexes.EnsureAll(ctx, db)
	if err != nil {
		t.Fatalf("EnsureAll failed: %v", err)
	}

	cur, err := db.Collection("organizations").Indexes().List(ctx)
	if err != nil {
		t.Fatalf("List indexes failed: %v", err)
	}
	defer cur.Close(ctx)

	indexNames := make(map[string]bool)
	for cur.Next(ctx) {
		var idx bson.M
		if err := cur.Decode(&idx); err != nil {
			continue
		}
		if name, ok := idx["name"].(string); ok {
			indexNames[name] = true
		}
	}

	expectedIndexes := []string{
		"uniq_orgs_workspace_nameci",
		"idx_orgs_workspace_status_nameci__id",
		"idx_orgs_nameci__id",
		"idx_orgs_status_nameci__id",
		"idx_orgs_cityci",
		"idx_orgs_stateci",
	}

	for _, name := range expectedIndexes {
		if !indexNames[name] {
			t.Errorf("expected index %q to exist on organizations collection", name)
		}
	}
}

func TestEnsureAll_CreatesGroupIndexes(t *testing.T) {
	db := testutil.SetupTestDB(t)
	ctx, cancel := testutil.TestContext()
	defer cancel()

	err := indexes.EnsureAll(ctx, db)
	if err != nil {
		t.Fatalf("EnsureAll failed: %v", err)
	}

	cur, err := db.Collection("groups").Indexes().List(ctx)
	if err != nil {
		t.Fatalf("List indexes failed: %v", err)
	}
	defer cur.Close(ctx)

	indexNames := make(map[string]bool)
	for cur.Next(ctx) {
		var idx bson.M
		if err := cur.Decode(&idx); err != nil {
			continue
		}
		if name, ok := idx["name"].(string); ok {
			indexNames[name] = true
		}
	}

	expectedIndexes := []string{
		"uniq_group_org_nameci",
		"idx_groups_org",
		"idx_groups_org_status_nameci__id",
	}

	for _, name := range expectedIndexes {
		if !indexNames[name] {
			t.Errorf("expected index %q to exist on groups collection", name)
		}
	}
}

func TestEnsureAll_CreatesGroupMembershipIndexes(t *testing.T) {
	db := testutil.SetupTestDB(t)
	ctx, cancel := testutil.TestContext()
	defer cancel()

	err := indexes.EnsureAll(ctx, db)
	if err != nil {
		t.Fatalf("EnsureAll failed: %v", err)
	}

	cur, err := db.Collection("group_memberships").Indexes().List(ctx)
	if err != nil {
		t.Fatalf("List indexes failed: %v", err)
	}
	defer cur.Close(ctx)

	indexNames := make(map[string]bool)
	for cur.Next(ctx) {
		var idx bson.M
		if err := cur.Decode(&idx); err != nil {
			continue
		}
		if name, ok := idx["name"].(string); ok {
			indexNames[name] = true
		}
	}

	expectedIndexes := []string{
		"uniq_gm_user_group",
		"idx_gm_group_role_user",
		"idx_gm_user_role_group",
		"idx_gm_org_role_group",
	}

	for _, name := range expectedIndexes {
		if !indexNames[name] {
			t.Errorf("expected index %q to exist on group_memberships collection", name)
		}
	}
}

func TestEnsureAll_CreatesResourceIndexes(t *testing.T) {
	db := testutil.SetupTestDB(t)
	ctx, cancel := testutil.TestContext()
	defer cancel()

	err := indexes.EnsureAll(ctx, db)
	if err != nil {
		t.Fatalf("EnsureAll failed: %v", err)
	}

	cur, err := db.Collection("resources").Indexes().List(ctx)
	if err != nil {
		t.Fatalf("List indexes failed: %v", err)
	}
	defer cur.Close(ctx)

	indexNames := make(map[string]bool)
	for cur.Next(ctx) {
		var idx bson.M
		if err := cur.Decode(&idx); err != nil {
			continue
		}
		if name, ok := idx["name"].(string); ok {
			indexNames[name] = true
		}
	}

	expectedIndexes := []string{
		"uniq_resources_titleci",
		"idx_resources_status_titleci__id",
		"idx_resources_subjectci",
		"idx_resources_type",
	}

	for _, name := range expectedIndexes {
		if !indexNames[name] {
			t.Errorf("expected index %q to exist on resources collection", name)
		}
	}
}

func TestEnsureAll_CreatesMaterialIndexes(t *testing.T) {
	db := testutil.SetupTestDB(t)
	ctx, cancel := testutil.TestContext()
	defer cancel()

	err := indexes.EnsureAll(ctx, db)
	if err != nil {
		t.Fatalf("EnsureAll failed: %v", err)
	}

	cur, err := db.Collection("materials").Indexes().List(ctx)
	if err != nil {
		t.Fatalf("List indexes failed: %v", err)
	}
	defer cur.Close(ctx)

	indexNames := make(map[string]bool)
	for cur.Next(ctx) {
		var idx bson.M
		if err := cur.Decode(&idx); err != nil {
			continue
		}
		if name, ok := idx["name"].(string); ok {
			indexNames[name] = true
		}
	}

	expectedIndexes := []string{
		"uniq_materials_titleci",
		"idx_materials_status_titleci__id",
		"idx_materials_type",
	}

	for _, name := range expectedIndexes {
		if !indexNames[name] {
			t.Errorf("expected index %q to exist on materials collection", name)
		}
	}
}

func TestEnsureAll_CreatesMaterialAssignmentIndexes(t *testing.T) {
	db := testutil.SetupTestDB(t)
	ctx, cancel := testutil.TestContext()
	defer cancel()

	err := indexes.EnsureAll(ctx, db)
	if err != nil {
		t.Fatalf("EnsureAll failed: %v", err)
	}

	cur, err := db.Collection("material_assignments").Indexes().List(ctx)
	if err != nil {
		t.Fatalf("List indexes failed: %v", err)
	}
	defer cur.Close(ctx)

	indexNames := make(map[string]bool)
	for cur.Next(ctx) {
		var idx bson.M
		if err := cur.Decode(&idx); err != nil {
			continue
		}
		if name, ok := idx["name"].(string); ok {
			indexNames[name] = true
		}
	}

	expectedIndexes := []string{
		"idx_matassign_org",
		"idx_matassign_leader",
		"idx_matassign_material",
		"idx_matassign_org_leader",
	}

	for _, name := range expectedIndexes {
		if !indexNames[name] {
			t.Errorf("expected index %q to exist on material_assignments collection", name)
		}
	}
}

func TestEnsureAll_CreatesCoordinatorAssignmentIndexes(t *testing.T) {
	db := testutil.SetupTestDB(t)
	ctx, cancel := testutil.TestContext()
	defer cancel()

	err := indexes.EnsureAll(ctx, db)
	if err != nil {
		t.Fatalf("EnsureAll failed: %v", err)
	}

	cur, err := db.Collection("coordinator_assignments").Indexes().List(ctx)
	if err != nil {
		t.Fatalf("List indexes failed: %v", err)
	}
	defer cur.Close(ctx)

	indexNames := make(map[string]bool)
	for cur.Next(ctx) {
		var idx bson.M
		if err := cur.Decode(&idx); err != nil {
			continue
		}
		if name, ok := idx["name"].(string); ok {
			indexNames[name] = true
		}
	}

	expectedIndexes := []string{
		"uniq_coordassign_user_org",
		"idx_coordassign_user",
		"idx_coordassign_org",
	}

	for _, name := range expectedIndexes {
		if !indexNames[name] {
			t.Errorf("expected index %q to exist on coordinator_assignments collection", name)
		}
	}
}

func TestEnsureAll_CreatesPageIndexes(t *testing.T) {
	db := testutil.SetupTestDB(t)
	ctx, cancel := testutil.TestContext()
	defer cancel()

	err := indexes.EnsureAll(ctx, db)
	if err != nil {
		t.Fatalf("EnsureAll failed: %v", err)
	}

	cur, err := db.Collection("pages").Indexes().List(ctx)
	if err != nil {
		t.Fatalf("List indexes failed: %v", err)
	}
	defer cur.Close(ctx)

	indexNames := make(map[string]bool)
	for cur.Next(ctx) {
		var idx bson.M
		if err := cur.Decode(&idx); err != nil {
			continue
		}
		if name, ok := idx["name"].(string); ok {
			indexNames[name] = true
		}
	}

	if !indexNames["uniq_pages_slug"] {
		t.Error("expected index uniq_pages_slug to exist on pages collection")
	}
}

func TestEnsureAll_CreatesWorkspaceIndexes(t *testing.T) {
	db := testutil.SetupTestDB(t)
	ctx, cancel := testutil.TestContext()
	defer cancel()

	err := indexes.EnsureAll(ctx, db)
	if err != nil {
		t.Fatalf("EnsureAll failed: %v", err)
	}

	cur, err := db.Collection("workspaces").Indexes().List(ctx)
	if err != nil {
		t.Fatalf("List indexes failed: %v", err)
	}
	defer cur.Close(ctx)

	indexNames := make(map[string]bool)
	for cur.Next(ctx) {
		var idx bson.M
		if err := cur.Decode(&idx); err != nil {
			continue
		}
		if name, ok := idx["name"].(string); ok {
			indexNames[name] = true
		}
	}

	expectedIndexes := []string{
		"uniq_workspaces_subdomain",
		"uniq_workspaces_nameci",
		"idx_workspaces_status_nameci__id",
	}

	for _, name := range expectedIndexes {
		if !indexNames[name] {
			t.Errorf("expected index %q to exist on workspaces collection", name)
		}
	}
}

func TestEnsureAll_CreatesEmailVerificationIndexes(t *testing.T) {
	db := testutil.SetupTestDB(t)
	ctx, cancel := testutil.TestContext()
	defer cancel()

	err := indexes.EnsureAll(ctx, db)
	if err != nil {
		t.Fatalf("EnsureAll failed: %v", err)
	}

	cur, err := db.Collection("email_verifications").Indexes().List(ctx)
	if err != nil {
		t.Fatalf("List indexes failed: %v", err)
	}
	defer cur.Close(ctx)

	indexNames := make(map[string]bool)
	for cur.Next(ctx) {
		var idx bson.M
		if err := cur.Decode(&idx); err != nil {
			continue
		}
		if name, ok := idx["name"].(string); ok {
			indexNames[name] = true
		}
	}

	expectedIndexes := []string{
		"idx_emailverify_expires_ttl",
		"idx_emailverify_token",
		"idx_emailverify_user",
	}

	for _, name := range expectedIndexes {
		if !indexNames[name] {
			t.Errorf("expected index %q to exist on email_verifications collection", name)
		}
	}
}

func TestEnsureAll_CreatesOAuthStateIndexes(t *testing.T) {
	db := testutil.SetupTestDB(t)
	ctx, cancel := testutil.TestContext()
	defer cancel()

	err := indexes.EnsureAll(ctx, db)
	if err != nil {
		t.Fatalf("EnsureAll failed: %v", err)
	}

	cur, err := db.Collection("oauth_states").Indexes().List(ctx)
	if err != nil {
		t.Fatalf("List indexes failed: %v", err)
	}
	defer cur.Close(ctx)

	indexNames := make(map[string]bool)
	for cur.Next(ctx) {
		var idx bson.M
		if err := cur.Decode(&idx); err != nil {
			continue
		}
		if name, ok := idx["name"].(string); ok {
			indexNames[name] = true
		}
	}

	expectedIndexes := []string{
		"uniq_oauth_state",
		"idx_oauth_expires_ttl",
	}

	for _, name := range expectedIndexes {
		if !indexNames[name] {
			t.Errorf("expected index %q to exist on oauth_states collection", name)
		}
	}
}

func TestEnsureAll_CreatesSiteSettingsIndexes(t *testing.T) {
	db := testutil.SetupTestDB(t)
	ctx, cancel := testutil.TestContext()
	defer cancel()

	err := indexes.EnsureAll(ctx, db)
	if err != nil {
		t.Fatalf("EnsureAll failed: %v", err)
	}

	cur, err := db.Collection("site_settings").Indexes().List(ctx)
	if err != nil {
		t.Fatalf("List indexes failed: %v", err)
	}
	defer cur.Close(ctx)

	indexNames := make(map[string]bool)
	for cur.Next(ctx) {
		var idx bson.M
		if err := cur.Decode(&idx); err != nil {
			continue
		}
		if name, ok := idx["name"].(string); ok {
			indexNames[name] = true
		}
	}

	if !indexNames["uniq_sitesettings_workspace"] {
		t.Error("expected index uniq_sitesettings_workspace to exist on site_settings collection")
	}
}

func TestEnsureAll_CreatesLoginRecordIndexes(t *testing.T) {
	db := testutil.SetupTestDB(t)
	ctx, cancel := testutil.TestContext()
	defer cancel()

	err := indexes.EnsureAll(ctx, db)
	if err != nil {
		t.Fatalf("EnsureAll failed: %v", err)
	}

	cur, err := db.Collection("login_records").Indexes().List(ctx)
	if err != nil {
		t.Fatalf("List indexes failed: %v", err)
	}
	defer cur.Close(ctx)

	indexNames := make(map[string]bool)
	for cur.Next(ctx) {
		var idx bson.M
		if err := cur.Decode(&idx); err != nil {
			continue
		}
		if name, ok := idx["name"].(string); ok {
			indexNames[name] = true
		}
	}

	expectedIndexes := []string{
		"idx_logins_user_created",
		"idx_logins_created",
	}

	for _, name := range expectedIndexes {
		if !indexNames[name] {
			t.Errorf("expected index %q to exist on login_records collection", name)
		}
	}
}

func TestEnsureAll_CreatesGroupResourceAssignmentIndexes(t *testing.T) {
	db := testutil.SetupTestDB(t)
	ctx, cancel := testutil.TestContext()
	defer cancel()

	err := indexes.EnsureAll(ctx, db)
	if err != nil {
		t.Fatalf("EnsureAll failed: %v", err)
	}

	cur, err := db.Collection("group_resource_assignments").Indexes().List(ctx)
	if err != nil {
		t.Fatalf("List indexes failed: %v", err)
	}
	defer cur.Close(ctx)

	indexNames := make(map[string]bool)
	for cur.Next(ctx) {
		var idx bson.M
		if err := cur.Decode(&idx); err != nil {
			continue
		}
		if name, ok := idx["name"].(string); ok {
			indexNames[name] = true
		}
	}

	expectedIndexes := []string{
		"idx_assign_group_resource",
		"idx_assign_group",
		"idx_assign_resource",
		"idx_assign_group_created",
		"idx_assign_resource_created",
	}

	for _, name := range expectedIndexes {
		if !indexNames[name] {
			t.Errorf("expected index %q to exist on group_resource_assignments collection", name)
		}
	}
}

func TestEnsureAll_UniqueIndexEnforced(t *testing.T) {
	db := testutil.SetupTestDB(t)
	ctx, cancel := testutil.TestContext()
	defer cancel()

	// Run EnsureAll to create indexes
	err := indexes.EnsureAll(ctx, db)
	if err != nil {
		t.Fatalf("EnsureAll failed: %v", err)
	}

	// Insert a page with slug "about"
	_, err = db.Collection("pages").InsertOne(ctx, bson.M{"slug": "about", "title": "About"})
	if err != nil {
		t.Fatalf("Insert page failed: %v", err)
	}

	// Try to insert another page with the same slug - should fail
	_, err = db.Collection("pages").InsertOne(ctx, bson.M{"slug": "about", "title": "Different About"})
	if err == nil {
		t.Error("expected duplicate key error for unique index on pages.slug")
	}
}
