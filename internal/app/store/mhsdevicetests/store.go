// Package mhsdevicetests provides access to the mhs_device_tests collection:
// one document per run of the Mission HydroSci device test (kind
// "devicetest") or per stored member load record (kind "member"). See
// models.MHSDeviceTest and docs/mission-hydrosci/mhs-loading-status-and-unit2-device-test-plan.md.
package mhsdevicetests

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/dalemusser/stratahub/internal/domain/models"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// ErrNotFound is returned when a run does not exist in the workspace.
var ErrNotFound = errors.New("mhsdevicetests: not found")

// ErrClosed is returned when a run no longer accepts writes (completed or
// past its expiry).
var ErrClosed = errors.New("mhsdevicetests: run is closed")

// Store provides access to the mhs_device_tests collection.
type Store struct {
	c *mongo.Collection
}

func New(db *mongo.Database) *Store {
	return &Store{c: db.Collection("mhs_device_tests")}
}

// Create inserts a run. The caller sets ID (the marked test id for device
// tests), WorkspaceID, Kind and the form; timestamps default to now.
func (s *Store) Create(ctx context.Context, t models.MHSDeviceTest) (models.MHSDeviceTest, error) {
	now := time.Now().UTC()
	if t.ID.IsZero() {
		t.ID = primitive.NewObjectID()
	}
	if t.StartedAt.IsZero() {
		t.StartedAt = now
	}
	t.StartedAt = t.StartedAt.UTC()
	t.LastSeenAt = now
	if t.ExpiresAt.IsZero() {
		t.ExpiresAt = now.Add(24 * time.Hour)
	}
	if t.Stage == "" {
		t.Stage = models.MHSDeviceTestStageRun
	}
	_, err := s.c.InsertOne(ctx, t)
	return t, err
}

// Get returns one run by id within a workspace.
func (s *Store) Get(ctx context.Context, workspaceID, id primitive.ObjectID) (models.MHSDeviceTest, error) {
	var t models.MHSDeviceTest
	err := s.c.FindOne(ctx, bson.M{"_id": id, "workspace_id": workspaceID}).Decode(&t)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return t, ErrNotFound
	}
	return t, err
}

// openFilter matches a run that still accepts writes: in this workspace,
// not completed, and not past its expiry.
func openFilter(workspaceID, id primitive.ObjectID, now time.Time) bson.M {
	return bson.M{
		"_id":               id,
		"workspace_id":      workspaceID,
		"expires_at":        bson.M{"$gt": now},
		"unit_completed_at": bson.M{"$exists": false},
	}
}

// update applies an update to an open run; a miss means closed or unknown.
func (s *Store) update(ctx context.Context, workspaceID, id primitive.ObjectID, update bson.M) error {
	now := time.Now().UTC()
	set, _ := update["$set"].(bson.M)
	if set == nil {
		set = bson.M{}
		update["$set"] = set
	}
	set["last_seen_at"] = now
	res, err := s.c.UpdateOne(ctx, openFilter(workspaceID, id, now), update)
	if err != nil {
		return err
	}
	if res.MatchedCount == 0 {
		// Distinguish unknown from closed for the caller's status code.
		if _, gerr := s.Get(ctx, workspaceID, id); gerr != nil {
			return gerr
		}
		return ErrClosed
	}
	return nil
}

// Apply runs an arbitrary update against an open run (ErrClosed / ErrNotFound
// otherwise). last_seen_at is always refreshed.
func (s *Store) Apply(ctx context.Context, workspaceID, id primitive.ObjectID, update bson.M) error {
	return s.update(ctx, workspaceID, id, update)
}

// AppendSteps appends a batch of step entries (newest MHSDeviceTestMaxSteps
// kept) and applies the stage/summary fields derived from them.
func (s *Store) AppendSteps(ctx context.Context, workspaceID, id primitive.ObjectID, steps []models.MHSDeviceTestStep, set bson.M) error {
	update := bson.M{}
	if len(steps) > 0 {
		update["$push"] = bson.M{"steps": bson.M{
			"$each":  steps,
			"$slice": -models.MHSDeviceTestMaxSteps,
		}}
	}
	if len(set) > 0 {
		update["$set"] = set
	}
	if len(update) == 0 {
		update["$set"] = bson.M{}
	}
	return s.update(ctx, workspaceID, id, update)
}

// SetFields sets summary fields on an open run (diagnostics, device columns,
// download/launch summaries, stage).
func (s *Store) SetFields(ctx context.Context, workspaceID, id primitive.ObjectID, set bson.M) error {
	if len(set) == 0 {
		return nil
	}
	return s.update(ctx, workspaceID, id, bson.M{"$set": set})
}

