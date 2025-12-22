package userstore

import (
	"context"
	"errors"
	"time"

	"github.com/dalemusser/stratahub/internal/app/system/normalize"
	"github.com/dalemusser/stratahub/internal/app/system/status"
	"github.com/dalemusser/stratahub/internal/domain/models"
	wafflemongo "github.com/dalemusser/waffle/pantry/mongo"
	"github.com/dalemusser/waffle/pantry/text"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
)

type Store struct {
	c *mongo.Collection
}

func New(db *mongo.Database) *Store {
	return &Store{c: db.Collection("users")}
}

// GetByID loads a user by ObjectID.
func (s *Store) GetByID(ctx context.Context, id primitive.ObjectID) (*models.User, error) {
	var u models.User
	if err := s.c.FindOne(ctx, bson.M{"_id": id}).Decode(&u); err != nil {
		return nil, err
	}
	return &u, nil
}

// GetMemberByID loads a user by ObjectID, returning an error if the user
// does not exist or is not a member role.
func (s *Store) GetMemberByID(ctx context.Context, id primitive.ObjectID) (*models.User, error) {
	var u models.User
	if err := s.c.FindOne(ctx, bson.M{"_id": id, "role": "member"}).Decode(&u); err != nil {
		return nil, err
	}
	return &u, nil
}

// GetLeaderByID loads a user by ObjectID, returning an error if the user
// does not exist or is not a leader role.
func (s *Store) GetLeaderByID(ctx context.Context, id primitive.ObjectID) (*models.User, error) {
	var u models.User
	if err := s.c.FindOne(ctx, bson.M{"_id": id, "role": "leader"}).Decode(&u); err != nil {
		return nil, err
	}
	return &u, nil
}

// GetByEmail looks up a user by case-insensitive email. Returns mongo.ErrNoDocuments if not found.
func (s *Store) GetByEmail(ctx context.Context, email string) (*models.User, error) {
	var u models.User
	if err := s.c.FindOne(ctx, bson.M{"email": normalize.Email(email)}).Decode(&u); err != nil {
		return nil, err
	}
	return &u, nil
}

var (
	// ErrDuplicateEmail is returned when attempting to create a user with an email that already exists.
	ErrDuplicateEmail = errors.New("a user with this email already exists")
	errBadRole        = errors.New(`role must be "admin"|"analyst"|"leader"|"member"`)
	errBadStatus      = errors.New(`status must be "active"|"disabled"`)
	errOrgNeeded      = errors.New("leader/member must have organization_id")
)

// Create inserts a new user after normalizing & validating fields.
// It does not write any group membership arrays.
func (s *Store) Create(ctx context.Context, u models.User) (models.User, error) {
	// Normalize core fields
	u.ID = primitive.NewObjectID()
	u.FullName = normalize.Name(u.FullName)
	u.FullNameCI = text.Fold(u.FullName)
	u.Email = normalize.Email(u.Email)
	if u.Status == "" {
		u.Status = status.Active
	}

	// Validate role
	switch u.Role {
	case "admin", "analyst", "leader", "member":
		// ok
	default:
		return models.User{}, errBadRole
	}

	// Validate status
	if !status.IsValid(u.Status) {
		return models.User{}, errBadStatus
	}

	// Leaders/members must be scoped to an org
	if (u.Role == "leader" || u.Role == "member") && u.OrganizationID == nil {
		return models.User{}, errOrgNeeded
	}

	// Timestamps
	now := time.Now()
	u.CreatedAt = now
	u.UpdatedAt = now

	// Insert
	if _, err := s.c.InsertOne(ctx, u); err != nil {
		if wafflemongo.IsDup(err) {
			return models.User{}, ErrDuplicateEmail
		}
		return models.User{}, err
	}
	return u, nil
}

// MemberUpdate holds the fields that can be updated for a member.
type MemberUpdate struct {
	FullName       string
	Email          string
	AuthMethod     string
	Status         string
	OrganizationID primitive.ObjectID
}

// UpdateMember updates a member's fields. Only updates users with role="member".
// Returns ErrDuplicateEmail if the email already exists for another user.
func (s *Store) UpdateMember(ctx context.Context, id primitive.ObjectID, upd MemberUpdate) error {
	set := bson.M{
		"full_name":       upd.FullName,
		"full_name_ci":    text.Fold(upd.FullName),
		"email":           normalize.Email(upd.Email),
		"auth_method":     upd.AuthMethod,
		"status":          upd.Status,
		"organization_id": upd.OrganizationID,
		"updated_at":      time.Now(),
	}

	_, err := s.c.UpdateOne(ctx, bson.M{"_id": id, "role": "member"}, bson.M{"$set": set})
	if err != nil {
		if wafflemongo.IsDup(err) {
			return ErrDuplicateEmail
		}
		return err
	}
	return nil
}

// DeleteMember deletes a user by ID, but only if they have role="member".
// Returns the number of documents deleted (0 or 1).
func (s *Store) DeleteMember(ctx context.Context, id primitive.ObjectID) (int64, error) {
	res, err := s.c.DeleteOne(ctx, bson.M{"_id": id, "role": "member"})
	if err != nil {
		return 0, err
	}
	return res.DeletedCount, nil
}

// EmailExistsForOther checks if an email already exists for a user other than the given ID.
func (s *Store) EmailExistsForOther(ctx context.Context, email string, excludeID primitive.ObjectID) (bool, error) {
	err := s.c.FindOne(ctx, bson.M{
		"email": normalize.Email(email),
		"_id":   bson.M{"$ne": excludeID},
	}).Err()
	if err == nil {
		return true, nil // found another user with this email
	}
	if err == mongo.ErrNoDocuments {
		return false, nil // no duplicate
	}
	return false, err // actual error
}
