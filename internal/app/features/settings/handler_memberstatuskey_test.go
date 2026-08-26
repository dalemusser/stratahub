package settings_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	settingsstore "github.com/dalemusser/stratahub/internal/app/store/settings"
	"github.com/dalemusser/stratahub/internal/app/system/auth"
	"github.com/dalemusser/stratahub/internal/app/system/workspace"
	"github.com/dalemusser/stratahub/internal/domain/models"
	"github.com/dalemusser/stratahub/internal/testutil"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

const testKey = "ms_0123456789abcdefghijklmnopqrstuvwxyzABCDEF" // 46 chars, the Generate button's shape

// postSettings submits the settings form for wsID with the given extra fields
// layered over a minimal valid form, swallowing any template-render panic on
// the error path (templates are not booted in unit tests).
func postSettings(t *testing.T, handler http.Handler, wsID primitive.ObjectID, extra map[string][]string) *httptest.ResponseRecorder {
	t.Helper()
	fields := map[string][]string{
		"site_name":       {"Test Site"},
		"auth_methods":    {"google"},
		"mhs_member_auth": {"trust"},
	}
	for k, v := range extra {
		fields[k] = v
	}
	body, contentType := multipartForm(t, fields)

	req := httptest.NewRequest("POST", "/settings", body)
	req.Header.Set("Content-Type", contentType)
	req = workspace.WithTestWorkspace(req, wsID, "test", "Test Workspace")
	req = auth.WithTestUser(req, &auth.SessionUser{
		ID:      primitive.NewObjectID().Hex(),
		Name:    "Admin User",
		LoginID: "admin@example.com",
		Role:    "admin",
	})
	rec := httptest.NewRecorder()
	func() {
		defer func() { _ = recover() }()
		handler.ServeHTTP(rec, req)
	}()
	return rec
}

func TestHandleSettings_MemberStatusKey_SetRoundTripClear(t *testing.T) {
	h := newTestHandler(t)
	handler := http.HandlerFunc(h.HandleSettings)
	store := settingsstore.New(h.DB)
	ctx, cancel := testutil.TestContext()
	defer cancel()

	wsID := primitive.NewObjectID()

	// 1. Set a key.
	rec := postSettings(t, handler, wsID, map[string][]string{"member_status_api_key": {testKey}})
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("set: status %d, want 303", rec.Code)
	}
	saved, err := store.Get(ctx, wsID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if saved.MemberStatusAPIKey != testKey {
		t.Errorf("key: got %q, want %q", saved.MemberStatusAPIKey, testKey)
	}
	if saved.MemberStatusAPIKeySetAt == nil {
		t.Fatal("set-at not recorded")
	}
	firstSetAt := *saved.MemberStatusAPIKeySetAt

	// 2. Re-save with the same key round-tripped: set-at must not move.
	rec = postSettings(t, handler, wsID, map[string][]string{"member_status_api_key": {testKey}})
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("round-trip: status %d, want 303", rec.Code)
	}
	saved, _ = store.Get(ctx, wsID)
	if saved.MemberStatusAPIKey != testKey {
		t.Errorf("round-trip key: got %q", saved.MemberStatusAPIKey)
	}
	if saved.MemberStatusAPIKeySetAt == nil || !saved.MemberStatusAPIKeySetAt.Equal(firstSetAt) {
		t.Errorf("set-at changed on an unchanged key: got %v, want %v", saved.MemberStatusAPIKeySetAt, firstSetAt)
	}

	// 3. Clear the key.
	rec = postSettings(t, handler, wsID, map[string][]string{"member_status_api_key": {""}})
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("clear: status %d, want 303", rec.Code)
	}
	saved, _ = store.Get(ctx, wsID)
	if saved.MemberStatusAPIKey != "" {
		t.Errorf("key after clear: got %q, want empty", saved.MemberStatusAPIKey)
	}
	if saved.MemberStatusAPIKeySetAt != nil {
		t.Errorf("set-at after clear: got %v, want nil", saved.MemberStatusAPIKeySetAt)
	}
}

func TestHandleSettings_MemberStatusKey_Validation(t *testing.T) {
	h := newTestHandler(t)
	handler := http.HandlerFunc(h.HandleSettings)
	store := settingsstore.New(h.DB)
	ctx, cancel := testutil.TestContext()
	defer cancel()

	wsID := primitive.NewObjectID()
	if err := store.Save(ctx, wsID, models.SiteSettings{SiteName: "Seed", MemberStatusAPIKey: testKey}); err != nil {
		t.Fatalf("seed: %v", err)
	}

	cases := map[string]string{
		"too short": "short",
		"too long":  strings.Repeat("x", 129),
		"has space": "ms_has a space in it here",
	}
	for name, bad := range cases {
		rec := postSettings(t, handler, wsID, map[string][]string{"member_status_api_key": {bad}})
		if rec.Code == http.StatusSeeOther {
			t.Errorf("%s: accepted (303) but should have been rejected", name)
		}
		saved, _ := store.Get(ctx, wsID)
		if saved.MemberStatusAPIKey != testKey {
			t.Errorf("%s: key was changed to %q despite rejection", name, saved.MemberStatusAPIKey)
		}
	}
}

func TestHandleSettings_MemberStatusKey_SurvivesOtherSaves(t *testing.T) {
	h := newTestHandler(t)
	handler := http.HandlerFunc(h.HandleSettings)
	store := settingsstore.New(h.DB)
	ctx, cancel := testutil.TestContext()
	defer cancel()

	wsID := primitive.NewObjectID()
	rec := postSettings(t, handler, wsID, map[string][]string{"member_status_api_key": {testKey}})
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("set: status %d", rec.Code)
	}

	// The form always carries the key; an unrelated change round-trips it.
	rec = postSettings(t, handler, wsID, map[string][]string{
		"member_status_api_key": {testKey},
		"landing_title":         {"Changed"},
	})
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("unrelated save: status %d", rec.Code)
	}
	saved, _ := store.Get(ctx, wsID)
	if saved.MemberStatusAPIKey != testKey || saved.LandingTitle != "Changed" {
		t.Errorf("after unrelated save: key=%q landing=%q", saved.MemberStatusAPIKey, saved.LandingTitle)
	}
}
