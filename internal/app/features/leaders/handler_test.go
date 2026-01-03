package leaders_test

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	uierrors "github.com/dalemusser/stratahub/internal/app/features/errors"
	"github.com/dalemusser/stratahub/internal/app/features/leaders"
	"github.com/dalemusser/stratahub/internal/app/system/auth"
	"github.com/dalemusser/stratahub/internal/testutil"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.uber.org/zap"
)

func newTestHandler(t *testing.T) (*leaders.Handler, *testutil.Fixtures) {
	t.Helper()
	db := testutil.SetupTestDB(t)
	logger := zap.NewNop()
	errLog := uierrors.NewErrorLogger(logger)
	handler := leaders.NewHandler(db, errLog, logger)
	fixtures := testutil.NewFixtures(t, db)
	return handler, fixtures
}

func adminRequest(r *http.Request) *http.Request {
	user := &auth.SessionUser{
		ID:      primitive.NewObjectID().Hex(),
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

	db := fixtures.DB()
	org := fixtures.CreateOrganization(ctx, "Test Org")

	form := url.Values{
		"full_name":   {"New Leader"},
		"login_id":    {"newleader@example.com"},
		"orgID":       {org.ID.Hex()},
		"auth_method": {"trust"},
	}

	req := httptest.NewRequest("POST", "/leaders", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req = adminRequest(req)

	rec := httptest.NewRecorder()
	handler.HandleCreate(rec, req)

	if rec.Code != http.StatusSeeOther {
		t.Errorf("expected status %d, got %d", http.StatusSeeOther, rec.Code)
	}

	// Verify leader was created
	var user struct {
		FullName       string             `bson:"full_name"`
		LoginID        string             `bson:"login_id"`
		Role           string             `bson:"role"`
		OrganizationID primitive.ObjectID `bson:"organization_id"`
	}
	err := db.Collection("users").FindOne(ctx, bson.M{"login_id": "newleader@example.com"}).Decode(&user)
	if err != nil {
		t.Fatalf("FindOne failed: %v", err)
	}
	if user.FullName != "New Leader" {
		t.Errorf("FullName: got %q, want %q", user.FullName, "New Leader")
	}
	if user.Role != "leader" {
		t.Errorf("Role: got %q, want %q", user.Role, "leader")
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
				"login_id": {"test@example.com"},
				"orgID":    {org.ID.Hex()},
			},
		},
		{
			name: "missing_login_id",
			form: url.Values{
				"full_name": {"Test Leader"},
				"orgID":     {org.ID.Hex()},
			},
		},
		{
			name: "missing_org",
			form: url.Values{
				"full_name": {"Test Leader"},
				"login_id":  {"test@example.com"},
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest("POST", "/leaders", strings.NewReader(tc.form.Encode()))
			req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
			req = adminRequest(req)

			rec := httptest.NewRecorder()
			func() {
				defer func() { recover() }()
				handler.HandleCreate(rec, req)
			}()

			// No new leader should be created
			count, _ := db.Collection("users").CountDocuments(ctx, bson.M{"role": "leader"})
			if count != 0 {
				t.Errorf("expected 0 leaders, got %d", count)
			}
		})
	}
}

