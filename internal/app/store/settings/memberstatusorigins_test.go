package settingsstore_test

import (
	"reflect"
	"testing"

	settingsstore "github.com/dalemusser/stratahub/internal/app/store/settings"
	"github.com/dalemusser/stratahub/internal/domain/models"
	"github.com/dalemusser/stratahub/internal/testutil"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

func TestStore_Save_PersistsMemberStatusAllowedOrigins(t *testing.T) {
	db := testutil.SetupTestDB(t)
	store := settingsstore.New(db)
	ctx, cancel := testutil.TestContext()
	defer cancel()

	wsID := primitive.NewObjectID()
	origins := []string{"https://surveys.example.com", "http://localhost:3000"}
	if err := store.Save(ctx, wsID, models.SiteSettings{
		SiteName:                      "Site",
		MemberStatusAPIAllowedOrigins: origins,
	}); err != nil {
		t.Fatalf("Save: %v", err)
	}
	got, err := store.Get(ctx, wsID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if !reflect.DeepEqual(got.MemberStatusAPIAllowedOrigins, origins) {
		t.Errorf("origins: got %v, want %v", got.MemberStatusAPIAllowedOrigins, origins)
	}

	// Saving with an empty list clears it.
	if err := store.Save(ctx, wsID, models.SiteSettings{SiteName: "Site"}); err != nil {
		t.Fatalf("Save (clear): %v", err)
	}
	got, _ = store.Get(ctx, wsID)
	if len(got.MemberStatusAPIAllowedOrigins) != 0 {
		t.Errorf("origins after clear: got %v, want none", got.MemberStatusAPIAllowedOrigins)
	}
}

func TestStore_SetMemberStatusAPIKey_LeavesOriginsAlone(t *testing.T) {
	db := testutil.SetupTestDB(t)
	store := settingsstore.New(db)
	ctx, cancel := testutil.TestContext()
	defer cancel()

	wsID := primitive.NewObjectID()
	origins := []string{"https://surveys.example.com"}
	if err := store.Save(ctx, wsID, models.SiteSettings{SiteName: "Site", MemberStatusAPIAllowedOrigins: origins}); err != nil {
		t.Fatalf("seed: %v", err)
	}
	if err := store.SetMemberStatusAPIKey(ctx, wsID, "ms_new_key_abcdefghijklmnop"); err != nil {
		t.Fatalf("SetMemberStatusAPIKey: %v", err)
	}
	got, _ := store.Get(ctx, wsID)
	if !reflect.DeepEqual(got.MemberStatusAPIAllowedOrigins, origins) {
		t.Errorf("origins disturbed by key set: got %v, want %v", got.MemberStatusAPIAllowedOrigins, origins)
	}
}
