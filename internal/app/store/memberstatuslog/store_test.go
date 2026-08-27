package memberstatuslog_test

import (
	"strings"
	"testing"
	"time"

	"github.com/dalemusser/stratahub/internal/app/store/memberstatuslog"
	"github.com/dalemusser/stratahub/internal/app/system/memberstatuscfg"
	"github.com/dalemusser/stratahub/internal/domain/models"
	"github.com/dalemusser/stratahub/internal/testutil"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
)

func entry(ws primitive.ObjectID, at time.Time, user, org *primitive.ObjectID, key, state, errCode string) models.MemberStatusLogEntry {
	e := models.MemberStatusLogEntry{
		WorkspaceID: ws, ReceivedAt: at, Source: models.MemberStatusSourceAPI, RemoteIP: "203.0.113.5",
		Request:  models.MemberStatusLogRequest{UserID: "raw", Entity: "Pre", State: strings.ToUpper(state), OccurredAt: "2026-08-25T14:03:11Z"},
		Resolved: models.MemberStatusLogResolved{UserID: user, OrganizationID: org, EntityKey: key, KnownEntity: !strings.HasPrefix(key, memberstatuscfg.UnknownKeyPrefix), StateApplied: state, ResultingState: state},
		Outcome:  models.MemberStatusLogOutcome{HTTPStatus: 200},
	}
	if errCode != "" {
		e.Outcome = models.MemberStatusLogOutcome{HTTPStatus: 404, Error: errCode, Message: "no"}
		e.Resolved = models.MemberStatusLogResolved{}
	}
	return e
}

