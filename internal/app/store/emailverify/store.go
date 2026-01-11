// internal/app/store/emailverify/store.go
package emailverify

// Terminology: User Identifiers
//   - UserID / userID / user_id: The MongoDB ObjectID (_id) that uniquely identifies a user record
//   - LoginID / loginID / login_id: The human-readable string users type to log in

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
	"golang.org/x/crypto/bcrypt"
)

const (
	// CodeLength is the length of the verification code (6 digits).
	CodeLength = 6
	// TokenLength is the length of the magic link token in bytes (32 bytes = 64 hex chars).
	TokenLength = 32
	// DefaultExpiry is how long a verification code is valid.
	DefaultExpiry = 10 * time.Minute
	// BcryptCost for hashing codes.
	BcryptCost = 10
	// MaxVerifyAttempts is the maximum number of code verification attempts per verification.
	MaxVerifyAttempts = 5
	// MaxResends is the maximum number of code resends within the rate limit window.
	MaxResends = 3
	// ResendWindow is the time window for tracking resend rate limiting.
	ResendWindow = 10 * time.Minute
)

var (
	// ErrNotFound is returned when a verification record is not found or expired.
	ErrNotFound = errors.New("verification not found or expired")
	// ErrInvalidCode is returned when the code doesn't match.
	ErrInvalidCode = errors.New("invalid verification code")
	// ErrTooManyAttempts is returned when too many verification attempts have been made.
	ErrTooManyAttempts = errors.New("too many verification attempts")
	// ErrTooManyResends is returned when too many resend requests have been made.
	ErrTooManyResends = errors.New("too many resend requests")
)

// Verification represents a pending email verification.
type Verification struct {
	ID          primitive.ObjectID `bson:"_id,omitempty"`
	UserID      primitive.ObjectID `bson:"user_id"`
	Email       string             `bson:"email"`
	CodeHash    string             `bson:"code_hash"`  // bcrypt hash of the 6-digit code
	Token       string             `bson:"token"`      // UUID for magic link
	ExpiresAt   time.Time          `bson:"expires_at"` // TTL index field
	CreatedAt   time.Time          `bson:"created_at"`
	Attempts    int                `bson:"attempts"`     // Number of failed verification attempts
	ResendCount int                `bson:"resend_count"` // Number of times code was resent
	WindowStart time.Time          `bson:"window_start"` // Start of rate limit window for resends
}

// Store manages email verification records.
type Store struct {
	c      *mongo.Collection
	expiry time.Duration
}

// New creates a new Store with the specified expiry duration.
// If expiry is 0 or negative, DefaultExpiry (10 minutes) is used.
func New(db *mongo.Database, expiry time.Duration) *Store {
	if expiry <= 0 {
		expiry = DefaultExpiry
	}
	return &Store{
		c:      db.Collection("email_verifications"),
		expiry: expiry,
	}
}

// Expiry returns the expiry duration for verification codes.
func (s *Store) Expiry() time.Duration {
	return s.expiry
}

// EnsureIndexes creates necessary indexes including TTL index for auto-cleanup.
func (s *Store) EnsureIndexes(ctx context.Context) error {
	indexes := []mongo.IndexModel{
		{
			Keys:    bson.D{{Key: "expires_at", Value: 1}},
			Options: options.Index().SetName("idx_emailverify_expires_ttl").SetExpireAfterSeconds(0), // TTL index
		},
		{
			Keys:    bson.D{{Key: "token", Value: 1}},
			Options: options.Index().SetName("idx_emailverify_token"),
		},
		{
			Keys:    bson.D{{Key: "user_id", Value: 1}},
			Options: options.Index().SetName("idx_emailverify_user"),
		},
	}
	_, err := s.c.Indexes().CreateMany(ctx, indexes)
	return err
}

// CreateResult contains the generated code and token for a verification.
type CreateResult struct {
	Code        string // Plain text code to send to user
	Token       string // Token for magic link
	ResendCount int    // Number of resends for this verification (for audit logging)
}

