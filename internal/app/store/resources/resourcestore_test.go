package resourcestore_test

import (
	"testing"

	resourcestore "github.com/dalemusser/stratahub/internal/app/store/resources"
	"github.com/dalemusser/stratahub/internal/domain/models"
	"github.com/dalemusser/stratahub/internal/testutil"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

func TestStore_Create(t *testing.T) {
	db := testutil.SetupTestDB(t)
	store := resourcestore.New(db)
	ctx, cancel := testutil.TestContext()
	defer cancel()

	resource := models.Resource{
		Title:         "Test Resource",
		LaunchURL:     "https://example.com/resource",
		Description:   "A test resource",
		Type:          "game",
		Subject:       "Math",
		ShowInLibrary: true,
	}

	created, err := store.Create(ctx, resource)
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	// Verify ID was assigned
	if created.ID == primitive.NilObjectID {
		t.Error("expected ID to be assigned")
	}

	// Verify normalized fields
	if created.TitleCI == "" {
		t.Error("expected TitleCI to be set")
	}
	if created.SubjectCI == "" {
		t.Error("expected SubjectCI to be set")
	}

	// Verify defaults
	if created.Status != "active" {
		t.Errorf("Status: got %q, want %q", created.Status, "active")
	}
	// ShowInLibrary preserves input value (no forced default)
	if !created.ShowInLibrary {
		t.Error("expected ShowInLibrary to be preserved as true")
	}

	// Verify timestamps
	if created.CreatedAt.IsZero() {
		t.Error("expected CreatedAt to be set")
	}
	if created.UpdatedAt == nil || created.UpdatedAt.IsZero() {
		t.Error("expected UpdatedAt to be set")
	}
}

func TestStore_Create_DefaultType(t *testing.T) {
	db := testutil.SetupTestDB(t)
	store := resourcestore.New(db)
	ctx, cancel := testutil.TestContext()
	defer cancel()

	resource := models.Resource{
		Title:     "Test Resource",
		LaunchURL: "https://example.com/resource",
		// Type not specified
	}

	created, err := store.Create(ctx, resource)
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	if created.Type != "game" {
		t.Errorf("Type: got %q, want %q (default)", created.Type, "game")
	}
}

func TestStore_Create_MissingTitle(t *testing.T) {
	db := testutil.SetupTestDB(t)
	store := resourcestore.New(db)
	ctx, cancel := testutil.TestContext()
	defer cancel()

	resource := models.Resource{
		LaunchURL: "https://example.com/resource",
		// Title missing
	}

	_, err := store.Create(ctx, resource)
	if err == nil {
		t.Fatal("expected error for missing title")
	}
}

func TestStore_Create_MissingLaunchURL(t *testing.T) {
	db := testutil.SetupTestDB(t)
	store := resourcestore.New(db)
	ctx, cancel := testutil.TestContext()
	defer cancel()

	resource := models.Resource{
		Title: "Test Resource",
		// LaunchURL missing
	}

	_, err := store.Create(ctx, resource)
	if err == nil {
		t.Fatal("expected error for missing launch_url")
	}
}

func TestStore_Create_InvalidLaunchURL(t *testing.T) {
	db := testutil.SetupTestDB(t)
	store := resourcestore.New(db)
	ctx, cancel := testutil.TestContext()
	defer cancel()

	resource := models.Resource{
		Title:     "Test Resource",
		LaunchURL: "not-a-valid-url",
	}

	_, err := store.Create(ctx, resource)
	if err == nil {
		t.Fatal("expected error for invalid launch_url")
	}
}

func TestStore_Create_InvalidStatus(t *testing.T) {
	db := testutil.SetupTestDB(t)
	store := resourcestore.New(db)
	ctx, cancel := testutil.TestContext()
	defer cancel()

	resource := models.Resource{
		Title:     "Test Resource",
		LaunchURL: "https://example.com/resource",
		Status:    "invalid_status",
	}

	_, err := store.Create(ctx, resource)
	if err == nil {
		t.Fatal("expected error for invalid status")
	}
}

func TestStore_Create_DuplicateTitle(t *testing.T) {
	db := testutil.SetupTestDB(t)
	store := resourcestore.New(db)
	ctx, cancel := testutil.TestContext()
	defer cancel()

	resource1 := models.Resource{
		Title:     "Duplicate Resource",
		LaunchURL: "https://example.com/resource1",
	}

	_, err := store.Create(ctx, resource1)
	if err != nil {
		t.Fatalf("first Create failed: %v", err)
	}

	resource2 := models.Resource{
		Title:     "Duplicate Resource",
		LaunchURL: "https://example.com/resource2",
	}

	_, err = store.Create(ctx, resource2)
	if err != resourcestore.ErrDuplicateTitle {
		t.Errorf("expected ErrDuplicateTitle, got %v", err)
	}
}

func TestStore_Create_CaseInsensitiveTitle(t *testing.T) {
	db := testutil.SetupTestDB(t)
	store := resourcestore.New(db)
	ctx, cancel := testutil.TestContext()
	defer cancel()

	resource := models.Resource{
		Title:     "École Française",
		LaunchURL: "https://example.com/ecole",
	}

	created, err := store.Create(ctx, resource)
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	if created.TitleCI != "ecole francaise" {
		t.Errorf("TitleCI: got %q, want %q", created.TitleCI, "ecole francaise")
	}
}

func TestStore_Update(t *testing.T) {
	db := testutil.SetupTestDB(t)
	store := resourcestore.New(db)
	ctx, cancel := testutil.TestContext()
	defer cancel()

	// Create a resource first
	resource := models.Resource{
		Title:     "Original Title",
		LaunchURL: "https://example.com/original",
		Type:      "game",
	}

	created, err := store.Create(ctx, resource)
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	// Update the resource
	updates := models.Resource{
		Title:       "Updated Title",
		Description: "Updated description",
		LaunchURL:   "https://example.com/updated",
		Status:      "disabled",
		Type:        "video",
	}

	err = store.Update(ctx, created.ID, updates)
	if err != nil {
		t.Fatalf("Update failed: %v", err)
	}

	// Note: We can't easily verify the update without a GetByID method,
	// but we've verified the Update method doesn't error
}

func TestStore_Update_InvalidLaunchURL(t *testing.T) {
	db := testutil.SetupTestDB(t)
	store := resourcestore.New(db)
	ctx, cancel := testutil.TestContext()
	defer cancel()

	// Create a resource first
	resource := models.Resource{
		Title:     "Test Resource",
		LaunchURL: "https://example.com/test",
	}

	created, err := store.Create(ctx, resource)
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	// Try to update with invalid URL
	updates := models.Resource{
		LaunchURL: "invalid-url",
	}

	err = store.Update(ctx, created.ID, updates)
	if err == nil {
		t.Fatal("expected error for invalid launch_url")
	}
}

func TestStore_Update_InvalidStatus(t *testing.T) {
	db := testutil.SetupTestDB(t)
	store := resourcestore.New(db)
	ctx, cancel := testutil.TestContext()
	defer cancel()

	// Create a resource first
	resource := models.Resource{
		Title:     "Test Resource",
		LaunchURL: "https://example.com/test",
	}

	created, err := store.Create(ctx, resource)
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	// Try to update with invalid status
	updates := models.Resource{
		Status: "invalid_status",
	}

	err = store.Update(ctx, created.ID, updates)
	if err == nil {
		t.Fatal("expected error for invalid status")
	}
}
