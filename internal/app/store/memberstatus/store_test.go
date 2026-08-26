package memberstatus_test

import (
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/dalemusser/stratahub/internal/app/store/memberstatus"
	"github.com/dalemusser/stratahub/internal/domain/models"
	"github.com/dalemusser/stratahub/internal/testutil"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
)

func newStore(t *testing.T) (*memberstatus.Store, *mongo.Database) {
	t.Helper()
	db := testutil.SetupTestDB(t)
	return memberstatus.New(db), db
}

func input(ws, user primitive.ObjectID, key, state string) memberstatus.RecordInput {
	return memberstatus.RecordInput{
		WorkspaceID: ws,
		UserID:      user,
		EntityKey:   key,
		Entity:      "Pre-Survey",
		State:       state,
		Source:      models.MemberStatusSourceAPI,
		RemoteIP:    "203.0.113.5",
	}
}

func TestRecord_CreatesDocument(t *testing.T) {
	store, _ := newStore(t)
	ctx, cancel := testutil.TestContext()
	defer cancel()

	ws, user := primitive.NewObjectID(), primitive.NewObjectID()
	in := input(ws, user, "pre", models.MemberStatusOpened)
	in.Source = models.MemberStatusSourceLaunch

	doc, err := store.Record(ctx, in)
	if err != nil {
		t.Fatalf("Record: %v", err)
	}

	if doc.ID.IsZero() {
		t.Error("ID not set")
	}
	if doc.WorkspaceID != ws || doc.UserID != user || doc.EntityKey != "pre" {
		t.Errorf("keys: got %s/%s/%s", doc.WorkspaceID.Hex(), doc.UserID.Hex(), doc.EntityKey)
	}
	if doc.Entity != "Pre-Survey" {
		t.Errorf("Entity: got %q, want Pre-Survey", doc.Entity)
	}
	if doc.State != models.MemberStatusOpened || doc.StateRank != 1 {
		t.Errorf("state: got %q/%d, want opened/1", doc.State, doc.StateRank)
	}
	if doc.OpenedAt == nil {
		t.Error("OpenedAt not set")
	}
	if doc.StartedAt != nil || doc.CompletedAt != nil {
		t.Error("StartedAt/CompletedAt must not be set by an 'opened' event")
	}
	if doc.Source != models.MemberStatusSourceLaunch {
		t.Errorf("Source: got %q, want launch", doc.Source)
	}
	if doc.FirstReceivedAt.IsZero() || doc.LastReceivedAt.IsZero() || doc.CreatedAt.IsZero() || doc.UpdatedAt.IsZero() {
		t.Error("timestamps not set")
	}
	if len(doc.History) != 1 || doc.History[0].State != models.MemberStatusOpened || doc.History[0].RemoteIP != "203.0.113.5" {
		t.Errorf("History: got %+v", doc.History)
	}
}

func TestRecord_RejectsBadInput(t *testing.T) {
	store, _ := newStore(t)
	ctx, cancel := testutil.TestContext()
	defer cancel()

	ws, user := primitive.NewObjectID(), primitive.NewObjectID()

	if _, err := store.Record(ctx, input(ws, user, "pre", "finished")); err != memberstatus.ErrInvalidState {
		t.Errorf("unknown state: got %v, want ErrInvalidState", err)
	}
	if _, err := store.Record(ctx, input(ws, user, "pre", models.MemberStatusNotStarted)); err != memberstatus.ErrInvalidState {
		t.Errorf("not_started: got %v, want ErrInvalidState", err)
	}
	if _, err := store.Record(ctx, input(ws, user, "", models.MemberStatusStarted)); err != memberstatus.ErrMissingKey {
		t.Errorf("empty key: got %v, want ErrMissingKey", err)
	}
	if n, _ := store.ListForUser(ctx, ws, user); len(n) != 0 {
		t.Errorf("rejected inputs must not create documents, found %d", len(n))
	}
}

