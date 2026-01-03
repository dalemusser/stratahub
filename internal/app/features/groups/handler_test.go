package groups_test

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	uierrors "github.com/dalemusser/stratahub/internal/app/features/errors"
	"github.com/dalemusser/stratahub/internal/app/features/groups"
	"github.com/dalemusser/stratahub/internal/app/system/auth"
	"github.com/dalemusser/stratahub/internal/testutil"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.uber.org/zap"
)

func newTestHandler(t *testing.T) (*groups.Handler, *testutil.Fixtures) {
	t.Helper()
	db := testutil.SetupTestDB(t)
	logger := zap.NewNop()
	errLog := uierrors.NewErrorLogger(logger)
	handler := groups.NewHandler(db, errLog, logger)
	fixtures := testutil.NewFixtures(t, db)
	return handler, fixtures
}

func adminUser() *auth.SessionUser {
	return &auth.SessionUser{
		ID:      primitive.NewObjectID().Hex(),
		Name:    "Test Admin",
		LoginID: "admin@test.com",
		Role:    "admin",
	}
}

func leaderUser(orgID primitive.ObjectID, userID primitive.ObjectID) *auth.SessionUser {
	return &auth.SessionUser{
		ID:             userID.Hex(),
		Name:           "Test Leader",
		LoginID:        "leader@test.com",
		Role:           "leader",
		OrganizationID: orgID.Hex(),
	}
}

