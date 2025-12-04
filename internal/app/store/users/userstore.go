package userstore

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/dalemusser/stratahub/internal/domain/models"
	"github.com/dalemusser/waffle/toolkit/db/mongodb"
	"github.com/dalemusser/waffle/toolkit/text/textfold"
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

// GetByEmail looks up a user by case-insensitive email. Returns mongo.ErrNoDocuments if not found.
func (s *Store) GetByEmail(ctx context.Context, email string) (*models.User, error) {
	e := strings.ToLower(strings.TrimSpace(email))
	var u models.User
	if err := s.c.FindOne(ctx, bson.M{"email": e}).Decode(&u); err != nil {
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
	u.FullName = strings.TrimSpace(u.FullName)
	u.FullNameCI = textfold.Fold(u.FullName)
	u.Email = strings.ToLower(strings.TrimSpace(u.Email))
	if u.Status == "" {
		u.Status = "active"
	}

	// Validate role
	switch u.Role {
	case "admin", "analyst", "leader", "member":
		// ok
	default:
		return models.User{}, errBadRole
	}

	// Validate status
	if u.Status != "active" && u.Status != "disabled" {
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
		if mongodb.IsDup(err) {
			return models.User{}, ErrDuplicateEmail
		}
		return models.User{}, err
	}
	return u, nil
}
