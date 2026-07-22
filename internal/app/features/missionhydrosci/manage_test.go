// internal/app/features/missionhydrosci/manage_test.go
package missionhydrosci

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	settingsstore "github.com/dalemusser/stratahub/internal/app/store/settings"
	"github.com/dalemusser/stratahub/internal/app/system/staffauth"
	"github.com/dalemusser/stratahub/internal/app/system/workspace"
	"github.com/dalemusser/stratahub/internal/domain/models"
	"github.com/dalemusser/stratahub/internal/testutil"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.uber.org/zap"
)

// newManageTestHandler builds a Handler with just the pieces the unlock/auth
// paths need, plus a keyword-mode workspace. Keyword mode avoids needing the
// staff-auth verifier in tests.
func newManageTestHandler(t *testing.T) (*Handler, primitive.ObjectID, *mongo.Database) {
	t.Helper()
	db := testutil.SetupTestDB(t)

	h := &Handler{
		DB:            db,
		Log:           zap.NewNop(),
		SettingsStore: settingsstore.New(db),
		UnlockStore:   staffauth.NewUnlockStore(db),
	}

	wsID := primitive.NewObjectID()
	ctx, cancel := testutil.TestContext()
	defer cancel()
	err := h.SettingsStore.Save(ctx, wsID, models.SiteSettings{
		SiteName:              "Test",
		MHSMemberAuth:         "keyword",
		MHSMemberAuthKeyword:  "open-sesame",
		MHSStaffUnlockMinutes: 10,
	})
	if err != nil {
		t.Fatalf("failed to save settings: %v", err)
	}
	return h, wsID, db
}

// memberRequest builds a request carrying a member session and workspace context.
func memberRequest(method, target string, user testutil.TestUser, wsID primitive.ObjectID, body []byte) *http.Request {
	var req *http.Request
	if body != nil {
		req = httptest.NewRequest(method, target, bytes.NewReader(body))
	} else {
		req = httptest.NewRequest(method, target, nil)
	}
	req = testutil.WithUser(req, user)
	return workspace.WithTestWorkspace(req, wsID, "test", "Test")
}

func TestCheckMemberAuth_KeywordGrantsAndHonorsUnlock(t *testing.T) {
	h, wsID, _ := newManageTestHandler(t)
	member := testutil.MemberUser(primitive.NewObjectID())

	// Wrong keyword → 403, no unlock created
	req := memberRequest(http.MethodPost, "/x", member, wsID, nil)
	if status, _ := h.checkMemberAuth(req, member.Role, "", "wrong"); status != http.StatusForbidden {
		t.Errorf("wrong keyword: status = %d, want 403", status)
	}
	key, _, _, _ := h.unlockKey(req)
	if u, _ := h.UnlockStore.GetActive(req.Context(), key); u != nil {
		t.Fatal("unlock should not exist after failed auth")
	}

	// No credentials, no unlock → 403
	if status, _ := h.checkMemberAuth(req, member.Role, "", ""); status != http.StatusForbidden {
		t.Errorf("no creds: status = %d, want 403", status)
	}

	// Correct keyword → authorized, unlock granted as side effect
	if status, msg := h.checkMemberAuth(req, member.Role, "", "open-sesame"); status != 0 {
		t.Fatalf("correct keyword: status = %d (%s), want 0", status, msg)
	}
	unlock, err := h.UnlockStore.GetActive(req.Context(), key)
	if err != nil || unlock == nil {
		t.Fatalf("expected unlock after successful auth (err=%v)", err)
	}
	if unlock.GrantedBy != "keyword" {
		t.Errorf("GrantedBy = %q, want %q", unlock.GrantedBy, "keyword")
	}
	firstExpiry := unlock.ExpiresAt

	// Subsequent action with NO credentials passes via the unlock and slides expiry
	if status, msg := h.checkMemberAuth(req, member.Role, "", ""); status != 0 {
		t.Fatalf("unlocked, no creds: status = %d (%s), want 0", status, msg)
	}
	refreshed, _ := h.UnlockStore.GetActive(req.Context(), key)
	if refreshed == nil {
		t.Fatal("unlock disappeared after refresh")
	}
	if refreshed.ExpiresAt.Before(firstExpiry) {
		t.Error("sliding expiry moved backwards")
	}

	// Lock → credentials required again
	if err := h.UnlockStore.Revoke(req.Context(), key); err != nil {
		t.Fatalf("revoke failed: %v", err)
	}
	if status, _ := h.checkMemberAuth(req, member.Role, "", ""); status != http.StatusForbidden {
		t.Errorf("after revoke: status = %d, want 403", status)
	}
}