// AddReport appends a tester note.
func (s *Store) AddReport(ctx context.Context, workspaceID, id primitive.ObjectID, note string) error {
	return s.update(ctx, workspaceID, id, bson.M{"$push": bson.M{"problem_reports": bson.M{
		"$each":  []models.MHSDeviceTestReport{{At: time.Now().UTC(), Note: note}},
		"$slice": -50,
	}}})
}

// Heartbeat records one liveness/memory sample. A non-closing beat from a
// run that had been marked "closed" reopens it (the tester came back or
// relaunched); a closing beat marks the run ended unless the unit was
// already completed. Beats are accepted until the run expires, completed
// or not, so a tester who keeps playing after finishing is still observed.
func (s *Store) Heartbeat(ctx context.Context, workspaceID, id primitive.ObjectID, beat models.MHSDeviceTestHeartbeat, closing bool) error {
	now := time.Now().UTC()
	if beat.At.IsZero() {
		beat.At = now
	}
	beat.At = beat.At.UTC()
	base := bson.M{"_id": id, "workspace_id": workspaceID, "expires_at": bson.M{"$gt": now}}

	if !closing {
		// Reopen a run the page had said it was leaving.
		reopen := bson.M{"_id": id, "workspace_id": workspaceID, "end_reason": models.MHSDeviceTestEndClosed}
		if _, err := s.c.UpdateOne(ctx, reopen, bson.M{"$unset": bson.M{"ended_at": "", "end_reason": ""}}); err != nil {
			return err
		}
	}
	update := bson.M{
		"$push": bson.M{"heartbeats": bson.M{"$each": []models.MHSDeviceTestHeartbeat{beat}, "$slice": -models.MHSDeviceTestMaxHeartbeats}},
		"$set":  bson.M{"last_heartbeat_at": beat.At, "last_heartbeat": beat, "last_seen_at": now},
		"$inc":  bson.M{"heartbeat_count": 1},
	}
	res, err := s.c.UpdateOne(ctx, base, update)
	if err != nil {
		return err
	}
	if res.MatchedCount == 0 {
		if _, gerr := s.Get(ctx, workspaceID, id); gerr != nil {
			return gerr
		}
		return ErrClosed
	}
	if closing {
		// Completion wins over a later "closed": only stamp an open run.
		closeFilter := bson.M{"_id": id, "workspace_id": workspaceID, "unit_completed_at": bson.M{"$exists": false}}
		_, err = s.c.UpdateOne(ctx, closeFilter, bson.M{"$set": bson.M{"ended_at": now, "end_reason": models.MHSDeviceTestEndClosed}})
	}
	return err
}

// Complete stamps unit completion and the completed stage. Idempotent.
func (s *Store) Complete(ctx context.Context, workspaceID, id primitive.ObjectID) error {
	now := time.Now().UTC()
	res, err := s.c.UpdateOne(ctx, bson.M{"_id": id, "workspace_id": workspaceID}, bson.M{
		"$set":         bson.M{"stage": models.MHSDeviceTestStageCompleted, "last_seen_at": now, "ended_at": now, "end_reason": models.MHSDeviceTestEndCompleted},
		"$setOnInsert": bson.M{},
		"$min":         bson.M{"unit_completed_at": now},
	})
	if err != nil {
		return err
	}
	if res.MatchedCount == 0 {
		return ErrNotFound
	}
	return nil
}

// Position is a cursor into the newest-first ordering.
type Position struct {
	StartedAt time.Time
	ID        primitive.ObjectID
}

// ListQuery selects runs. Zero values mean "any".
type ListQuery struct {
	WorkspaceID primitive.ObjectID
	// Scope is the constraint from viewscope.Scope.Filter(ctx, "organization_id",
	// "user_id"); nil = unrestricted. Device-test runs carry neither field, so
	// they never match a restricted scope.
	Scope bson.M

	From, To   time.Time // started_at bounds
	Kind       string    // models.MHSDeviceTestKindDeviceTest / KindMember
	Stage      string
	DeviceType string
	School     string // case-insensitive prefix
	ID         *primitive.ObjectID

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
		f["started_at"] = rng
	}
	if q.Kind != "" {
		f["kind"] = q.Kind
	}
	if q.Stage != "" {
		f["stage"] = q.Stage
	}
	if q.DeviceType != "" {
		f["device_type"] = q.DeviceType
	}
	if s := strings.TrimSpace(q.School); s != "" {
		f["form.school"] = bson.M{"$regex": "^" + regexQuote(s), "$options": "i"}
	}
	if q.ID != nil {
		f["_id"] = *q.ID
	}
	if q.After != nil {
		and = append(and, bson.M{"$or": []bson.M{
			{"started_at": bson.M{"$lt": q.After.StartedAt.UTC()}},
			{"started_at": q.After.StartedAt.UTC(), "_id": bson.M{"$lt": q.After.ID}},
		}})
	}
	if len(and) > 0 {
		f["$and"] = and
	}
	return f
}

