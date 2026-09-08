package settings_test

import (
	"net/http"
	"reflect"
	"testing"

	settingsstore "github.com/dalemusser/stratahub/internal/app/store/settings"
	"github.com/dalemusser/stratahub/internal/domain/models"
	"github.com/dalemusser/stratahub/internal/testutil"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

func TestHandleSettings_MemberStatusOrigins_SaveNormalizeClear(t *testing.T) {
	h := newTestHandler(t)
	handler := http.HandlerFunc(h.HandleSettings)
	store := settingsstore.New(h.DB)
	ctx, cancel := testutil.TestContext()
	defer cancel()
	wsID := primitive.NewObjectID()

	// Messy input: mixed case, trailing slash, blank lines, a duplicate.
	rec := postSettings(t, handler, wsID, map[string][]string{
		"member_status_api_allowed_origins": {"https://Surveys.Example.com/\n\nhttps://surveys.example.com\r\nhttp://localhost:3000\n"},
	})
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("save: got %d, want 303 (body: %s)", rec.Code, rec.Body.String())
	}
	saved, err := store.Get(ctx, wsID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	want := []string{"https://surveys.example.com", "http://localhost:3000"}
	if !reflect.DeepEqual(saved.MemberStatusAPIAllowedOrigins, want) {
		t.Errorf("origins: got %v, want %v", saved.MemberStatusAPIAllowedOrigins, want)
	}

	// The form round-trips the normalized list unchanged.
	rec = postSettings(t, handler, wsID, map[string][]string{
		"member_status_api_allowed_origins": {"https://surveys.example.com\nhttp://localhost:3000"},
		"landing_title":                     {"Changed"},
	})
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("round-trip: got %d, want 303", rec.Code)
	}
	saved, _ = store.Get(ctx, wsID)
	if !reflect.DeepEqual(saved.MemberStatusAPIAllowedOrigins, want) || saved.LandingTitle != "Changed" {
		t.Errorf("after round-trip: origins=%v landing=%q", saved.MemberStatusAPIAllowedOrigins, saved.LandingTitle)
	}

	// Clearing the field turns browser calls off.
	rec = postSettings(t, handler, wsID, map[string][]string{"member_status_api_allowed_origins": {" \n "}})
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("clear: got %d, want 303", rec.Code)
	}
	saved, _ = store.Get(ctx, wsID)
	if len(saved.MemberStatusAPIAllowedOrigins) != 0 {
		t.Errorf("after clear: got %v, want none", saved.MemberStatusAPIAllowedOrigins)
	}
}

func TestHandleSettings_MemberStatusOrigins_Validation(t *testing.T) {
	h := newTestHandler(t)
	handler := http.HandlerFunc(h.HandleSettings)
	store := settingsstore.New(h.DB)
	ctx, cancel := testutil.TestContext()
	defer cancel()

	wsID := primitive.NewObjectID()
	seeded := []string{"https://surveys.example.com"}
	if err := store.Save(ctx, wsID, models.SiteSettings{SiteName: "Seed", MemberStatusAPIAllowedOrigins: seeded}); err != nil {
		t.Fatalf("seed: %v", err)
	}

	cases := map[string]string{
		"has a path":     "https://surveys.example.com/wix/p1.aspx",
		"no scheme":      "surveys.example.com",
		"plain http":     "http://surveys.example.com",
		"wildcard":       "https://*.example.com",
		"one bad of two": "https://ok.example\nhttps://bad.example?x=1",
	}
	for name, bad := range cases {
		rec := postSettings(t, handler, wsID, map[string][]string{"member_status_api_allowed_origins": {bad}})
		if rec.Code == http.StatusSeeOther {
			t.Errorf("%s: accepted (303) but should have been rejected", name)
		}
		saved, _ := store.Get(ctx, wsID)
		if !reflect.DeepEqual(saved.MemberStatusAPIAllowedOrigins, seeded) {
			t.Errorf("%s: origins changed to %v despite rejection", name, saved.MemberStatusAPIAllowedOrigins)
		}
	}
}

func TestHandleSettings_MemberStatusOrigins_LeaveKeyAlone(t *testing.T) {
	h := newTestHandler(t)
	handler := http.HandlerFunc(h.HandleSettings)
	store := settingsstore.New(h.DB)
	ctx, cancel := testutil.TestContext()
	defer cancel()

	wsID := primitive.NewObjectID()
	if err := store.SetMemberStatusAPIKey(ctx, wsID, testKey); err != nil {
		t.Fatalf("seed key: %v", err)
	}
	before, _ := store.Get(ctx, wsID)

	rec := postSettings(t, handler, wsID, map[string][]string{
		"member_status_api_key":             {testKey},
		"member_status_api_allowed_origins": {"https://surveys.example.com"},
	})
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("save: got %d, want 303", rec.Code)
	}
	after, _ := store.Get(ctx, wsID)
	if after.MemberStatusAPIKey != testKey {
		t.Errorf("key: got %q, want unchanged", after.MemberStatusAPIKey)
	}
	if after.MemberStatusAPIKeySetAt == nil || !after.MemberStatusAPIKeySetAt.Equal(*before.MemberStatusAPIKeySetAt) {
		t.Errorf("key set-at changed by an origins save: %v → %v", before.MemberStatusAPIKeySetAt, after.MemberStatusAPIKeySetAt)
	}
	if len(after.MemberStatusAPIAllowedOrigins) != 1 || after.MemberStatusAPIAllowedOrigins[0] != "https://surveys.example.com" {
		t.Errorf("origins: got %v", after.MemberStatusAPIAllowedOrigins)
	}
}
