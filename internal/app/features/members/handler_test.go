package members_test

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	uierrors "github.com/dalemusser/stratahub/internal/app/features/errors"
	"github.com/dalemusser/stratahub/internal/app/features/members"
	"github.com/dalemusser/stratahub/internal/app/system/auth"
	"github.com/dalemusser/stratahub/internal/testutil"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.uber.org/zap"
)

func newTestHandler(t *testing.T) (*members.Handler, *testutil.Fixtures) {
	t.Helper()
	db := testutil.SetupTestDB(t)
	logger := zap.NewNop()
	errLog := uierrors.NewErrorLogger(logger)
	handler := members.NewHandler(db, errLog, logger)
	fixtures := testutil.NewFixtures(t, db)
	return handler, fixtures
}

func adminUser() *auth.SessionUser {
	return &auth.SessionUser{
		ID:    primitive.NewObjectID().Hex(),
		Name:  "Test Admin",
		Email: "admin@test.com",
		Role:  "admin",
	}
}

func leaderUser(orgID primitive.ObjectID, userID primitive.ObjectID) *auth.SessionUser {
	return &auth.SessionUser{
		ID:             userID.Hex(),
		Name:           "Test Leader",
		Email:          "leader@test.com",
		Role:           "leader",
		OrganizationID: orgID.Hex(),
	}
}

