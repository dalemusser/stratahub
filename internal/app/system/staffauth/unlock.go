package staffauth

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// Unlock is a server-side "staff unlock" record for the MHS manage page.
// After staff authorize on a member's device, gated actions pass without
// re-prompting until the unlock expires (sliding) or is revoked. The record —
// not any client state — is the source of truth; every gated request checks it.
type Unlock struct {
	ID          primitive.ObjectID `bson:"_id,omitempty"`
	Key         string             `bson:"key"` // SHA-256 of workspace|user|session token
	WorkspaceID primitive.ObjectID `bson:"workspace_id"`
	UserID      primitive.ObjectID `bson:"user_id"`
	GrantedBy   string             `bson:"granted_by"` // staff name, or "keyword" in keyword mode
	CreatedAt   time.Time          `bson:"created_at"`
	ExpiresAt   time.Time          `bson:"expires_at"` // TTL index; slides on each gated action
}

// UnlockStore manages staff unlock records.
type UnlockStore struct {
	c *mongo.Collection
}

// NewUnlockStore creates a new UnlockStore.
func NewUnlockStore(db *mongo.Database) *UnlockStore {
	return &UnlockStore{c: db.Collection("mhs_staff_unlocks")}
}

// EnsureIndexes creates the TTL and lookup indexes.
func (s *UnlockStore) EnsureIndexes(ctx context.Context) error {
	indexes := []mongo.IndexModel{
		{
			Keys:    bson.D{{Key: "expires_at", Value: 1}},
			Options: options.Index().SetName("idx_staffunlock_expires_ttl").SetExpireAfterSeconds(0),
		},
		{
			Keys:    bson.D{{Key: "key", Value: 1}},
			Options: options.Index().SetName("idx_staffunlock_key").SetUnique(true),
		},
	}
	_, err := s.c.Indexes().CreateMany(ctx, indexes)
	return err
}

// UnlockKey derives the lookup key for an unlock. Including the session token
// scopes the unlock to the member's current session: it dies with logout and
// cannot follow the account to another device.
func UnlockKey(wsID, userID primitive.ObjectID, sessionToken string) string {
	h := sha256.Sum256([]byte(wsID.Hex() + "|" + userID.Hex() + "|" + sessionToken))
	return hex.EncodeToString(h[:])
}

// Grant creates (or replaces) the unlock for a key with the given duration.
func (s *UnlockStore) Grant(ctx context.Context, key string, wsID, userID primitive.ObjectID, grantedBy string, d time.Duration) error {
	now := time.Now().UTC()
	filter := bson.M{"key": key}
	update := bson.M{
		"$set": bson.M{
			"workspace_id": wsID,
			"user_id":      userID,
			"granted_by":   grantedBy,
			"created_at":   now,
			"expires_at":   now.Add(d),
		},
		"$setOnInsert": bson.M{
			"_id": primitive.NewObjectID(),
			"key": key,
		},
	}
	_, err := s.c.UpdateOne(ctx, filter, update, options.Update().SetUpsert(true))
	return err
}

// GetActive returns the unexpired unlock for a key, or nil when none exists.
func (s *UnlockStore) GetActive(ctx context.Context, key string) (*Unlock, error) {
	var u Unlock
	err := s.c.FindOne(ctx, bson.M{"key": key, "expires_at": bson.M{"$gt": time.Now().UTC()}}).Decode(&u)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			return nil, nil
		}
		return nil, err
	}
	return &u, nil
}

// Refresh slides an active unlock's expiry to now+d. A no-op if the unlock
// has already expired (the TTL monitor may not have removed it yet).
func (s *UnlockStore) Refresh(ctx context.Context, key string, d time.Duration) error {
	now := time.Now().UTC()
	_, err := s.c.UpdateOne(ctx,
		bson.M{"key": key, "expires_at": bson.M{"$gt": now}},
		bson.M{"$set": bson.M{"expires_at": now.Add(d)}},
	)
	return err
}

// Revoke removes the unlock for a key ("Lock now").
func (s *UnlockStore) Revoke(ctx context.Context, key string) error {
	_, err := s.c.DeleteOne(ctx, bson.M{"key": key})
	return err
}