func TestRecord_LadderAndFirstWinsTimestamps(t *testing.T) {
	store, _ := newStore(t)
	ctx, cancel := testutil.TestContext()
	defer cancel()

	ws, user := primitive.NewObjectID(), primitive.NewObjectID()

	// opened → started → completed, each strictly later than the last.
	opened, err := store.Record(ctx, input(ws, user, "pre", models.MemberStatusOpened))
	if err != nil {
		t.Fatalf("opened: %v", err)
	}
	time.Sleep(5 * time.Millisecond)
	started, err := store.Record(ctx, input(ws, user, "pre", models.MemberStatusStarted))
	if err != nil {
		t.Fatalf("started: %v", err)
	}
	time.Sleep(5 * time.Millisecond)
	completed, err := store.Record(ctx, input(ws, user, "pre", models.MemberStatusCompleted))
	if err != nil {
		t.Fatalf("completed: %v", err)
	}

	if started.State != models.MemberStatusStarted || completed.State != models.MemberStatusCompleted {
		t.Errorf("ladder: started=%q completed=%q", started.State, completed.State)
	}
	if completed.OpenedAt == nil || completed.StartedAt == nil || completed.CompletedAt == nil {
		t.Fatalf("all three timestamps expected, got opened=%v started=%v completed=%v", completed.OpenedAt, completed.StartedAt, completed.CompletedAt)
	}
	if !completed.OpenedAt.Equal(*opened.OpenedAt) {
		t.Error("OpenedAt changed by later events")
	}
	if !completed.StartedAt.Equal(*started.StartedAt) {
		t.Error("StartedAt changed by later events")
	}
	if !completed.OpenedAt.Before(*completed.StartedAt) || !completed.StartedAt.Before(*completed.CompletedAt) {
		t.Errorf("timestamps out of order: %v %v %v", completed.OpenedAt, completed.StartedAt, completed.CompletedAt)
	}
	if !completed.FirstReceivedAt.Equal(opened.FirstReceivedAt) {
		t.Error("FirstReceivedAt changed by later events")
	}
	if !completed.LastReceivedAt.After(opened.LastReceivedAt) {
		t.Error("LastReceivedAt not advanced")
	}
	if len(completed.History) != 3 {
		t.Errorf("History length: got %d, want 3", len(completed.History))
	}

	// Repeating "completed" is idempotent: timestamp unchanged, history grows.
	time.Sleep(5 * time.Millisecond)
	again, err := store.Record(ctx, input(ws, user, "pre", models.MemberStatusCompleted))
	if err != nil {
		t.Fatalf("completed again: %v", err)
	}
	if !again.CompletedAt.Equal(*completed.CompletedAt) {
		t.Error("CompletedAt changed by a repeated event (must be first-wins)")
	}
	if !again.LastReceivedAt.After(completed.LastReceivedAt) {
		t.Error("LastReceivedAt not advanced by a repeated event")
	}
	if len(again.History) != 4 {
		t.Errorf("History length after repeat: got %d, want 4", len(again.History))
	}
}

func TestRecord_NeverRegresses(t *testing.T) {
	store, _ := newStore(t)
	ctx, cancel := testutil.TestContext()
	defer cancel()

	ws, user := primitive.NewObjectID(), primitive.NewObjectID()

	// completed arrives first (provider may never send "started").
	completed, err := store.Record(ctx, input(ws, user, "post", models.MemberStatusCompleted))
	if err != nil {
		t.Fatalf("completed: %v", err)
	}
	if completed.StartedAt != nil || completed.OpenedAt != nil {
		t.Error("completed alone must not fabricate started/opened timestamps")
	}

	// A late "started" is recorded but does not lower the state.
	time.Sleep(5 * time.Millisecond)
	late, err := store.Record(ctx, input(ws, user, "post", models.MemberStatusStarted))
	if err != nil {
		t.Fatalf("late started: %v", err)
	}
	if late.State != models.MemberStatusCompleted || late.StateRank != 3 {
		t.Errorf("state regressed: got %q/%d, want completed/3", late.State, late.StateRank)
	}
	if late.StartedAt == nil {
		t.Error("late started should still record StartedAt")
	}
	if !late.CompletedAt.Equal(*completed.CompletedAt) {
		t.Error("CompletedAt changed by a late lower-state event")
	}
	if late.Source != models.MemberStatusSourceAPI {
		t.Errorf("Source: got %q", late.Source)
	}
	if len(late.History) != 2 || late.History[1].State != models.MemberStatusStarted {
		t.Errorf("history should hold the late event: %+v", late.History)
	}

	// And a late "opened" from a resource launch behaves the same way.
	in := input(ws, user, "post", models.MemberStatusOpened)
	in.Source = models.MemberStatusSourceLaunch
	opened, err := store.Record(ctx, in)
	if err != nil {
		t.Fatalf("late opened: %v", err)
	}
	if opened.State != models.MemberStatusCompleted {
		t.Errorf("state regressed on late opened: got %q", opened.State)
	}
	if opened.OpenedAt == nil {
		t.Error("late opened should still record OpenedAt")
	}
}

