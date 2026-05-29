package resourceurl

import (
	"net/url"
	"testing"

	"github.com/dalemusser/stratahub/internal/domain/models"
)

func fullCtx() IdentityContext {
	return IdentityContext{
		WorkspaceSubdomain: "mhs",
		WorkspaceID:        "695f5a3fa323f290a63b3fce",
		OrgName:            "Intelligence Builders",
		OrgID:              "69a1000000000000000000a1",
		GroupName:          "Earth Science",
		GroupID:            "69cdc800d385e3b94310f6e2",
		UserName:           "Jane Doe",
		UserID:             "69b7b2328cac2be5f60efb09",
		LoginID:            "jane@school.org",
	}
}

func TestBuildLaunchURL_Modes(t *testing.T) {
	const base = "https://survey.example.com/start"
	ctx := fullCtx()

	tests := []struct {
		name       string
		mode       string
		wantParams map[string]string
		wantNone   bool // expect URL returned unchanged (no query added)
	}{
		{name: "none", mode: models.URLIdentityNone, wantNone: true},
		{name: "empty means none", mode: "", wantNone: true},
		{name: "unrecognized means none", mode: "bogus", wantNone: true},
		{
			name: "hex",
			mode: models.URLIdentityHex,
			wantParams: map[string]string{
				"ws_id":    ctx.WorkspaceID,
				"org_id":   ctx.OrgID,
				"group_id": ctx.GroupID,
				"user_id":  ctx.UserID,
			},
		},
		{
			name: "human",
			mode: models.URLIdentityHuman,
			wantParams: map[string]string{
				"ws":       ctx.WorkspaceSubdomain,
				"org":      ctx.OrgName,
				"group":    ctx.GroupName,
				"user":     ctx.UserName,
				"login_id": ctx.LoginID,
			},
		},
		{
			name: "both",
			mode: models.URLIdentityBoth,
			wantParams: map[string]string{
				"ws":       ctx.WorkspaceSubdomain,
				"ws_id":    ctx.WorkspaceID,
				"org":      ctx.OrgName,
				"org_id":   ctx.OrgID,
				"group":    ctx.GroupName,
				"group_id": ctx.GroupID,
				"user":     ctx.UserName,
				"user_id":  ctx.UserID,
				"login_id": ctx.LoginID,
			},
		},
		{
			name: "legacy uses id=login_id, no user_id, no workspace",
			mode: models.URLIdentityLegacy,
			wantParams: map[string]string{
				"id":    ctx.LoginID,
				"org":   ctx.OrgName,
				"group": ctx.GroupName,
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := BuildLaunchURL(base, tc.mode, ctx)

			if tc.wantNone {
				if got != base {
					t.Fatalf("mode %q: expected URL unchanged %q, got %q", tc.mode, base, got)
				}
				return
			}

			u, err := url.Parse(got)
			if err != nil {
				t.Fatalf("result not parseable: %v", err)
			}
			q := u.Query()

			for k, want := range tc.wantParams {
				if q.Get(k) != want {
					t.Errorf("param %q = %q, want %q", k, q.Get(k), want)
				}
			}
			if len(q) != len(tc.wantParams) {
				t.Errorf("got %d params %v, want %d %v", len(q), q, len(tc.wantParams), tc.wantParams)
			}
		})
	}
}

// Legacy must never emit user_id, and must carry the login under the "id" key.
func TestBuildLaunchURL_LegacyContract(t *testing.T) {
	got := BuildLaunchURL("https://x.test/p", models.URLIdentityLegacy, fullCtx())
	u, _ := url.Parse(got)
	q := u.Query()
	if q.Has("user_id") {
		t.Errorf("legacy must not emit user_id; it uses id=login_id only: %s", got)
	}
	if q.Get("id") != "jane@school.org" {
		t.Errorf("legacy must emit id=login_id: %s", got)
	}
	if q.Has("ws") || q.Has("ws_id") {
		t.Errorf("legacy must not emit workspace (predates workspaces): %s", got)
	}
}

// Empty context values are omitted rather than emitted as empty params.
func TestBuildLaunchURL_OmitsEmptyValues(t *testing.T) {
	ctx := IdentityContext{UserID: "69b7b2328cac2be5f60efb09"} // only user_id set
	got := BuildLaunchURL("https://x.test/p", models.URLIdentityHex, ctx)
	u, _ := url.Parse(got)
	q := u.Query()
	if q.Get("user_id") != ctx.UserID {
		t.Errorf("expected user_id present, got %s", got)
	}
	for _, empty := range []string{"ws_id", "org_id", "group_id"} {
		if q.Has(empty) {
			t.Errorf("expected %q omitted (empty context value), got %s", empty, got)
		}
	}
}

// An empty base URL stays empty regardless of mode.
func TestBuildLaunchURL_EmptyBase(t *testing.T) {
	if got := BuildLaunchURL("", models.URLIdentityHex, fullCtx()); got != "" {
		t.Errorf("empty base should stay empty, got %q", got)
	}
}

func TestHasPII(t *testing.T) {
	clean := []string{models.URLIdentityNone, models.URLIdentityHex, ""}
	pii := []string{models.URLIdentityHuman, models.URLIdentityBoth, models.URLIdentityLegacy}
	for _, m := range clean {
		if HasPII(m) {
			t.Errorf("mode %q should be PII-free", m)
		}
	}
	for _, m := range pii {
		if !HasPII(m) {
			t.Errorf("mode %q should be flagged as carrying PII", m)
		}
	}
}
