// internal/app/store/users/upsert_member.go
package userstore

import (
	"context"
	"strings"
	"time"

	"github.com/dalemusser/waffle/toolkit/text/textfold"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
)

// UpsertMemberInOrg creates or updates a *member* inside the given orgID only.
// Returns (updated=true) if an existing user in the same org was updated.
// Returns (conflictErr!=nil) if an email exists in a different org.
// Returns err on database errors.
//
// NOTE: This does not move users between organizations.
func (s *Store) UpsertMembersInOrg(
	ctx context.Context,
	orgID primitive.ObjectID,
	fullName, email, authMethod string,
) (updated bool, conflictErr error, err error) {

	email = strings.TrimSpace(strings.ToLower(email))
	fullName = strings.TrimSpace(fullName)
	if email == "" || fullName == "" {
		return false, nil, nil
	}

	// Lookup by email once to decide create/update/conflict.
	var existing struct {
		ID   primitive.ObjectID `bson:"_id"`
		Org  primitive.ObjectID `bson:"organization_id"`
		Role string             `bson:"role"`
	}
	findErr := s.c.FindOne(ctx, bson.M{"email": email}).Decode(&existing)
	switch findErr {
	case mongo.ErrNoDocuments:
		// Insert new member in this org
		now := time.Now()
		doc := bson.M{
			"full_name":       fullName,
			"full_name_ci":    textfold.Fold(fullName),
			"email":           email,
			"role":            "member",
			"organization_id": orgID,
			"status":          "active",
			"auth_method":     strings.ToLower(strings.TrimSpace(authMethod)),
			"created_at":      now,
			"updated_at":      now,
		}
		_, err := s.c.InsertOne(ctx, doc)
		return false, nil, err

	default:
		if findErr != nil {
			return false, nil, findErr
		}
		// Exists: only update if already in this org; otherwise conflict.
		if existing.Org != orgID {
			return false, ErrDifferentOrg, nil
		}
		_, err := s.c.UpdateByID(ctx, existing.ID, bson.M{
			"$set": bson.M{
				"full_name":    fullName,
				"full_name_ci": textfold.Fold(fullName),
				"auth_method":  strings.ToLower(strings.TrimSpace(authMethod)),
				"updated_at":   time.Now(),
			},
		})
		return true, nil, err
	}
}

var ErrDifferentOrg = mongo.CommandError{Message: "email exists in a different organization"}
