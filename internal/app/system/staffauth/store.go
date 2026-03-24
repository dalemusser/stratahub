// Package staffauth provides a reusable "supervisor override" verification flow.
// It verifies that a staff-level user (leader+) authenticated successfully and
// issues a short-lived, single-use authorization token. It does NOT create sessions.
package staffauth

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

const (
	// TokenLength is the length of random tokens in bytes (16 bytes = 32 hex chars).
	TokenLength = 16
	// DefaultChallengeExpiry is how long a challenge/token is valid if no expiry is specified.
	DefaultChallengeExpiry = 10 * time.Minute
)

var (
	ErrChallengeNotFound = errors.New("challenge not found")
	ErrChallengeExpired  = errors.New("challenge expired")
	ErrTokenNotFound     = errors.New("authorization token not found")
	ErrTokenUsed         = errors.New("authorization token already used")
)

// Challenge represents a pending or completed staff authentication challenge.
type Challenge struct {
	ID          primitive.ObjectID `bson:"_id,omitempty"`
	ChallengeID string             `bson:"challenge_id"` // Random hex lookup key
	Token       string             `bson:"token"`        // Random hex, empty until verified
	WorkspaceID primitive.ObjectID `bson:"workspace_id"`
	UserID      primitive.ObjectID `bson:"user_id"`
	StaffName   string             `bson:"staff_name"`
	Method      string             `bson:"method"` // trust, password, email
	Verified    bool               `bson:"verified"`
	Used        bool               `bson:"used"`
	CreatedAt   time.Time          `bson:"created_at"`
	ExpiresAt   time.Time          `bson:"expires_at"` // TTL index
}

// Store manages staff auth challenge/token records.
type Store struct {
	c      *mongo.Collection
	expiry time.Duration
}

// NewStore creates a new Store with the specified challenge expiry duration.
// If expiry is 0 or negative, DefaultChallengeExpiry is used.
func NewStore(db *mongo.Database, expiry time.Duration) *Store {
	if expiry <= 0 {
		expiry = DefaultChallengeExpiry
	}
	return &Store{
		c:      db.Collection("staff_auth_tokens"),
		expiry: expiry,
	}
}

// EnsureIndexes creates necessary indexes including TTL index for auto-cleanup.
func (s *Store) EnsureIndexes(ctx context.Context) error {
	indexes := []mongo.IndexModel{
		{
			Keys:    bson.D{{Key: "expires_at", Value: 1}},
			Options: options.Index().SetName("idx_staffauth_expires_ttl").SetExpireAfterSeconds(0),
		},
		{
			Keys:    bson.D{{Key: "challenge_id", Value: 1}},
			Options: options.Index().SetName("idx_staffauth_challenge_id"),
		},
		{
			Keys:    bson.D{{Key: "token", Value: 1}},
			Options: options.Index().SetName("idx_staffauth_token").SetSparse(true),
		},
	}
	_, err := s.c.Indexes().CreateMany(ctx, indexes)
	return err
}

// CreateChallenge stores a new challenge record and returns the challenge ID.
func (s *Store) CreateChallenge(ctx context.Context, wsID, userID primitive.ObjectID, staffName, method string) (string, error) {
	challengeID := generateHex()
	now := time.Now()

	ch := Challenge{
		ID:          primitive.NewObjectID(),
		ChallengeID: challengeID,
		WorkspaceID: wsID,
		UserID:      userID,
		StaffName:   staffName,
		Method:      method,
		Verified:    false,
		Used:        false,
		CreatedAt:   now,
		ExpiresAt:   now.Add(s.expiry),
	}

	if _, err := s.c.InsertOne(ctx, ch); err != nil {
		return "", fmt.Errorf("insert challenge: %w", err)
	}
	return challengeID, nil
}

// CreateVerifiedToken stores a pre-verified challenge (for trust auth method)
// and returns the token directly.
func (s *Store) CreateVerifiedToken(ctx context.Context, wsID, userID primitive.ObjectID, staffName string) (string, error) {
	challengeID := generateHex()
	token := generateHex()
	now := time.Now()

	ch := Challenge{
		ID:          primitive.NewObjectID(),
		ChallengeID: challengeID,
		Token:       token,
		WorkspaceID: wsID,
		UserID:      userID,
		StaffName:   staffName,
		Method:      "trust",
		Verified:    true,
		Used:        false,
		CreatedAt:   now,
		ExpiresAt:   now.Add(s.expiry),
	}

	if _, err := s.c.InsertOne(ctx, ch); err != nil {
		return "", fmt.Errorf("insert verified token: %w", err)
	}
	return token, nil
}

// GetChallenge retrieves a challenge by its challenge ID.
func (s *Store) GetChallenge(ctx context.Context, challengeID string) (*Challenge, error) {
	var ch Challenge
	err := s.c.FindOne(ctx, bson.M{"challenge_id": challengeID}).Decode(&ch)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			return nil, ErrChallengeNotFound
		}
		return nil, err
	}
	if time.Now().After(ch.ExpiresAt) {
		return nil, ErrChallengeExpired
	}
	return &ch, nil
}

// MarkVerified sets a challenge as verified and assigns a token.
// Returns the generated token.
func (s *Store) MarkVerified(ctx context.Context, challengeID string) (string, error) {
	token := generateHex()
	res, err := s.c.UpdateOne(ctx,
		bson.M{"challenge_id": challengeID, "verified": false},
		bson.M{"$set": bson.M{"verified": true, "token": token}},
	)
	if err != nil {
		return "", err
	}
	if res.MatchedCount == 0 {
		return "", ErrChallengeNotFound
	}
	return token, nil
}

// ValidateAndConsumeToken checks that a token exists, is verified, and unused.
// On success, marks it as used (single-use) and returns the challenge.
func (s *Store) ValidateAndConsumeToken(ctx context.Context, token string) (*Challenge, error) {
	var ch Challenge
	err := s.c.FindOneAndUpdate(ctx,
		bson.M{"token": token, "verified": true, "used": false, "expires_at": bson.M{"$gt": time.Now()}},
		bson.M{"$set": bson.M{"used": true}},
	).Decode(&ch)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			return nil, ErrTokenNotFound
		}
		return nil, err
	}
	return &ch, nil
}

func generateHex() string {
	b := make([]byte, TokenLength)
	if _, err := rand.Read(b); err != nil {
		panic("crypto/rand.Read failed: " + err.Error())
	}
	return hex.EncodeToString(b)
}
