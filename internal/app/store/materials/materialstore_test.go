package materialstore_test

import (
	"testing"

	materialstore "github.com/dalemusser/stratahub/internal/app/store/materials"
	"github.com/dalemusser/stratahub/internal/domain/models"
	"github.com/dalemusser/stratahub/internal/testutil"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
)

func TestStore_Create(t *testing.T) {
	db := testutil.SetupTestDB(t)
	store := materialstore.New(db)
	ctx, cancel := testutil.TestContext()
	defer cancel()

	m := models.Material{
		Title:     "Test Material",
		Type:      "document",
		LaunchURL: "https://example.com/material",
	}

	created, err := store.Create(ctx, m)
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	if created.ID == primitive.NilObjectID {
		t.Error("expected ID to be assigned")
	}
	if created.TitleCI == "" {
		t.Error("expected TitleCI to be set")
	}
	if created.Status != "active" {
		t.Errorf("expected default status 'active', got %q", created.Status)
	}
	if created.CreatedAt.IsZero() {
		t.Error("expected CreatedAt to be set")
	}
	if created.UpdatedAt == nil || created.UpdatedAt.IsZero() {
		t.Error("expected UpdatedAt to be set")
	}
}

func TestStore_Create_WithFile(t *testing.T) {
	db := testutil.SetupTestDB(t)
	store := materialstore.New(db)
	ctx, cancel := testutil.TestContext()
	defer cancel()

	m := models.Material{
		Title:    "File Material",
		Type:     "document",
		FilePath: "/uploads/test.pdf",
		FileName: "test.pdf",
		FileSize: 1024,
	}

	created, err := store.Create(ctx, m)
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	if created.FilePath != "/uploads/test.pdf" {
		t.Errorf("FilePath: got %q, want %q", created.FilePath, "/uploads/test.pdf")
	}
}

func TestStore_Create_MissingTitle(t *testing.T) {
	db := testutil.SetupTestDB(t)
	store := materialstore.New(db)
	ctx, cancel := testutil.TestContext()
	defer cancel()

	m := models.Material{
		Type:      "document",
		LaunchURL: "https://example.com/material",
	}

	_, err := store.Create(ctx, m)
	if err == nil {
		t.Fatal("expected error for missing title")
	}
}

func TestStore_Create_MissingURLAndFile(t *testing.T) {
	db := testutil.SetupTestDB(t)
	store := materialstore.New(db)
	ctx, cancel := testutil.TestContext()
	defer cancel()

	m := models.Material{
		Title: "No Content Material",
		Type:  "document",
	}

	_, err := store.Create(ctx, m)
	if err == nil {
		t.Fatal("expected error when neither URL nor file provided")
	}
}

func TestStore_Create_BothURLAndFile(t *testing.T) {
	db := testutil.SetupTestDB(t)
	store := materialstore.New(db)
	ctx, cancel := testutil.TestContext()
	defer cancel()

	m := models.Material{
		Title:     "Conflicting Material",
		Type:      "document",
		LaunchURL: "https://example.com/material",
		FilePath:  "/uploads/test.pdf",
	}

	_, err := store.Create(ctx, m)
	if err == nil {
		t.Fatal("expected error when both URL and file provided")
	}
}

func TestStore_Create_InvalidURL(t *testing.T) {
	db := testutil.SetupTestDB(t)
	store := materialstore.New(db)
	ctx, cancel := testutil.TestContext()
	defer cancel()

	m := models.Material{
		Title:     "Invalid URL Material",
		Type:      "document",
		LaunchURL: "not-a-valid-url",
	}

	_, err := store.Create(ctx, m)
	if err == nil {
		t.Fatal("expected error for invalid URL")
	}
}

func TestStore_Create_InvalidStatus(t *testing.T) {
	db := testutil.SetupTestDB(t)
	store := materialstore.New(db)
	ctx, cancel := testutil.TestContext()
	defer cancel()

	m := models.Material{
		Title:     "Invalid Status Material",
		Type:      "document",
		Status:    "invalid_status",
		LaunchURL: "https://example.com/material",
	}

	_, err := store.Create(ctx, m)
	if err == nil {
		t.Fatal("expected error for invalid status")
	}
}

func TestStore_Create_DuplicateTitle(t *testing.T) {
	db := testutil.SetupTestDB(t)
	store := materialstore.New(db)
	ctx, cancel := testutil.TestContext()
	defer cancel()

	m1 := models.Material{
		Title:     "Duplicate Material",
		Type:      "document",
		LaunchURL: "https://example.com/material1",
	}

	_, err := store.Create(ctx, m1)
	if err != nil {
		t.Fatalf("First create failed: %v", err)
	}

	m2 := models.Material{
		Title:     "Duplicate Material",
		Type:      "document",
		LaunchURL: "https://example.com/material2",
	}

	_, err = store.Create(ctx, m2)
	if err != materialstore.ErrDuplicateTitle {
		t.Errorf("expected ErrDuplicateTitle, got %v", err)
	}
}

