package resources_test

// Terminology: User Identifiers
//   - UserID / userID / user_id: The MongoDB ObjectID (_id) that uniquely identifies a user record
//   - LoginID / loginID / login_id: The human-readable string users type to log in

import (
	"bytes"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"

	uierrors "github.com/dalemusser/stratahub/internal/app/features/errors"
	"github.com/dalemusser/stratahub/internal/app/features/resources"
	"github.com/dalemusser/stratahub/internal/app/system/auth"
	"github.com/dalemusser/stratahub/internal/testutil"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.uber.org/zap"
)

// createMultipartRequest creates a multipart form request with the given form values.
func createMultipartRequest(t *testing.T, urlPath string, formValues map[string]string) (*http.Request, string) {
	t.Helper()
	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)
	for key, val := range formValues {
		if err := writer.WriteField(key, val); err != nil {
			t.Fatalf("failed to write field %s: %v", key, err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("failed to close multipart writer: %v", err)
	}
	req := httptest.NewRequest("POST", urlPath, &buf)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	return req, writer.FormDataContentType()
}

func newTestAdminHandler(t *testing.T) (*resources.AdminHandler, *testutil.Fixtures) {
	t.Helper()
	db := testutil.SetupTestDB(t)
	logger := zap.NewNop()
	errLog := uierrors.NewErrorLogger(logger)
	// Pass nil for storage and audit logger since tests don't use file uploads or audit
	handler := resources.NewAdminHandler(db, nil, errLog, nil, logger)
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

func TestHandleCreate_Success(t *testing.T) {
	handler, fixtures := newTestAdminHandler(t)
	ctx, cancel := testutil.TestContext()
	defer cancel()

	db := fixtures.DB()

	formValues := map[string]string{
		"title":      "Test Resource",
		"launch_url": "https://example.com/resource",
		"type":       "game",
		"status":     "active",
	}

	req, _ := createMultipartRequest(t, "/resources", formValues)
	req = auth.WithTestUser(req, adminUser())

	rec := httptest.NewRecorder()
	handler.HandleCreate(rec, req)

	if rec.Code != http.StatusSeeOther {
		t.Errorf("expected status %d, got %d", http.StatusSeeOther, rec.Code)
	}

	// Verify resource was created
	var resource struct {
		Title     string `bson:"title"`
		LaunchURL string `bson:"launch_url"`
		Type      string `bson:"type"`
		Status    string `bson:"status"`
	}
	err := db.Collection("resources").FindOne(ctx, bson.M{"title": "Test Resource"}).Decode(&resource)
	if err != nil {
		t.Fatalf("FindOne failed: %v", err)
	}
	if resource.Title != "Test Resource" {
		t.Errorf("Title: got %q, want %q", resource.Title, "Test Resource")
	}
	if resource.LaunchURL != "https://example.com/resource" {
		t.Errorf("LaunchURL: got %q, want %q", resource.LaunchURL, "https://example.com/resource")
	}
	if resource.Type != "game" {
		t.Errorf("Type: got %q, want %q", resource.Type, "game")
	}
	if resource.Status != "active" {
		t.Errorf("Status: got %q, want %q", resource.Status, "active")
	}
}

func TestHandleCreate_WithAllFields(t *testing.T) {
	handler, fixtures := newTestAdminHandler(t)
	ctx, cancel := testutil.TestContext()
	defer cancel()

	db := fixtures.DB()

	formValues := map[string]string{
		"title":                "Complete Resource",
		"subject":              "Mathematics",
		"description":          "A complete test resource",
		"launch_url":           "https://example.com/complete",
		"type":                 "video",
		"status":               "active",
		"show_in_library":      "on",
		"default_instructions": "Complete all exercises",
	}

	req, _ := createMultipartRequest(t, "/resources", formValues)
	req = auth.WithTestUser(req, adminUser())

	rec := httptest.NewRecorder()
	handler.HandleCreate(rec, req)

	if rec.Code != http.StatusSeeOther {
		t.Errorf("expected status %d, got %d", http.StatusSeeOther, rec.Code)
	}

	// Verify all fields
	var resource struct {
		Subject             string `bson:"subject"`
		Description         string `bson:"description"`
		ShowInLibrary       bool   `bson:"show_in_library"`
		DefaultInstructions string `bson:"default_instructions"`
	}
	err := db.Collection("resources").FindOne(ctx, bson.M{"title": "Complete Resource"}).Decode(&resource)
	if err != nil {
		t.Fatalf("FindOne failed: %v", err)
	}
	if resource.Subject != "Mathematics" {
		t.Errorf("Subject: got %q, want %q", resource.Subject, "Mathematics")
	}
	if resource.Description != "A complete test resource" {
		t.Errorf("Description: got %q, want %q", resource.Description, "A complete test resource")
	}
	if !resource.ShowInLibrary {
		t.Error("ShowInLibrary: expected true")
	}
	if resource.DefaultInstructions != "Complete all exercises" {
		t.Errorf("DefaultInstructions: got %q, want %q", resource.DefaultInstructions, "Complete all exercises")
	}
}

func TestHandleCreate_MissingTitle(t *testing.T) {
	handler, fixtures := newTestAdminHandler(t)
	ctx, cancel := testutil.TestContext()
	defer cancel()

	db := fixtures.DB()

	formValues := map[string]string{
		"launch_url": "https://example.com/resource",
		"status":     "active",
	}

	req, _ := createMultipartRequest(t, "/resources", formValues)
	req = auth.WithTestUser(req, adminUser())

	rec := httptest.NewRecorder()
	func() {
		defer func() { recover() }()
		handler.HandleCreate(rec, req)
	}()

	// No resource should be created
	count, _ := db.Collection("resources").CountDocuments(ctx, bson.M{})
	if count != 0 {
		t.Errorf("expected 0 resources (missing title), got %d", count)
	}
}

func TestHandleCreate_MissingLaunchURL(t *testing.T) {
	handler, fixtures := newTestAdminHandler(t)
	ctx, cancel := testutil.TestContext()
	defer cancel()

	db := fixtures.DB()

	formValues := map[string]string{
		"title":  "Test Resource",
		"status": "active",
	}

	req, _ := createMultipartRequest(t, "/resources", formValues)
	req = auth.WithTestUser(req, adminUser())

	rec := httptest.NewRecorder()
	func() {
		defer func() { recover() }()
		handler.HandleCreate(rec, req)
	}()

	// No resource should be created
	count, _ := db.Collection("resources").CountDocuments(ctx, bson.M{})
	if count != 0 {
		t.Errorf("expected 0 resources (missing launch_url), got %d", count)
	}
}

func TestHandleCreate_InvalidLaunchURL(t *testing.T) {
	handler, fixtures := newTestAdminHandler(t)
	ctx, cancel := testutil.TestContext()
	defer cancel()

	db := fixtures.DB()

	formValues := map[string]string{
		"title":      "Test Resource",
		"launch_url": "not-a-valid-url",
		"status":     "active",
	}

	req, _ := createMultipartRequest(t, "/resources", formValues)
	req = auth.WithTestUser(req, adminUser())

	rec := httptest.NewRecorder()
	func() {
		defer func() { recover() }()
		handler.HandleCreate(rec, req)
	}()

	// No resource should be created
	count, _ := db.Collection("resources").CountDocuments(ctx, bson.M{})
	if count != 0 {
		t.Errorf("expected 0 resources (invalid launch_url), got %d", count)
	}
}

func TestHandleCreate_DuplicateTitle(t *testing.T) {
	handler, fixtures := newTestAdminHandler(t)
	ctx, cancel := testutil.TestContext()
	defer cancel()

	db := fixtures.DB()
	fixtures.CreateResource(ctx, "Existing Resource", "https://example.com/existing")

	formValues := map[string]string{
		"title":      "Existing Resource",
		"launch_url": "https://example.com/duplicate",
		"status":     "active",
	}

	req, _ := createMultipartRequest(t, "/resources", formValues)
	req = auth.WithTestUser(req, adminUser())

	rec := httptest.NewRecorder()
	func() {
		defer func() { recover() }()
		handler.HandleCreate(rec, req)
	}()

	// Should still have only 1 resource
	count, _ := db.Collection("resources").CountDocuments(ctx, bson.M{})
	if count != 1 {
		t.Errorf("expected 1 resource (duplicate rejected), got %d", count)
	}
}

func TestHandleCreate_DefaultsApplied(t *testing.T) {
	handler, fixtures := newTestAdminHandler(t)
	ctx, cancel := testutil.TestContext()
	defer cancel()

	db := fixtures.DB()

	formValues := map[string]string{
		"title":      "Minimal Resource",
		"launch_url": "https://example.com/minimal",
		// type and status not provided
	}

	req, _ := createMultipartRequest(t, "/resources", formValues)
	req = auth.WithTestUser(req, adminUser())

	rec := httptest.NewRecorder()
	handler.HandleCreate(rec, req)

	if rec.Code != http.StatusSeeOther {
		t.Errorf("expected status %d, got %d", http.StatusSeeOther, rec.Code)
	}

	// Verify defaults were applied
	var resource struct {
		Type   string `bson:"type"`
		Status string `bson:"status"`
	}
	err := db.Collection("resources").FindOne(ctx, bson.M{"title": "Minimal Resource"}).Decode(&resource)
	if err != nil {
		t.Fatalf("FindOne failed: %v", err)
	}
	// Default type should be applied (from models.DefaultResourceType)
	if resource.Type == "" {
		t.Error("Type should have a default value")
	}
	if resource.Status != "active" {
		t.Errorf("Status: got %q, want %q (default)", resource.Status, "active")
	}
}
