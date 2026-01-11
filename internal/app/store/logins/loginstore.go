// internal/app/features/store/logins/loginstore.go
package loginstore

// Terminology: User Identifiers
//   - UserID / userID / user_id: The MongoDB ObjectID (_id) that uniquely identifies a user record
//   - LoginID / loginID / login_id: The human-readable string users type to log in

import (
	"context"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/dalemusser/stratahub/internal/domain/models"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
)

type Store struct {
	c *mongo.Collection
}

func New(db *mongo.Database) *Store {
	return &Store{c: db.Collection("login_records")}
}

// Create inserts a LoginRecord. If CreatedAt is zero, it's set to time.Now().UTC().
func (s *Store) Create(ctx context.Context, rec models.LoginRecord) error {
	if rec.CreatedAt.IsZero() {
		rec.CreatedAt = time.Now().UTC()
	}
	_, err := s.c.InsertOne(ctx, rec)
	return err
}

// CreateFrom builds a LoginRecord from the HTTP request and inserts it.
// It extracts client IP (X-Forwarded-For → X-Real-IP → RemoteAddr) and user agent.
func (s *Store) CreateFrom(ctx context.Context, r *http.Request, userID primitive.ObjectID, provider string) error {
	rec := models.LoginRecord{
		UserID:    userID.String(),
		CreatedAt: time.Now().UTC(),
		IP:        clientIP(r),
		Provider:  provider,
	}
	_, err := s.c.InsertOne(ctx, rec)
	return err
}

func clientIP(r *http.Request) string {
	// Respect common proxy headers first
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		// XFF may contain a list; first is original client
		parts := strings.Split(xff, ",")
		if len(parts) > 0 {
			return strings.TrimSpace(parts[0])
		}
	}
	if xr := r.Header.Get("X-Real-IP"); xr != "" {
		return strings.TrimSpace(xr)
	}
	// Fallback: parse RemoteAddr "ip:port"
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err == nil && host != "" {
		return host
	}
	return r.RemoteAddr
}
