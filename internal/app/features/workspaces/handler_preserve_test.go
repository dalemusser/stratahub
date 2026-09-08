package workspaces_test

import (
	"bytes"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"

	settingsstore "github.com/dalemusser/stratahub/internal/app/store/settings"
	workspacestore "github.com/dalemusser/stratahub/internal/app/store/workspaces"
	"github.com/dalemusser/stratahub/internal/app/system/auth"
	"github.com/dalemusser/stratahub/internal/domain/models"
	"github.com/dalemusser/stratahub/internal/testutil"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

// multipartForm builds a multipart/form-data body from simple string fields.
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

// TestHandleSettings_PreservesFieldsNotOnForm guards against the apex
// workspace-settings form blanking site settings it does not carry (landing
// page, MHS member auth, active MHS collection, AI summaries) and against the
// workspace update clearing the workspace's own logo/status fields.
func TestHandleSettings_PreservesFieldsNotOnForm(t *testing.T) {
	handler := newTestHandler(t)
	ctx, cancel := testutil.TestContext()
	defer cancel()

	wsStore := workspacestore.New(handler.DB)
	ws, err := wsStore.Create(ctx, models.Workspace{
		Name:      "Old Workspace",
		Subdomain: "oldws",
		LogoPath:  "logos/old.png",
		LogoName:  "old.png",
	})
	if err != nil {
		t.Fatalf("seed workspace: %v", err)
	}

	store := settingsstore.New(handler.DB)
	collID := primitive.NewObjectID()
	seed := models.SiteSettings{
		SiteName:                      "Old Site",
		LandingTitle:                  "Old Landing",
		LandingContent:                "<p>Old content</p>",
		MHSMemberAuth:                 "keyword",
		MHSMemberAuthKeyword:          "open-sesame",
		MHSStaffUnlockMinutes:         30,
		MHSActiveCollectionID:         &collID,
		EnableClaudeSummaries:         true,
		ClaudeModel:                   "claude-haiku-4-5-20251001",
		MemberStatusAPIKey:            "ms_seeded_key_0123456789abcdef",
		MemberStatusAPIAllowedOrigins: []string{"https://surveys.example.com"},
	}
	if err := store.Save(ctx, ws.ID, seed); err != nil {
		t.Fatalf("seed settings: %v", err)
	}

	body, contentType := multipartForm(t, map[string][]string{
		"workspace_name": {"New Workspace"},
		"subdomain":      {"newws"},
		"site_name":      {"New Site"},
		"auth_methods":   {"google"},
	})

	req := httptest.NewRequest("POST", "/workspaces/"+ws.ID.Hex()+"/settings", body)
	req.Header.Set("Content-Type", contentType)
	req = testutil.WithChiURLParam(req, "id", ws.ID.Hex())
	req = auth.WithTestUser(req, &auth.SessionUser{
		ID:      primitive.NewObjectID().Hex(),
		Name:    "Super Admin",
		LoginID: "super@example.com",
		Role:    "superadmin",
	})
	rec := httptest.NewRecorder()

	handler.HandleSettings(rec, req)

	if rec.Code != http.StatusSeeOther {
		t.Fatalf("status: got %d, want %d (body: %s)", rec.Code, http.StatusSeeOther, rec.Body.String())
	}

	saved, err := store.Get(ctx, ws.ID)
	if err != nil {
		t.Fatalf("Get settings: %v", err)
	}

	// Fields the form carries are updated.
	if saved.SiteName != "New Site" {
		t.Errorf("SiteName: got %q, want %q", saved.SiteName, "New Site")
	}
	if len(saved.EnabledAuthMethods) != 1 || saved.EnabledAuthMethods[0] != "google" {
		t.Errorf("EnabledAuthMethods: got %v, want [google]", saved.EnabledAuthMethods)
	}

	// Fields the form does not carry survive the save.
	if saved.LandingTitle != "Old Landing" {
		t.Errorf("LandingTitle: got %q, want %q (must be preserved)", saved.LandingTitle, "Old Landing")
	}
	if saved.LandingContent != "<p>Old content</p>" {
		t.Errorf("LandingContent: got %q, want preserved", saved.LandingContent)
	}
	if saved.MHSMemberAuth != "keyword" || saved.MHSMemberAuthKeyword != "open-sesame" {
		t.Errorf("MHS member auth: got %q/%q, want keyword/open-sesame (must be preserved)", saved.MHSMemberAuth, saved.MHSMemberAuthKeyword)
	}
	if saved.MHSStaffUnlockMinutes != 30 {
		t.Errorf("MHSStaffUnlockMinutes: got %d, want 30 (must be preserved)", saved.MHSStaffUnlockMinutes)
	}
	if saved.MHSActiveCollectionID == nil || *saved.MHSActiveCollectionID != collID {
		t.Errorf("MHSActiveCollectionID: got %v, want %s (must be preserved)", saved.MHSActiveCollectionID, collID.Hex())
	}
	if !saved.EnableClaudeSummaries || saved.ClaudeModel != "claude-haiku-4-5-20251001" {
		t.Errorf("Claude settings: got %v/%q, want true/claude-haiku-4-5-20251001 (must be preserved)", saved.EnableClaudeSummaries, saved.ClaudeModel)
	}
	if saved.MemberStatusAPIKey != "ms_seeded_key_0123456789abcdef" {
		t.Errorf("MemberStatusAPIKey: got %q, want the seeded key (must be preserved)", saved.MemberStatusAPIKey)
	}
	if len(saved.MemberStatusAPIAllowedOrigins) != 1 || saved.MemberStatusAPIAllowedOrigins[0] != "https://surveys.example.com" {
		t.Errorf("MemberStatusAPIAllowedOrigins: got %v, want [https://surveys.example.com] (must be preserved)", saved.MemberStatusAPIAllowedOrigins)
	}

	// The workspace record: name/subdomain updated, everything else intact.
	updated, err := wsStore.GetByID(ctx, ws.ID)
	if err != nil {
		t.Fatalf("GetByID workspace: %v", err)
	}
	if updated.Name != "New Workspace" || updated.Subdomain != "newws" {
		t.Errorf("workspace name/subdomain: got %q/%q, want New Workspace/newws", updated.Name, updated.Subdomain)
	}
	if updated.LogoPath != "logos/old.png" || updated.LogoName != "old.png" {
		t.Errorf("workspace logo: got %q/%q, want logos/old.png/old.png (must be preserved)", updated.LogoPath, updated.LogoName)
	}
	if updated.Status != ws.Status {
		t.Errorf("workspace status: got %q, want %q (must be preserved)", updated.Status, ws.Status)
	}
}

// TestHandleSettings_UnknownWorkspace verifies that a save against a workspace
// that does not exist fails before any settings document is written.
func TestHandleSettings_UnknownWorkspace(t *testing.T) {
	handler := newTestHandler(t)
	ctx, cancel := testutil.TestContext()
	defer cancel()

	wsID := primitive.NewObjectID()
	body, contentType := multipartForm(t, map[string][]string{
		"workspace_name": {"Ghost"},
		"subdomain":      {"ghost"},
		"site_name":      {"Ghost Site"},
		"auth_methods":   {"google"},
	})

	req := httptest.NewRequest("POST", "/workspaces/"+wsID.Hex()+"/settings", body)
	req.Header.Set("Content-Type", contentType)
	req = testutil.WithChiURLParam(req, "id", wsID.Hex())
	rec := httptest.NewRecorder()

	handler.HandleSettings(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status: got %d, want %d", rec.Code, http.StatusNotFound)
	}
	exists, err := settingsstore.New(handler.DB).Exists(ctx, wsID)
	if err != nil {
		t.Fatalf("Exists: %v", err)
	}
	if exists {
		t.Error("settings document was written for a workspace that does not exist")
	}
}
