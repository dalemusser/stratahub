// internal/app/store/memberstatus/store.go
package memberstatus

// Terminology: User Identifiers
//   - UserID / userID / user_id: The MongoDB ObjectID (_id) that uniquely identifies a user record
//   - LoginID / loginID / login_id: The human-readable string users type to log in

import (
	"context"
	"errors"
	"time"

	"github.com/dalemusser/stratahub/internal/domain/models"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// ErrInvalidState is returned by Record for a state that cannot be stored.
var ErrInvalidState = errors.New("memberstatus: invalid state")

// ErrMissingKey is returned by Record when the entity key is empty.
var ErrMissingKey = errors.New("memberstatus: entity key is required")

// Store provides access to the member_status collection.
type Store struct {
	c *mongo.Collection
}

// New creates a new member status store.
func New(db *mongo.Database) *Store {
	return &Store{c: db.Collection("member_status")}
}

// RecordInput describes one status event to record.
type RecordInput struct {
	WorkspaceID primitive.ObjectID
	UserID      primitive.ObjectID
	EntityKey   string     // canonical key (config item id, or "name:<folded>" for unknown names)
	Entity      string     // display name; stored only when the document is first created
	State       string     // models.MemberStatusOpened / Started / Completed
	Source      string     // models.MemberStatusSourceAPI / Launch (defaults to API)
	OccurredAt  *time.Time // the writer's own timestamp, if any
	RemoteIP    string
}

// Record applies one status event to the (workspace, user, entity) document,
// creating it if needed, and returns the resulting document.
//
// Semantics — idempotent and monotonic:
//   - The state only ever moves up the ladder (opened < started < completed).
//     A lower state arriving later is kept in history but never regresses State.
//   - Each state's timestamp is first-wins: the first time StrataHub records
//     that state sets <state>_at; repeats leave it unchanged.
//   - Every accepted event is appended to History, capped at
//     models.MemberStatusHistoryLimit (oldest dropped first).
//
// The whole update is a single upsert built from $setOnInsert / $set / $max /
// $min / $push, all of which DocumentDB supports; no update pipeline is used.
func (s *Store) Record(ctx context.Context, in RecordInput) (models.MemberStatus, error) {
	rank := models.MemberStatusRank(in.State)
	if rank == 0 {
		return models.MemberStatus{}, ErrInvalidState
	}
	if in.EntityKey == "" {
		return models.MemberStatus{}, ErrMissingKey
	}
	if in.Source == "" {
		in.Source = models.MemberStatusSourceAPI
	}

	now := time.Now().UTC()
	event := models.MemberStatusEvent{
		State:      in.State,
		Source:     in.Source,
		ReceivedAt: now,
		OccurredAt: in.OccurredAt,
		RemoteIP:   in.RemoteIP,
	}

	filter := bson.M{
		"workspace_id": in.WorkspaceID,
		"user_id":      in.UserID,
		"entity_key":   in.EntityKey,
	}
	update := bson.M{
		"$setOnInsert": bson.M{
			"_id":               primitive.NewObjectID(),
			"entity":            in.Entity,
			"state":             in.State, // corrected below if a higher rank already exists
			"created_at":        now,
			"first_received_at": now,
		},
		"$set": bson.M{
			"source":           in.Source,
			"last_received_at": now,
			"updated_at":       now,
		},
		// Raise the state, never lower it.
		"$max": bson.M{"state_rank": rank},
		// First-wins timestamp for this state ($min sets a missing field).
		"$min": bson.M{in.State + "_at": now},
		"$push": bson.M{
			"history": bson.M{
				"$each":  []models.MemberStatusEvent{event},
				"$slice": -models.MemberStatusHistoryLimit,
			},
		},
	}

	opts := options.FindOneAndUpdate().
		SetUpsert(true).
		SetReturnDocument(options.After)

	var doc models.MemberStatus
	err := s.c.FindOneAndUpdate(ctx, filter, update, opts).Decode(&doc)
	if mongo.IsDuplicateKeyError(err) {
		// Two concurrent first events for the same (user, entity) raced the
		// upsert: both missed the filter and both tried to insert. The loser's
		// retry now matches the winner's document and becomes a plain update.
		err = s.c.FindOneAndUpdate(ctx, filter, update, opts).Decode(&doc)
	}
	if err != nil {
		return models.MemberStatus{}, err
	}

	// Keep the human-readable State in step with the rank $max produced. The
	// write is conditioned on the rank we observed so a concurrent, higher
	// event's label can never be overwritten by a lower one.
	if want := models.MemberStatusStateForRank(doc.StateRank); doc.State != want {
		_, err = s.c.UpdateOne(ctx,
			bson.M{"_id": doc.ID, "state_rank": doc.StateRank},
			bson.M{"$set": bson.M{"state": want}},
		)
		if err != nil {
			return models.MemberStatus{}, err
		}
		doc.State = want
	}
	return doc, nil
}

// Get returns the document for one (workspace, user, entity), or
// mongo.ErrNoDocuments if the member has no recorded status for it.
func (s *Store) Get(ctx context.Context, workspaceID, userID primitive.ObjectID, entityKey string) (models.MemberStatus, error) {
	var doc models.MemberStatus
	err := s.c.FindOne(ctx, bson.M{
		"workspace_id": workspaceID,
		"user_id":      userID,
		"entity_key":   entityKey,
	}).Decode(&doc)
	return doc, err
}

// ListForUser returns every status document for one member in a workspace,
// ordered by entity key.
func (s *Store) ListForUser(ctx context.Context, workspaceID, userID primitive.ObjectID) ([]models.MemberStatus, error) {
	cur, err := s.c.Find(ctx,
		bson.M{"workspace_id": workspaceID, "user_id": userID},
		options.Find().SetSort(bson.D{{Key: "entity_key", Value: 1}}),
	)
	if err != nil {
		return nil, err
	}
	defer cur.Close(ctx)

	var results []models.MemberStatus
	if err := cur.All(ctx, &results); err != nil {
		return nil, err
	}
	return results, nil
}

// ListByUserIDs returns the status documents for a set of members in a
// workspace, keyed by user ID hex and then by entity key. This is the batch
// lookup the dashboard uses for one group at a time.
func (s *Store) ListByUserIDs(ctx context.Context, workspaceID primitive.ObjectID, userIDs []primitive.ObjectID) (map[string]map[string]models.MemberStatus, error) {
	result := make(map[string]map[string]models.MemberStatus)
	if len(userIDs) == 0 {
		return result, nil
	}

	cur, err := s.c.Find(ctx, bson.M{
		"workspace_id": workspaceID,
		"user_id":      bson.M{"$in": userIDs},
	})
	if err != nil {
		return nil, err
	}
	defer cur.Close(ctx)

	for cur.Next(ctx) {
		var doc models.MemberStatus
		if err := cur.Decode(&doc); err != nil {
			return nil, err
		}
		userHex := doc.UserID.Hex()
		if result[userHex] == nil {
			result[userHex] = make(map[string]models.MemberStatus)
		}
		result[userHex][doc.EntityKey] = doc
	}
	return result, cur.Err()
}

// DeleteByUser removes all status documents for a member in a workspace.
func (s *Store) DeleteByUser(ctx context.Context, workspaceID, userID primitive.ObjectID) (int64, error) {
	res, err := s.c.DeleteMany(ctx, bson.M{"workspace_id": workspaceID, "user_id": userID})
	if err != nil {
		return 0, err
	}
	return res.DeletedCount, nil
}