func TestHandleCreateGroup_Admin_Success(t *testing.T) {
	handler, fixtures := newTestHandler(t)
	ctx, cancel := testutil.TestContext()
	defer cancel()

	db := fixtures.DB()
	org := fixtures.CreateOrganization(ctx, "Test Org")

	form := url.Values{
		"name":        {"Test Group"},
		"description": {"A test group description"},
		"orgID":       {org.ID.Hex()},
	}

	req := httptest.NewRequest("POST", "/groups", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req = auth.WithTestUser(req, adminUser())

	rec := httptest.NewRecorder()
	handler.HandleCreateGroup(rec, req)

	if rec.Code != http.StatusSeeOther {
		t.Errorf("expected status %d, got %d", http.StatusSeeOther, rec.Code)
	}

	// Verify group was created
	count, err := db.Collection("groups").CountDocuments(ctx, bson.M{"name": "Test Group"})
	if err != nil {
		t.Fatalf("CountDocuments failed: %v", err)
	}
	if count != 1 {
		t.Errorf("expected 1 group, got %d", count)
	}
}

func TestHandleCreateGroup_Leader_Success(t *testing.T) {
	handler, fixtures := newTestHandler(t)
	ctx, cancel := testutil.TestContext()
	defer cancel()

	db := fixtures.DB()
	org := fixtures.CreateOrganization(ctx, "Test Org")
	leader := fixtures.CreateLeader(ctx, "Test Leader", "leader@test.com", org.ID)

	form := url.Values{
		"name":        {"Leader's Group"},
		"description": {"Created by leader"},
	}

	req := httptest.NewRequest("POST", "/groups", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req = auth.WithTestUser(req, leaderUser(org.ID, leader.ID))

	rec := httptest.NewRecorder()
	handler.HandleCreateGroup(rec, req)

	if rec.Code != http.StatusSeeOther {
		t.Errorf("expected status %d, got %d", http.StatusSeeOther, rec.Code)
	}

	// Verify group was created in the leader's org
	var group struct {
		Name           string             `bson:"name"`
		OrganizationID primitive.ObjectID `bson:"organization_id"`
	}
	err := db.Collection("groups").FindOne(ctx, bson.M{"name": "Leader's Group"}).Decode(&group)
	if err != nil {
		t.Fatalf("FindOne failed: %v", err)
	}
	if group.OrganizationID != org.ID {
		t.Errorf("expected org %v, got %v", org.ID, group.OrganizationID)
	}

	// Verify leader was auto-assigned to the group
	count, err := db.Collection("group_memberships").CountDocuments(ctx, bson.M{
		"user_id": leader.ID,
		"role":    "leader",
	})
	if err != nil {
		t.Fatalf("CountDocuments failed: %v", err)
	}
	if count != 1 {
		t.Errorf("expected leader to be assigned, got %d memberships", count)
	}
}

func TestHandleCreateGroup_MissingName(t *testing.T) {
	handler, fixtures := newTestHandler(t)
	ctx, cancel := testutil.TestContext()
	defer cancel()

	db := fixtures.DB()
	org := fixtures.CreateOrganization(ctx, "Test Org")

	form := url.Values{
		"description": {"Missing name"},
		"orgID":       {org.ID.Hex()},
	}

	req := httptest.NewRequest("POST", "/groups", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req = auth.WithTestUser(req, adminUser())

	rec := httptest.NewRecorder()
	func() {
		defer func() { recover() }()
		handler.HandleCreateGroup(rec, req)
	}()

	// Verify no group was created
	count, _ := db.Collection("groups").CountDocuments(ctx, bson.M{})
	if count != 0 {
		t.Errorf("expected 0 groups, got %d", count)
	}
}

func TestHandleCreateGroup_DuplicateNameSameOrg(t *testing.T) {
	handler, fixtures := newTestHandler(t)
	ctx, cancel := testutil.TestContext()
	defer cancel()

	db := fixtures.DB()
	org := fixtures.CreateOrganization(ctx, "Test Org")
	fixtures.CreateGroup(ctx, "Existing Group", org.ID)

	form := url.Values{
		"name":  {"Existing Group"},
		"orgID": {org.ID.Hex()},
	}

	req := httptest.NewRequest("POST", "/groups", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req = auth.WithTestUser(req, adminUser())

	rec := httptest.NewRecorder()
	func() {
		defer func() { recover() }()
		handler.HandleCreateGroup(rec, req)
	}()

	// Verify only the original group exists
	count, _ := db.Collection("groups").CountDocuments(ctx, bson.M{})
	if count != 1 {
		t.Errorf("expected 1 group (duplicate rejected), got %d", count)
	}
}

func TestHandleCreateGroup_SameNameDifferentOrg(t *testing.T) {
	handler, fixtures := newTestHandler(t)
	ctx, cancel := testutil.TestContext()
	defer cancel()

	db := fixtures.DB()
	org1 := fixtures.CreateOrganization(ctx, "Org One")
	org2 := fixtures.CreateOrganization(ctx, "Org Two")
	fixtures.CreateGroup(ctx, "Same Name", org1.ID)

	form := url.Values{
		"name":  {"Same Name"},
		"orgID": {org2.ID.Hex()},
	}

	req := httptest.NewRequest("POST", "/groups", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req = auth.WithTestUser(req, adminUser())

	rec := httptest.NewRecorder()
	handler.HandleCreateGroup(rec, req)

	if rec.Code != http.StatusSeeOther {
		t.Errorf("expected status %d (same name different org should succeed), got %d", http.StatusSeeOther, rec.Code)
	}

	// Verify both groups exist
	count, _ := db.Collection("groups").CountDocuments(ctx, bson.M{})
	if count != 2 {
		t.Errorf("expected 2 groups, got %d", count)
	}
}

func TestHandleCreateGroup_MemberRole_Forbidden(t *testing.T) {
	handler, fixtures := newTestHandler(t)
	ctx, cancel := testutil.TestContext()
	defer cancel()

	db := fixtures.DB()
	org := fixtures.CreateOrganization(ctx, "Test Org")

	memberUser := &auth.SessionUser{
		ID:             primitive.NewObjectID().Hex(),
		Name:           "Test Member",
		LoginID:        "member@test.com",
		Role:           "member",
		OrganizationID: org.ID.Hex(),
	}

	form := url.Values{
		"name":  {"Member Group"},
		"orgID": {org.ID.Hex()},
	}

	req := httptest.NewRequest("POST", "/groups", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req = auth.WithTestUser(req, memberUser)

	rec := httptest.NewRecorder()
	func() {
		defer func() { recover() }()
		handler.HandleCreateGroup(rec, req)
	}()

	// Members cannot create groups
	count, _ := db.Collection("groups").CountDocuments(ctx, bson.M{})
	if count != 0 {
		t.Errorf("expected 0 groups (member forbidden), got %d", count)
	}
}

func TestHandleEditGroup_Success(t *testing.T) {
	handler, fixtures := newTestHandler(t)
	ctx, cancel := testutil.TestContext()
	defer cancel()

	db := fixtures.DB()
	org := fixtures.CreateOrganization(ctx, "Test Org")
	group := fixtures.CreateGroup(ctx, "Original Name", org.ID)

	form := url.Values{
		"name":        {"Updated Name"},
		"description": {"Updated description"},
	}

	req := httptest.NewRequest("POST", "/groups/"+group.ID.Hex()+"/edit", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req = testutil.WithChiURLParam(req, "id", group.ID.Hex())
	req = auth.WithTestUser(req, adminUser())

	rec := httptest.NewRecorder()
	handler.HandleEditGroup(rec, req)

	if rec.Code != http.StatusSeeOther {
		t.Errorf("expected status %d, got %d", http.StatusSeeOther, rec.Code)
	}

	// Verify group was updated
	var updated struct {
		Name        string `bson:"name"`
		Description string `bson:"description"`
	}
	err := db.Collection("groups").FindOne(ctx, bson.M{"_id": group.ID}).Decode(&updated)
	if err != nil {
		t.Fatalf("FindOne failed: %v", err)
	}
	if updated.Name != "Updated Name" {
		t.Errorf("Name: got %q, want %q", updated.Name, "Updated Name")
	}
	if updated.Description != "Updated description" {
		t.Errorf("Description: got %q, want %q", updated.Description, "Updated description")
	}
}

func TestHandleEditGroup_DuplicateName(t *testing.T) {
	handler, fixtures := newTestHandler(t)
	ctx, cancel := testutil.TestContext()
	defer cancel()

	db := fixtures.DB()
	org := fixtures.CreateOrganization(ctx, "Test Org")
	fixtures.CreateGroup(ctx, "First Group", org.ID)
	group2 := fixtures.CreateGroup(ctx, "Second Group", org.ID)

	// Try to rename group2 to "First Group" (should fail)
	form := url.Values{
		"name": {"First Group"},
	}

	req := httptest.NewRequest("POST", "/groups/"+group2.ID.Hex()+"/edit", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req = testutil.WithChiURLParam(req, "id", group2.ID.Hex())
	req = auth.WithTestUser(req, adminUser())

	rec := httptest.NewRecorder()
	func() {
		defer func() { recover() }()
		handler.HandleEditGroup(rec, req)
	}()

	// Verify group2 still has original name
	var group struct {
		Name string `bson:"name"`
	}
	err := db.Collection("groups").FindOne(ctx, bson.M{"_id": group2.ID}).Decode(&group)
	if err != nil {
		t.Fatalf("FindOne failed: %v", err)
	}
	if group.Name != "Second Group" {
		t.Errorf("Name should be unchanged: got %q, want %q", group.Name, "Second Group")
	}
}

func TestHandleEditGroup_NotFound(t *testing.T) {
	handler, _ := newTestHandler(t)

	nonExistentID := "507f1f77bcf86cd799439011"
	form := url.Values{
		"name": {"Updated Name"},
	}

	req := httptest.NewRequest("POST", "/groups/"+nonExistentID+"/edit", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req = testutil.WithChiURLParam(req, "id", nonExistentID)
	req = auth.WithTestUser(req, adminUser())

	rec := httptest.NewRecorder()
	func() {
		defer func() { recover() }()
		handler.HandleEditGroup(rec, req)
	}()

	// Should not be a redirect (error case)
	if rec.Code == http.StatusSeeOther {
		t.Error("expected error response for non-existent group, got redirect")
	}
}

func TestHandleDeleteGroup_Success(t *testing.T) {
	handler, fixtures := newTestHandler(t)
	ctx, cancel := testutil.TestContext()
	defer cancel()

	db := fixtures.DB()
	org := fixtures.CreateOrganization(ctx, "Test Org")
	group := fixtures.CreateGroup(ctx, "To Be Deleted", org.ID)

	req := httptest.NewRequest("POST", "/groups/"+group.ID.Hex()+"/delete", nil)
	req = testutil.WithChiURLParam(req, "id", group.ID.Hex())
	req = auth.WithTestUser(req, adminUser())

	rec := httptest.NewRecorder()
	handler.HandleDeleteGroup(rec, req)

	if rec.Code != http.StatusSeeOther {
		t.Errorf("expected status %d, got %d", http.StatusSeeOther, rec.Code)
	}

	// Verify group was deleted
	count, err := db.Collection("groups").CountDocuments(ctx, bson.M{"_id": group.ID})
	if err != nil {
		t.Fatalf("CountDocuments failed: %v", err)
	}
	if count != 0 {
		t.Errorf("expected group to be deleted, but found %d", count)
	}
}

func TestHandleDeleteGroup_CascadeDeletesMemberships(t *testing.T) {
	handler, fixtures := newTestHandler(t)
	ctx, cancel := testutil.TestContext()
	defer cancel()

	db := fixtures.DB()
	org := fixtures.CreateOrganization(ctx, "Test Org")
	group := fixtures.CreateGroup(ctx, "Group With Members", org.ID)
	member := fixtures.CreateMember(ctx, "Test Member", "member@example.com", org.ID)
	fixtures.CreateGroupMembership(ctx, member.ID, group.ID, org.ID, "member")

	// Verify membership exists before delete
	membershipCount, _ := db.Collection("group_memberships").CountDocuments(ctx, bson.M{"group_id": group.ID})
	if membershipCount != 1 {
		t.Fatalf("expected 1 membership before delete, got %d", membershipCount)
	}

	req := httptest.NewRequest("POST", "/groups/"+group.ID.Hex()+"/delete", nil)
	req = testutil.WithChiURLParam(req, "id", group.ID.Hex())
	req = auth.WithTestUser(req, adminUser())

	rec := httptest.NewRecorder()
	handler.HandleDeleteGroup(rec, req)

	if rec.Code != http.StatusSeeOther {
		t.Errorf("expected status %d, got %d", http.StatusSeeOther, rec.Code)
	}

	// Verify memberships were cascade deleted
	membershipCount, _ = db.Collection("group_memberships").CountDocuments(ctx, bson.M{"group_id": group.ID})
	if membershipCount != 0 {
		t.Errorf("expected 0 memberships after cascade delete, got %d", membershipCount)
	}
}

func TestHandleDeleteGroup_NonAdmin_Forbidden(t *testing.T) {
	handler, fixtures := newTestHandler(t)
	ctx, cancel := testutil.TestContext()
	defer cancel()

	db := fixtures.DB()
	org := fixtures.CreateOrganization(ctx, "Test Org")
	group := fixtures.CreateGroup(ctx, "Cannot Delete", org.ID)
	leader := fixtures.CreateLeader(ctx, "Test Leader", "leader@test.com", org.ID)

	req := httptest.NewRequest("POST", "/groups/"+group.ID.Hex()+"/delete", nil)
	req = testutil.WithChiURLParam(req, "id", group.ID.Hex())
	req = auth.WithTestUser(req, leaderUser(org.ID, leader.ID))

	rec := httptest.NewRecorder()
	func() {
		defer func() { recover() }()
		handler.HandleDeleteGroup(rec, req)
	}()

	// Verify group was NOT deleted (only admins can delete)
	count, _ := db.Collection("groups").CountDocuments(ctx, bson.M{"_id": group.ID})
	if count != 1 {
		t.Errorf("expected group to still exist (leader cannot delete), got %d", count)
	}
}