func TestStore_GetByID(t *testing.T) {
	db := testutil.SetupTestDB(t)
	store := materialstore.New(db)
	ctx, cancel := testutil.TestContext()
	defer cancel()

	m := models.Material{
		Title:     "Test Material",
		Type:      "document",
		LaunchURL: "https://example.com/material",
	}

	created, err := store.Create(ctx, m)
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	found, err := store.GetByID(ctx, created.ID)
	if err != nil {
		t.Fatalf("GetByID failed: %v", err)
	}

	if found.ID != created.ID {
		t.Errorf("ID: got %v, want %v", found.ID, created.ID)
	}
	if found.Title != created.Title {
		t.Errorf("Title: got %q, want %q", found.Title, created.Title)
	}
}

func TestStore_GetByID_NotFound(t *testing.T) {
	db := testutil.SetupTestDB(t)
	store := materialstore.New(db)
	ctx, cancel := testutil.TestContext()
	defer cancel()

	fakeID := primitive.NewObjectID()
	_, err := store.GetByID(ctx, fakeID)
	if err != mongo.ErrNoDocuments {
		t.Errorf("expected mongo.ErrNoDocuments, got %v", err)
	}
}

func TestStore_Update(t *testing.T) {
	db := testutil.SetupTestDB(t)
	store := materialstore.New(db)
	ctx, cancel := testutil.TestContext()
	defer cancel()

	m := models.Material{
		Title:       "Original Title",
		Type:        "document",
		Subject:     "Original Subject",
		Description: "Original Description",
		LaunchURL:   "https://example.com/original",
	}

	created, err := store.Create(ctx, m)
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	// Update the material
	err = store.Update(ctx, created.ID, models.Material{
		Title:       "Updated Title",
		Subject:     "Updated Subject",
		Description: "Updated Description",
	})
	if err != nil {
		t.Fatalf("Update failed: %v", err)
	}

	// Verify the changes
	found, err := store.GetByID(ctx, created.ID)
	if err != nil {
		t.Fatalf("GetByID failed: %v", err)
	}

	if found.Title != "Updated Title" {
		t.Errorf("Title: got %q, want %q", found.Title, "Updated Title")
	}
	if found.Subject != "Updated Subject" {
		t.Errorf("Subject: got %q, want %q", found.Subject, "Updated Subject")
	}
	if found.Description != "Updated Description" {
		t.Errorf("Description: got %q, want %q", found.Description, "Updated Description")
	}
}

func TestStore_Update_SwitchToFile(t *testing.T) {
	db := testutil.SetupTestDB(t)
	store := materialstore.New(db)
	ctx, cancel := testutil.TestContext()
	defer cancel()

	// Create with URL
	m := models.Material{
		Title:     "URL Material",
		Type:      "document",
		LaunchURL: "https://example.com/material",
	}

	created, err := store.Create(ctx, m)
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	// Switch to file
	err = store.Update(ctx, created.ID, models.Material{
		FilePath: "/uploads/new.pdf",
		FileName: "new.pdf",
		FileSize: 2048,
	})
	if err != nil {
		t.Fatalf("Update failed: %v", err)
	}

	// Verify URL is cleared and file is set
	found, err := store.GetByID(ctx, created.ID)
	if err != nil {
		t.Fatalf("GetByID failed: %v", err)
	}

	if found.LaunchURL != "" {
		t.Errorf("LaunchURL should be cleared, got %q", found.LaunchURL)
	}
	if found.FilePath != "/uploads/new.pdf" {
		t.Errorf("FilePath: got %q, want %q", found.FilePath, "/uploads/new.pdf")
	}
}

func TestStore_Update_SwitchToURL(t *testing.T) {
	db := testutil.SetupTestDB(t)
	store := materialstore.New(db)
	ctx, cancel := testutil.TestContext()
	defer cancel()

	// Create with file
	m := models.Material{
		Title:    "File Material",
		Type:     "document",
		FilePath: "/uploads/old.pdf",
		FileName: "old.pdf",
		FileSize: 1024,
	}

	created, err := store.Create(ctx, m)
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	// Switch to URL
	err = store.Update(ctx, created.ID, models.Material{
		LaunchURL: "https://example.com/new",
	})
	if err != nil {
		t.Fatalf("Update failed: %v", err)
	}

	// Verify file is cleared and URL is set
	found, err := store.GetByID(ctx, created.ID)
	if err != nil {
		t.Fatalf("GetByID failed: %v", err)
	}

	if found.FilePath != "" {
		t.Errorf("FilePath should be cleared, got %q", found.FilePath)
	}
	if found.LaunchURL != "https://example.com/new" {
		t.Errorf("LaunchURL: got %q, want %q", found.LaunchURL, "https://example.com/new")
	}
}