// Create creates a new verification record.
// Returns the plain text code (to send via email) and the token (for magic link).
// If isResend is true, this counts against the resend rate limit.
func (s *Store) Create(ctx context.Context, userID primitive.ObjectID, email string, isResend bool) (*CreateResult, error) {
	now := time.Now()

	// Check for existing verification record
	var existing Verification
	err := s.c.FindOne(ctx, bson.M{"user_id": userID}).Decode(&existing)
	existingFound := err == nil

	// Rate limit resends
	if isResend && existingFound {
		// Check if we're within the rate limit window
		if now.Before(existing.WindowStart.Add(ResendWindow)) {
			if existing.ResendCount >= MaxResends {
				return nil, ErrTooManyResends
			}
		}
	}

	// Generate 6-digit code
	code := generateCode()

	// Hash the code for storage
	hash, err := bcrypt.GenerateFromPassword([]byte(code), BcryptCost)
	if err != nil {
		return nil, fmt.Errorf("hash code: %w", err)
	}

	// Generate token for magic link
	token := generateToken()

	// Calculate resend count and window start
	resendCount := 0
	windowStart := now
	if existingFound {
		// If within the window, carry over the count
		if now.Before(existing.WindowStart.Add(ResendWindow)) {
			windowStart = existing.WindowStart
			if isResend {
				resendCount = existing.ResendCount + 1
			} else {
				resendCount = existing.ResendCount
			}
		}
		// Otherwise, start fresh (window expired)
	}

	// Delete any existing verifications for this user
	_, _ = s.c.DeleteMany(ctx, bson.M{"user_id": userID})

	v := Verification{
		ID:          primitive.NewObjectID(),
		UserID:      userID,
		Email:       email,
		CodeHash:    string(hash),
		Token:       token,
		ExpiresAt:   now.Add(s.expiry),
		CreatedAt:   now,
		Attempts:    0,
		ResendCount: resendCount,
		WindowStart: windowStart,
	}

	if _, err := s.c.InsertOne(ctx, v); err != nil {
		return nil, fmt.Errorf("insert verification: %w", err)
	}

	return &CreateResult{
		Code:        code,
		Token:       token,
		ResendCount: resendCount,
	}, nil
}

// VerifyCode verifies a code for a user and returns the verification record if valid.
// The record is deleted after successful verification.
// Returns ErrTooManyAttempts if the maximum number of attempts has been exceeded.
func (s *Store) VerifyCode(ctx context.Context, userID primitive.ObjectID, code string) (*Verification, error) {
	var v Verification
	err := s.c.FindOne(ctx, bson.M{
		"user_id":    userID,
		"expires_at": bson.M{"$gt": time.Now()},
	}).Decode(&v)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			return nil, ErrNotFound
		}
		return nil, err
	}

	// Check if too many attempts have been made
	if v.Attempts >= MaxVerifyAttempts {
		return nil, ErrTooManyAttempts
	}

	// Increment attempts counter before checking (counts both valid and invalid attempts)
	_, _ = s.c.UpdateOne(ctx, bson.M{"_id": v.ID}, bson.M{"$inc": bson.M{"attempts": 1}})

	// Verify the code
	if err := bcrypt.CompareHashAndPassword([]byte(v.CodeHash), []byte(code)); err != nil {
		return nil, ErrInvalidCode
	}

	// Delete the verification record (single use)
	_, _ = s.c.DeleteOne(ctx, bson.M{"_id": v.ID})

	return &v, nil
}

// VerifyToken verifies a magic link token and returns the verification record if valid.
// The record is deleted after successful verification.
func (s *Store) VerifyToken(ctx context.Context, token string) (*Verification, error) {
	var v Verification
	err := s.c.FindOne(ctx, bson.M{
		"token":      token,
		"expires_at": bson.M{"$gt": time.Now()},
	}).Decode(&v)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			return nil, ErrNotFound
		}
		return nil, err
	}

	// Delete the verification record (single use)
	_, _ = s.c.DeleteOne(ctx, bson.M{"_id": v.ID})

	return &v, nil
}

// DeleteByUser deletes all verification records for a user.
func (s *Store) DeleteByUser(ctx context.Context, userID primitive.ObjectID) error {
	_, err := s.c.DeleteMany(ctx, bson.M{"user_id": userID})
	return err
}

// generateCode generates a random 6-digit numeric code.
// Panics if the system's cryptographic random number generator fails.
func generateCode() string {
	b := make([]byte, 4)
	if _, err := rand.Read(b); err != nil {
		panic("crypto/rand.Read failed: " + err.Error())
	}
	// Convert to a number and take last 6 digits
	n := int(b[0])<<24 | int(b[1])<<16 | int(b[2])<<8 | int(b[3])
	if n < 0 {
		n = -n
	}
	// Ensure 6 digits (100000 to 999999)
	code := (n % 900000) + 100000
	return fmt.Sprintf("%06d", code)
}

// generateToken generates a random token for magic links.
// Panics if the system's cryptographic random number generator fails.
func generateToken() string {
	b := make([]byte, TokenLength)
	if _, err := rand.Read(b); err != nil {
		panic("crypto/rand.Read failed: " + err.Error())
	}
	return hex.EncodeToString(b)
}
