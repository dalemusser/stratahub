package settingsstore_test

import (
	"testing"
	"time"

	settingsstore "github.com/dalemusser/stratahub/internal/app/store/settings"
	"github.com/dalemusser/stratahub/internal/domain/models"
	"github.com/dalemusser/stratahub/internal/testutil"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

func TestStore_Save_PersistsMemberStatusKey(t *testing.T) {
	db := testutil.SetupTestDB(t)
	store := settingsstore.New(db)
	ctx, cancel := testutil.TestContext()
	defer cancel()

	wsID := primitive.NewObjectID()
	setAt := time.Date(2026, 8, 26, 12, 0, 0, 0, time.UTC)
	if err := store.Save(ctx, wsID, models.SiteSettings{
		SiteName:                "Site",
		MemberStatusAPIKey:      "ms_persisted_key_0123456789",
		MemberStatusAPIKeySetAt: &setAt,
	}); err != nil {
		t.Fatalf("Save: %v", err)
	}

	got, err := store.Get(ctx, wsID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.MemberStatusAPIKey != "ms_persisted_key_0123456789" {
		t.Errorf("key: got %q", got.MemberStatusAPIKey)
	}
	if got.MemberStatusAPIKeySetAt == nil || !got.MemberStatusAPIKeySetAt.Equal(setAt) {
		t.Errorf("set-at: got %v, want %v", got.MemberStatusAPIKeySetAt, setAt)
	}
}

func TestStore_SetMemberStatusAPIKey_OnlyTouchesTheKey(t *testing.T) {
	db := testutil.SetupTestDB(t)
	store := settingsstore.New(db)
	ctx, cancel := testutil.TestContext()
	defer cancel()

	wsID := primitive.NewObjectID()
	collID := primitive.NewObjectID()
	if err := store.Save(ctx, wsID, models.SiteSettings{
		SiteName:              "Site",
		LandingTitle:          "Landing",
		MHSActiveCollectionID: &collID,
		ClaudeModel:           "claude-haiku-4-5-20251001",
	}); err != nil {
		t.Fatalf("seed: %v", err)
	}

	// Set
	if err := store.SetMemberStatusAPIKey(ctx, wsID, "ms_new_key_abcdefghijklmnop"); err != nil {
		t.Fatalf("SetMemberStatusAPIKey: %v", err)
	}
	got, _ := store.Get(ctx, wsID)
	if got.MemberStatusAPIKey != "ms_new_key_abcdefghijklmnop" || got.MemberStatusAPIKeySetAt == nil {
		t.Errorf("after set: key=%q setAt=%v", got.MemberStatusAPIKey, got.MemberStatusAPIKeySetAt)
	}
	if got.SiteName != "Site" || got.LandingTitle != "Landing" || got.ClaudeModel != "claude-haiku-4-5-20251001" ||
		got.MHSActiveCollectionID == nil || *got.MHSActiveCollectionID != collID {
		t.Errorf("other settings disturbed: %+v", got)
	}

	// Clear
	if err := store.SetMemberStatusAPIKey(ctx, wsID, ""); err != nil {
		t.Fatalf("clear: %v", err)
	}
	got, _ = store.Get(ctx, wsID)
	if got.MemberStatusAPIKey != "" || got.MemberStatusAPIKeySetAt != nil {
		t.Errorf("after clear: key=%q setAt=%v", got.MemberStatusAPIKey, got.MemberStatusAPIKeySetAt)
	}
	if got.LandingTitle != "Landing" {
		t.Errorf("landing disturbed by clear: %q", got.LandingTitle)
	}
}

func TestStore_SetMemberStatusAPIKey_CreatesDocumentIfMissing(t *testing.T) {
	db := testutil.SetupTestDB(t)
	store := settingsstore.New(db)
	ctx, cancel := testutil.TestContext()
	defer cancel()

	wsID := primitive.NewObjectID()
	if err := store.SetMemberStatusAPIKey(ctx, wsID, "ms_first_write_0123456789"); err != nil {
		t.Fatalf("SetMemberStatusAPIKey: %v", err)
	}
	exists, err := store.Exists(ctx, wsID)
	if err != nil || !exists {
		t.Fatalf("document not created: exists=%v err=%v", exists, err)
	}
	got, _ := store.Get(ctx, wsID)
	if got.MemberStatusAPIKey != "ms_first_write_0123456789" {
		t.Errorf("key: got %q", got.MemberStatusAPIKey)
	}
}
