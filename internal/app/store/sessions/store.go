// internal/app/store/sessions/store.go
package sessions

// Terminology: User Identifiers
//   - UserID / userID / user_id: The MongoDB ObjectID (_id) that uniquely identifies a user record
//   - LoginID / loginID / login_id: The human-readable string users type to log in

import (
	"context"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// Session end reasons
const (
	EndReasonLogout          = "logout"           // User explicitly logged out
	EndReasonExpired         = "expired"          // Session expired via TTL
	EndReasonInactive        = "inactive"         // Closed due to inactivity
	EndReasonAdminTerminated = "admin_terminated" // Closed by admin
)

// Session creation sources
const (
	CreatedByLogin     = "login"     // User explicitly logged in
	CreatedByHeartbeat = "heartbeat" // Session recreated by heartbeat after timeout
)

// Session tracks a user's login session for activity monitoring.
// Uses token-based identification for security (aligned with strata).
type Session struct {
	ID    primitive.ObjectID `bson:"_id,omitempty"`
	Token string             `bson:"token"` // Unique 32-byte random token

	UserID         primitive.ObjectID  `bson:"user_id"`
	OrganizationID *primitive.ObjectID `bson:"organization_id,omitempty"` // Stratahub-specific

	// Client info
	IP        string `bson:"ip"`
	UserAgent string `bson:"user_agent,omitempty"`

	// Activity tracking
	CurrentPage  string     `bson:"current_page,omitempty"`  // Current page user is viewing
	LoginAt      time.Time  `bson:"login_at"`                // When session started
	LogoutAt     *time.Time `bson:"logout_at,omitempty"`     // When session ended (nil if active)
	LastActivity time.Time  `bson:"last_activity"`           // Last heartbeat/activity
	EndReason    string     `bson:"end_reason,omitempty"`    // "logout", "expired", "inactive"
	DurationSecs int64      `bson:"duration_secs,omitempty"` // Computed on close

	// How was session created?
	CreatedBy string `bson:"created_by,omitempty"` // "login" or "heartbeat"

	// TTL expiration
	ExpiresAt time.Time `bson:"expires_at"`

	// Timestamps
	CreatedAt time.Time `bson:"created_at"`
	UpdatedAt time.Time `bson:"updated_at"`
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
		// Lookup by token (unique, sparse) - primary lookup method
		// Sparse allows existing sessions without tokens to remain
		{
			Keys:    bson.D{{Key: "token", Value: 1}},
			Options: options.Index().SetUnique(true).SetSparse(true).SetName("idx_session_token"),
		},
		// Lookup by user
		{
			Keys:    bson.D{{Key: "user_id", Value: 1}},
			Options: options.Index().SetName("idx_session_user"),
		},
		// TTL index for automatic cleanup
		{
			Keys:    bson.D{{Key: "expires_at", Value: 1}},
			Options: options.Index().SetExpireAfterSeconds(0).SetName("idx_session_ttl"),
		},
		// Active sessions query (who's online)
		{
			Keys:    bson.D{{Key: "logout_at", Value: 1}, {Key: "last_activity", Value: -1}},
			Options: options.Index().SetName("idx_session_active"),
		},
		// Organization queries (stratahub-specific)
		{
			Keys:    bson.D{{Key: "organization_id", Value: 1}, {Key: "login_at", Value: -1}},
			Options: options.Index().SetName("idx_session_org"),
		},
	}
	_, err := s.c.Indexes().CreateMany(ctx, indexes)
	return err
}

// Create creates a new session with a token.
func (s *Store) Create(ctx context.Context, session Session) error {
	if session.ID.IsZero() {
		session.ID = primitive.NewObjectID()
	}
	now := time.Now()
	session.CreatedAt = now
	session.UpdatedAt = now
	if session.LoginAt.IsZero() {
		session.LoginAt = now
	}
	if session.LastActivity.IsZero() {
		session.LastActivity = now
	}
	if session.CreatedBy == "" {
		session.CreatedBy = CreatedByLogin
	}
	_, err := s.c.InsertOne(ctx, session)
	return err
}

// CreateWithAutoClose creates a session and closes any existing open sessions for the user.
// This is the stratahub pattern where new login closes old sessions.
func (s *Store) CreateWithAutoClose(ctx context.Context, session Session) error {
	now := time.Now()

	// Close any existing open sessions for this user
	_, _ = s.c.UpdateMany(ctx,
		bson.M{
			"user_id":   session.UserID,
			"logout_at": nil,
		},
		bson.M{
			"$set": bson.M{
				"logout_at":  now,
				"end_reason": EndReasonInactive,
				"updated_at": now,
			},
		},
	)

	return s.Create(ctx, session)
}

// GetByToken retrieves an active session by token.
// Returns nil if the session has been logged out or expired.
func (s *Store) GetByToken(ctx context.Context, token string) (*Session, error) {
	var session Session
	err := s.c.FindOne(ctx, bson.M{
		"token":      token,
		"logout_at":  nil,
		"expires_at": bson.M{"$gt": time.Now()},
	}).Decode(&session)
	if err != nil {
		return nil, err
	}
	return &session, nil
}

