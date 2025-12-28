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
		ID:    primitive.NewObjectID().Hex(),
		Name:  "Test Admin",
		Email: "admin@test.com",
		Role:  "admin",
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
		"email":       {"newleader@example.com"},
		"orgID":       {org.ID.Hex()},
		"auth_method": {"internal"},
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
		Email          string             `bson:"email"`
		Role           string             `bson:"role"`
		OrganizationID primitive.ObjectID `bson:"organization_id"`
	}
	err := db.Collection("users").FindOne(ctx, bson.M{"email": "newleader@example.com"}).Decode(&user)
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
				"email": {"test@example.com"},
				"orgID": {org.ID.Hex()},
			},
		},
		{
			name: "missing_email",
			form: url.Values{
				"full_name": {"Test Leader"},
				"orgID":     {org.ID.Hex()},
			},
		},
		{
			name: "missing_org",
			form: url.Values{
				"full_name": {"Test Leader"},
				"email":     {"test@example.com"},
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

func TestHandleCreate_DuplicateEmail(t *testing.T) {
	handler, fixtures := newTestHandler(t)
	ctx, cancel := testutil.TestContext()
	defer cancel()

	db := fixtures.DB()
	org := fixtures.CreateOrganization(ctx, "Test Org")
	fixtures.CreateLeader(ctx, "Existing Leader", "existing@example.com", org.ID)

	form := url.Values{
		"full_name": {"New Leader"},
		"email":     {"existing@example.com"},
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

func TestHandleCreate_InvalidEmail(t *testing.T) {
	handler, fixtures := newTestHandler(t)
	ctx, cancel := testutil.TestContext()
	defer cancel()

	db := fixtures.DB()
	org := fixtures.CreateOrganization(ctx, "Test Org")

	form := url.Values{
		"full_name": {"Test Leader"},
		"email":     {"not-an-email"},
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

	// No leader should be created
	count, _ := db.Collection("users").CountDocuments(ctx, bson.M{"role": "leader"})
	if count != 0 {
		t.Errorf("expected 0 leaders (invalid email), got %d", count)
	}
}

func TestHandleCreate_InvalidOrgID(t *testing.T) {
	handler, fixtures := newTestHandler(t)
	ctx, cancel := testutil.TestContext()
	defer cancel()

	db := fixtures.DB()

	form := url.Values{
		"full_name": {"Test Leader"},
		"email":     {"test@example.com"},
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
		"email":     {"test@example.com"},
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

	// Verify auth_method defaults to internal
	var user struct {
		AuthMethod string `bson:"auth_method"`
	}
	err := db.Collection("users").FindOne(ctx, bson.M{"email": "test@example.com"}).Decode(&user)
	if err != nil {
		t.Fatalf("FindOne failed: %v", err)
	}
	if user.AuthMethod != "internal" {
		t.Errorf("AuthMethod: got %q, want %q", user.AuthMethod, "internal")
	}
}
