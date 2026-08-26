package resources

import (
	"testing"

	uierrors "github.com/dalemusser/stratahub/internal/app/features/errors"
	"github.com/dalemusser/stratahub/internal/app/store/memberstatus"
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
	if h.SurveyConfig == nil || h.MemberStatus == nil {
		t.Fatal("member handler did not initialize survey tracking")
	}

	wsID := primitive.NewObjectID()
	userID := primitive.NewObjectID()
	store := memberstatus.New(db)

	// Linked resource → "opened" recorded under the canonical key with the launch source.
	linked := models.Resource{ID: primitive.NewObjectID(), Title: "Pre survey link", TrackedEntityID: "pre"}
	h.recordSurveyOpened(ctx, userID, &wsID, linked)
	doc, err := store.Get(ctx, wsID, userID, "pre")
	if err != nil {
		t.Fatalf("expected an opened document: %v", err)
	}
	if doc.State != models.MemberStatusOpened || doc.OpenedAt == nil || doc.Source != models.MemberStatusSourceLaunch || doc.Entity != "Pre-Survey" {
		t.Errorf("doc: %+v", doc)
	}

	// A second launch changes nothing but history.
	h.recordSurveyOpened(ctx, userID, &wsID, linked)
	again, _ := store.Get(ctx, wsID, userID, "pre")
	if !again.OpenedAt.Equal(*doc.OpenedAt) || len(again.History) != 2 {
		t.Errorf("second launch: openedAt=%v history=%d", again.OpenedAt, len(again.History))
	}

	// After the provider reports completion, a launch never regresses it.
	if _, err := store.Record(ctx, memberstatus.RecordInput{WorkspaceID: wsID, UserID: userID, EntityKey: "pre", State: models.MemberStatusCompleted}); err != nil {
		t.Fatal(err)
	}
	h.recordSurveyOpened(ctx, userID, &wsID, linked)
	final, _ := store.Get(ctx, wsID, userID, "pre")
	if final.State != models.MemberStatusCompleted {
		t.Errorf("launch regressed state to %q", final.State)
	}

	// Unlinked, stale-linked, and workspace-less cases record nothing.
	h.recordSurveyOpened(ctx, userID, &wsID, models.Resource{ID: primitive.NewObjectID(), Title: "Game"})
	h.recordSurveyOpened(ctx, userID, &wsID, models.Resource{ID: primitive.NewObjectID(), Title: "Old", TrackedEntityID: "gone"})
	h.recordSurveyOpened(ctx, primitive.NewObjectID(), nil, linked)
	docs, _ := store.ListForUser(ctx, wsID, userID)
	if len(docs) != 1 {
		t.Errorf("unexpected documents recorded: %d", len(docs))
	}
	if _, err := store.Get(ctx, wsID, userID, memberstatuscfg.UnknownKey("gone")); err == nil {
		t.Error("stale link should not record anything")
	}

	// Nil config → silently not tracked.
	h.SurveyConfig = nil
	other := primitive.NewObjectID()
	h.recordSurveyOpened(ctx, other, &wsID, linked)
	if docs, _ := store.ListForUser(ctx, wsID, other); len(docs) != 0 {
		t.Error("nil config should not record")
	}
}