// GetByID retrieves a session by its ID.
func (s *Store) GetByID(ctx context.Context, sessionID primitive.ObjectID) (*Session, error) {
	var sess Session
	err := s.c.FindOne(ctx, bson.M{"_id": sessionID}).Decode(&sess)
	if err != nil {
		return nil, err
	}
	return &sess, nil
}

// Close closes a session by token with a reason and computes duration.
func (s *Store) Close(ctx context.Context, token string, reason string) error {
	var session Session
	err := s.c.FindOne(ctx, bson.M{"token": token}).Decode(&session)
	if err != nil {
		return err
	}

	now := time.Now()
	duration := int64(now.Sub(session.LoginAt).Seconds())

	_, err = s.c.UpdateOne(ctx, bson.M{"token": token}, bson.M{
		"$set": bson.M{
			"logout_at":     now,
			"end_reason":    reason,
			"duration_secs": duration,
			"updated_at":    now,
		},
	})
	return err
}

// CloseByID closes a session by ID with a reason and computes duration.
func (s *Store) CloseByID(ctx context.Context, sessionID primitive.ObjectID, reason string) error {
	var sess Session
	err := s.c.FindOne(ctx, bson.M{"_id": sessionID}).Decode(&sess)
	if err != nil {
		return err
	}

	now := time.Now()
	duration := int64(now.Sub(sess.LoginAt).Seconds())

	_, err = s.c.UpdateOne(ctx, bson.M{"_id": sessionID}, bson.M{
		"$set": bson.M{
			"logout_at":     now,
			"end_reason":    reason,
			"duration_secs": duration,
			"updated_at":    now,
		},
	})
	return err
}

// CloseByUser closes all sessions for a user with the given reason.
func (s *Store) CloseByUser(ctx context.Context, userID primitive.ObjectID, reason string) error {
	now := time.Now()
	_, err := s.c.UpdateMany(ctx,
		bson.M{
			"user_id":   userID,
			"logout_at": nil,
		},
		bson.M{
			"$set": bson.M{
				"logout_at":  now,
				"end_reason": reason,
				"updated_at": now,
			},
		},
	)
	return err
}

// CloseByUserExcept closes all sessions for a user except the specified token.
func (s *Store) CloseByUserExcept(ctx context.Context, userID primitive.ObjectID, exceptToken string, reason string) error {
	now := time.Now()
	_, err := s.c.UpdateMany(ctx,
		bson.M{
			"user_id":   userID,
			"token":     bson.M{"$ne": exceptToken},
			"logout_at": nil,
		},
		bson.M{
			"$set": bson.M{
				"logout_at":  now,
				"end_reason": reason,
				"updated_at": now,
			},
		},
	)
	return err
}

// UpdateResult contains the result of an UpdateCurrentPage operation.
type UpdateResult struct {
	Updated      bool   // Whether the session was updated
	PreviousPage string // The previous current_page value (before update)
}

