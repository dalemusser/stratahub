package pagestore_test

import (
	"testing"

	pagestore "github.com/dalemusser/stratahub/internal/app/store/pages"
	"github.com/dalemusser/stratahub/internal/domain/models"
	"github.com/dalemusser/stratahub/internal/testutil"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
)

func TestStore_Upsert_Create(t *testing.T) {
	db := testutil.SetupTestDB(t)
	store := pagestore.New(db)
	ctx, cancel := testutil.TestContext()
	defer cancel()

	page := models.Page{
		Slug:    "about",
		Title:   "About Us",
		Content: "<p>About content</p>",
	}

	err := store.Upsert(ctx, page)
	if err != nil {
		t.Fatalf("Upsert failed: %v", err)
	}

	// Verify page was created
	saved, err := store.GetBySlug(ctx, "about")
	if err != nil {
		t.Fatalf("GetBySlug failed: %v", err)
	}

	if saved.Title != "About Us" {
		t.Errorf("expected title 'About Us', got %q", saved.Title)
	}
	if saved.Content != "<p>About content</p>" {
		t.Errorf("expected content '<p>About content</p>', got %q", saved.Content)
	}
	if saved.UpdatedAt == nil {
		t.Error("expected UpdatedAt to be set")
	}
}

func TestStore_Upsert_Update(t *testing.T) {
	db := testutil.SetupTestDB(t)
	store := pagestore.New(db)
	ctx, cancel := testutil.TestContext()
	defer cancel()

	// Create initial page
	page := models.Page{
		Slug:    "contact",
		Title:   "Contact Us",
		Content: "<p>Original content</p>",
	}
	err := store.Upsert(ctx, page)
	if err != nil {
		t.Fatalf("Initial Upsert failed: %v", err)
	}

	// Update the page
	page.Title = "Updated Contact"
	page.Content = "<p>Updated content</p>"
	err = store.Upsert(ctx, page)
	if err != nil {
		t.Fatalf("Update Upsert failed: %v", err)
	}

	// Verify page was updated
	saved, err := store.GetBySlug(ctx, "contact")
	if err != nil {
		t.Fatalf("GetBySlug failed: %v", err)
	}

	if saved.Title != "Updated Contact" {
		t.Errorf("expected title 'Updated Contact', got %q", saved.Title)
	}
	if saved.Content != "<p>Updated content</p>" {
		t.Errorf("expected updated content, got %q", saved.Content)
	}
}

func TestStore_Upsert_WithAuditFields(t *testing.T) {
	db := testutil.SetupTestDB(t)
	store := pagestore.New(db)
	ctx, cancel := testutil.TestContext()
	defer cancel()

	userID := primitive.NewObjectID()
	page := models.Page{
		Slug:          "terms",
		Title:         "Terms of Service",
		Content:       "<p>Terms content</p>",
		UpdatedByID:   &userID,
		UpdatedByName: "Admin User",
	}

	err := store.Upsert(ctx, page)
	if err != nil {
		t.Fatalf("Upsert failed: %v", err)
	}

	saved, err := store.GetBySlug(ctx, "terms")
	if err != nil {
		t.Fatalf("GetBySlug failed: %v", err)
	}

	if saved.UpdatedByID == nil || *saved.UpdatedByID != userID {
		t.Error("expected UpdatedByID to be set")
	}
	if saved.UpdatedByName != "Admin User" {
		t.Errorf("expected UpdatedByName 'Admin User', got %q", saved.UpdatedByName)
	}
}

func TestStore_GetBySlug(t *testing.T) {
	db := testutil.SetupTestDB(t)
	store := pagestore.New(db)
	ctx, cancel := testutil.TestContext()
	defer cancel()

	page := models.Page{
		Slug:    "privacy",
		Title:   "Privacy Policy",
		Content: "<p>Privacy content</p>",
	}
	err := store.Upsert(ctx, page)
	if err != nil {
		t.Fatalf("Upsert failed: %v", err)
	}

	saved, err := store.GetBySlug(ctx, "privacy")
	if err != nil {
		t.Fatalf("GetBySlug failed: %v", err)
	}

	if saved.Slug != "privacy" {
		t.Errorf("expected slug 'privacy', got %q", saved.Slug)
	}
	if saved.Title != "Privacy Policy" {
		t.Errorf("expected title 'Privacy Policy', got %q", saved.Title)
	}
}

func TestStore_GetBySlug_NotFound(t *testing.T) {
	db := testutil.SetupTestDB(t)
	store := pagestore.New(db)
	ctx, cancel := testutil.TestContext()
	defer cancel()

	_, err := store.GetBySlug(ctx, "non-existent")
	if err != mongo.ErrNoDocuments {
		t.Errorf("expected mongo.ErrNoDocuments, got %v", err)
	}
}

