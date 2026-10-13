// Package mhsactivitydays stores the rolled-up MHS activity days
// (models.MHSActivityDay, one per workspace, user and local day) and the
// marks that say which organization-days are final.
package mhsactivitydays

import (
	"context"
	"time"

	"github.com/dalemusser/stratahub/internal/domain/models"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// Store is the access to both collections.
type Store struct {
	days  *mongo.Collection
	marks *mongo.Collection
}

// New creates a store.
func New(db *mongo.Database) *Store {
	return &Store{days: db.Collection("mhs_activity_days"), marks: db.Collection("mhs_activity_marks")}
}

// ReplaceDay stores the day documents of the given organizations for one
// day: each is upserted by (workspace, user, day) and marked Final, then any
// stored document of those organizations for that day that is not in docs
// is removed. Running it twice with the same input leaves the same state.
func (s *Store) ReplaceDay(ctx context.Context, wsID primitive.ObjectID, day string, orgIDs []primitive.ObjectID, docs []models.MHSActivityDay) error {
	keep := make([]primitive.ObjectID, 0, len(docs))
	if len(docs) > 0 {
		writes := make([]mongo.WriteModel, 0, len(docs))
		for _, d := range docs {
			d.ID = primitive.NilObjectID
			d.WorkspaceID = wsID
			d.Day = day
			d.Final = true
			keep = append(keep, d.UserID)
			writes = append(writes, mongo.NewReplaceOneModel().
				SetFilter(bson.M{"workspace_id": wsID, "user_id": d.UserID, "day": day}).
				SetReplacement(d).
				SetUpsert(true))
		}
		if _, err := s.days.BulkWrite(ctx, writes, options.BulkWrite().SetOrdered(false)); err != nil {
			return err
		}
	}
	if len(orgIDs) == 0 {
		return nil
	}
	_, err := s.days.DeleteMany(ctx, bson.M{
		"workspace_id":    wsID,
		"day":             day,
		"organization_id": bson.M{"$in": orgIDs},
		"user_id":         bson.M{"$nin": keep},
	})
	return err
}

// DaysQuery selects stored days. From and To are inclusive day strings;
// nil OrgIDs / UserIDs mean any.
type DaysQuery struct {
	WorkspaceID primitive.ObjectID
	From, To    string
	OrgIDs      []primitive.ObjectID
	UserIDs     []primitive.ObjectID
}

// ListDays returns the stored days matching the query, ordered by day then
// user id.
func (s *Store) ListDays(ctx context.Context, q DaysQuery) ([]models.MHSActivityDay, error) {
	f := bson.M{"workspace_id": q.WorkspaceID}
	if q.From != "" || q.To != "" {
		rng := bson.M{}
		if q.From != "" {
			rng["$gte"] = q.From
		}
		if q.To != "" {
			rng["$lte"] = q.To
		}
		f["day"] = rng
	}
	if q.OrgIDs != nil {
		f["organization_id"] = bson.M{"$in": q.OrgIDs}
	}
	if q.UserIDs != nil {
		f["user_id"] = bson.M{"$in": q.UserIDs}
	}
	cur, err := s.days.Find(ctx, f, options.Find().SetSort(bson.D{{Key: "day", Value: 1}, {Key: "user_id", Value: 1}}))
	if err != nil {
		return nil, err
	}
	defer cur.Close(ctx)
	var out []models.MHSActivityDay
	if err := cur.All(ctx, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// MarkKey is how GetMarks keys its result.
func MarkKey(orgID primitive.ObjectID, day string) string { return orgID.Hex() + "|" + day }

// GetMarks returns the marks of the given organizations for the days in
// [from, to], keyed by MarkKey.
func (s *Store) GetMarks(ctx context.Context, wsID primitive.ObjectID, from, to string, orgIDs []primitive.ObjectID) (map[string]models.MHSActivityMark, error) {
	f := bson.M{"workspace_id": wsID, "day": bson.M{"$gte": from, "$lte": to}}
	if orgIDs != nil {
		f["organization_id"] = bson.M{"$in": orgIDs}
	}
	cur, err := s.marks.Find(ctx, f)
	if err != nil {
		return nil, err
	}
	defer cur.Close(ctx)
	out := map[string]models.MHSActivityMark{}
	for cur.Next(ctx) {
		var m models.MHSActivityMark
		if err := cur.Decode(&m); err != nil {
			return nil, err
		}
		out[MarkKey(m.OrganizationID, m.Day)] = m
	}
	return out, cur.Err()
}

// SetMark upserts a mark by (workspace, organization, day).
func (s *Store) SetMark(ctx context.Context, m models.MHSActivityMark) error {
	if m.ComputedAt.IsZero() {
		m.ComputedAt = time.Now().UTC()
	}
	m.ID = primitive.NilObjectID
	_, err := s.marks.ReplaceOne(ctx,
		bson.M{"workspace_id": m.WorkspaceID, "organization_id": m.OrganizationID, "day": m.Day},
		m, options.Replace().SetUpsert(true))
	return err
}

// DeleteRange removes the stored days and marks of a workspace in [from, to]
// (a backfill that starts over).
func (s *Store) DeleteRange(ctx context.Context, wsID primitive.ObjectID, from, to string) error {
	f := bson.M{"workspace_id": wsID, "day": bson.M{"$gte": from, "$lte": to}}
	if _, err := s.days.DeleteMany(ctx, f); err != nil {
		return err
	}
	_, err := s.marks.DeleteMany(ctx, f)
	return err
}

// OutdatedMarks returns up to limit marks whose Version is older than
// version (days computed by an earlier reading), oldest day first.
func (s *Store) OutdatedMarks(ctx context.Context, wsID primitive.ObjectID, version, limit int) ([]models.MHSActivityMark, error) {
	cur, err := s.marks.Find(ctx, bson.M{"workspace_id": wsID, "version": bson.M{"$lt": version}},
		options.Find().SetSort(bson.D{{Key: "day", Value: 1}}).SetLimit(int64(limit)))
	if err != nil {
		return nil, err
	}
	defer cur.Close(ctx)
	var out []models.MHSActivityMark
	if err := cur.All(ctx, &out); err != nil {
		return nil, err
	}
	return out, nil
}
