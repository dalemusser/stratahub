package settingsstore_test

import (
	"testing"

	settingsstore "github.com/dalemusser/stratahub/internal/app/store/settings"
	"github.com/dalemusser/stratahub/internal/domain/models"
	"github.com/dalemusser/stratahub/internal/testutil"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

func TestStore_Get_NoSettings(t *testing.T) {
	db := testutil.SetupTestDB(t)
	store := settingsstore.New(db)
	ctx, cancel := testutil.TestContext()
	defer cancel()

	wsID := primitive.NewObjectID()

	// Get settings for workspace with no saved settings
	settings, err := store.Get(ctx, wsID)
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}

	// Should return defaults
	if settings.WorkspaceID != wsID {
		t.Errorf("WorkspaceID: got %v, want %v", settings.WorkspaceID, wsID)
	}
	if settings.SiteName != models.DefaultSiteName {
		t.Errorf("SiteName: got %q, want default %q", settings.SiteName, models.DefaultSiteName)
	}
	if settings.LandingTitle != models.DefaultLandingTitle {
		t.Errorf("LandingTitle: got %q, want default %q", settings.LandingTitle, models.DefaultLandingTitle)
	}
}

func TestStore_Save_NewSettings(t *testing.T) {
	db := testutil.SetupTestDB(t)
	store := settingsstore.New(db)
	ctx, cancel := testutil.TestContext()
	defer cancel()

	wsID := primitive.NewObjectID()

	settings := models.SiteSettings{
		SiteName:      "My Site",
		LandingTitle:  "Welcome",
		LandingContent: "Hello world",
	}

	err := store.Save(ctx, wsID, settings)
	if err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	// Verify settings were saved
	saved, err := store.Get(ctx, wsID)
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}

	if saved.SiteName != "My Site" {
		t.Errorf("SiteName: got %q, want %q", saved.SiteName, "My Site")
	}
	if saved.LandingTitle != "Welcome" {
		t.Errorf("LandingTitle: got %q, want %q", saved.LandingTitle, "Welcome")
	}
	if saved.LandingContent != "Hello world" {
		t.Errorf("LandingContent: got %q, want %q", saved.LandingContent, "Hello world")
	}
}

func TestStore_Save_UpdateSettings(t *testing.T) {
	db := testutil.SetupTestDB(t)
	store := settingsstore.New(db)
	ctx, cancel := testutil.TestContext()
	defer cancel()

	wsID := primitive.NewObjectID()

	// Save initial settings
	settings := models.SiteSettings{
		SiteName:     "Original Site",
		LandingTitle: "Original Title",
	}
	err := store.Save(ctx, wsID, settings)
	if err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	// Update settings
	settings.SiteName = "Updated Site"
	settings.LandingTitle = "Updated Title"
	err = store.Save(ctx, wsID, settings)
	if err != nil {
		t.Fatalf("Save update failed: %v", err)
	}

	// Verify update
	saved, err := store.Get(ctx, wsID)
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}

	if saved.SiteName != "Updated Site" {
		t.Errorf("SiteName: got %q, want %q", saved.SiteName, "Updated Site")
	}
	if saved.LandingTitle != "Updated Title" {
		t.Errorf("LandingTitle: got %q, want %q", saved.LandingTitle, "Updated Title")
	}
}

func TestStore_Save_SetsUpdatedAt(t *testing.T) {
	db := testutil.SetupTestDB(t)
	store := settingsstore.New(db)
	ctx, cancel := testutil.TestContext()
	defer cancel()

	wsID := primitive.NewObjectID()

	settings := models.SiteSettings{
		SiteName: "Test Site",
	}

	err := store.Save(ctx, wsID, settings)
	if err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	saved, err := store.Get(ctx, wsID)
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}

	if saved.UpdatedAt == nil {
		t.Error("expected UpdatedAt to be set")
	}
}

