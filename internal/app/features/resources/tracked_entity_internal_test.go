package resources

import (
	"testing"

	uierrors "github.com/dalemusser/stratahub/internal/app/features/errors"
	"github.com/dalemusser/stratahub/internal/app/store/memberstatus"
	"github.com/dalemusser/stratahub/internal/app/store/memberstatuslog"
	"github.com/dalemusser/stratahub/internal/app/system/memberstatuscfg"
	"github.com/dalemusser/stratahub/internal/domain/models"
	"github.com/dalemusser/stratahub/internal/testutil"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.uber.org/zap"
)

func TestTrackedEntityHelpers(t *testing.T) {
	opts := trackedEntityOptions()
	if len(opts) != 4 || opts[0].ID != "pre" || opts[0].Label != "Pre-Survey" || opts[3].ID != "post" {
		t.Errorf("options from embedded config: %+v", opts)
	}
	for _, id := range []string{"", "pre", "mhs", "ews", "post"} {
		if !isValidTrackedEntityID(id) {
			t.Errorf("%q should be valid", id)
		}
	}
	for _, id := range []string{"bogus", "Pre", "name:pre"} {
		if isValidTrackedEntityID(id) {
			t.Errorf("%q should be invalid", id)
		}
	}
	if got := trackedEntityLabel(""); got != "Not tracked" {
		t.Errorf("empty label: %q", got)
	}
	if got := trackedEntityLabel("mhs"); got != "MHS Engagement" {
		t.Errorf("mhs label: %q", got)
	}
	if got := trackedEntityLabel("gone"); got != "gone (no longer configured)" {
		t.Errorf("stale label: %q", got)
	}
}

func TestRecordSurveyOpened(t *testing.T) {
	db := testutil.SetupTestDB(t)
	ctx, cancel := testutil.TestContext()
	defer cancel()

	h := NewMemberHandler(db, nil, uierrors.NewErrorLogger(zap.NewNop()), nil, nil, nil, zap.NewNop())
	if h.SurveyConfig == nil || h.MemberStatus == nil || h.StatusLog == nil {
		t.Fatal("member handler did not initialize survey tracking")
	}

	wsID, orgID := primitive.NewObjectID(), primitive.NewObjectID()
	userID := primitive.NewObjectID()
	store := memberstatus.New(db)
	logStore := memberstatuslog.New(db)
	logCount := func() int64 {
		n, err := logStore.Count(ctx, memberstatuslog.ListQuery{WorkspaceID: wsID})
		if err != nil {
			t.Fatal(err)
		}
		return n
	}

	// Linked resource → "opened" recorded under the canonical key with the
	// launch source, and one log entry describing it.
	linked := models.Resource{ID: primitive.NewObjectID(), Title: "Pre survey link", TrackedEntityID: "pre"}
	h.recordSurveyOpened(ctx, userID, &wsID, &orgID, linked)
	doc, err := store.Get(ctx, wsID, userID, "pre")
	if err != nil {
		t.Fatalf("expected an opened document: %v", err)
	}
	if doc.State != models.MemberStatusOpened || doc.OpenedAt == nil || doc.Source != models.MemberStatusSourceLaunch || doc.Entity != "Pre-Survey" {
		t.Errorf("doc: %+v", doc)
	}
	entries, _, err := logStore.List(ctx, memberstatuslog.ListQuery{WorkspaceID: wsID, Source: models.MemberStatusSourceLaunch})
	if err != nil || len(entries) != 1 {
		t.Fatalf("launch log entries: %d, %v", len(entries), err)
	}
	e := entries[0]
	if e.Request.ResourceID != linked.ID.Hex() || e.Request.UserID != userID.Hex() || e.Request.StateNorm != models.MemberStatusOpened ||
		e.Resolved.UserID == nil || *e.Resolved.UserID != userID || e.Resolved.OrganizationID == nil || *e.Resolved.OrganizationID != orgID ||
		e.Resolved.EntityKey != "pre" || e.Resolved.EntityTitle != "Pre-Survey" || !e.Resolved.KnownEntity ||
		e.Resolved.StateApplied != models.MemberStatusOpened || e.Resolved.ResultingState != models.MemberStatusOpened || !e.Accepted() {
		t.Errorf("launch log entry: %+v", e)
	}

	// A second launch changes nothing but history — and adds a log entry.
	h.recordSurveyOpened(ctx, userID, &wsID, &orgID, linked)
	again, _ := store.Get(ctx, wsID, userID, "pre")
	if !again.OpenedAt.Equal(*doc.OpenedAt) || len(again.History) != 2 {
		t.Errorf("second launch: openedAt=%v history=%d", again.OpenedAt, len(again.History))
	}
	if n := logCount(); n != 2 {
		t.Errorf("log entries after second launch: %d, want 2", n)
	}

	// After the provider reports completion, a launch never regresses it; the
	// log records the resulting (still completed) state.
	if _, err := store.Record(ctx, memberstatus.RecordInput{WorkspaceID: wsID, UserID: userID, EntityKey: "pre", State: models.MemberStatusCompleted}); err != nil {
		t.Fatal(err)
	}
	h.recordSurveyOpened(ctx, userID, &wsID, &orgID, linked)
	final, _ := store.Get(ctx, wsID, userID, "pre")
	if final.State != models.MemberStatusCompleted {
		t.Errorf("launch regressed state to %q", final.State)
	}
	latest, _, _ := logStore.List(ctx, memberstatuslog.ListQuery{WorkspaceID: wsID, Limit: 1})
	if len(latest) != 1 || latest[0].Resolved.ResultingState != models.MemberStatusCompleted {
		t.Errorf("latest log entry should show the completed resulting state: %+v", latest)
	}

	// Unlinked, stale-linked, and workspace-less cases record nothing — and log nothing.
	before := logCount()
	h.recordSurveyOpened(ctx, userID, &wsID, &orgID, models.Resource{ID: primitive.NewObjectID(), Title: "Game"})
	h.recordSurveyOpened(ctx, userID, &wsID, &orgID, models.Resource{ID: primitive.NewObjectID(), Title: "Old", TrackedEntityID: "gone"})
	h.recordSurveyOpened(ctx, primitive.NewObjectID(), nil, nil, linked)
	docs, _ := store.ListForUser(ctx, wsID, userID)
	if len(docs) != 1 {
		t.Errorf("unexpected documents recorded: %d", len(docs))
	}
	if _, err := store.Get(ctx, wsID, userID, memberstatuscfg.UnknownKey("gone")); err == nil {
		t.Error("stale link should not record anything")
	}
	if n := logCount(); n != before {
		t.Errorf("non-tracked launches must not be logged: %d → %d", before, n)
	}

	// Nil config → silently not tracked.
	h.SurveyConfig = nil
	other := primitive.NewObjectID()
	h.recordSurveyOpened(ctx, other, &wsID, &orgID, linked)
	if docs, _ := store.ListForUser(ctx, wsID, other); len(docs) != 0 {
		t.Error("nil config should not record")
	}
}