func TestRecord_StateLabelMatchesRankInDB(t *testing.T) {
	store, db := newStore(t)
	ctx, cancel := testutil.TestContext()
	defer cancel()

	ws, user := primitive.NewObjectID(), primitive.NewObjectID()
	if _, err := store.Record(ctx, input(ws, user, "mhs", models.MemberStatusCompleted)); err != nil {
		t.Fatalf("completed: %v", err)
	}
	if _, err := store.Record(ctx, input(ws, user, "mhs", models.MemberStatusOpened)); err != nil {
		t.Fatalf("opened: %v", err)
	}

	// Read the raw document: the stored label must agree with the rank.
	var raw struct {
		State     string `bson:"state"`
		StateRank int    `bson:"state_rank"`
	}
	err := db.Collection("member_status").FindOne(ctx, bson.M{"workspace_id": ws, "user_id": user, "entity_key": "mhs"}).Decode(&raw)
	if err != nil {
		t.Fatalf("raw read: %v", err)
	}
	if raw.State != models.MemberStatusCompleted || raw.StateRank != 3 {
		t.Errorf("stored state/rank: got %q/%d, want completed/3", raw.State, raw.StateRank)
	}
}

func TestRecord_HistoryIsCapped(t *testing.T) {
	store, _ := newStore(t)
	ctx, cancel := testutil.TestContext()
	defer cancel()

	ws, user := primitive.NewObjectID(), primitive.NewObjectID()
	n := models.MemberStatusHistoryLimit + 10

	var last models.MemberStatus
	for i := 0; i < n; i++ {
		var err error
		last, err = store.Record(ctx, input(ws, user, "ews", models.MemberStatusStarted))
		if err != nil {
			t.Fatalf("Record #%d: %v", i, err)
		}
	}
	if len(last.History) != models.MemberStatusHistoryLimit {
		t.Errorf("History length: got %d, want %d", len(last.History), models.MemberStatusHistoryLimit)
	}
	// The kept events are the most recent ones: the last event's ReceivedAt
	// must equal the document's LastReceivedAt.
	if !last.History[len(last.History)-1].ReceivedAt.Equal(last.LastReceivedAt) {
		t.Error("history should keep the newest events")
	}
}

func TestRecord_OccurredAtIsKeptInHistoryOnly(t *testing.T) {
	store, _ := newStore(t)
	ctx, cancel := testutil.TestContext()
	defer cancel()

	ws, user := primitive.NewObjectID(), primitive.NewObjectID()
	occurred := time.Date(2026, 8, 25, 14, 3, 11, 0, time.UTC)
	in := input(ws, user, "pre", models.MemberStatusCompleted)
	in.OccurredAt = &occurred

	doc, err := store.Record(ctx, in)
	if err != nil {
		t.Fatalf("Record: %v", err)
	}
	if doc.History[0].OccurredAt == nil || !doc.History[0].OccurredAt.Equal(occurred) {
		t.Errorf("history OccurredAt: got %v, want %v", doc.History[0].OccurredAt, occurred)
	}
	// CompletedAt is StrataHub's receipt time, not the provider's clock.
	if doc.CompletedAt.Equal(occurred) {
		t.Error("CompletedAt must be the receipt time, not the provider's occurred_at")
	}
}

func TestRecord_ConcurrentFirstEvents(t *testing.T) {
	store, db := newStore(t)
	ctx, cancel := testutil.TestContext()
	defer cancel()

	ws, user := primitive.NewObjectID(), primitive.NewObjectID()
	const workers = 8

	var wg sync.WaitGroup
	errs := make(chan error, workers)
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			state := models.MemberStatusStarted
			if i%2 == 0 {
				state = models.MemberStatusCompleted
			}
			if _, err := store.Record(ctx, input(ws, user, "pre", state)); err != nil {
				errs <- fmt.Errorf("worker %d: %w", i, err)
			}
		}(i)
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Error(err)
	}

	count, err := db.Collection("member_status").CountDocuments(ctx, bson.M{"workspace_id": ws, "user_id": user, "entity_key": "pre"})
	if err != nil {
		t.Fatalf("count: %v", err)
	}
	if count != 1 {
		t.Fatalf("documents: got %d, want 1 (unique index + retry must collapse the race)", count)
	}

	doc, err := store.Get(ctx, ws, user, "pre")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if doc.State != models.MemberStatusCompleted || doc.StateRank != 3 {
		t.Errorf("state after race: got %q/%d, want completed/3", doc.State, doc.StateRank)
	}
	if len(doc.History) != workers {
		t.Errorf("history after race: got %d events, want %d", len(doc.History), workers)
	}
}