func TestStore_Update_InvalidURL(t *testing.T) {
	db := testutil.SetupTestDB(t)
	store := materialstore.New(db)
	ctx, cancel := testutil.TestContext()
	defer cancel()

	m := models.Material{
		Title:     "Test Material",
		Type:      "document",
		LaunchURL: "https://example.com/material",
	}

	created, err := store.Create(ctx, m)
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	err = store.Update(ctx, created.ID, models.Material{
		LaunchURL: "not-a-valid-url",
	})
	if err == nil {
		t.Fatal("expected error for invalid URL in update")
	}
}

func TestStore_Update_InvalidStatus(t *testing.T) {
	db := testutil.SetupTestDB(t)
	store := materialstore.New(db)
	ctx, cancel := testutil.TestContext()
	defer cancel()

	m := models.Material{
		Title:     "Test Material",
		Type:      "document",
		LaunchURL: "https://example.com/material",
	}

	created, err := store.Create(ctx, m)
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	err = store.Update(ctx, created.ID, models.Material{
		Status: "invalid_status",
	})
	if err == nil {
		t.Fatal("expected error for invalid status in update")
	}
}

func TestStore_Delete(t *testing.T) {
	db := testutil.SetupTestDB(t)
	store := materialstore.New(db)
	ctx, cancel := testutil.TestContext()
	defer cancel()

	m := models.Material{
		Title:     "Test Material",
		Type:      "document",
		LaunchURL: "https://example.com/material",
	}

	created, err := store.Create(ctx, m)
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	count, err := store.Delete(ctx, created.ID)
	if err != nil {
		t.Fatalf("Delete failed: %v", err)
	}

	if count != 1 {
		t.Errorf("expected 1 deleted, got %d", count)
	}

	// Verify it's gone
	_, err = store.GetByID(ctx, created.ID)
	if err != mongo.ErrNoDocuments {
		t.Errorf("expected mongo.ErrNoDocuments after delete, got %v", err)
	}
}

func TestStore_Delete_NonExistent(t *testing.T) {
	db := testutil.SetupTestDB(t)
	store := materialstore.New(db)
	ctx, cancel := testutil.TestContext()
	defer cancel()

	fakeID := primitive.NewObjectID()
	count, err := store.Delete(ctx, fakeID)
	if err != nil {
		t.Fatalf("Delete should not error: %v", err)
	}

	if count != 0 {
		t.Errorf("expected 0 deleted for non-existent, got %d", count)
	}
}

func TestStore_GetByIDs(t *testing.T) {
	db := testutil.SetupTestDB(t)
	store := materialstore.New(db)
	ctx, cancel := testutil.TestContext()
	defer cancel()

	m1, _ := store.Create(ctx, models.Material{
		Title:     "Material 1",
		Type:      "document",
		LaunchURL: "https://example.com/1",
	})
	m2, _ := store.Create(ctx, models.Material{
		Title:     "Material 2",
		Type:      "document",
		LaunchURL: "https://example.com/2",
	})
	_, _ = store.Create(ctx, models.Material{
		Title:     "Material 3",
		Type:      "document",
		LaunchURL: "https://example.com/3",
	})

	// Get only first two
	ids := []primitive.ObjectID{m1.ID, m2.ID}
	materials, err := store.GetByIDs(ctx, ids)
	if err != nil {
		t.Fatalf("GetByIDs failed: %v", err)
	}

	if len(materials) != 2 {
		t.Errorf("expected 2 materials, got %d", len(materials))
	}
}

func TestStore_GetByIDs_Empty(t *testing.T) {
	db := testutil.SetupTestDB(t)
	store := materialstore.New(db)
	ctx, cancel := testutil.TestContext()
	defer cancel()

	materials, err := store.GetByIDs(ctx, []primitive.ObjectID{})
	if err != nil {
		t.Fatalf("GetByIDs failed: %v", err)
	}

	if materials != nil {
		t.Errorf("expected nil for empty IDs, got %v", materials)
	}
}

func TestStore_Count(t *testing.T) {
	db := testutil.SetupTestDB(t)
	store := materialstore.New(db)
	ctx, cancel := testutil.TestContext()
	defer cancel()

	// Create 3 materials
	for i := 1; i <= 3; i++ {
		_, _ = store.Create(ctx, models.Material{
			Title:     "Material " + string(rune('A'+i-1)),
			Type:      "document",
			LaunchURL: "https://example.com/" + string(rune('0'+i)),
		})
	}

	count, err := store.Count(ctx, nil)
	if err != nil {
		t.Fatalf("Count failed: %v", err)
	}

	if count != 3 {
		t.Errorf("expected count 3, got %d", count)
	}
}
