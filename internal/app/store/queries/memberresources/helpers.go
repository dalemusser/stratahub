// internal/app/store/queries/memberresources/helpers.go
package memberresources

import (
	"context"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
)

// FindGroupNameForMemberResource returns the name of one group (if any) for which
// the given member (userID) is a member and which assigns the specified resourceID.
// It uses a single aggregation over group_memberships → group_resource_assignments → groups.
func FindGroupNameForMemberResource(ctx context.Context, db *mongo.Database, userID, resourceID primitive.ObjectID) (string, bool, error) {
	pipe := mongo.Pipeline{
		bson.D{{Key: "$match", Value: bson.M{
			"user_id": userID,
			"role":    "member",
		}}},
		bson.D{{Key: "$lookup", Value: bson.M{
			"from":         "group_resource_assignments",
			"localField":   "group_id",
			"foreignField": "group_id",
			"as":           "asg",
		}}},
		bson.D{{Key: "$unwind", Value: "$asg"}},
		bson.D{{Key: "$match", Value: bson.M{"asg.resource_id": resourceID}}},
		bson.D{{Key: "$lookup", Value: bson.M{
			"from":         "groups",
			"localField":   "group_id",
			"foreignField": "_id",
			"as":           "g",
		}}},
		bson.D{{Key: "$unwind", Value: "$g"}},
		bson.D{{Key: "$project", Value: bson.M{"group_name": "$g.name"}}},
		bson.D{{Key: "$limit", Value: 1}},
	}
	cur, err := db.Collection("group_memberships").Aggregate(ctx, pipe)
	if err != nil {
		return "", false, err
	}
	defer cur.Close(ctx)
	var row struct {
		GroupName string `bson:"group_name"`
	}
	if cur.Next(ctx) {
		if err := cur.Decode(&row); err != nil {
			return "", false, err
		}
		return row.GroupName, true, nil
	}
	return "", false, nil
}
