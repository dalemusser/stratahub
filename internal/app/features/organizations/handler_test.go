package organizations_test

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	uierrors "github.com/dalemusser/stratahub/internal/app/features/errors"
	"github.com/dalemusser/stratahub/internal/app/features/organizations"
	"github.com/dalemusser/stratahub/internal/app/system/auth"
	"github.com/dalemusser/stratahub/internal/testutil"
	"go.mongodb.org/mongo-driver/bson"
	"go.uber.org/zap"
)

func newTestHandler(t *testing.T) (*organizations.Handler, *testutil.Fixtures) {
	t.Helper()
	db := testutil.SetupTestDB(t)
	logger := zap.NewNop()
	errLog := uierrors.NewErrorLogger(logger)
	handler := organizations.NewHandler(db, errLog, logger)
	fixtures := testutil.NewFixtures(t, db)
	return handler, fixtures
}

func adminRequest(r *http.Request) *http.Request {
	user := &auth.SessionUser{
		ID:      "507f1f77bcf86cd799439011",
		Name:    "Test Admin",
		LoginID: "admin@test.com",
		Role:    "admin",
	}
	return auth.WithTestUser(r, user)
}

func TestHandleCreate_Success(t *testing.T) {
	handler, fixtures := newTestHandler(t)
	ctx, cancel := testutil.TestContext()
	defer cancel()

	// Get database reference from fixtures
	db := fixtures.DB()

	form := url.Values{
		"name":     {"Test Organization"},
		"city":     {"New York"},
		"state":    {"NY"},
		"contact":  {"test@example.com"},
		"timezone": {"America/New_York"},
	}

	req := httptest.NewRequest("POST", "/organizations", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req = adminRequest(req)

	rec := httptest.NewRecorder()
	handler.HandleCreate(rec, req)

	// Should redirect on success
	if rec.Code != http.StatusSeeOther {
		t.Errorf("expected status %d, got %d", http.StatusSeeOther, rec.Code)
	}

	// Verify organization was created in database
	count, err := db.Collection("organizations").CountDocuments(ctx, bson.M{"name": "Test Organization"})
	if err != nil {
		t.Fatalf("CountDocuments failed: %v", err)
	}
	if count != 1 {
		t.Errorf("expected 1 organization, got %d", count)
	}
}

func TestHandleCreate_MissingRequiredField(t *testing.T) {
	handler, fixtures := newTestHandler(t)
	ctx, cancel := testutil.TestContext()
	defer cancel()

	db := fixtures.DB()

	// Missing name (required field)
	form := url.Values{
		"city":     {"New York"},
		"state":    {"NY"},
		"timezone": {"America/New_York"},
	}

	req := httptest.NewRequest("POST", "/organizations", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req = adminRequest(req)

	rec := httptest.NewRecorder()

	// This will try to render a template on error, which may panic or fail
	// We use recover to catch any panics and check that no org was created
	func() {
		defer func() {
			if r := recover(); r != nil {
				// Template rendering failed, which is expected in tests
				t.Logf("recovered from panic (expected - template not initialized): %v", r)
			}
		}()
		handler.HandleCreate(rec, req)
	}()

	// Verify no organization was created
	count, err := db.Collection("organizations").CountDocuments(ctx, bson.M{})
	if err != nil {
		t.Fatalf("CountDocuments failed: %v", err)
	}
	if count != 0 {
		t.Errorf("expected 0 organizations (validation should fail), got %d", count)
	}
}

func TestHandleCreate_DuplicateName(t *testing.T) {
	handler, fixtures := newTestHandler(t)
	ctx, cancel := testutil.TestContext()
	defer cancel()

	db := fixtures.DB()

	// Create an organization first
	fixtures.CreateOrganization(ctx, "Existing Org")

	// Try to create another with the same name
	form := url.Values{
		"name":     {"Existing Org"},
		"city":     {"Boston"},
		"state":    {"MA"},
		"timezone": {"America/New_York"},
	}

	req := httptest.NewRequest("POST", "/organizations", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req = adminRequest(req)

	rec := httptest.NewRecorder()

	func() {
		defer func() {
			if r := recover(); r != nil {
				t.Logf("recovered from panic (expected - template not initialized): %v", r)
			}
		}()
		handler.HandleCreate(rec, req)
	}()

	// Verify only the original organization exists (no duplicate created)
	count, err := db.Collection("organizations").CountDocuments(ctx, bson.M{})
	if err != nil {
		t.Fatalf("CountDocuments failed: %v", err)
	}
	if count != 1 {
		t.Errorf("expected 1 organization (duplicate should be rejected), got %d", count)
	}
}

func TestHandleCreate_InvalidTimezone(t *testing.T) {
	handler, fixtures := newTestHandler(t)
	ctx, cancel := testutil.TestContext()
	defer cancel()

	db := fixtures.DB()

	form := url.Values{
		"name":     {"Test Org"},
		"city":     {"New York"},
		"state":    {"NY"},
		"timezone": {"Invalid/Timezone"},
	}

	req := httptest.NewRequest("POST", "/organizations", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req = adminRequest(req)

	rec := httptest.NewRecorder()

	func() {
		defer func() {
			if r := recover(); r != nil {
				t.Logf("recovered from panic (expected - template not initialized): %v", r)
			}
		}()
		handler.HandleCreate(rec, req)
	}()

	// Verify no organization was created
	count, err := db.Collection("organizations").CountDocuments(ctx, bson.M{})
	if err != nil {
		t.Fatalf("CountDocuments failed: %v", err)
	}
	if count != 0 {
		t.Errorf("expected 0 organizations (invalid timezone), got %d", count)
	}
}

func TestHandleCreate_CaseInsensitiveDuplicate(t *testing.T) {
	handler, fixtures := newTestHandler(t)
	ctx, cancel := testutil.TestContext()
	defer cancel()

	db := fixtures.DB()

	// Create an organization
	fixtures.CreateOrganization(ctx, "Test Organization")

	// Try to create with different case
	form := url.Values{
		"name":     {"TEST ORGANIZATION"},
		"city":     {"Boston"},
		"state":    {"MA"},
		"timezone": {"America/New_York"},
	}

	req := httptest.NewRequest("POST", "/organizations", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req = adminRequest(req)

	rec := httptest.NewRecorder()

	func() {
		defer func() {
			if r := recover(); r != nil {
				t.Logf("recovered from panic (expected - template not initialized): %v", r)
			}
		}()
		handler.HandleCreate(rec, req)
	}()

	// Verify only one organization exists (case-insensitive duplicate rejected)
	count, err := db.Collection("organizations").CountDocuments(ctx, bson.M{})
	if err != nil {
		t.Fatalf("CountDocuments failed: %v", err)
	}
	if count != 1 {
		t.Errorf("expected 1 organization (case-insensitive duplicate rejected), got %d", count)
	}
}

func TestHandleEdit_Success(t *testing.T) {
	handler, fixtures := newTestHandler(t)
	ctx, cancel := testutil.TestContext()
	defer cancel()

	db := fixtures.DB()

	// Create an organization first
	org := fixtures.CreateOrganization(ctx, "Original Name")

	// Edit the organization
	form := url.Values{
		"name":     {"Updated Name"},
		"city":     {"Boston"},
		"state":    {"MA"},
		"contact":  {"updated@example.com"},
		"timezone": {"America/Chicago"},
	}

	req := httptest.NewRequest("POST", "/organizations/"+org.ID.Hex()+"/edit", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req = testutil.WithChiURLParam(req, "id", org.ID.Hex())
	req = adminRequest(req)

	rec := httptest.NewRecorder()
	handler.HandleEdit(rec, req)

	// Should redirect on success
	if rec.Code != http.StatusSeeOther {
		t.Errorf("expected status %d, got %d", http.StatusSeeOther, rec.Code)
	}

	// Verify organization was updated
	var updated struct {
		Name     string `bson:"name"`
		City     string `bson:"city"`
		State    string `bson:"state"`
		TimeZone string `bson:"time_zone"`
	}
	err := db.Collection("organizations").FindOne(ctx, bson.M{"_id": org.ID}).Decode(&updated)
	if err != nil {
		t.Fatalf("FindOne failed: %v", err)
	}
	if updated.Name != "Updated Name" {
		t.Errorf("Name: got %q, want %q", updated.Name, "Updated Name")
	}
	if updated.City != "Boston" {
		t.Errorf("City: got %q, want %q", updated.City, "Boston")
	}
	if updated.TimeZone != "America/Chicago" {
		t.Errorf("TimeZone: got %q, want %q", updated.TimeZone, "America/Chicago")
	}
}

func TestHandleEdit_DuplicateName(t *testing.T) {
	handler, fixtures := newTestHandler(t)
	ctx, cancel := testutil.TestContext()
	defer cancel()

	db := fixtures.DB()

	// Create two organizations
	fixtures.CreateOrganization(ctx, "First Org")
	org2 := fixtures.CreateOrganization(ctx, "Second Org")

	// Try to rename org2 to "First Org" (should fail)
	form := url.Values{
		"name":     {"First Org"},
		"city":     {"Boston"},
		"state":    {"MA"},
		"timezone": {"America/New_York"},
	}

	req := httptest.NewRequest("POST", "/organizations/"+org2.ID.Hex()+"/edit", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req = testutil.WithChiURLParam(req, "id", org2.ID.Hex())
	req = adminRequest(req)

	rec := httptest.NewRecorder()

	func() {
		defer func() { recover() }()
		handler.HandleEdit(rec, req)
	}()

	// Verify org2 still has original name
	var org struct {
		Name string `bson:"name"`
	}
	err := db.Collection("organizations").FindOne(ctx, bson.M{"_id": org2.ID}).Decode(&org)
	if err != nil {
		t.Fatalf("FindOne failed: %v", err)
	}
	if org.Name != "Second Org" {
		t.Errorf("Name should be unchanged: got %q, want %q", org.Name, "Second Org")
	}
}

func TestHandleEdit_InvalidID(t *testing.T) {
	handler, _ := newTestHandler(t)

	form := url.Values{
		"name":     {"Updated Name"},
		"timezone": {"America/New_York"},
	}

	req := httptest.NewRequest("POST", "/organizations/invalid-id/edit", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req = testutil.WithChiURLParam(req, "id", "invalid-id")
	req = adminRequest(req)

	rec := httptest.NewRecorder()

	func() {
		defer func() { recover() }()
		handler.HandleEdit(rec, req)
	}()

	// Should not be a redirect (error case)
	if rec.Code == http.StatusSeeOther {
		t.Error("expected error response for invalid ID, got redirect")
	}
}

func TestHandleEdit_NotFound(t *testing.T) {
	handler, _ := newTestHandler(t)

	// Use a valid ObjectID format but non-existent
	nonExistentID := "507f1f77bcf86cd799439011"

	form := url.Values{
		"name":     {"Updated Name"},
		"timezone": {"America/New_York"},
	}

	req := httptest.NewRequest("POST", "/organizations/"+nonExistentID+"/edit", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req = testutil.WithChiURLParam(req, "id", nonExistentID)
	req = adminRequest(req)

	rec := httptest.NewRecorder()

	func() {
		defer func() { recover() }()
		handler.HandleEdit(rec, req)
	}()

	// Should not be a redirect (error case)
	if rec.Code == http.StatusSeeOther {
		t.Error("expected error response for non-existent org, got redirect")
	}
}

func TestHandleDelete_Success(t *testing.T) {
	handler, fixtures := newTestHandler(t)
	ctx, cancel := testutil.TestContext()
	defer cancel()

	db := fixtures.DB()

	// Create an organization
	org := fixtures.CreateOrganization(ctx, "To Be Deleted")

	req := httptest.NewRequest("POST", "/organizations/"+org.ID.Hex()+"/delete", nil)
	req = testutil.WithChiURLParam(req, "id", org.ID.Hex())
	req = adminRequest(req)

	rec := httptest.NewRecorder()
	handler.HandleDelete(rec, req)

	// Should redirect on success
	if rec.Code != http.StatusSeeOther {
		t.Errorf("expected status %d, got %d", http.StatusSeeOther, rec.Code)
	}

	// Verify organization was deleted
	count, err := db.Collection("organizations").CountDocuments(ctx, bson.M{"_id": org.ID})
	if err != nil {
		t.Fatalf("CountDocuments failed: %v", err)
	}
	if count != 0 {
		t.Errorf("expected organization to be deleted, but found %d", count)
	}
}

func TestHandleDelete_CascadeDeletesRelatedData(t *testing.T) {
	handler, fixtures := newTestHandler(t)
	ctx, cancel := testutil.TestContext()
	defer cancel()

	db := fixtures.DB()

	// Create org with related data
	org := fixtures.CreateOrganization(ctx, "Org With Data")
	fixtures.CreateLeader(ctx, "Test Leader", "leader@example.com", org.ID)
	fixtures.CreateMember(ctx, "Test Member", "member@example.com", org.ID)
	group := fixtures.CreateGroup(ctx, "Test Group", org.ID)
	_ = group // group created for cascade test

	// Verify data exists before delete
	userCount, _ := db.Collection("users").CountDocuments(ctx, bson.M{"organization_id": org.ID})
	if userCount != 2 {
		t.Fatalf("expected 2 users before delete, got %d", userCount)
	}
	groupCount, _ := db.Collection("groups").CountDocuments(ctx, bson.M{"organization_id": org.ID})
	if groupCount != 1 {
		t.Fatalf("expected 1 group before delete, got %d", groupCount)
	}

	// Delete the organization
	req := httptest.NewRequest("POST", "/organizations/"+org.ID.Hex()+"/delete", nil)
	req = testutil.WithChiURLParam(req, "id", org.ID.Hex())
	req = adminRequest(req)

	rec := httptest.NewRecorder()
	handler.HandleDelete(rec, req)

	if rec.Code != http.StatusSeeOther {
		t.Errorf("expected status %d, got %d", http.StatusSeeOther, rec.Code)
	}

	// Verify all related data was deleted
	userCount, _ = db.Collection("users").CountDocuments(ctx, bson.M{"organization_id": org.ID})
	if userCount != 0 {
		t.Errorf("expected 0 users after cascade delete, got %d", userCount)
	}
	groupCount, _ = db.Collection("groups").CountDocuments(ctx, bson.M{"organization_id": org.ID})
	if groupCount != 0 {
		t.Errorf("expected 0 groups after cascade delete, got %d", groupCount)
	}
	orgCount, _ := db.Collection("organizations").CountDocuments(ctx, bson.M{"_id": org.ID})
	if orgCount != 0 {
		t.Errorf("expected 0 orgs after delete, got %d", orgCount)
	}
}

func TestHandleDelete_InvalidID(t *testing.T) {
	handler, _ := newTestHandler(t)

	req := httptest.NewRequest("POST", "/organizations/invalid-id/delete", nil)
	req = testutil.WithChiURLParam(req, "id", "invalid-id")
	req = adminRequest(req)

	rec := httptest.NewRecorder()

	func() {
		defer func() { recover() }()
		handler.HandleDelete(rec, req)
	}()

	// Should not be a redirect (error case)
	if rec.Code == http.StatusSeeOther {
		t.Error("expected error response for invalid ID, got redirect")
	}
}
