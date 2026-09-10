package reports_test

import (
	"encoding/csv"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	uierrors "github.com/dalemusser/stratahub/internal/app/features/errors"
	"github.com/dalemusser/stratahub/internal/app/features/reports"
	"github.com/dalemusser/stratahub/internal/testutil"
	"go.uber.org/zap"
)

// TestServeMembersCSV_Identity checks that the identity selection controls
// exactly which columns the export carries: a de-identified file never
// contains a name, login, or email, and an identified file never contains a
// hex id.
func TestServeMembersCSV_Identity(t *testing.T) {
	db := testutil.SetupTestDB(t)
	logger := zap.NewNop()
	h := reports.NewHandler(db, uierrors.NewErrorLogger(logger), logger)
	fx := testutil.NewFixtures(t, db)
	ctx, cancel := testutil.TestContext()
	defer cancel()

	org := fx.CreateOrganization(ctx, "Hillsdale Middle School")
	group := fx.CreateGroup(ctx, "Fun Science", org.ID)
	leader := fx.CreateLeader(ctx, "Dale Musser", "dale@example.org", org.ID)
	member := fx.CreateMember(ctx, "Adrian Cole", "acole@students.example.org", org.ID)
	fx.CreateGroupMembership(ctx, leader.ID, group.ID, org.ID, "leader")
	fx.CreateGroupMembership(ctx, member.ID, group.ID, org.ID, "member")

	export := func(t *testing.T, queryString string) (string, [][]string) {
		t.Helper()
		req := testutil.NewAuthenticatedRequest("GET", "/reports/members.csv"+queryString, testutil.AdminUser())
		rec := httptest.NewRecorder()
		h.ServeMembersCSV(rec, req)
		if rec.Code != 200 {
			t.Fatalf("status %d, body: %s", rec.Code, rec.Body.String())
		}
		body := strings.TrimPrefix(rec.Body.String(), "\ufeff")
		rows, err := csv.NewReader(strings.NewReader(body)).ReadAll()
		if err != nil {
			t.Fatalf("parse CSV: %v", err)
		}
		if len(rows) != 2 {
			t.Fatalf("want header + 1 row, got %d rows: %v", len(rows), rows)
		}
		return rec.Header().Get("Content-Disposition"), rows
	}

	hexValues := []string{member.ID.Hex(), org.ID.Hex(), group.ID.Hex()}
	humanValues := []string{"Adrian Cole", "acole@students.example.org", "Hillsdale Middle School", "Fun Science", "Dale Musser"}
	mustContain := func(t *testing.T, row []string, values []string) {
		t.Helper()
		joined := strings.Join(row, "\n")
		for _, v := range values {
			if !strings.Contains(joined, v) {
				t.Errorf("row %v should contain %q", row, v)
			}
		}
	}
	mustNotContain := func(t *testing.T, row []string, values []string) {
		t.Helper()
		joined := strings.Join(row, "\n")
		for _, v := range values {
			if strings.Contains(joined, v) {
				t.Errorf("row %v must not contain %q", row, v)
			}
		}
	}

	t.Run("deidentified", func(t *testing.T) {
		disposition, rows := export(t, "?identity=deidentified")
		want := []string{"workspace_id", "user_id", "organization_id", "group_id", "status"}
		if !reflect.DeepEqual(rows[0], want) {
			t.Errorf("header = %v, want %v", rows[0], want)
		}
		if rows[1][1] != member.ID.Hex() || rows[1][4] != "active" {
			t.Errorf("row = %v", rows[1])
		}
		mustContain(t, rows[1], hexValues)
		mustNotContain(t, rows[1], humanValues)
		if !strings.Contains(disposition, "members_deidentified_") {
			t.Errorf("default filename should name the selection: %s", disposition)
		}
	})

	t.Run("identified", func(t *testing.T) {
		disposition, rows := export(t, "?identity=identified")
		want := []string{"workspace", "full_name", "login_id", "email", "organization", "group", "leaders", "status"}
		if !reflect.DeepEqual(rows[0], want) {
			t.Errorf("header = %v, want %v", rows[0], want)
		}
		mustContain(t, rows[1], humanValues)
		mustNotContain(t, rows[1], hexValues)
		if !strings.Contains(disposition, "members_identified_") {
			t.Errorf("default filename should name the selection: %s", disposition)
		}
	})

	t.Run("both is the default and the full crosswalk", func(t *testing.T) {
		want := []string{"workspace", "workspace_id", "user_id", "full_name", "login_id", "email", "organization", "organization_id", "group", "group_id", "leaders", "status"}
		for _, qs := range []string{"", "?identity=both", "?identity=nonsense"} {
			disposition, rows := export(t, qs)
			if !reflect.DeepEqual(rows[0], want) {
				t.Errorf("%q: header = %v, want %v", qs, rows[0], want)
			}
			mustContain(t, rows[1], hexValues)
			mustContain(t, rows[1], humanValues)
			if strings.Contains(disposition, "_deidentified_") || strings.Contains(disposition, "_identified_") {
				t.Errorf("%q: the default keeps the plain filename: %s", qs, disposition)
			}
		}
	})

	t.Run("a caller-supplied filename is kept as given", func(t *testing.T) {
		disposition, _ := export(t, "?identity=deidentified&filename=roster")
		if !strings.Contains(disposition, `filename="roster.csv"`) {
			t.Errorf("disposition = %s", disposition)
		}
	})
}
