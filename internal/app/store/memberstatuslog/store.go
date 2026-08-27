// internal/app/store/memberstatuslog/store.go
package memberstatuslog

// Terminology: User Identifiers
//   - UserID / userID / user_id: The MongoDB ObjectID (_id) that uniquely identifies a user record
//   - LoginID / loginID / login_id: The human-readable string users type to log in

import (
	"context"
	"strings"
	"time"

	"github.com/dalemusser/stratahub/internal/app/system/memberstatuscfg"
	"github.com/dalemusser/stratahub/internal/domain/models"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// Store provides access to the member_status_log collection: an append-only
// record of every member-status event received (see models.MemberStatusLogEntry).
type Store struct {
	c *mongo.Collection
}

// New creates a new member status log store.
func New(db *mongo.Database) *Store {
	return &Store{c: db.Collection("member_status_log")}
}

// Append stores one entry, assigning its id and receipt time when unset, and
// returns it. Request strings are clipped defensively.
func (s *Store) Append(ctx context.Context, e models.MemberStatusLogEntry) (models.MemberStatusLogEntry, error) {
	if e.ID.IsZero() {
		e.ID = primitive.NewObjectID()
	}
	if e.ReceivedAt.IsZero() {
		e.ReceivedAt = time.Now().UTC()
	}
	e.ReceivedAt = e.ReceivedAt.UTC()
	e.Request.UserID = models.ClipForLog(e.Request.UserID)
	e.Request.Entity = models.ClipForLog(e.Request.Entity)
	e.Request.State = models.ClipForLog(e.Request.State)
	e.Request.OccurredAt = models.ClipForLog(e.Request.OccurredAt)
	e.Request.ResourceID = models.ClipForLog(e.Request.ResourceID)
	if e.Request.StateNorm == "" {
		e.Request.StateNorm = strings.ToLower(strings.TrimSpace(e.Request.State))
	}
	e.Outcome.Message = models.ClipForLog(e.Outcome.Message)
	_, err := s.c.InsertOne(ctx, e)
	return e, err
}

// Get returns one entry by id within a workspace.
func (s *Store) Get(ctx context.Context, workspaceID, id primitive.ObjectID) (models.MemberStatusLogEntry, error) {
	var e models.MemberStatusLogEntry
	err := s.c.FindOne(ctx, bson.M{"_id": id, "workspace_id": workspaceID}).Decode(&e)
	return e, err
}

// Outcome filter values for ListQuery.Outcome, besides a specific error code.
const (
	OutcomeAccepted = "accepted"
	OutcomeRejected = "rejected"
)

// EntityUnrecognized is the ListQuery.EntityKey value that matches entries
// whose name was not in the configuration (keys with the "name:" prefix).
const EntityUnrecognized = "unrecognized"

// Position is a cursor into the newest-first ordering.
type Position struct {
	ReceivedAt time.Time
	ID         primitive.ObjectID
}

// ListQuery selects entries. Zero values mean "any".
type ListQuery struct {
	WorkspaceID primitive.ObjectID
	// Scope is the constraint from viewscope.Scope.Filter(ctx,
	// "resolved.organization_id", "resolved.user_id"); nil = unrestricted.
	// Entries with no resolved user/org (rejected before resolution) never
	// match a restricted scope.
	Scope bson.M

	From, To  time.Time // received_at bounds
	Source    string    // models.MemberStatusSourceAPI / Launch
	EntityKey string    // a configured id, or EntityUnrecognized
	StateSent string    // normalized state as sent (opened/started/completed)
	Outcome   string    // OutcomeAccepted, OutcomeRejected, or a specific error code
	UserID    *primitive.ObjectID
	RawUserID string // request.user_id exactly as sent (finds unresolved ids too)
	EventID   *primitive.ObjectID

	After *Position
	Limit int
}

func (q ListQuery) filter() bson.M {
	f := bson.M{"workspace_id": q.WorkspaceID}
	var and []bson.M
	if q.Scope != nil {
		and = append(and, q.Scope)
	}
	if !q.From.IsZero() || !q.To.IsZero() {
		rng := bson.M{}
		if !q.From.IsZero() {
			rng["$gte"] = q.From.UTC()
		}
		if !q.To.IsZero() {
			rng["$lte"] = q.To.UTC()
		}
		f["received_at"] = rng
	}
	if q.Source != "" {
		f["source"] = q.Source
	}
	switch q.EntityKey {
	case "":
	case EntityUnrecognized:
		f["resolved.entity_key"] = bson.M{"$regex": "^" + memberstatuscfg.UnknownKeyPrefix}
	default:
		f["resolved.entity_key"] = q.EntityKey
	}
	if q.StateSent != "" {
		f["request.state_norm"] = strings.ToLower(strings.TrimSpace(q.StateSent))
	}
	switch q.Outcome {
	case "":
	case OutcomeAccepted:
		f["outcome.error"] = bson.M{"$in": []interface{}{"", nil}}
	case OutcomeRejected:
		f["outcome.error"] = bson.M{"$nin": []interface{}{"", nil}}
	default:
		f["outcome.error"] = q.Outcome
	}
	if q.UserID != nil {
		f["resolved.user_id"] = *q.UserID
	}
	if q.RawUserID != "" {
		f["request.user_id"] = q.RawUserID
	}
	if q.EventID != nil {
		f["_id"] = *q.EventID
	}
	if q.After != nil {
		and = append(and, bson.M{"$or": []bson.M{
			{"received_at": bson.M{"$lt": q.After.ReceivedAt.UTC()}},
			{"received_at": q.After.ReceivedAt.UTC(), "_id": bson.M{"$lt": q.After.ID}},
		}})
	}
	if len(and) > 0 {
		f["$and"] = and
	}
	return f
}

// List returns up to Limit entries newest first (received_at desc, _id desc)
// and whether more follow. Limit defaults to 50.
func (s *Store) List(ctx context.Context, q ListQuery) (entries []models.MemberStatusLogEntry, more bool, err error) {
	limit := q.Limit
	if limit <= 0 {
		limit = 50
	}
	opts := options.Find().
		SetSort(bson.D{{Key: "received_at", Value: -1}, {Key: "_id", Value: -1}}).
		SetLimit(int64(limit + 1))
	cur, err := s.c.Find(ctx, q.filter(), opts)
	if err != nil {
		return nil, false, err
	}
	defer cur.Close(ctx)
	if err := cur.All(ctx, &entries); err != nil {
		return nil, false, err
	}
	if len(entries) > limit {
		return entries[:limit], true, nil
	}
	return entries, false, nil
}

// Count returns the number of entries matching the query (ignoring paging).
func (s *Store) Count(ctx context.Context, q ListQuery) (int64, error) {
	q.After = nil
	return s.c.CountDocuments(ctx, q.filter())
}

// Summary describes the entries matching a query.
type Summary struct {
	Total    int64
	Accepted int64
	Rejected int64
	Last     *time.Time // receipt time of the newest matching entry
}

// Summarize computes totals for the header chips.
func (s *Store) Summarize(ctx context.Context, q ListQuery) (Summary, error) {
	q.After = nil
	var sum Summary
	var err error
	if sum.Total, err = s.c.CountDocuments(ctx, q.filter()); err != nil {
		return sum, err
	}
	rq := q
	rq.Outcome = OutcomeRejected
	if q.Outcome == "" || q.Outcome == OutcomeRejected {
		if sum.Rejected, err = s.c.CountDocuments(ctx, rq.filter()); err != nil {
			return sum, err
		}
	} else if q.Outcome != OutcomeAccepted {
		// A specific error code: everything matching is rejected.
		sum.Rejected = sum.Total
	}
	sum.Accepted = sum.Total - sum.Rejected

	var last struct {
		ReceivedAt time.Time `bson:"received_at"`
	}
	err = s.c.FindOne(ctx, q.filter(), options.FindOne().SetSort(bson.D{{Key: "received_at", Value: -1}, {Key: "_id", Value: -1}}).SetProjection(bson.M{"received_at": 1})).Decode(&last)
	switch {
	case err == mongo.ErrNoDocuments:
	case err != nil:
		return sum, err
	default:
		t := last.ReceivedAt.UTC()
		sum.Last = &t
	}
	return sum, nil
}
