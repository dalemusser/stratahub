package wsauth_test

import (
	"testing"

	settingsstore "github.com/dalemusser/stratahub/internal/app/store/settings"
	"github.com/dalemusser/stratahub/internal/app/system/wsauth"
	"github.com/dalemusser/stratahub/internal/domain/models"
	"github.com/dalemusser/stratahub/internal/testutil"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

func TestGetEnabledAuthMethodsForWorkspace_NoSettings(t *testing.T) {
	db := testutil.SetupTestDB(t)
	ctx, cancel := testutil.TestContext()
	defer cancel()

	wsID := primitive.NewObjectID()

	// No settings exist for this workspace - should return all methods
	methods := wsauth.GetEnabledAuthMethodsForWorkspace(ctx, db, wsID)

	// Should return all auth methods (default behavior)
	if len(methods) != len(models.AllAuthMethods) {
		t.Errorf("expected %d methods (all), got %d", len(models.AllAuthMethods), len(methods))
	}
}

func TestGetEnabledAuthMethodsForWorkspace_WithSettings(t *testing.T) {
	db := testutil.SetupTestDB(t)
	store := settingsstore.New(db)
	ctx, cancel := testutil.TestContext()
	defer cancel()

	wsID := primitive.NewObjectID()

	// Save settings with only password and trust enabled
	settings := models.SiteSettings{
		WorkspaceID:        wsID,
		EnabledAuthMethods: []string{"password", "trust"},
	}
	err := store.Save(ctx, wsID, settings)
	if err != nil {
		t.Fatalf("Save settings failed: %v", err)
	}

	// Get enabled methods
	methods := wsauth.GetEnabledAuthMethodsForWorkspace(ctx, db, wsID)

	if len(methods) != 2 {
		t.Errorf("expected 2 methods, got %d", len(methods))
	}

	// Verify the correct methods are returned
	hasPassword := false
	hasTrust := false
	for _, m := range methods {
		if m.Value == "password" {
			hasPassword = true
		}
		if m.Value == "trust" {
			hasTrust = true
		}
	}
	if !hasPassword {
		t.Error("expected password method to be enabled")
	}
	if !hasTrust {
		t.Error("expected trust method to be enabled")
	}
}

func TestIsAuthMethodEnabledForWorkspace_NoSettings(t *testing.T) {
	db := testutil.SetupTestDB(t)
	ctx, cancel := testutil.TestContext()
	defer cancel()

	wsID := primitive.NewObjectID()

	// No settings - all valid methods should be enabled
	if !wsauth.IsAuthMethodEnabledForWorkspace(ctx, db, wsID, "password") {
		t.Error("expected password to be enabled when no settings exist")
	}
	if !wsauth.IsAuthMethodEnabledForWorkspace(ctx, db, wsID, "google") {
		t.Error("expected google to be enabled when no settings exist")
	}

	// Invalid methods should still return false
	if wsauth.IsAuthMethodEnabledForWorkspace(ctx, db, wsID, "invalid_method") {
		t.Error("expected invalid_method to return false")
	}
}

func TestIsAuthMethodEnabledForWorkspace_WithSettings(t *testing.T) {
	db := testutil.SetupTestDB(t)
	store := settingsstore.New(db)
	ctx, cancel := testutil.TestContext()
	defer cancel()

	wsID := primitive.NewObjectID()

	// Save settings with only google enabled
	settings := models.SiteSettings{
		WorkspaceID:        wsID,
		EnabledAuthMethods: []string{"google"},
	}
	err := store.Save(ctx, wsID, settings)
	if err != nil {
		t.Fatalf("Save settings failed: %v", err)
	}

	// google should be enabled
	if !wsauth.IsAuthMethodEnabledForWorkspace(ctx, db, wsID, "google") {
		t.Error("expected google to be enabled")
	}

	// password should NOT be enabled
	if wsauth.IsAuthMethodEnabledForWorkspace(ctx, db, wsID, "password") {
		t.Error("expected password to NOT be enabled")
	}
}

func TestGetEnabledAuthMethodMapForWorkspace_NoSettings(t *testing.T) {
	db := testutil.SetupTestDB(t)
	ctx, cancel := testutil.TestContext()
	defer cancel()

	wsID := primitive.NewObjectID()

	// No settings - should return all methods
	methodMap := wsauth.GetEnabledAuthMethodMapForWorkspace(ctx, db, wsID)

	// All auth methods should be in the map
	for _, m := range models.AllAuthMethods {
		if !methodMap[m.Value] {
			t.Errorf("expected %q to be in map", m.Value)
		}
	}
}

func TestGetEnabledAuthMethodMapForWorkspace_WithSettings(t *testing.T) {
	db := testutil.SetupTestDB(t)
	store := settingsstore.New(db)
	ctx, cancel := testutil.TestContext()
	defer cancel()

	wsID := primitive.NewObjectID()

	// Save settings with specific methods
	settings := models.SiteSettings{
		WorkspaceID:        wsID,
		EnabledAuthMethods: []string{"email", "microsoft"},
	}
	err := store.Save(ctx, wsID, settings)
	if err != nil {
		t.Fatalf("Save settings failed: %v", err)
	}

	methodMap := wsauth.GetEnabledAuthMethodMapForWorkspace(ctx, db, wsID)

	// email and microsoft should be in the map
	if !methodMap["email"] {
		t.Error("expected email to be in map")
	}
	if !methodMap["microsoft"] {
		t.Error("expected microsoft to be in map")
	}

	// Others should NOT be in the map
	if methodMap["password"] {
		t.Error("expected password to NOT be in map")
	}
	if methodMap["google"] {
		t.Error("expected google to NOT be in map")
	}
}

func TestGetEnabledAuthMethodsForWorkspace_EmptySettings(t *testing.T) {
	db := testutil.SetupTestDB(t)
	store := settingsstore.New(db)
	ctx, cancel := testutil.TestContext()
	defer cancel()

	wsID := primitive.NewObjectID()

	// Save settings with empty enabled methods list
	settings := models.SiteSettings{
		WorkspaceID:        wsID,
		SiteName:           "Test Site",
		EnabledAuthMethods: []string{}, // Empty list
	}
	err := store.Save(ctx, wsID, settings)
	if err != nil {
		t.Fatalf("Save settings failed: %v", err)
	}

	// Empty list means all methods enabled (default)
	methods := wsauth.GetEnabledAuthMethodsForWorkspace(ctx, db, wsID)

	if len(methods) != len(models.AllAuthMethods) {
		t.Errorf("expected %d methods (all), got %d", len(models.AllAuthMethods), len(methods))
	}
}

func TestIsAuthMethodEnabledForWorkspace_AllMethods(t *testing.T) {
	db := testutil.SetupTestDB(t)
	store := settingsstore.New(db)
	ctx, cancel := testutil.TestContext()
	defer cancel()

	wsID := primitive.NewObjectID()

	// Save settings with all methods enabled
	allMethods := make([]string, len(models.AllAuthMethods))
	for i, m := range models.AllAuthMethods {
		allMethods[i] = m.Value
	}
	settings := models.SiteSettings{
		WorkspaceID:        wsID,
		EnabledAuthMethods: allMethods,
	}
	err := store.Save(ctx, wsID, settings)
	if err != nil {
		t.Fatalf("Save settings failed: %v", err)
	}

	// All methods should be enabled
	for _, m := range models.AllAuthMethods {
		if !wsauth.IsAuthMethodEnabledForWorkspace(ctx, db, wsID, m.Value) {
			t.Errorf("expected %q to be enabled", m.Value)
		}
	}
}