func TestStore_GetAll(t *testing.T) {
	db := testutil.SetupTestDB(t)
	store := pagestore.New(db)
	ctx, cancel := testutil.TestContext()
	defer cancel()

	// Create some pages
	pages := []models.Page{
		{Slug: "about", Title: "About"},
		{Slug: "contact", Title: "Contact"},
		{Slug: "terms", Title: "Terms"},
	}

	for _, p := range pages {
		err := store.Upsert(ctx, p)
		if err != nil {
			t.Fatalf("Upsert failed for %s: %v", p.Slug, err)
		}
	}

	// Get all
	all, err := store.GetAll(ctx)
	if err != nil {
		t.Fatalf("GetAll failed: %v", err)
	}

	if len(all) != 3 {
		t.Errorf("expected 3 pages, got %d", len(all))
	}
}

func TestStore_GetAll_Empty(t *testing.T) {
	db := testutil.SetupTestDB(t)
	store := pagestore.New(db)
	ctx, cancel := testutil.TestContext()
	defer cancel()

	all, err := store.GetAll(ctx)
	if err != nil {
		t.Fatalf("GetAll failed: %v", err)
	}

	if len(all) != 0 {
		t.Errorf("expected 0 pages, got %d", len(all))
	}
}

func TestStore_Exists_True(t *testing.T) {
	db := testutil.SetupTestDB(t)
	store := pagestore.New(db)
	ctx, cancel := testutil.TestContext()
	defer cancel()

	page := models.Page{
		Slug:  "about",
		Title: "About",
	}
	err := store.Upsert(ctx, page)
	if err != nil {
		t.Fatalf("Upsert failed: %v", err)
	}

	exists, err := store.Exists(ctx, "about")
	if err != nil {
		t.Fatalf("Exists failed: %v", err)
	}

	if !exists {
		t.Error("expected page to exist")
	}
}

func TestStore_Exists_False(t *testing.T) {
	db := testutil.SetupTestDB(t)
	store := pagestore.New(db)
	ctx, cancel := testutil.TestContext()
	defer cancel()

	exists, err := store.Exists(ctx, "non-existent")
	if err != nil {
		t.Fatalf("Exists failed: %v", err)
	}

	if exists {
		t.Error("expected page to not exist")
	}
}

func TestStore_Upsert_PreservesID(t *testing.T) {
	db := testutil.SetupTestDB(t)
	store := pagestore.New(db)
	ctx, cancel := testutil.TestContext()
	defer cancel()

	// Create initial page
	page := models.Page{
		Slug:    "help",
		Title:   "Help",
		Content: "<p>Help content</p>",
	}
	err := store.Upsert(ctx, page)
	if err != nil {
		t.Fatalf("Initial Upsert failed: %v", err)
	}

	// Get the ID
	saved1, err := store.GetBySlug(ctx, "help")
	if err != nil {
		t.Fatalf("GetBySlug failed: %v", err)
	}
	originalID := saved1.ID

	// Update the page
	page.Title = "Updated Help"
	err = store.Upsert(ctx, page)
	if err != nil {
		t.Fatalf("Update Upsert failed: %v", err)
	}

	// Verify ID is preserved
	saved2, err := store.GetBySlug(ctx, "help")
	if err != nil {
		t.Fatalf("GetBySlug failed: %v", err)
	}

	if saved2.ID != originalID {
		t.Errorf("expected ID to be preserved, got %v vs %v", saved2.ID, originalID)
	}
}

func TestStore_Upsert_UpdatesTimestamp(t *testing.T) {
	db := testutil.SetupTestDB(t)
	store := pagestore.New(db)
	ctx, cancel := testutil.TestContext()
	defer cancel()

	page := models.Page{
		Slug:  "faq",
		Title: "FAQ",
	}
	err := store.Upsert(ctx, page)
	if err != nil {
		t.Fatalf("Upsert failed: %v", err)
	}

	saved, err := store.GetBySlug(ctx, "faq")
	if err != nil {
		t.Fatalf("GetBySlug failed: %v", err)
	}

	if saved.UpdatedAt == nil {
		t.Error("expected UpdatedAt to be set on create")
	}
}

func TestStore_MultiplePages(t *testing.T) {
	db := testutil.SetupTestDB(t)
	store := pagestore.New(db)
	ctx, cancel := testutil.TestContext()
	defer cancel()

	// Create multiple pages
	slugs := []string{"about", "contact", "terms", "privacy", "faq"}
	for _, slug := range slugs {
		err := store.Upsert(ctx, models.Page{
			Slug:  slug,
			Title: "Page " + slug,
		})
		if err != nil {
			t.Fatalf("Upsert %s failed: %v", slug, err)
		}
	}

	// Verify all exist
	for _, slug := range slugs {
		exists, err := store.Exists(ctx, slug)
		if err != nil {
			t.Fatalf("Exists %s failed: %v", slug, err)
		}
		if !exists {
			t.Errorf("expected %s to exist", slug)
		}
	}

	// Get all
	all, err := store.GetAll(ctx)
	if err != nil {
		t.Fatalf("GetAll failed: %v", err)
	}
	if len(all) != 5 {
		t.Errorf("expected 5 pages, got %d", len(all))
	}
}