func TestStore_Save_WithAuthMethods(t *testing.T) {
	db := testutil.SetupTestDB(t)
	store := settingsstore.New(db)
	ctx, cancel := testutil.TestContext()
	defer cancel()

	wsID := primitive.NewObjectID()

	settings := models.SiteSettings{
		SiteName:           "Test Site",
		EnabledAuthMethods: []string{"password", "google"},
	}

	err := store.Save(ctx, wsID, settings)
	if err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	saved, err := store.Get(ctx, wsID)
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}

	if len(saved.EnabledAuthMethods) != 2 {
		t.Errorf("expected 2 auth methods, got %d", len(saved.EnabledAuthMethods))
	}

	// Verify methods via helper
	methods := saved.GetEnabledAuthMethods()
	if len(methods) != 2 {
		t.Errorf("expected 2 enabled auth methods, got %d", len(methods))
	}
}

func TestStore_Exists_False(t *testing.T) {
	db := testutil.SetupTestDB(t)
	store := settingsstore.New(db)
	ctx, cancel := testutil.TestContext()
	defer cancel()

	wsID := primitive.NewObjectID()

	exists, err := store.Exists(ctx, wsID)
	if err != nil {
		t.Fatalf("Exists failed: %v", err)
	}

	if exists {
		t.Error("expected Exists to return false for new workspace")
	}
}

func TestStore_Exists_True(t *testing.T) {
	db := testutil.SetupTestDB(t)
	store := settingsstore.New(db)
	ctx, cancel := testutil.TestContext()
	defer cancel()

	wsID := primitive.NewObjectID()

	// Save settings
	settings := models.SiteSettings{SiteName: "Test Site"}
	err := store.Save(ctx, wsID, settings)
	if err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	// Check exists
	exists, err := store.Exists(ctx, wsID)
	if err != nil {
		t.Fatalf("Exists failed: %v", err)
	}

	if !exists {
		t.Error("expected Exists to return true after save")
	}
}

func TestStore_Delete(t *testing.T) {
	db := testutil.SetupTestDB(t)
	store := settingsstore.New(db)
	ctx, cancel := testutil.TestContext()
	defer cancel()

	wsID := primitive.NewObjectID()

	// Save settings
	settings := models.SiteSettings{SiteName: "Test Site"}
	err := store.Save(ctx, wsID, settings)
	if err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	// Verify exists
	exists, _ := store.Exists(ctx, wsID)
	if !exists {
		t.Fatal("expected settings to exist before delete")
	}

	// Delete
	err = store.Delete(ctx, wsID)
	if err != nil {
		t.Fatalf("Delete failed: %v", err)
	}

	// Verify deleted
	exists, _ = store.Exists(ctx, wsID)
	if exists {
		t.Error("expected settings to not exist after delete")
	}
}

func TestStore_Delete_NonExistent(t *testing.T) {
	db := testutil.SetupTestDB(t)
	store := settingsstore.New(db)
	ctx, cancel := testutil.TestContext()
	defer cancel()

	wsID := primitive.NewObjectID()

	// Delete non-existent settings should not error
	err := store.Delete(ctx, wsID)
	if err != nil {
		t.Fatalf("Delete non-existent should not error: %v", err)
	}
}

func TestStore_MultipleWorkspaces(t *testing.T) {
	db := testutil.SetupTestDB(t)
	store := settingsstore.New(db)
	ctx, cancel := testutil.TestContext()
	defer cancel()

	ws1 := primitive.NewObjectID()
	ws2 := primitive.NewObjectID()

	// Save settings for ws1
	err := store.Save(ctx, ws1, models.SiteSettings{SiteName: "Workspace One"})
	if err != nil {
		t.Fatalf("Save ws1 failed: %v", err)
	}

	// Save settings for ws2
	err = store.Save(ctx, ws2, models.SiteSettings{SiteName: "Workspace Two"})
	if err != nil {
		t.Fatalf("Save ws2 failed: %v", err)
	}

	// Get ws1
	settings1, _ := store.Get(ctx, ws1)
	if settings1.SiteName != "Workspace One" {
		t.Errorf("ws1 SiteName: got %q, want %q", settings1.SiteName, "Workspace One")
	}

	// Get ws2
	settings2, _ := store.Get(ctx, ws2)
	if settings2.SiteName != "Workspace Two" {
		t.Errorf("ws2 SiteName: got %q, want %q", settings2.SiteName, "Workspace Two")
	}

	// Delete ws1 should not affect ws2
	_ = store.Delete(ctx, ws1)

	exists, _ := store.Exists(ctx, ws2)
	if !exists {
		t.Error("ws2 settings should still exist after ws1 delete")
	}
}