func TestHandleCreate_Admin_Success(t *testing.T) {
	handler, fixtures := newTestHandler(t)
	ctx, cancel := testutil.TestContext()
	defer cancel()

	db := fixtures.DB()
	org := fixtures.CreateOrganization(ctx, "Test Org")

	form := url.Values{
		"full_name":   {"New Member"},
		"email":       {"newmember@example.com"},
		"orgID":       {org.ID.Hex()},
		"auth_method": {"internal"},
	}

	req := httptest.NewRequest("POST", "/members", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req = auth.WithTestUser(req, adminUser())

	rec := httptest.NewRecorder()
	handler.HandleCreate(rec, req)

	if rec.Code != http.StatusSeeOther {
		t.Errorf("expected status %d, got %d", http.StatusSeeOther, rec.Code)
	}

	// Verify member was created
	var user struct {
		FullName       string             `bson:"full_name"`
		Email          string             `bson:"email"`
		Role           string             `bson:"role"`
		Status         string             `bson:"status"`
		OrganizationID primitive.ObjectID `bson:"organization_id"`
	}
	err := db.Collection("users").FindOne(ctx, bson.M{"email": "newmember@example.com"}).Decode(&user)
	if err != nil {
		t.Fatalf("FindOne failed: %v", err)
	}
	if user.FullName != "New Member" {
		t.Errorf("FullName: got %q, want %q", user.FullName, "New Member")
	}
	if user.Role != "member" {
		t.Errorf("Role: got %q, want %q", user.Role, "member")
	}
	if user.Status != "active" {
		t.Errorf("Status: got %q, want %q", user.Status, "active")
	}
	if user.OrganizationID != org.ID {
		t.Errorf("OrganizationID: got %v, want %v", user.OrganizationID, org.ID)
	}
}

func TestHandleCreate_Leader_Success(t *testing.T) {
	handler, fixtures := newTestHandler(t)
	ctx, cancel := testutil.TestContext()
	defer cancel()

	db := fixtures.DB()
	org := fixtures.CreateOrganization(ctx, "Test Org")
	leader := fixtures.CreateLeader(ctx, "Test Leader", "leader@test.com", org.ID)

	form := url.Values{
		"full_name": {"Leader's Member"},
		"email":     {"leadermember@example.com"},
	}

	req := httptest.NewRequest("POST", "/members", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req = auth.WithTestUser(req, leaderUser(org.ID, leader.ID))

	rec := httptest.NewRecorder()
	handler.HandleCreate(rec, req)

	if rec.Code != http.StatusSeeOther {
		t.Errorf("expected status %d, got %d", http.StatusSeeOther, rec.Code)
	}

	// Verify member was created in the leader's org
	var user struct {
		OrganizationID primitive.ObjectID `bson:"organization_id"`
	}
	err := db.Collection("users").FindOne(ctx, bson.M{"email": "leadermember@example.com"}).Decode(&user)
	if err != nil {
		t.Fatalf("FindOne failed: %v", err)
	}
	if user.OrganizationID != org.ID {
		t.Errorf("OrganizationID: got %v, want %v", user.OrganizationID, org.ID)
	}
}

func TestHandleCreate_MissingRequiredFields(t *testing.T) {
	handler, fixtures := newTestHandler(t)
	ctx, cancel := testutil.TestContext()
	defer cancel()

	db := fixtures.DB()
	org := fixtures.CreateOrganization(ctx, "Test Org")

	tests := []struct {
		name string
		form url.Values
	}{
		{
			name: "missing_full_name",
			form: url.Values{
				"email": {"test@example.com"},
				"orgID": {org.ID.Hex()},
			},
		},
		{
			name: "missing_email",
			form: url.Values{
				"full_name": {"Test Member"},
				"orgID":     {org.ID.Hex()},
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest("POST", "/members", strings.NewReader(tc.form.Encode()))
			req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
			req = auth.WithTestUser(req, adminUser())

			rec := httptest.NewRecorder()
			func() {
				defer func() { recover() }()
				handler.HandleCreate(rec, req)
			}()

			// No new member should be created
			count, _ := db.Collection("users").CountDocuments(ctx, bson.M{"role": "member"})
			if count != 0 {
				t.Errorf("expected 0 members, got %d", count)
			}
		})
	}
}

func TestHandleCreate_Admin_MissingOrg(t *testing.T) {
	handler, fixtures := newTestHandler(t)
	ctx, cancel := testutil.TestContext()
	defer cancel()

	db := fixtures.DB()

	form := url.Values{
		"full_name": {"Test Member"},
		"email":     {"test@example.com"},
		// orgID not provided
	}

	req := httptest.NewRequest("POST", "/members", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req = auth.WithTestUser(req, adminUser())

	rec := httptest.NewRecorder()
	func() {
		defer func() { recover() }()
		handler.HandleCreate(rec, req)
	}()

	// No member should be created
	count, _ := db.Collection("users").CountDocuments(ctx, bson.M{"role": "member"})
	if count != 0 {
		t.Errorf("expected 0 members (org required for admin), got %d", count)
	}
}

func TestHandleCreate_DuplicateEmail(t *testing.T) {
	handler, fixtures := newTestHandler(t)
	ctx, cancel := testutil.TestContext()
	defer cancel()

	db := fixtures.DB()
	org := fixtures.CreateOrganization(ctx, "Test Org")

	// Use unique email based on org ID to avoid collisions across test runs
	uniqueEmail := "existing_" + org.ID.Hex()[:8] + "@example.com"
	fixtures.CreateMember(ctx, "Existing Member", uniqueEmail, org.ID)

	form := url.Values{
		"full_name": {"New Member"},
		"email":     {uniqueEmail},
		"orgID":     {org.ID.Hex()},
	}

	req := httptest.NewRequest("POST", "/members", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req = auth.WithTestUser(req, adminUser())

	rec := httptest.NewRecorder()
	func() {
		defer func() { recover() }()
		handler.HandleCreate(rec, req)
	}()

	// Should still have only 1 user with this email (duplicate rejected)
	count, _ := db.Collection("users").CountDocuments(ctx, bson.M{"email": uniqueEmail})
	if count != 1 {
		t.Errorf("expected 1 user with email %s (duplicate rejected), got %d", uniqueEmail, count)
	}
}

func TestHandleCreate_InvalidEmail(t *testing.T) {
	handler, fixtures := newTestHandler(t)
	ctx, cancel := testutil.TestContext()
	defer cancel()

	db := fixtures.DB()
	org := fixtures.CreateOrganization(ctx, "Test Org")

	form := url.Values{
		"full_name": {"Test Member"},
		"email":     {"not-an-email"},
		"orgID":     {org.ID.Hex()},
	}

	req := httptest.NewRequest("POST", "/members", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req = auth.WithTestUser(req, adminUser())

	rec := httptest.NewRecorder()
	func() {
		defer func() { recover() }()
		handler.HandleCreate(rec, req)
	}()

	// No member should be created
	count, _ := db.Collection("users").CountDocuments(ctx, bson.M{"role": "member"})
	if count != 0 {
		t.Errorf("expected 0 members (invalid email), got %d", count)
	}
}

func TestHandleEdit_Success(t *testing.T) {
	handler, fixtures := newTestHandler(t)
	ctx, cancel := testutil.TestContext()
	defer cancel()

	db := fixtures.DB()
	org := fixtures.CreateOrganization(ctx, "Test Org")
	member := fixtures.CreateMember(ctx, "Original Name", "original@example.com", org.ID)

	form := url.Values{
		"full_name":   {"Updated Name"},
		"email":       {"updated@example.com"},
		"auth_method": {"google"},
		"status":      {"active"},
		"orgID":       {org.ID.Hex()},
	}

	req := httptest.NewRequest("POST", "/members/"+member.ID.Hex()+"/edit", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req = testutil.WithChiURLParam(req, "id", member.ID.Hex())
	req = auth.WithTestUser(req, adminUser())

	rec := httptest.NewRecorder()
	handler.HandleEdit(rec, req)

	if rec.Code != http.StatusSeeOther {
		t.Errorf("expected status %d, got %d", http.StatusSeeOther, rec.Code)
	}

	// Verify member was updated
	var updated struct {
		FullName   string `bson:"full_name"`
		Email      string `bson:"email"`
		AuthMethod string `bson:"auth_method"`
	}
	err := db.Collection("users").FindOne(ctx, bson.M{"_id": member.ID}).Decode(&updated)
	if err != nil {
		t.Fatalf("FindOne failed: %v", err)
	}
	if updated.FullName != "Updated Name" {
		t.Errorf("FullName: got %q, want %q", updated.FullName, "Updated Name")
	}
	if updated.Email != "updated@example.com" {
		t.Errorf("Email: got %q, want %q", updated.Email, "updated@example.com")
	}
	if updated.AuthMethod != "google" {
		t.Errorf("AuthMethod: got %q, want %q", updated.AuthMethod, "google")
	}
}

func TestHandleEdit_DuplicateEmail(t *testing.T) {
	handler, fixtures := newTestHandler(t)
	ctx, cancel := testutil.TestContext()
	defer cancel()

	db := fixtures.DB()
	org := fixtures.CreateOrganization(ctx, "Test Org")
	fixtures.CreateMember(ctx, "First Member", "first@example.com", org.ID)
	member2 := fixtures.CreateMember(ctx, "Second Member", "second@example.com", org.ID)

	// Try to change member2's email to first@example.com (should fail)
	form := url.Values{
		"full_name":   {"Second Member"},
		"email":       {"first@example.com"},
		"auth_method": {"internal"},
		"status":      {"active"},
		"orgID":       {org.ID.Hex()},
	}

	req := httptest.NewRequest("POST", "/members/"+member2.ID.Hex()+"/edit", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req = testutil.WithChiURLParam(req, "id", member2.ID.Hex())
	req = auth.WithTestUser(req, adminUser())

	rec := httptest.NewRecorder()
	func() {
		defer func() { recover() }()
		handler.HandleEdit(rec, req)
	}()

	// Verify member2 still has original email
	var member struct {
		Email string `bson:"email"`
	}
	err := db.Collection("users").FindOne(ctx, bson.M{"_id": member2.ID}).Decode(&member)
	if err != nil {
		t.Fatalf("FindOne failed: %v", err)
	}
	if member.Email != "second@example.com" {
		t.Errorf("Email should be unchanged: got %q, want %q", member.Email, "second@example.com")
	}
}

func TestHandleEdit_NotFound(t *testing.T) {
	handler, _ := newTestHandler(t)

	nonExistentID := "507f1f77bcf86cd799439011"
	form := url.Values{
		"full_name": {"Updated Name"},
		"email":     {"updated@example.com"},
	}

	req := httptest.NewRequest("POST", "/members/"+nonExistentID+"/edit", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req = testutil.WithChiURLParam(req, "id", nonExistentID)
	req = auth.WithTestUser(req, adminUser())

	rec := httptest.NewRecorder()
	func() {
		defer func() { recover() }()
		handler.HandleEdit(rec, req)
	}()

	// Should not be a redirect (error case)
	if rec.Code == http.StatusSeeOther {
		t.Error("expected error response for non-existent member, got redirect")
	}
}

func TestHandleDelete_Success(t *testing.T) {
	handler, fixtures := newTestHandler(t)
	ctx, cancel := testutil.TestContext()
	defer cancel()

	db := fixtures.DB()
	org := fixtures.CreateOrganization(ctx, "Test Org")
	member := fixtures.CreateMember(ctx, "To Be Deleted", "delete@example.com", org.ID)

	req := httptest.NewRequest("POST", "/members/"+member.ID.Hex()+"/delete", nil)
	req = testutil.WithChiURLParam(req, "id", member.ID.Hex())
	req = auth.WithTestUser(req, adminUser())

	rec := httptest.NewRecorder()
	handler.HandleDelete(rec, req)

	if rec.Code != http.StatusSeeOther {
		t.Errorf("expected status %d, got %d", http.StatusSeeOther, rec.Code)
	}

	// Verify member was deleted
	count, err := db.Collection("users").CountDocuments(ctx, bson.M{"_id": member.ID})
	if err != nil {
		t.Fatalf("CountDocuments failed: %v", err)
	}
	if count != 0 {
		t.Errorf("expected member to be deleted, but found %d", count)
	}
}

func TestHandleDelete_CascadeDeletesMemberships(t *testing.T) {
	handler, fixtures := newTestHandler(t)
	ctx, cancel := testutil.TestContext()
	defer cancel()

	db := fixtures.DB()
	org := fixtures.CreateOrganization(ctx, "Test Org")
	member := fixtures.CreateMember(ctx, "Member With Groups", "grouped@example.com", org.ID)
	group := fixtures.CreateGroup(ctx, "Test Group", org.ID)
	fixtures.CreateGroupMembership(ctx, member.ID, group.ID, org.ID, "member")

	// Verify membership exists before delete
	membershipCount, _ := db.Collection("group_memberships").CountDocuments(ctx, bson.M{"user_id": member.ID})
	if membershipCount != 1 {
		t.Fatalf("expected 1 membership before delete, got %d", membershipCount)
	}

	req := httptest.NewRequest("POST", "/members/"+member.ID.Hex()+"/delete", nil)
	req = testutil.WithChiURLParam(req, "id", member.ID.Hex())
	req = auth.WithTestUser(req, adminUser())

	rec := httptest.NewRecorder()
	handler.HandleDelete(rec, req)

	if rec.Code != http.StatusSeeOther {
		t.Errorf("expected status %d, got %d", http.StatusSeeOther, rec.Code)
	}

	// Verify memberships were cascade deleted
	membershipCount, _ = db.Collection("group_memberships").CountDocuments(ctx, bson.M{"user_id": member.ID})
	if membershipCount != 0 {
		t.Errorf("expected 0 memberships after cascade delete, got %d", membershipCount)
	}
}