// UpdateCurrentPage updates the current page and last activity time for a session.
// Only updates sessions that are not already closed (logout_at is nil).
// Returns UpdateResult with whether session was updated and the previous page value.
func (s *Store) UpdateCurrentPage(ctx context.Context, token string, page string) (UpdateResult, error) {
	now := time.Now()
	update := bson.M{
		"last_activity": now,
		"updated_at":    now,
	}
	if page != "" {
		update["current_page"] = page
	}

	// Use FindOneAndUpdate to get the previous state
	opts := options.FindOneAndUpdate().
		SetReturnDocument(options.Before) // Return document BEFORE update

	var oldSession struct {
		CurrentPage string `bson:"current_page"`
	}
	err := s.c.FindOneAndUpdate(ctx,
		bson.M{
			"token":     token,
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

// UpdateActivity updates the last activity time and optionally the IP and user agent.
func (s *Store) UpdateActivity(ctx context.Context, token string, ip string, userAgent string) error {
	now := time.Now()
	update := bson.M{
		"$set": bson.M{
			"last_activity": now,
			"updated_at":    now,
		},
	}

	if ip != "" {
		update["$set"].(bson.M)["ip"] = ip
	}
	if userAgent != "" {
		update["$set"].(bson.M)["user_agent"] = userAgent
	}

	_, err := s.c.UpdateOne(ctx, bson.M{"token": token}, update)
	return err
}

// Delete removes a session by token.
func (s *Store) Delete(ctx context.Context, token string) error {
	_, err := s.c.DeleteOne(ctx, bson.M{"token": token})
	return err
}

// DeleteByUser removes all sessions for a user.
func (s *Store) DeleteByUser(ctx context.Context, userID primitive.ObjectID) error {
	_, err := s.c.DeleteMany(ctx, bson.M{"user_id": userID})
	return err
}

// DeleteByID removes a session by ID.
func (s *Store) DeleteByID(ctx context.Context, id primitive.ObjectID) error {
	_, err := s.c.DeleteOne(ctx, bson.M{"_id": id})
	return err
}

// DeleteByUserExcept removes all sessions for a user except the specified token.
func (s *Store) DeleteByUserExcept(ctx context.Context, userID primitive.ObjectID, exceptToken string) error {
	_, err := s.c.DeleteMany(ctx, bson.M{
		"user_id": userID,
		"token":   bson.M{"$ne": exceptToken},
	})
	return err
}

// ListByUser retrieves all active sessions for a user.
func (s *Store) ListByUser(ctx context.Context, userID primitive.ObjectID) ([]Session, error) {
	cursor, err := s.c.Find(ctx, bson.M{
		"user_id":    userID,
		"expires_at": bson.M{"$gt": time.Now()},
	}, options.Find().SetSort(bson.D{{Key: "last_activity", Value: -1}}))
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)

	var sessions []Session
	if err := cursor.All(ctx, &sessions); err != nil {
		return nil, err
	}
	return sessions, nil
}

// GetActiveByUser returns active (not logged out) sessions for a user.
func (s *Store) GetActiveByUser(ctx context.Context, userID primitive.ObjectID) ([]Session, error) {
	cursor, err := s.c.Find(ctx, bson.M{
		"user_id":    userID,
		"logout_at":  nil,
		"expires_at": bson.M{"$gt": time.Now()},
	}, options.Find().SetSort(bson.D{{Key: "last_activity", Value: -1}}))
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)

	var sessions []Session
	if err := cursor.All(ctx, &sessions); err != nil {
		return nil, err
	}
	return sessions, nil
}

// GetByUser retrieves session history for a user.
func (s *Store) GetByUser(ctx context.Context, userID primitive.ObjectID, limit int64) ([]Session, error) {
	opts := options.Find().
		SetSort(bson.D{{Key: "login_at", Value: -1}}).
		SetLimit(limit)

	cursor, err := s.c.Find(ctx, bson.M{"user_id": userID}, opts)
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)

	var sessions []Session
	if err := cursor.All(ctx, &sessions); err != nil {
		return nil, err
	}
	return sessions, nil
}

// GetByOrganization retrieves sessions for an organization.
func (s *Store) GetByOrganization(ctx context.Context, orgID primitive.ObjectID, limit int64) ([]Session, error) {
	opts := options.Find().
		SetSort(bson.D{{Key: "login_at", Value: -1}}).
		SetLimit(limit)

	cursor, err := s.c.Find(ctx, bson.M{"organization_id": orgID}, opts)
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)

	var sessions []Session
	if err := cursor.All(ctx, &sessions); err != nil {
		return nil, err
	}
	return sessions, nil
}

// GetActiveSessions retrieves all currently active (not logged out) sessions.
func (s *Store) GetActiveSessions(ctx context.Context, limit int64) ([]Session, error) {
	opts := options.Find().SetSort(bson.D{{Key: "last_activity", Value: -1}})
	if limit > 0 {
		opts.SetLimit(limit)
	}

	cursor, err := s.c.Find(ctx, bson.M{
		"logout_at":  nil,
		"expires_at": bson.M{"$gt": time.Now()},
	}, opts)
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)

	var sessions []Session
	if err := cursor.All(ctx, &sessions); err != nil {
		return nil, err
	}
	return sessions, nil
}

// CloseInactiveSessions closes sessions that haven't had activity within the threshold.
// Returns the number of sessions closed.
func (s *Store) CloseInactiveSessions(ctx context.Context, threshold time.Duration) (int64, error) {
	cutoff := time.Now().Add(-threshold)
	now := time.Now()

	result, err := s.c.UpdateMany(ctx,
		bson.M{
			"logout_at":     nil,
			"last_activity": bson.M{"$lt": cutoff},
		},
		bson.M{
			"$set": bson.M{
				"logout_at":  now,
				"end_reason": EndReasonInactive,
				"updated_at": now,
			},
		},
	)
	if err != nil {
		return 0, err
	}
	return result.ModifiedCount, nil
}

// CountActive counts currently active sessions (not logged out and not expired).
func (s *Store) CountActive(ctx context.Context) (int64, error) {
	return s.c.CountDocuments(ctx, bson.M{
		"logout_at":  nil,
		"expires_at": bson.M{"$gt": time.Now()},
	})
}

// CountActiveInOrg counts currently active sessions in an organization.
func (s *Store) CountActiveInOrg(ctx context.Context, orgID primitive.ObjectID, activeThreshold time.Duration) (int64, error) {
	cutoff := time.Now().Add(-activeThreshold)

	return s.c.CountDocuments(ctx, bson.M{
		"organization_id": orgID,
		"logout_at":       nil,
		"last_activity":   bson.M{"$gte": cutoff},
	})
}
