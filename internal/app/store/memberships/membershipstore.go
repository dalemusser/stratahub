// internal/app/store/memberships/membershipstore.go
package membershipstore

import (
	"context"
	"errors"
	"time"

	"github.com/dalemusser/stratahub/internal/domain/models"
	wafflemongo "github.com/dalemusser/waffle/pantry/mongo"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
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
		if wafflemongo.IsDup(err) {
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

// MembershipEntry represents a user to add to a group.
type MembershipEntry struct {
	UserID primitive.ObjectID
	Role   string // "leader" or "member"
}

// AddBatchResult contains counts from a batch membership add operation.
type AddBatchResult struct {
	Added      int
	Duplicates int
}

// AddBatch adds multiple memberships in a single batch operation.
// Caller must have already verified that all users belong to the same org as the group.
// This method skips individual user/group lookups for efficiency.
// Duplicates are silently counted (not treated as errors).
func (s *Store) AddBatch(ctx context.Context, groupID, orgID primitive.ObjectID, entries []MembershipEntry) (AddBatchResult, error) {
	if len(entries) == 0 {
		return AddBatchResult{}, nil
	}

	// Validate roles and build documents
	now := time.Now().UTC()
	docs := make([]interface{}, 0, len(entries))
	for _, e := range entries {
		if e.Role != "leader" && e.Role != "member" {
			return AddBatchResult{}, errBadRole
		}
		docs = append(docs, bson.M{
			"group_id":   groupID,
			"user_id":    e.UserID,
			"org_id":     orgID,
			"role":       e.Role,
			"created_at": now,
		})
	}

	// Use ordered:false so all inserts are attempted even if some fail (duplicates)
	opts := options.InsertMany().SetOrdered(false)
	result, err := s.c.InsertMany(ctx, docs, opts)

	// Count successful inserts
	added := 0
	if result != nil {
		added = len(result.InsertedIDs)
	}
	duplicates := len(entries) - added

	// InsertMany with ordered:false returns a BulkWriteException for duplicate key errors.
	// We treat duplicates as expected (not an error), but propagate other errors.
	if err != nil {
		if bulkErr, ok := err.(mongo.BulkWriteException); ok {
			// Check if all errors are duplicate key errors (code 11000)
			for _, we := range bulkErr.WriteErrors {
				if we.Code != 11000 {
					return AddBatchResult{Added: added, Duplicates: duplicates}, err
				}
			}
			// All errors were duplicates - this is expected
			return AddBatchResult{Added: added, Duplicates: duplicates}, nil
		}
		return AddBatchResult{Added: added, Duplicates: duplicates}, err
	}

	return AddBatchResult{Added: added, Duplicates: duplicates}, nil
}
