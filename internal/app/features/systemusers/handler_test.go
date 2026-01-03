package systemusers_test

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	uierrors "github.com/dalemusser/stratahub/internal/app/features/errors"
	"github.com/dalemusser/stratahub/internal/app/features/systemusers"
	"github.com/dalemusser/stratahub/internal/app/system/auth"
	"github.com/dalemusser/stratahub/internal/testutil"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.uber.org/zap"
)

func newTestHandler(t *testing.T) (*systemusers.Handler, *testutil.Fixtures) {
	t.Helper()
	db := testutil.SetupTestDB(t)
	logger := zap.NewNop()
	errLog := uierrors.NewErrorLogger(logger)
	handler := systemusers.NewHandler(db, errLog, logger)
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

func TestHandleCreate_Admin_Success(t *testing.T) {
	handler, fixtures := newTestHandler(t)
	ctx, cancel := testutil.TestContext()
	defer cancel()

	db := fixtures.DB()

	form := url.Values{
		"full_name":   {"New Admin"},
		"email":       {"newadmin@example.com"},
		"role":        {"admin"},
		"auth_method": {"email"}, // email method uses email as login_id
	}

	req := httptest.NewRequest("POST", "/system-users", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req = auth.WithTestUser(req, adminUser())

	rec := httptest.NewRecorder()
	handler.HandleCreate(rec, req)

	if rec.Code != http.StatusSeeOther {
		t.Errorf("expected status %d, got %d", http.StatusSeeOther, rec.Code)
	}

	// Verify admin was created
	var user struct {
		FullName   string `bson:"full_name"`
		LoginID    string `bson:"login_id"`
		Role       string `bson:"role"`
		Status     string `bson:"status"`
		AuthMethod string `bson:"auth_method"`
	}
	err := db.Collection("users").FindOne(ctx, bson.M{"login_id": "newadmin@example.com"}).Decode(&user)
	if err != nil {
		t.Fatalf("FindOne failed: %v", err)
	}
	if user.FullName != "New Admin" {
		t.Errorf("FullName: got %q, want %q", user.FullName, "New Admin")
	}
	if user.Role != "admin" {
		t.Errorf("Role: got %q, want %q", user.Role, "admin")
	}
	if user.Status != "active" {
		t.Errorf("Status: got %q, want %q", user.Status, "active")
	}
}

func TestHandleCreate_Analyst_Success(t *testing.T) {
	handler, fixtures := newTestHandler(t)
	ctx, cancel := testutil.TestContext()
	defer cancel()

	db := fixtures.DB()

	form := url.Values{
		"full_name":   {"New Analyst"},
		"email":       {"analyst@example.com"},
		"role":        {"analyst"},
		"auth_method": {"google"},
	}

	req := httptest.NewRequest("POST", "/system-users", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req = auth.WithTestUser(req, adminUser())

	rec := httptest.NewRecorder()
	handler.HandleCreate(rec, req)

	if rec.Code != http.StatusSeeOther {
		t.Errorf("expected status %d, got %d", http.StatusSeeOther, rec.Code)
	}

	// Verify analyst was created
	var user struct {
		Role       string `bson:"role"`
		AuthMethod string `bson:"auth_method"`
	}
	err := db.Collection("users").FindOne(ctx, bson.M{"login_id": "analyst@example.com"}).Decode(&user)
	if err != nil {
		t.Fatalf("FindOne failed: %v", err)
	}
	if user.Role != "analyst" {
		t.Errorf("Role: got %q, want %q", user.Role, "analyst")
	}
	if user.AuthMethod != "google" {
		t.Errorf("AuthMethod: got %q, want %q", user.AuthMethod, "google")
	}
}

func TestHandleCreate_InvalidRole(t *testing.T) {
	handler, fixtures := newTestHandler(t)
	ctx, cancel := testutil.TestContext()
	defer cancel()

	db := fixtures.DB()

	// Try to create with member or leader role (only admin/analyst/coordinator allowed)
	form := url.Values{
		"full_name":   {"Invalid User"},
		"email":       {"invalid@example.com"},
		"role":        {"member"},
		"auth_method": {"email"},
	}

	req := httptest.NewRequest("POST", "/system-users", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req = auth.WithTestUser(req, adminUser())

	rec := httptest.NewRecorder()
	func() {
		defer func() { recover() }()
		handler.HandleCreate(rec, req)
	}()

	// No user should be created
	count, _ := db.Collection("users").CountDocuments(ctx, bson.M{})
	if count != 0 {
		t.Errorf("expected 0 users (invalid role), got %d", count)
	}
}

func TestHandleCreate_MissingRequiredFields(t *testing.T) {
	handler, fixtures := newTestHandler(t)
	ctx, cancel := testutil.TestContext()
	defer cancel()

	db := fixtures.DB()

	tests := []struct {
		name string
		form url.Values
	}{
		{
			name: "missing_full_name",
			form: url.Values{
				"email":       {"test@example.com"},
				"role":        {"admin"},
				"auth_method": {"email"},
			},
		},
		{
			name: "missing_email",
			form: url.Values{
				"full_name":   {"Test User"},
				"role":        {"admin"},
				"auth_method": {"email"},
			},
		},
		{
			name: "missing_role",
			form: url.Values{
				"full_name":   {"Test User"},
				"email":       {"test@example.com"},
				"auth_method": {"email"},
			},
		},
		{
			name: "missing_auth_method",
			form: url.Values{
				"full_name": {"Test User"},
				"email":     {"test@example.com"},
				"role":      {"admin"},
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest("POST", "/system-users", strings.NewReader(tc.form.Encode()))
			req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
			req = auth.WithTestUser(req, adminUser())

			rec := httptest.NewRecorder()
			func() {
				defer func() { recover() }()
				handler.HandleCreate(rec, req)
			}()

			// No user should be created
			count, _ := db.Collection("users").CountDocuments(ctx, bson.M{})
			if count != 0 {
				t.Errorf("expected 0 users, got %d", count)
			}
		})
	}
}

func TestHandleCreate_DuplicateEmail(t *testing.T) {
	handler, fixtures := newTestHandler(t)
	ctx, cancel := testutil.TestContext()
	defer cancel()

	db := fixtures.DB()
	fixtures.CreateAdmin(ctx, "Existing Admin", "existing@example.com")

	form := url.Values{
		"full_name":   {"New Admin"},
		"email":       {"existing@example.com"},
		"role":        {"admin"},
		"auth_method": {"email"},
	}

	req := httptest.NewRequest("POST", "/system-users", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req = auth.WithTestUser(req, adminUser())

	rec := httptest.NewRecorder()
	func() {
		defer func() { recover() }()
		handler.HandleCreate(rec, req)
	}()

	// Should still have only 1 admin
	count, _ := db.Collection("users").CountDocuments(ctx, bson.M{"role": "admin"})
	if count != 1 {
		t.Errorf("expected 1 admin (duplicate rejected), got %d", count)
	}
}

func TestHandleCreate_InvalidEmail(t *testing.T) {
	handler, fixtures := newTestHandler(t)
	ctx, cancel := testutil.TestContext()
	defer cancel()

	db := fixtures.DB()

	form := url.Values{
		"full_name":   {"Test User"},
		"email":       {"not-an-email"},
		"role":        {"admin"},
		"auth_method": {"email"},
	}

	req := httptest.NewRequest("POST", "/system-users", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req = auth.WithTestUser(req, adminUser())

	rec := httptest.NewRecorder()
	func() {
		defer func() { recover() }()
		handler.HandleCreate(rec, req)
	}()

	// No user should be created
	count, _ := db.Collection("users").CountDocuments(ctx, bson.M{})
	if count != 0 {
		t.Errorf("expected 0 users (invalid email), got %d", count)
	}
}

func TestHandleCreate_Unauthenticated(t *testing.T) {
	handler, fixtures := newTestHandler(t)
	ctx, cancel := testutil.TestContext()
	defer cancel()

	db := fixtures.DB()

	form := url.Values{
		"full_name":   {"Test User"},
		"email":       {"test@example.com"},
		"role":        {"admin"},
		"auth_method": {"email"},
	}

	req := httptest.NewRequest("POST", "/system-users", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	// Not authenticated

	rec := httptest.NewRecorder()
	func() {
		defer func() { recover() }()
		handler.HandleCreate(rec, req)
	}()

	// No user should be created
	count, _ := db.Collection("users").CountDocuments(ctx, bson.M{})
	if count != 0 {
		t.Errorf("expected 0 users (unauthenticated), got %d", count)
	}
}
