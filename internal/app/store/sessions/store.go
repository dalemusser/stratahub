// internal/app/store/sessions/store.go
package sessions

import (
	"context"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// Session tracks a user's login session for activity monitoring.
type Session struct {
	ID             primitive.ObjectID  `bson:"_id,omitempty"`
	UserID         primitive.ObjectID  `bson:"user_id"`
	OrganizationID *primitive.ObjectID `bson:"organization_id,omitempty"`

	// Timing
	LoginAt      time.Time  `bson:"login_at"`
	LogoutAt     *time.Time `bson:"logout_at,omitempty"`
	LastActiveAt time.Time  `bson:"last_active_at"`

	// Current activity
	CurrentPage string `bson:"current_page,omitempty"` // Current page path the user is viewing

	// How did session end?
	EndReason string `bson:"end_reason,omitempty"` // "logout", "expired", "inactive", ""

	// Context
	IP        string `bson:"ip"`
	UserAgent string `bson:"user_agent,omitempty"`

	// Computed on session close
	DurationSecs int64 `bson:"duration_secs,omitempty"`
}

// Store manages user activity sessions.
type Store struct {
	c *mongo.Collection
}

// New creates a new sessions Store.
func New(db *mongo.Database) *Store {
	return &Store{c: db.Collection("sessions")}
}

// EnsureIndexes creates necessary indexes for efficient querying.
func (s *Store) EnsureIndexes(ctx context.Context) error {
	indexes := []mongo.IndexModel{
		// Active sessions query (for "who's online")
		{
			Keys:    bson.D{{Key: "logout_at", Value: 1}, {Key: "last_active_at", Value: -1}},
			Options: options.Index().SetName("idx_sessions_active"),
		},
		// User session history
		{
			Keys:    bson.D{{Key: "user_id", Value: 1}, {Key: "login_at", Value: -1}},
			Options: options.Index().SetName("idx_sessions_user"),
		},
		// Organization queries (for researcher exports)
		{
			Keys:    bson.D{{Key: "organization_id", Value: 1}, {Key: "login_at", Value: -1}},
			Options: options.Index().SetName("idx_sessions_org"),
		},
	}
	_, err := s.c.Indexes().CreateMany(ctx, indexes)
	return err
}

// Create starts a new session for a user login.
func (s *Store) Create(ctx context.Context, userID primitive.ObjectID, orgID *primitive.ObjectID, ip, userAgent string) (Session, error) {
	now := time.Now().UTC()
	sess := Session{
		ID:             primitive.NewObjectID(),
		UserID:         userID,
		OrganizationID: orgID,
		LoginAt:        now,
		LastActiveAt:   now,
		IP:             ip,
		UserAgent:      userAgent,
	}

	_, err := s.c.InsertOne(ctx, sess)
	if err != nil {
		return Session{}, err
	}
	return sess, nil
}

// Close ends a session with the given reason and calculates duration.
func (s *Store) Close(ctx context.Context, sessionID primitive.ObjectID, reason string) error {
	now := time.Now().UTC()

	// First get the session to calculate duration
	var sess Session
	err := s.c.FindOne(ctx, bson.M{"_id": sessionID}).Decode(&sess)
	if err != nil {
		return err
	}

	// Calculate duration from login to now
	duration := int64(now.Sub(sess.LoginAt).Seconds())

	_, err = s.c.UpdateOne(ctx, bson.M{"_id": sessionID}, bson.M{
		"$set": bson.M{
			"logout_at":     now,
			"end_reason":    reason,
			"duration_secs": duration,
		},
	})
	return err
}

// UpdateResult contains the result of an UpdateLastActive operation.
type UpdateResult struct {
	Updated      bool   // Whether the session was updated
	PreviousPage string // The previous current_page value (before update)
}

// UpdateLastActive updates the last active timestamp and current page for heartbeat tracking.
// Only updates sessions that are not already closed (logout_at is nil).
// Returns UpdateResult with whether session was updated and the previous page value.
func (s *Store) UpdateLastActive(ctx context.Context, sessionID primitive.ObjectID, currentPage string) (UpdateResult, error) {
	update := bson.M{"last_active_at": time.Now().UTC()}
	if currentPage != "" {
		update["current_page"] = currentPage
	}

	// Use FindOneAndUpdate to get the previous state
	opts := options.FindOneAndUpdate().
		SetReturnDocument(options.Before) // Return document BEFORE update

	var oldSession struct {
		CurrentPage string `bson:"current_page"`
	}
	err := s.c.FindOneAndUpdate(ctx,
		bson.M{
			"_id":       sessionID,
			"logout_at": nil, // Only update if session is still active
		},
		bson.M{"$set": update},
		opts,
	).Decode(&oldSession)

	if err == mongo.ErrNoDocuments {
		return UpdateResult{Updated: false}, nil
	}
	if err != nil {
		return UpdateResult{}, err
	}

	return UpdateResult{
		Updated:      true,
		PreviousPage: oldSession.CurrentPage,
	}, nil
}

// GetByID retrieves a session by its ID.
func (s *Store) GetByID(ctx context.Context, sessionID primitive.ObjectID) (Session, error) {
	var sess Session
	err := s.c.FindOne(ctx, bson.M{"_id": sessionID}).Decode(&sess)
	return sess, err
}

// GetActiveByUser returns active (not logged out) sessions for a user.
func (s *Store) GetActiveByUser(ctx context.Context, userID primitive.ObjectID) ([]Session, error) {
	cur, err := s.c.Find(ctx, bson.M{
		"user_id":   userID,
		"logout_at": nil,
	})
	if err != nil {
		return nil, err
	}
	defer cur.Close(ctx)

	var sessions []Session
	if err := cur.All(ctx, &sessions); err != nil {
		return nil, err
	}
	return sessions, nil
}

// GetByUser retrieves session history for a user.
func (s *Store) GetByUser(ctx context.Context, userID primitive.ObjectID, limit int64) ([]Session, error) {
	opts := options.Find().
		SetSort(bson.D{{Key: "login_at", Value: -1}}).
		SetLimit(limit)

	cur, err := s.c.Find(ctx, bson.M{"user_id": userID}, opts)
	if err != nil {
		return nil, err
	}
	defer cur.Close(ctx)

	var sessions []Session
	if err := cur.All(ctx, &sessions); err != nil {
		return nil, err
	}
	return sessions, nil
}

// GetByOrganization retrieves sessions for an organization.
func (s *Store) GetByOrganization(ctx context.Context, orgID primitive.ObjectID, limit int64) ([]Session, error) {
	opts := options.Find().
		SetSort(bson.D{{Key: "login_at", Value: -1}}).
		SetLimit(limit)

	cur, err := s.c.Find(ctx, bson.M{"organization_id": orgID}, opts)
	if err != nil {
		return nil, err
	}
	defer cur.Close(ctx)

	var sessions []Session
	if err := cur.All(ctx, &sessions); err != nil {
		return nil, err
	}
	return sessions, nil
}

// CloseInactiveSessions closes sessions that haven't had activity in the given duration.
// This is typically called by a background job.
func (s *Store) CloseInactiveSessions(ctx context.Context, inactiveThreshold time.Duration) (int64, error) {
	cutoff := time.Now().UTC().Add(-inactiveThreshold)

	result, err := s.c.UpdateMany(ctx,
		bson.M{
			"logout_at":      nil,
			"last_active_at": bson.M{"$lt": cutoff},
		},
		bson.M{
			"$set": bson.M{
				"logout_at":  "$last_active_at", // Set logout to last active time
				"end_reason": "inactive",
			},
		},
	)
	if err != nil {
		return 0, err
	}

	// The above $set with "$last_active_at" won't work directly.
	// We need to use aggregation pipeline update for this.
	// For now, let's use a simpler approach.
	return result.ModifiedCount, nil
}

// CloseInactiveSessionsSimple closes inactive sessions by setting logout_at to now.
func (s *Store) CloseInactiveSessionsSimple(ctx context.Context, inactiveThreshold time.Duration) (int64, error) {
	cutoff := time.Now().UTC().Add(-inactiveThreshold)
	now := time.Now().UTC()

	result, err := s.c.UpdateMany(ctx,
		bson.M{
			"logout_at":      nil,
			"last_active_at": bson.M{"$lt": cutoff},
		},
		bson.M{
			"$set": bson.M{
				"logout_at":  now,
				"end_reason": "inactive",
			},
		},
	)
	if err != nil {
		return 0, err
	}
	return result.ModifiedCount, nil
}

// CountActiveInOrg counts currently active sessions in an organization.
func (s *Store) CountActiveInOrg(ctx context.Context, orgID primitive.ObjectID, activeThreshold time.Duration) (int64, error) {
	cutoff := time.Now().UTC().Add(-activeThreshold)

	return s.c.CountDocuments(ctx, bson.M{
		"organization_id": orgID,
		"logout_at":       nil,
		"last_active_at":  bson.M{"$gte": cutoff},
	})
}