func TestCheckMemberAuth_NonMembersAndTrustPass(t *testing.T) {
	h, wsID, _ := newManageTestHandler(t)

	// Staff roles pass with no credentials and never create unlocks
	admin := testutil.AdminUser()
	req := memberRequest(http.MethodPost, "/x", admin, wsID, nil)
	if status, _ := h.checkMemberAuth(req, admin.Role, "", ""); status != 0 {
		t.Errorf("admin: status = %d, want 0", status)
	}

	// Trust workspace: members pass with no credentials
	trustWS := primitive.NewObjectID()
	ctx, cancel := testutil.TestContext()
	defer cancel()
	if err := h.SettingsStore.Save(ctx, trustWS, models.SiteSettings{SiteName: "T", MHSMemberAuth: "trust"}); err != nil {
		t.Fatalf("save settings: %v", err)
	}
	member := testutil.MemberUser(primitive.NewObjectID())
	req = memberRequest(http.MethodPost, "/x", member, trustWS, nil)
	if status, _ := h.checkMemberAuth(req, member.Role, "", ""); status != 0 {
		t.Errorf("trust member: status = %d, want 0", status)
	}
}

func TestManageUnlockLockStatusEndpoints(t *testing.T) {
	h, wsID, _ := newManageTestHandler(t)
	member := testutil.MemberUser(primitive.NewObjectID())

	// Status before unlock: gated, locked
	req := memberRequest(http.MethodGet, "/missionhydrosci/api/manage/status", member, wsID, nil)
	rec := httptest.NewRecorder()
	h.ServeManageStatus(rec, req)
	var status map[string]any
	json.NewDecoder(rec.Body).Decode(&status)
	if status["gated"] != true || status["unlocked"] != false {
		t.Errorf("pre-unlock status = %v, want gated+locked", status)
	}

	// Unlock with wrong keyword → 403
	body, _ := json.Marshal(map[string]string{"keyword": "nope"})
	req = memberRequest(http.MethodPost, "/missionhydrosci/api/manage/unlock", member, wsID, body)
	rec = httptest.NewRecorder()
	h.HandleManageUnlock(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Errorf("bad keyword unlock: status = %d, want 403", rec.Code)
	}

	// Unlock with correct keyword → ok with expiry info
	body, _ = json.Marshal(map[string]string{"keyword": "open-sesame"})
	req = memberRequest(http.MethodPost, "/missionhydrosci/api/manage/unlock", member, wsID, body)
	rec = httptest.NewRecorder()
	h.HandleManageUnlock(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("unlock: status = %d, want 200 (body: %s)", rec.Code, rec.Body.String())
	}
	var unlockResp map[string]any
	json.NewDecoder(rec.Body).Decode(&unlockResp)
	if unlockResp["ok"] != true {
		t.Error("unlock response missing ok:true")
	}
	if unlockResp["granted_by"] != "keyword" {
		t.Errorf("granted_by = %v, want keyword", unlockResp["granted_by"])
	}
	if exp, ok := unlockResp["expires_at_ms"].(float64); !ok || exp <= 0 {
		t.Errorf("expires_at_ms = %v, want positive number", unlockResp["expires_at_ms"])
	}

	// Status now: unlocked
	req = memberRequest(http.MethodGet, "/missionhydrosci/api/manage/status", member, wsID, nil)
	rec = httptest.NewRecorder()
	h.ServeManageStatus(rec, req)
	status = nil
	json.NewDecoder(rec.Body).Decode(&status)
	if status["unlocked"] != true {
		t.Errorf("post-unlock status = %v, want unlocked", status)
	}

	// Lock now → status back to locked
	req = memberRequest(http.MethodPost, "/missionhydrosci/api/manage/lock", member, wsID, []byte(`{}`))
	rec = httptest.NewRecorder()
	h.HandleManageLock(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("lock: status = %d, want 200", rec.Code)
	}
	req = memberRequest(http.MethodGet, "/missionhydrosci/api/manage/status", member, wsID, nil)
	rec = httptest.NewRecorder()
	h.ServeManageStatus(rec, req)
	status = nil
	json.NewDecoder(rec.Body).Decode(&status)
	if status["unlocked"] != false {
		t.Errorf("post-lock status = %v, want locked", status)
	}

	// Staff are never gated
	admin := testutil.AdminUser()
	req = memberRequest(http.MethodGet, "/missionhydrosci/api/manage/status", admin, wsID, nil)
	rec = httptest.NewRecorder()
	h.ServeManageStatus(rec, req)
	status = nil
	json.NewDecoder(rec.Body).Decode(&status)
	if status["gated"] != false {
		t.Errorf("admin status = %v, want gated:false", status)
	}
}

func TestUnlockIsSessionScoped_DifferentUserBlocked(t *testing.T) {
	h, wsID, _ := newManageTestHandler(t)

	memberA := testutil.MemberUser(primitive.NewObjectID())
	reqA := memberRequest(http.MethodPost, "/x", memberA, wsID, nil)
	if status, _ := h.checkMemberAuth(reqA, memberA.Role, "", "open-sesame"); status != 0 {
		t.Fatal("member A auth should succeed")
	}

	// A different member in the same workspace does not inherit A's unlock
	memberB := testutil.MemberUser(primitive.NewObjectID())
	reqB := memberRequest(http.MethodPost, "/x", memberB, wsID, nil)
	if status, _ := h.checkMemberAuth(reqB, memberB.Role, "", ""); status != http.StatusForbidden {
		t.Errorf("member B without creds: status = %d, want 403", status)
	}
}