func TestHandleCreate_DuplicateLoginID(t *testing.T) {
	handler, fixtures := newTestHandler(t)
	ctx, cancel := testutil.TestContext()
	defer cancel()

	db := fixtures.DB()
	org := fixtures.CreateOrganization(ctx, "Test Org")
	fixtures.CreateLeader(ctx, "Existing Leader", "existing@example.com", org.ID)

	form := url.Values{
		"full_name": {"New Leader"},
		"login_id":  {"existing@example.com"},
		"orgID":     {org.ID.Hex()},
	}

	req := httptest.NewRequest("POST", "/leaders", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req = adminRequest(req)

	rec := httptest.NewRecorder()
	func() {
		defer func() { recover() }()
		handler.HandleCreate(rec, req)
	}()

	// Should still have only 1 leader
	count, _ := db.Collection("users").CountDocuments(ctx, bson.M{"role": "leader"})
	if count != 1 {
		t.Errorf("expected 1 leader (duplicate rejected), got %d", count)
	}
}

func TestHandleCreate_LoginIDAccepted(t *testing.T) {
	handler, fixtures := newTestHandler(t)
	ctx, cancel := testutil.TestContext()
	defer cancel()

	db := fixtures.DB()
	org := fixtures.CreateOrganization(ctx, "Test Org")

	// Login ID can be any string (not required to be email)
	form := url.Values{
		"full_name": {"Test Leader"},
		"login_id":  {"username123"},
		"orgID":     {org.ID.Hex()},
	}

	req := httptest.NewRequest("POST", "/leaders", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req = adminRequest(req)

	rec := httptest.NewRecorder()
	handler.HandleCreate(rec, req)

	// Leader should be created
	if rec.Code != http.StatusSeeOther {
		t.Errorf("expected status %d, got %d", http.StatusSeeOther, rec.Code)
	}
	count, _ := db.Collection("users").CountDocuments(ctx, bson.M{"role": "leader"})
	if count != 1 {
		t.Errorf("expected 1 leader, got %d", count)
	}
}

func TestHandleCreate_InvalidOrgID(t *testing.T) {
	handler, fixtures := newTestHandler(t)
	ctx, cancel := testutil.TestContext()
	defer cancel()

	db := fixtures.DB()

	form := url.Values{
		"full_name": {"Test Leader"},
		"login_id":  {"test@example.com"},
		"orgID":     {"not-a-valid-objectid"},
	}

	req := httptest.NewRequest("POST", "/leaders", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req = adminRequest(req)

	rec := httptest.NewRecorder()
	func() {
		defer func() { recover() }()
		handler.HandleCreate(rec, req)
	}()

	// No leader should be created
	count, _ := db.Collection("users").CountDocuments(ctx, bson.M{"role": "leader"})
	if count != 0 {
		t.Errorf("expected 0 leaders (invalid org ID), got %d", count)
	}
}

func TestHandleCreate_DefaultAuthMethod(t *testing.T) {
	handler, fixtures := newTestHandler(t)
	ctx, cancel := testutil.TestContext()
	defer cancel()

	db := fixtures.DB()
	org := fixtures.CreateOrganization(ctx, "Test Org")

	form := url.Values{
		"full_name": {"Test Leader"},
		"login_id":  {"test@example.com"},
		"orgID":     {org.ID.Hex()},
		// auth_method not specified
	}

	req := httptest.NewRequest("POST", "/leaders", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req = adminRequest(req)

	rec := httptest.NewRecorder()
	handler.HandleCreate(rec, req)

	if rec.Code != http.StatusSeeOther {
		t.Errorf("expected status %d, got %d", http.StatusSeeOther, rec.Code)
	}

	// Verify auth_method defaults to trust
	var user struct {
		AuthMethod string `bson:"auth_method"`
	}
	err := db.Collection("users").FindOne(ctx, bson.M{"login_id": "test@example.com"}).Decode(&user)
	if err != nil {
		t.Fatalf("FindOne failed: %v", err)
	}
	if user.AuthMethod != "trust" {
		t.Errorf("AuthMethod: got %q, want %q", user.AuthMethod, "trust")
	}
}

func TestHandleCreate_EmailAuthMethod_UsesEmailAsLoginID(t *testing.T) {
	handler, fixtures := newTestHandler(t)
	ctx, cancel := testutil.TestContext()
	defer cancel()

	db := fixtures.DB()
	org := fixtures.CreateOrganization(ctx, "Test Org")

	// For email auth method, email field is used for both login_id and email
	form := url.Values{
		"full_name":   {"Test Leader"},
		"email":       {"test@example.com"},
		"orgID":       {org.ID.Hex()},
		"auth_method": {"email"},
	}

	req := httptest.NewRequest("POST", "/leaders", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req = adminRequest(req)

	rec := httptest.NewRecorder()
	handler.HandleCreate(rec, req)

	if rec.Code != http.StatusSeeOther {
		t.Errorf("expected status %d, got %d", http.StatusSeeOther, rec.Code)
	}

	var user struct {
		LoginID    string `bson:"login_id"`
		Email      string `bson:"email"`
		AuthMethod string `bson:"auth_method"`
	}
	err := db.Collection("users").FindOne(ctx, bson.M{"login_id": "test@example.com"}).Decode(&user)
	if err != nil {
		t.Fatalf("FindOne failed: %v", err)
	}
	if user.LoginID != "test@example.com" {
		t.Errorf("LoginID: got %q, want %q", user.LoginID, "test@example.com")
	}
	if user.Email != "test@example.com" {
		t.Errorf("Email: got %q, want %q", user.Email, "test@example.com")
	}
	if user.AuthMethod != "email" {
		t.Errorf("AuthMethod: got %q, want %q", user.AuthMethod, "email")
	}
}

func TestHandleCreate_EmailAuthMethod_RequiresEmail(t *testing.T) {
	handler, fixtures := newTestHandler(t)
	ctx, cancel := testutil.TestContext()
	defer cancel()

	db := fixtures.DB()
	org := fixtures.CreateOrganization(ctx, "Test Org")

	// Missing email for email auth method
	form := url.Values{
		"full_name":   {"Test Leader"},
		"orgID":       {org.ID.Hex()},
		"auth_method": {"email"},
	}

	req := httptest.NewRequest("POST", "/leaders", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req = adminRequest(req)

	rec := httptest.NewRecorder()
	func() {
		defer func() { recover() }()
		handler.HandleCreate(rec, req)
	}()

	// No leader should be created
	count, _ := db.Collection("users").CountDocuments(ctx, bson.M{"role": "leader"})
	if count != 0 {
		t.Errorf("expected 0 leaders (missing email), got %d", count)
	}
}

func TestHandleCreate_CleverAuthMethod_RequiresAuthReturnID(t *testing.T) {
	handler, fixtures := newTestHandler(t)
	ctx, cancel := testutil.TestContext()
	defer cancel()

	db := fixtures.DB()
	org := fixtures.CreateOrganization(ctx, "Test Org")

	// Missing auth_return_id for clever auth method
	form := url.Values{
		"full_name":   {"Test Leader"},
		"login_id":    {"cleveruser123"},
		"orgID":       {org.ID.Hex()},
		"auth_method": {"clever"},
	}

	req := httptest.NewRequest("POST", "/leaders", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req = adminRequest(req)

	rec := httptest.NewRecorder()
	func() {
		defer func() { recover() }()
		handler.HandleCreate(rec, req)
	}()

	// No leader should be created
	count, _ := db.Collection("users").CountDocuments(ctx, bson.M{"role": "leader"})
	if count != 0 {
		t.Errorf("expected 0 leaders (missing auth_return_id), got %d", count)
	}
}

func TestHandleCreate_CleverAuthMethod_Success(t *testing.T) {
	handler, fixtures := newTestHandler(t)
	ctx, cancel := testutil.TestContext()
	defer cancel()

	db := fixtures.DB()
	org := fixtures.CreateOrganization(ctx, "Test Org")

	form := url.Values{
		"full_name":      {"Test Leader"},
		"login_id":       {"cleveruser123"},
		"auth_return_id": {"clever-abc-123"},
		"orgID":          {org.ID.Hex()},
		"auth_method":    {"clever"},
	}

	req := httptest.NewRequest("POST", "/leaders", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req = adminRequest(req)

	rec := httptest.NewRecorder()
	handler.HandleCreate(rec, req)

	if rec.Code != http.StatusSeeOther {
		t.Errorf("expected status %d, got %d", http.StatusSeeOther, rec.Code)
	}

	var user struct {
		LoginID      string `bson:"login_id"`
		AuthReturnID string `bson:"auth_return_id"`
		AuthMethod   string `bson:"auth_method"`
	}
	err := db.Collection("users").FindOne(ctx, bson.M{"login_id": "cleveruser123"}).Decode(&user)
	if err != nil {
		t.Fatalf("FindOne failed: %v", err)
	}
	if user.AuthReturnID != "clever-abc-123" {
		t.Errorf("AuthReturnID: got %q, want %q", user.AuthReturnID, "clever-abc-123")
	}
	if user.AuthMethod != "clever" {
		t.Errorf("AuthMethod: got %q, want %q", user.AuthMethod, "clever")
	}
}
