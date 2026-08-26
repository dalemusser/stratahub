package settings_test

import (
	"bytes"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"

	settingsstore "github.com/dalemusser/stratahub/internal/app/store/settings"
	"github.com/dalemusser/stratahub/internal/app/system/auth"
	"github.com/dalemusser/stratahub/internal/app/system/workspace"
	"github.com/dalemusser/stratahub/internal/domain/models"
	"github.com/dalemusser/stratahub/internal/testutil"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

// multipartForm builds a multipart/form-data body from simple string fields.
// Repeated keys (e.g. auth_methods) are passed as multiple values.
func multipartForm(t *testing.T, fields map[string][]string) (*bytes.Buffer, string) {
	t.Helper()
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	for key, values := range fields {
		for _, v := range values {
			if err := mw.WriteField(key, v); err != nil {
				t.Fatalf("WriteField(%s): %v", key, err)
			}
		}
	}
	if err := mw.Close(); err != nil {
		t.Fatalf("multipart Close: %v", err)
	}
	return &buf, mw.FormDataContentType()
}

// TestHandleSettings_PreservesFieldsNotOnForm guards against the settings form
// blanking fields it does not carry. The store's Save writes every whitelisted
// field, so the handler must overlay the form onto the current document rather
// than build a fresh one. The active MHS collection is the canary: it is set
// from the MHS Builds pages and never appears on this form.
func TestHandleSettings_PreservesFieldsNotOnForm(t *testing.T) {
	handler := newTestHandler(t)
	store := settingsstore.New(handler.DB)
	ctx, cancel := testutil.TestContext()
	defer cancel()

	wsID := primitive.NewObjectID()
	collID := primitive.NewObjectID()

	seed := models.SiteSettings{
		SiteName:              "Old Site",
		LandingTitle:          "Old Landing",
		MHSMemberAuth:         "keyword",
		MHSMemberAuthKeyword:  "open-sesame",
		MHSStaffUnlockMinutes: 30,
		MHSActiveCollectionID: &collID,
		EnableClaudeSummaries: true,
		ClaudeModel:           "claude-haiku-4-5-20251001",
	}
	if err := store.Save(ctx, wsID, seed); err != nil {
		t.Fatalf("seed Save: %v", err)
	}

	body, contentType := multipartForm(t, map[string][]string{
		"site_name":               {"New Site"},
		"landing_title":           {"New Landing"},
		"auth_methods":            {"google"},
		"mhs_member_auth":         {"trust"},
		"enable_claude_summaries": {"1"},
		"claude_model":            {"claude-haiku-4-5-20251001"},
	})

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

	handler.HandleSettings(rec, req)

	if rec.Code != http.StatusSeeOther {
		t.Fatalf("status: got %d, want %d (body: %s)", rec.Code, http.StatusSeeOther, rec.Body.String())
	}

	saved, err := store.Get(ctx, wsID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}

	// Fields the form carries are updated.
	if saved.SiteName != "New Site" {
		t.Errorf("SiteName: got %q, want %q", saved.SiteName, "New Site")
	}
	if saved.LandingTitle != "New Landing" {
		t.Errorf("LandingTitle: got %q, want %q", saved.LandingTitle, "New Landing")
	}
	if saved.MHSMemberAuth != "trust" {
		t.Errorf("MHSMemberAuth: got %q, want %q", saved.MHSMemberAuth, "trust")
	}
	if len(saved.EnabledAuthMethods) != 1 || saved.EnabledAuthMethods[0] != "google" {
		t.Errorf("EnabledAuthMethods: got %v, want [google]", saved.EnabledAuthMethods)
	}

	// Fields the form does not carry survive the save.
	if saved.MHSActiveCollectionID == nil || *saved.MHSActiveCollectionID != collID {
		t.Errorf("MHSActiveCollectionID: got %v, want %s (must be preserved)", saved.MHSActiveCollectionID, collID.Hex())
	}
}
