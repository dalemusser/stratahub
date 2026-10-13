package mhsactivitydays

import (
	"testing"
	"time"

	"github.com/dalemusser/stratahub/internal/domain/models"
	"github.com/dalemusser/stratahub/internal/testutil"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

func day(ws, org, user primitive.ObjectID, d string, played bool) models.MHSActivityDay {
	return models.MHSActivityDay{WorkspaceID: ws, UserID: user, OrganizationID: org, Role: models.MHSActivityRoleMember, Day: d, TZ: "America/Chicago", Opened: true, Played: played, ComputedAt: time.Now().UTC(), Version: models.MHSActivityVersion}
}

func TestReplaceDayIsIdempotentAndDropsStaleRows(t *testing.T) {
	db := testutil.SetupTestDB(t)
	ctx, cancel := testutil.TestContext()
	defer cancel()
	s := New(db)
	ws, orgA, orgB := primitive.NewObjectID(), primitive.NewObjectID(), primitive.NewObjectID()
	u1, u2, u3 := primitive.NewObjectID(), primitive.NewObjectID(), primitive.NewObjectID()

	docs := []models.MHSActivityDay{day(ws, orgA, u1, "2026-10-12", true), day(ws, orgA, u2, "2026-10-12", false), day(ws, orgB, u3, "2026-10-12", true)}
	if err := s.ReplaceDay(ctx, ws, "2026-10-12", []primitive.ObjectID{orgA, orgB}, docs); err != nil {
		t.Fatal(err)
	}
	got, err := s.ListDays(ctx, DaysQuery{WorkspaceID: ws, From: "2026-10-12", To: "2026-10-12"})
	if err != nil || len(got) != 3 || !got[0].Final {
		t.Fatalf("after first replace: %v %d", err, len(got))
	}
	// The same input again: still three.
	if err := s.ReplaceDay(ctx, ws, "2026-10-12", []primitive.ObjectID{orgA, orgB}, docs); err != nil {
		t.Fatal(err)
	}
	got, _ = s.ListDays(ctx, DaysQuery{WorkspaceID: ws, From: "2026-10-12", To: "2026-10-12"})
	if len(got) != 3 {
		t.Fatalf("after second replace: %d", len(got))
	}
	// Recomputing org A without u2 removes u2's row and leaves org B alone.
	if err := s.ReplaceDay(ctx, ws, "2026-10-12", []primitive.ObjectID{orgA}, docs[:1]); err != nil {
		t.Fatal(err)
	}
	got, _ = s.ListDays(ctx, DaysQuery{WorkspaceID: ws, From: "2026-10-12", To: "2026-10-12"})
	if len(got) != 2 {
		t.Fatalf("after narrowing: %d", len(got))
	}
	// Org filter and user filter.
	got, _ = s.ListDays(ctx, DaysQuery{WorkspaceID: ws, From: "2026-10-01", To: "2026-10-31", OrgIDs: []primitive.ObjectID{orgB}})
	if len(got) != 1 || got[0].UserID != u3 {
		t.Errorf("org filter %+v", got)
	}
	got, _ = s.ListDays(ctx, DaysQuery{WorkspaceID: ws, UserIDs: []primitive.ObjectID{u1}})
	if len(got) != 1 || got[0].UserID != u1 {
		t.Errorf("user filter %+v", got)
	}
	// An empty recompute of org B clears it.
	if err := s.ReplaceDay(ctx, ws, "2026-10-12", []primitive.ObjectID{orgB}, nil); err != nil {
		t.Fatal(err)
	}
	got, _ = s.ListDays(ctx, DaysQuery{WorkspaceID: ws})
	if len(got) != 1 {
		t.Errorf("after clearing org B: %d", len(got))
	}
}

func TestMarksAndDeleteRange(t *testing.T) {
	db := testutil.SetupTestDB(t)
	ctx, cancel := testutil.TestContext()
	defer cancel()
	s := New(db)
	ws, org := primitive.NewObjectID(), primitive.NewObjectID()
	m := models.MHSActivityMark{WorkspaceID: ws, OrganizationID: org, Day: "2026-10-12", TZ: "America/Chicago", Final: false, Partial: true, Attempts: 1, Version: 1}
	if err := s.SetMark(ctx, m); err != nil {
		t.Fatal(err)
	}
	m.Final, m.Partial, m.Attempts = true, false, 2
	if err := s.SetMark(ctx, m); err != nil {
		t.Fatal(err)
	}
	marks, err := s.GetMarks(ctx, ws, "2026-10-12", "2026-10-12", []primitive.ObjectID{org})
	if err != nil || len(marks) != 1 {
		t.Fatalf("marks %v %d", err, len(marks))
	}
	got := marks[MarkKey(org, "2026-10-12")]
	if !got.Final || got.Partial || got.Attempts != 2 || got.ComputedAt.IsZero() {
		t.Errorf("mark %+v", got)
	}
	if err := s.ReplaceDay(ctx, ws, "2026-10-12", []primitive.ObjectID{org}, []models.MHSActivityDay{day(ws, org, primitive.NewObjectID(), "2026-10-12", true)}); err != nil {
		t.Fatal(err)
	}
	if err := s.DeleteRange(ctx, ws, "2026-10-01", "2026-10-31"); err != nil {
		t.Fatal(err)
	}
	marks, _ = s.GetMarks(ctx, ws, "2026-10-01", "2026-10-31", nil)
	days, _ := s.ListDays(ctx, DaysQuery{WorkspaceID: ws})
	if len(marks) != 0 || len(days) != 0 {
		t.Errorf("after delete: %d marks %d days", len(marks), len(days))
	}
}
