package settings

import (
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	uierrors "github.com/dalemusser/stratahub/internal/app/features/errors"
	_ "github.com/dalemusser/stratahub/internal/app/features/missionhydrosci" // registers a template func the shared layout uses
	appresources "github.com/dalemusser/stratahub/internal/app/resources"
	"github.com/dalemusser/stratahub/internal/app/system/auth"
	"github.com/dalemusser/stratahub/internal/app/system/workspace"
	"github.com/dalemusser/stratahub/internal/domain/models"
	"github.com/dalemusser/stratahub/internal/testutil"
	"github.com/dalemusser/waffle/pantry/templates"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.uber.org/zap"
)

var bootTemplatesOnce sync.Once

// bootTemplates boots the template engine once with the shared layout and the
// settings templates, so a syntax error in settings.gohtml fails here instead
// of at the first request in production.
func bootTemplates(t *testing.T) {
	t.Helper()
	var bootErr error
	bootTemplatesOnce.Do(func() {
		appresources.LoadSharedTemplates()
		eng := templates.New(false)
		if err := eng.Boot(zap.NewNop()); err != nil {
			bootErr = err
			return
		}
		templates.UseEngine(eng, zap.NewNop())
	})
	if bootErr != nil {
		t.Fatalf("template engine boot failed: %v", bootErr)
	}
}

// TestSettingsPageRenders executes the real settings template with a populated
// view model — including the Member Status API section — and checks the
// output, catching runtime template errors (nil derefs, missing fields) that
// parsing alone would not.
func TestSettingsPageRenders(t *testing.T) {
	bootTemplates(t)
	db := testutil.SetupTestDB(t)
	h := NewHandler(db, nil, uierrors.NewErrorLogger(zap.NewNop()), nil, zap.NewNop())

	wsID := primitive.NewObjectID()
	userID := primitive.NewObjectID()
	req := httptest.NewRequest("GET", "/settings", nil)
	req.Host = "mhs.example.com"
	req = workspace.WithTestWorkspace(req, wsID, "mhs", "MHS")
	req = auth.WithTestUser(req, &auth.SessionUser{ID: userID.Hex(), Name: "Admin", LoginID: "admin@example.com", Role: "admin"})

	setAt := time.Date(2026, 8, 26, 14, 30, 0, 0, time.UTC)
	settings := models.SiteSettings{
		SiteName:                      "MHS",
		MemberStatusAPIKey:            "ms_render_test_key_0123456789",
		MemberStatusAPIKeySetAt:       &setAt,
		MemberStatusAPIAllowedOrigins: []string{"https://surveys.example.com", "http://localhost:3000"},
	}

	for _, tc := range []struct {
		name     string
		settings models.SiteSettings
		want     []string
	}{
		{
			name:     "with key",
			settings: settings,
			want: []string{
				"Member Status API",
				`value="ms_render_test_key_0123456789"`,
				"Set on Aug 26, 2026 14:30 UTC",
				"POST https://mhs.example.com/api/member-status",
				"Allowed browser origins",
				"https://surveys.example.com\nhttp://localhost:3000</textarea>",
			},
		},
		{
			name:     "without key",
			settings: models.SiteSettings{SiteName: "MHS"},
			want:     []string{"Member Status API", "No key is set.", `name="member_status_api_allowed_origins"`, "></textarea>"},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			vm := h.buildVM(req.Context(), req, userID, tc.settings, "")
			rec := httptest.NewRecorder()
			templates.Render(rec, req, "settings", vm)
			if rec.Code != 200 {
				t.Fatalf("status %d; body: %s", rec.Code, rec.Body.String())
			}
			body := rec.Body.String()
			for _, want := range tc.want {
				if !strings.Contains(body, want) {
					t.Errorf("rendered page missing %q", want)
				}
			}
		})
	}
}

func TestMemberStatusEndpoint(t *testing.T) {
	cases := []struct {
		host, proto string
		tls         bool
		want        string
	}{
		{"hub.example.org", "", false, "https://hub.example.org/api/member-status"},
		{"localhost:8080", "", false, "http://localhost:8080/api/member-status"},
		{"127.0.0.1:8080", "", false, "http://127.0.0.1:8080/api/member-status"},
		{"localhost:8080", "https", false, "https://localhost:8080/api/member-status"},
	}
	for _, tc := range cases {
		req := httptest.NewRequest("GET", "/settings", nil)
		req.Host = tc.host
		if tc.proto != "" {
			req.Header.Set("X-Forwarded-Proto", tc.proto)
		}
		if got := memberStatusEndpoint(req); got != tc.want {
			t.Errorf("host %q proto %q: got %q, want %q", tc.host, tc.proto, got, tc.want)
		}
	}
}

func TestValidateMemberStatusKey(t *testing.T) {
	if msg := validateMemberStatusKey(""); msg != "" {
		t.Errorf("empty key should be accepted, got %q", msg)
	}
	if msg := validateMemberStatusKey("ms_0123456789abcdefghijklmnopqrstuvwxyzABCDEF"); msg != "" {
		t.Errorf("generated-shape key should be accepted, got %q", msg)
	}
	for name, bad := range map[string]string{
		"short":  "abc",
		"long":   strings.Repeat("k", 129),
		"spaces": "ms_with a space 0123456789",
		"tab":    "ms_with\ttab_0123456789",
	} {
		if msg := validateMemberStatusKey(bad); msg == "" {
			t.Errorf("%s: should be rejected", name)
		}
	}
}
