// internal/app/store/memberships/membershipstore.go
package membershipstore

import (
	"context"
	"errors"
	"time"

	"github.com/dalemusser/stratahub/internal/domain/models"
	"github.com/dalemusser/waffle/toolkit/db/mongodb"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
)

type Store struct {
	c      *mongo.Collection
	users  *mongo.Collection
	groups *mongo.Collection
}

func New(db *mongo.Database) *Store {
	return &Store{
		c:      db.Collection("group_memberships"),
		users:  db.Collection("users"),
		groups: db.Collection("groups"),
	}
}

var (
	errBadRole      = errors.New(`role must be "leader" or "member"`)
	errOrgMismatch  = errors.New("user and group belong to different organizations")
	errMissingOrgID = errors.New("user missing organization_id")
)

var ErrDuplicateMembership = errors.New("user is already a member of this group")

// Add creates a membership after enforcing org invariant and role validity.
func (s *Store) Add(ctx context.Context, groupID, userID primitive.ObjectID, role string) error {
	if role != "leader" && role != "member" {
		return errBadRole
	}

	// Load group (org required)
	var g models.Group
	if err := s.groups.FindOne(ctx, bson.M{"_id": groupID}).Decode(&g); err != nil {
		return err
	}

	// Load user (org must match)
	var u models.User
	if err := s.users.FindOne(ctx, bson.M{"_id": userID}).Decode(&u); err != nil {
		return err
	}
	if u.OrganizationID == nil {
		return errMissingOrgID
	}
	if g.OrganizationID != *u.OrganizationID {
		return errOrgMismatch
	}

	doc := bson.M{
		"group_id":   groupID,
		"user_id":    userID,
		"org_id":     g.OrganizationID,
		"role":       role,
		"created_at": time.Now().UTC(),
	}
	_, err := s.c.InsertOne(ctx, doc)
	if err != nil {
		if mongodb.IsDup(err) {
			return ErrDuplicateMembership
		}
		return err
	}
	return nil
}

// Remove deletes the membership document for (groupID, userID).
func (s *Store) Remove(ctx context.Context, groupID, userID primitive.ObjectID) error {
	_, err := s.c.DeleteOne(ctx, bson.M{"group_id": groupID, "user_id": userID})
	return err
}
