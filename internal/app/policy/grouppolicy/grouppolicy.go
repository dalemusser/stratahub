// internal/app/policy/grouppolicy.go
package grouppolicy

import (
	"context"
	"net/http"

	"github.com/dalemusser/stratahub/internal/app/system/authz"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
)

// IsLeader returns true if the given user is a leader of the given group
// according to the authoritative group_memberships collection.
func IsLeader(ctx context.Context, db *mongo.Database, groupID, userID primitive.ObjectID) (bool, error) {
	c := db.Collection("group_memberships")
	n, err := c.CountDocuments(ctx, bson.M{
		"group_id": groupID,
		"user_id":  userID,
		"role":     "leader",
	})
	if err != nil {
		return false, err
	}
	return n > 0, nil
}

// CanManageGroup reports whether the current request user can manage the group:
// admins always can; leaders can only if they are a leader of this specific group.
func CanManageGroup(ctx context.Context, db *mongo.Database, r *http.Request, groupID primitive.ObjectID) bool {
	role, _, uid, ok := authz.UserCtx(r)
	if !ok {
		return false
	}
	if role == "admin" {
		return true
	}
	if role != "leader" {
		return false
	}
	okLeader, err := IsLeader(ctx, db, groupID, uid)
	return err == nil && okLeader
}