func TestListByUserIDs_AndWorkspaceIsolation(t *testing.T) {
	store, _ := newStore(t)
	ctx, cancel := testutil.TestContext()
	defer cancel()

	ws, otherWS := primitive.NewObjectID(), primitive.NewObjectID()
	alice, bob, carol := primitive.NewObjectID(), primitive.NewObjectID(), primitive.NewObjectID()

	must := func(in memberstatus.RecordInput) {
		t.Helper()
		if _, err := store.Record(ctx, in); err != nil {
			t.Fatalf("Record: %v", err)
		}
	}
	must(input(ws, alice, "pre", models.MemberStatusCompleted))
	must(input(ws, alice, "post", models.MemberStatusStarted))
	must(input(ws, bob, "pre", models.MemberStatusOpened))
	must(input(otherWS, alice, "pre", models.MemberStatusOpened)) // same user, other workspace
	// carol has nothing.

	got, err := store.ListByUserIDs(ctx, ws, []primitive.ObjectID{alice, bob, carol})
	if err != nil {
		t.Fatalf("ListByUserIDs: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("users with data: got %d, want 2 (%v)", len(got), got)
	}
	if a := got[alice.Hex()]; len(a) != 2 || a["pre"].State != models.MemberStatusCompleted || a["post"].State != models.MemberStatusStarted {
		t.Errorf("alice: %+v", a)
	}
	if b := got[bob.Hex()]; len(b) != 1 || b["pre"].State != models.MemberStatusOpened {
		t.Errorf("bob: %+v", b)
	}
	if _, ok := got[carol.Hex()]; ok {
		t.Error("carol should be absent")
	}

	// Empty input is a cheap no-op.
	empty, err := store.ListByUserIDs(ctx, ws, nil)
	if err != nil || len(empty) != 0 {
		t.Errorf("empty input: got %v, %v", empty, err)
	}

	// ListForUser is ordered by entity key and workspace-scoped.
	list, err := store.ListForUser(ctx, ws, alice)
	if err != nil {
		t.Fatalf("ListForUser: %v", err)
	}
	if len(list) != 2 || list[0].EntityKey != "post" || list[1].EntityKey != "pre" {
		t.Errorf("ListForUser order: %+v", list)
	}
	other, err := store.ListForUser(ctx, otherWS, alice)
	if err != nil || len(other) != 1 {
		t.Errorf("other workspace: got %d docs, %v", len(other), err)
	}
}

func TestDeleteByUser(t *testing.T) {
	store, _ := newStore(t)
	ctx, cancel := testutil.TestContext()
	defer cancel()

	ws := primitive.NewObjectID()
	alice, bob := primitive.NewObjectID(), primitive.NewObjectID()
	for _, key := range []string{"pre", "mhs", "ews"} {
		if _, err := store.Record(ctx, input(ws, alice, key, models.MemberStatusStarted)); err != nil {
			t.Fatalf("Record: %v", err)
		}
	}
	if _, err := store.Record(ctx, input(ws, bob, "pre", models.MemberStatusStarted)); err != nil {
		t.Fatalf("Record: %v", err)
	}

	n, err := store.DeleteByUser(ctx, ws, alice)
	if err != nil {
		t.Fatalf("DeleteByUser: %v", err)
	}
	if n != 3 {
		t.Errorf("deleted: got %d, want 3", n)
	}
	if left, _ := store.ListForUser(ctx, ws, alice); len(left) != 0 {
		t.Errorf("alice still has %d docs", len(left))
	}
	if left, _ := store.ListForUser(ctx, ws, bob); len(left) != 1 {
		t.Errorf("bob should be untouched, has %d docs", len(left))
	}

	if _, err := store.Get(ctx, ws, alice, "pre"); err != mongo.ErrNoDocuments {
		t.Errorf("Get after delete: got %v, want ErrNoDocuments", err)
	}
}
