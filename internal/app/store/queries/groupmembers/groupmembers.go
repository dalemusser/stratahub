package groupmembers

import (
	"context"

	"github.com/dalemusser/stratahub/internal/domain/models"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
)

type GroupMember struct {
	User models.User `bson:"user" json:"user"`
	Role string      `bson:"role" json:"role"`
}

// MemberFilter controls filtering for users in the membership list.
// Leave User empty ("") to include all statuses.
type MemberFilter struct {
	User string // "active" | "disabled" | ""
}

// ListGroupMembersWithStatus returns leaders and members for a group with an optional user-status filter.
func ListGroupMembersWithStatus(ctx context.Context, db *mongo.Database, groupID primitive.ObjectID, f MemberFilter) ([]GroupMember, error) {
	pipe := mongo.Pipeline{
		bson.D{{Key: "$match", Value: bson.M{"group_id": groupID}}},
		bson.D{{Key: "$lookup", Value: bson.M{
			"from":         "users",
			"localField":   "user_id",
			"foreignField": "_id",
			"as":           "user",
		}}},
		bson.D{{Key: "$unwind", Value: "$user"}},
	}

	// Optional filter on user status ("active" or "disabled")
	if f.User == "active" || f.User == "disabled" {
		pipe = append(pipe, bson.D{{Key: "$match", Value: bson.M{"user.status": f.User}}})
	}

	// Stable order: leaders first, then members; then by full_name_ci, then _id
	pipe = append(pipe,
		bson.D{{Key: "$addFields", Value: bson.M{
			"role_rank": bson.M{"$cond": bson.A{
				bson.M{"$eq": bson.A{"$role", "leader"}}, 0, 1,
			}},
		}}},
		bson.D{{Key: "$sort", Value: bson.D{
			{Key: "role_rank", Value: 1},
			{Key: "user.full_name_ci", Value: 1},
			{Key: "user._id", Value: 1},
		}}},
		bson.D{{Key: "$project", Value: bson.M{"user": "$user", "role": 1}}},
	)

	cur, err := db.Collection("group_memberships").Aggregate(ctx, pipe)
	if err != nil {
		return nil, err
	}
	defer cur.Close(ctx)

	var out []GroupMember
	if err := cur.All(ctx, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// Backward-compatible wrapper: same signature as before (no user-status filter).
func ListGroupMembers(ctx context.Context, db *mongo.Database, groupID primitive.ObjectID) ([]GroupMember, error) {
	return ListGroupMembersWithStatus(ctx, db, groupID, MemberFilter{User: ""})
}