func TestAppendAndGet(t *testing.T) {
	db := testutil.SetupTestDB(t)
	store := memberstatuslog.New(db)
	ctx, cancel := testutil.TestContext()
	defer cancel()

	ws, user, org := primitive.NewObjectID(), primitive.NewObjectID(), primitive.NewObjectID()
	long := strings.Repeat("x", 500)
	in := entry(ws, time.Time{}, &user, &org, "pre", "started", "")
	in.Request.Entity = long
	in.Request.State = "  Started "
	saved, err := store.Append(ctx, in)
	if err != nil {
		t.Fatalf("Append: %v", err)
	}
	if saved.ID.IsZero() || saved.ReceivedAt.IsZero() {
		t.Error("Append must assign id and receipt time")
	}
	if len([]rune(saved.Request.Entity)) != models.MemberStatusLogMaxField+1 {
		t.Errorf("entity not clipped: %d runes", len([]rune(saved.Request.Entity)))
	}
	if saved.Request.StateNorm != "started" {
		t.Errorf("StateNorm: %q", saved.Request.StateNorm)
	}

	got, err := store.Get(ctx, ws, saved.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.Resolved.UserID == nil || *got.Resolved.UserID != user || got.Resolved.EntityKey != "pre" || !got.Accepted() {
		t.Errorf("Get: %+v", got)
	}
	if _, err := store.Get(ctx, primitive.NewObjectID(), saved.ID); err != mongo.ErrNoDocuments {
		t.Errorf("Get from another workspace: %v, want ErrNoDocuments", err)
	}
}

func TestListFiltersAndPaging(t *testing.T) {
	db := testutil.SetupTestDB(t)
	store := memberstatuslog.New(db)
	ctx, cancel := testutil.TestContext()
	defer cancel()

	ws, other := primitive.NewObjectID(), primitive.NewObjectID()
	orgA, orgB := primitive.NewObjectID(), primitive.NewObjectID()
	alice, bob := primitive.NewObjectID(), primitive.NewObjectID()
	now := time.Now().UTC().Truncate(time.Millisecond)

	seed := []models.MemberStatusLogEntry{
		entry(ws, now.Add(-1*time.Minute), &alice, &orgA, "pre", "completed", ""),
		entry(ws, now.Add(-2*time.Minute), &alice, &orgA, "mhs", "started", ""),
		entry(ws, now.Add(-3*time.Minute), &bob, &orgB, "pre", "started", ""),
		entry(ws, now.Add(-4*time.Minute), nil, nil, "", "started", "unknown_user"),
		entry(ws, now.Add(-5*time.Minute), &alice, &orgA, memberstatuscfg.UnknownKey("Mid Survey"), "started", ""),
		entry(ws, now.Add(-40*24*time.Hour), &alice, &orgA, "post", "completed", ""),
		entry(other, now, &alice, &orgA, "pre", "completed", ""),
	}
	seed[2].Source = models.MemberStatusSourceLaunch
	seed[2].Request.State = "opened"
	seed[2].Resolved.StateApplied = "opened"
	seed[3].Request.UserID = "0123456789abcdef01234567"
	var ids []primitive.ObjectID
	for _, e := range seed {
		saved, err := store.Append(ctx, e)
		if err != nil {
			t.Fatal(err)
		}
		ids = append(ids, saved.ID)
	}
	keysOf := func(es []models.MemberStatusLogEntry) string {
		var parts []string
		for _, e := range es {
			k := e.Resolved.EntityKey
			if k == "" {
				k = "-" + e.Outcome.Error
			}
			parts = append(parts, k)
		}
		return strings.Join(parts, ",")
	}
	list := func(q memberstatuslog.ListQuery) ([]models.MemberStatusLogEntry, bool) {
		t.Helper()
		q.WorkspaceID = ws
		es, more, err := store.List(ctx, q)
		if err != nil {
			t.Fatalf("List: %v", err)
		}
		return es, more
	}

	// Newest first, workspace-isolated.
	es, more := list(memberstatuslog.ListQuery{})
	if got := keysOf(es); got != "pre,mhs,pre,-unknown_user,name:mid survey,post" || more {
		t.Errorf("all: %s more=%v", got, more)
	}

	// Paging: two per page with a cursor.
	es, more = list(memberstatuslog.ListQuery{Limit: 2})
	if keysOf(es) != "pre,mhs" || !more {
		t.Errorf("page 1: %s more=%v", keysOf(es), more)
	}
	es, more = list(memberstatuslog.ListQuery{Limit: 2, After: &memberstatuslog.Position{ReceivedAt: es[1].ReceivedAt, ID: es[1].ID}})
	if keysOf(es) != "pre,-unknown_user" || !more {
		t.Errorf("page 2: %s more=%v", keysOf(es), more)
	}
	es, more = list(memberstatuslog.ListQuery{Limit: 2, After: &memberstatuslog.Position{ReceivedAt: es[1].ReceivedAt, ID: es[1].ID}})
	if keysOf(es) != "name:mid survey,post" || more {
		t.Errorf("page 3: %s more=%v", keysOf(es), more)
	}

	// Filters.
	cases := map[string]struct {
		q    memberstatuslog.ListQuery
		want string
	}{
		"source launch":     {memberstatuslog.ListQuery{Source: models.MemberStatusSourceLaunch}, "pre"},
		"entity pre":        {memberstatuslog.ListQuery{EntityKey: "pre"}, "pre,pre"},
		"unrecognized":      {memberstatuslog.ListQuery{EntityKey: memberstatuslog.EntityUnrecognized}, "name:mid survey"},
		"state completed":   {memberstatuslog.ListQuery{StateSent: "Completed"}, "pre,post"},
		"accepted":          {memberstatuslog.ListQuery{Outcome: memberstatuslog.OutcomeAccepted}, "pre,mhs,pre,name:mid survey,post"},
		"rejected":          {memberstatuslog.ListQuery{Outcome: memberstatuslog.OutcomeRejected}, "-unknown_user"},
		"specific error":    {memberstatuslog.ListQuery{Outcome: "unknown_user"}, "-unknown_user"},
		"other error":       {memberstatuslog.ListQuery{Outcome: "invalid_state"}, ""},
		"user bob":          {memberstatuslog.ListQuery{UserID: &bob}, "pre"},
		"user ids":          {memberstatuslog.ListQuery{UserIDs: []primitive.ObjectID{bob, alice}}, "pre,mhs,pre,name:mid survey,post"},
		"user ids empty":    {memberstatuslog.ListQuery{UserIDs: []primitive.ObjectID{}}, ""},
		"raw user id":       {memberstatuslog.ListQuery{RawUserID: "0123456789abcdef01234567"}, "-unknown_user"},
		"event id":          {memberstatuslog.ListQuery{EventID: &ids[1]}, "mhs"},
		"last 7 days":       {memberstatuslog.ListQuery{From: now.Add(-7 * 24 * time.Hour)}, "pre,mhs,pre,-unknown_user,name:mid survey"},
		"window":            {memberstatuslog.ListQuery{From: now.Add(-3*time.Minute - time.Second), To: now.Add(-2 * time.Minute)}, "mhs,pre"},
		"scope org A":       {memberstatuslog.ListQuery{Scope: bson.M{"resolved.organization_id": bson.M{"$in": []primitive.ObjectID{orgA}}}}, "pre,mhs,name:mid survey,post"},
		"scope users {bob}": {memberstatuslog.ListQuery{Scope: bson.M{"resolved.user_id": bson.M{"$in": []primitive.ObjectID{bob}}}}, "pre"},
		"scope empty":       {memberstatuslog.ListQuery{Scope: bson.M{"resolved.user_id": bson.M{"$in": []primitive.ObjectID{}}}}, ""},
	}
	for name, tc := range cases {
		es, _ := list(tc.q)
		if got := keysOf(es); got != tc.want {
			t.Errorf("%s: got %q, want %q", name, got, tc.want)
		}
	}

	// Count and summary.
	n, err := store.Count(ctx, memberstatuslog.ListQuery{WorkspaceID: ws, EntityKey: "pre"})
	if err != nil || n != 2 {
		t.Errorf("Count: %d %v", n, err)
	}
	sum, err := store.Summarize(ctx, memberstatuslog.ListQuery{WorkspaceID: ws})
	if err != nil {
		t.Fatalf("Summarize: %v", err)
	}
	if sum.Total != 6 || sum.Accepted != 5 || sum.Rejected != 1 || sum.Last == nil || !sum.Last.Equal(now.Add(-1*time.Minute)) {
		t.Errorf("summary: %+v", sum)
	}
	sum, _ = store.Summarize(ctx, memberstatuslog.ListQuery{WorkspaceID: ws, Outcome: "unknown_user"})
	if sum.Total != 1 || sum.Rejected != 1 || sum.Accepted != 0 {
		t.Errorf("summary for an error code: %+v", sum)
	}
	sum, _ = store.Summarize(ctx, memberstatuslog.ListQuery{WorkspaceID: primitive.NewObjectID()})
	if sum.Total != 0 || sum.Last != nil {
		t.Errorf("empty summary: %+v", sum)
	}
}

func TestIndexesIncludeTTL(t *testing.T) {
	db := testutil.SetupTestDB(t) // runs indexes.EnsureAll
	ctx, cancel := testutil.TestContext()
	defer cancel()
	cur, err := db.Collection("member_status_log").Indexes().List(ctx)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	ttl := false
	for cur.Next(ctx) {
		var idx struct {
			Name               string `bson:"name"`
			ExpireAfterSeconds *int32 `bson:"expireAfterSeconds"`
		}
		if err := cur.Decode(&idx); err != nil {
			t.Fatal(err)
		}
		names = append(names, idx.Name)
		if idx.ExpireAfterSeconds != nil && *idx.ExpireAfterSeconds == int32(models.MemberStatusLogRetention.Seconds()) {
			ttl = true
		}
	}
	joined := strings.Join(names, ",")
	for _, want := range []string{"idx_mslog_ws_received", "idx_mslog_ws_user_received", "idx_mslog_ws_entity_received", "ttl_mslog_received"} {
		if !strings.Contains(joined, want) {
			t.Errorf("missing index %s in %s", want, joined)
		}
	}
	if !ttl {
		t.Errorf("TTL index with %d seconds not found", int(models.MemberStatusLogRetention.Seconds()))
	}
}