func regexQuote(s string) string {
	var b strings.Builder
	for _, r := range s {
		if strings.ContainsRune(`\.+*?()|[]{}^$`, r) {
			b.WriteRune('\\')
		}
		b.WriteRune(r)
	}
	return b.String()
}

// listProjection leaves out the bulky embedded arrays for the table.
var listProjection = bson.M{"steps": 0, "diagnostics": 0, "problem_reports": 0, "heartbeats": 0}

// List returns up to Limit runs newest first (started_at desc, _id desc)
// without their step logs and diagnostics, and whether more follow.
func (s *Store) List(ctx context.Context, q ListQuery) (runs []models.MHSDeviceTest, more bool, err error) {
	limit := q.Limit
	if limit <= 0 {
		limit = 50
	}
	opts := options.Find().
		SetSort(bson.D{{Key: "started_at", Value: -1}, {Key: "_id", Value: -1}}).
		SetProjection(listProjection).
		SetLimit(int64(limit + 1))
	cur, err := s.c.Find(ctx, q.filter(), opts)
	if err != nil {
		return nil, false, err
	}
	defer cur.Close(ctx)
	if err := cur.All(ctx, &runs); err != nil {
		return nil, false, err
	}
	if len(runs) > limit {
		return runs[:limit], true, nil
	}
	return runs, false, nil
}

// ListFull is List with every field (steps, diagnostics, reports), for
// exports.
func (s *Store) ListFull(ctx context.Context, q ListQuery) (runs []models.MHSDeviceTest, more bool, err error) {
	limit := q.Limit
	if limit <= 0 {
		limit = 50
	}
	opts := options.Find().
		SetSort(bson.D{{Key: "started_at", Value: -1}, {Key: "_id", Value: -1}}).
		SetLimit(int64(limit + 1))
	cur, err := s.c.Find(ctx, q.filter(), opts)
	if err != nil {
		return nil, false, err
	}
	defer cur.Close(ctx)
	if err := cur.All(ctx, &runs); err != nil {
		return nil, false, err
	}
	if len(runs) > limit {
		return runs[:limit], true, nil
	}
	return runs, false, nil
}

// Count returns the number of runs matching the query (ignoring paging).
func (s *Store) Count(ctx context.Context, q ListQuery) (int64, error) {
	q.After = nil
	return s.c.CountDocuments(ctx, q.filter())
}

// Summary describes the runs matching a query.
type Summary struct {
	Total     int64
	Gameplay  int64 // reached gameplay or completed
	Completed int64
	Failed    int64 // currently failed (last step is a failure)
	Last      *time.Time
}

// Summarize computes totals for the header chips.
func (s *Store) Summarize(ctx context.Context, q ListQuery) (Summary, error) {
	q.After = nil
	var sum Summary
	var err error
	if sum.Total, err = s.c.CountDocuments(ctx, q.filter()); err != nil {
		return sum, err
	}
	countStage := func(stage string) (int64, error) {
		sq := q
		sq.Stage = stage
		return s.c.CountDocuments(ctx, sq.filter())
	}
	if q.Stage == "" || q.Stage == models.MHSDeviceTestStageCompleted {
		if sum.Completed, err = countStage(models.MHSDeviceTestStageCompleted); err != nil {
			return sum, err
		}
	}
	if q.Stage == "" || q.Stage == models.MHSDeviceTestStageGameplay {
		var g int64
		if g, err = countStage(models.MHSDeviceTestStageGameplay); err != nil {
			return sum, err
		}
		sum.Gameplay = g + sum.Completed
	} else if q.Stage == models.MHSDeviceTestStageCompleted {
		sum.Gameplay = sum.Completed
	}
	if q.Stage == "" || q.Stage == models.MHSDeviceTestStageFailed {
		if sum.Failed, err = countStage(models.MHSDeviceTestStageFailed); err != nil {
			return sum, err
		}
	}
	var last struct {
		StartedAt time.Time `bson:"started_at"`
	}
	err = s.c.FindOne(ctx, q.filter(), options.FindOne().
		SetSort(bson.D{{Key: "started_at", Value: -1}, {Key: "_id", Value: -1}}).
		SetProjection(bson.M{"started_at": 1})).Decode(&last)
	switch {
	case errors.Is(err, mongo.ErrNoDocuments):
	case err != nil:
		return sum, err
	default:
		t := last.StartedAt.UTC()
		sum.Last = &t
	}
	return sum, nil
}
