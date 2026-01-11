// Package reportqueries provides complex read-only queries for reports.
package reportqueries

// Terminology: User Identifiers
//   - UserID / userID / user_id: The MongoDB ObjectID (_id) that uniquely identifies a user record
//   - LoginID / loginID / login_id: The human-readable string users type to log in

import (
	"context"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
)

// CountGroupMembersPerGroup returns member counts for each group, filtered by user status.
// The aggregation joins group_memberships with users to filter by user status.
func CountGroupMembersPerGroup(
	ctx context.Context,
	db *mongo.Database,
	groupIDs []primitive.ObjectID,
	memberStatus string, // "active", "disabled", or "" for all
) (map[primitive.ObjectID]int64, error) {
	result := make(map[primitive.ObjectID]int64)

	if len(groupIDs) == 0 {
		return result, nil
	}

	gmMatch := bson.M{
		"group_id": bson.M{"$in": groupIDs},
		"role":     "member",
	}

	userMatch := bson.M{"user.role": "member"}
	if memberStatus == "active" || memberStatus == "disabled" {
		userMatch["user.status"] = memberStatus
	}

	pipeline := []bson.M{
		{"$match": gmMatch},
		{"$lookup": bson.M{
			"from":         "users",
			"localField":   "user_id",
			"foreignField": "_id",
			"as":           "user",
		}},
		{"$unwind": "$user"},
		{"$match": userMatch},
		{"$group": bson.M{"_id": "$group_id", "count": bson.M{"$sum": 1}}},
	}

	cur, err := db.Collection("group_memberships").Aggregate(ctx, pipeline)
	if err != nil {
		return nil, err
	}
	defer cur.Close(ctx)

	for cur.Next(ctx) {
		var row struct {
			ID    primitive.ObjectID `bson:"_id"`
			Count int64              `bson:"count"`
		}
		if err := cur.Decode(&row); err != nil {
			return nil, err
		}
		result[row.ID] = row.Count
	}

	return result, nil
}

// ExportCounts holds the computed export counts for the members report.
type ExportCounts struct {
	MembersInGroupsCount int64
	ExportRecordCount    int64
}

// CountMembershipStats calculates membership statistics for export.
// Returns the count of distinct members with memberships and total membership count.
func CountMembershipStats(
	ctx context.Context,
	db *mongo.Database,
	scopeOrg *primitive.ObjectID,
	selectedGroup string,
	memberStatus string,
) (membershipCount int64, distinctMembersWithMembership int64, err error) {
	gmMatch := bson.M{"role": "member"}
	if scopeOrg != nil {
		gmMatch["org_id"] = *scopeOrg
	}
	if selectedGroup != "" {
		if gid, parseErr := primitive.ObjectIDFromHex(selectedGroup); parseErr == nil {
			gmMatch["group_id"] = gid
		}
	}

	userMatch := bson.M{"user.role": "member"}
	if memberStatus == "active" || memberStatus == "disabled" {
		userMatch["user.status"] = memberStatus
	}

	// Count total memberships
	pipeline := []bson.M{
		{"$match": gmMatch},
		{"$lookup": bson.M{
			"from":         "users",
			"localField":   "user_id",
			"foreignField": "_id",
			"as":           "user",
		}},
		{"$unwind": "$user"},
		{"$match": userMatch},
		{"$count": "count"},
	}

	cur, aggErr := db.Collection("group_memberships").Aggregate(ctx, pipeline)
	if aggErr != nil {
		err = aggErr
		return
	}
	defer cur.Close(ctx)

	if cur.Next(ctx) {
		var row struct {
			Count int64 `bson:"count"`
		}
		if decErr := cur.Decode(&row); decErr == nil {
			membershipCount = row.Count
		}
	}

	// Count distinct members with memberships
	pipeline2 := []bson.M{
		{"$match": gmMatch},
		{"$lookup": bson.M{
			"from":         "users",
			"localField":   "user_id",
			"foreignField": "_id",
			"as":           "user",
		}},
		{"$unwind": "$user"},
		{"$match": userMatch},
		{"$group": bson.M{"_id": "$user._id"}},
		{"$count": "count"},
	}

	cur2, aggErr := db.Collection("group_memberships").Aggregate(ctx, pipeline2)
	if aggErr != nil {
		err = aggErr
		return
	}
	defer cur2.Close(ctx)

	if cur2.Next(ctx) {
		var row struct {
			Count int64 `bson:"count"`
		}
		if decErr := cur2.Decode(&row); decErr == nil {
			distinctMembersWithMembership = row.Count
		}
	}

	return
}
