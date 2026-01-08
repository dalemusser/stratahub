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
// - Admins always can
// - Coordinators can if the group is in one of their assigned organizations
// - Leaders can only if they are a leader of this specific group AND the group belongs to their organization
// Returns an error if the database check fails, allowing callers to distinguish
// between "not authorized" (false, nil) and "database error" (false, err).
func CanManageGroup(ctx context.Context, db *mongo.Database, r *http.Request, groupID, groupOrgID primitive.ObjectID) (bool, error) {
	role, _, uid, ok := authz.UserCtx(r)
	if !ok {
		return false, nil
	}
	if role == "superadmin" || role == "admin" {
		return true, nil
	}
	if role == "coordinator" {
		// Coordinators can manage groups in any of their assigned organizations
		return authz.CanAccessOrg(r, groupOrgID), nil
	}
	if role != "leader" {
		return false, nil
	}
	// Leaders can only manage groups in their own organization
	userOrgID := authz.UserOrgID(r)
	if userOrgID == primitive.NilObjectID || userOrgID != groupOrgID {
		return false, nil
	}
	return IsLeader(ctx, db, groupID, uid)
}
