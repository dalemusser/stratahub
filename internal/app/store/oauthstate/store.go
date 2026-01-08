// internal/app/store/oauthstate/store.go
package oauthstate

import (
	"context"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// State represents an OAuth2 state token stored for CSRF protection.
type State struct {
	State     string    `bson:"state"`
	ReturnURL string    `bson:"return_url,omitempty"` // Where to redirect after auth
	Workspace string    `bson:"workspace,omitempty"`  // Workspace subdomain (for multi-workspace OAuth)
	ExpiresAt time.Time `bson:"expires_at"`
	CreatedAt time.Time `bson:"created_at"`
}

// Store manages OAuth2 state tokens in MongoDB.
type Store struct {
	c *mongo.Collection
}

// New creates a new OAuth state Store.
func New(db *mongo.Database) *Store {
	return &Store{c: db.Collection("oauth_states")}
}

// EnsureIndexes creates indexes for efficient querying and TTL expiration.
func (s *Store) EnsureIndexes(ctx context.Context) error {
	indexes := []mongo.IndexModel{
		// Primary lookup by state
		{
			Keys:    bson.D{{Key: "state", Value: 1}},
			Options: options.Index().SetUnique(true).SetName("idx_oauth_state"),
		},
		// TTL index for automatic cleanup
		{
			Keys:    bson.D{{Key: "expires_at", Value: 1}},
			Options: options.Index().SetExpireAfterSeconds(0).SetName("idx_oauth_ttl"),
		},
	}
	_, err := s.c.Indexes().CreateMany(ctx, indexes)
	return err
}

// Save stores a state token with the given expiration time.
// Optionally includes a return URL and workspace subdomain for multi-workspace OAuth.
func (s *Store) Save(ctx context.Context, state, returnURL, workspace string, expiresAt time.Time) error {
	st := State{
		State:     state,
		ReturnURL: returnURL,
		Workspace: workspace,
		ExpiresAt: expiresAt,
		CreatedAt: time.Now().UTC(),
	}
	_, err := s.c.InsertOne(ctx, st)
	return err
}

// Validate checks if a state token exists and is not expired.
// If valid, it deletes the token (one-time use) and returns the associated return URL and workspace.
// Returns empty strings and false if the state is invalid or expired.
func (s *Store) Validate(ctx context.Context, state string) (returnURL, workspace string, valid bool, err error) {
	var st State
	err = s.c.FindOneAndDelete(ctx, bson.M{
		"state":      state,
		"expires_at": bson.M{"$gt": time.Now().UTC()},
	}).Decode(&st)

	if err == mongo.ErrNoDocuments {
		return "", "", false, nil
	}
	if err != nil {
		return "", "", false, err
	}

	return st.ReturnURL, st.Workspace, true, nil
}

// CleanupExpired removes expired state tokens.
// This is a backup for when TTL index cleanup is delayed.
func (s *Store) CleanupExpired(ctx context.Context) (int64, error) {
	result, err := s.c.DeleteMany(ctx, bson.M{
		"expires_at": bson.M{"$lt": time.Now().UTC()},
	})
	if err != nil {
		return 0, err
	}
	return result.DeletedCount, nil
}
