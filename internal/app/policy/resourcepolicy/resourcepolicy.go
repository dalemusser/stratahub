// Package resourcepolicy provides authorization policies for member resource access.
//
// Authorization rules:
//   - Members can only access resources assigned to groups they belong to
//   - Resource visibility is controlled by assignment windows (visible_from/visible_until)
//   - The route middleware RequireRole("member") handles basic role enforcement
package resourcepolicy

// Terminology: User Identifiers
//   - UserID / userID / user_id: The MongoDB ObjectID (_id) that uniquely identifies a user record
//   - LoginID / loginID / login_id: The human-readable string users type to log in

import (
	"context"
	"net/http"
	"time"

	"github.com/dalemusser/stratahub/internal/app/system/authz"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
)

// MemberInfo contains the member data needed for resource access checks.
type MemberInfo struct {
	ID             primitive.ObjectID
	Email          string
	OrganizationID *primitive.ObjectID
}

// ResourceAssignment contains the assignment details when a member has access
// to a resource through a group.
type ResourceAssignment struct {
	GroupName    string
	VisibleFrom  *time.Time
	VisibleUntil *time.Time
	Instructions string
}

// VerifyMemberAccess checks that the authenticated user exists as a member in the database.
// Returns the member info if valid, nil if not found or not a member.
//
// This is used to verify database state after route middleware has already checked the role.
func VerifyMemberAccess(ctx context.Context, db *mongo.Database, r *http.Request) (*MemberInfo, error) {
	_, _, memberOID, ok := authz.UserCtx(r)
	if !ok {
		return nil, nil
	}

	var result struct {
		ID             primitive.ObjectID  `bson:"_id"`
		Email          string              `bson:"email"`
		OrganizationID *primitive.ObjectID `bson:"organization_id"`
	}

	err := db.Collection("users").FindOne(ctx, bson.M{
		"_id":  memberOID,
		"role": "member",
	}).Decode(&result)

	if err == mongo.ErrNoDocuments {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	return &MemberInfo{
		ID:             result.ID,
		Email:          result.Email,
		OrganizationID: result.OrganizationID,
	}, nil
}

// CanViewResource checks if a member can view a specific resource through their
// group memberships. Returns the assignment details if access is granted.
//
// Authorization:
//   - Member must belong to a group that has the resource assigned
//   - Returns nil assignment if no access (member not in any group with this resource)
//   - Callers should check visibility windows separately for time-based access
func CanViewResource(ctx context.Context, db *mongo.Database, memberID, resourceID primitive.ObjectID) (*ResourceAssignment, error) {
	// Aggregation pipeline to find if member has access through any group assignment
	pipe := mongo.Pipeline{
		// Start from group_memberships for this member
		bson.D{{Key: "$match", Value: bson.M{
			"user_id": memberID,
			"role":    "member",
		}}},
		// Join to group_resource_assignments on group_id
		bson.D{{Key: "$lookup", Value: bson.M{
			"from":         "group_resource_assignments",
			"localField":   "group_id",
			"foreignField": "group_id",
			"as":           "asg",
		}}},
		bson.D{{Key: "$unwind", Value: "$asg"}},
		// Filter to the specific resource
		bson.D{{Key: "$match", Value: bson.M{"asg.resource_id": resourceID}}},
		// Join to groups to get the group name
		bson.D{{Key: "$lookup", Value: bson.M{
			"from":         "groups",
			"localField":   "group_id",
			"foreignField": "_id",
			"as":           "g",
		}}},
		bson.D{{Key: "$unwind", Value: "$g"}},
		// Project the fields we need
		bson.D{{Key: "$project", Value: bson.M{
			"group_name":    "$g.name",
			"visible_from":  "$asg.visible_from",
			"visible_until": "$asg.visible_until",
			"instructions":  "$asg.instructions",
		}}},
		// Only need one result
		bson.D{{Key: "$limit", Value: 1}},
	}

	cur, err := db.Collection("group_memberships").Aggregate(ctx, pipe)
	if err != nil {
		return nil, err
	}
	defer cur.Close(ctx)

	if !cur.Next(ctx) {
		// No assignment found - member doesn't have access
		return nil, nil
	}

	var row struct {
		GroupName    string     `bson:"group_name"`
		VisibleFrom  *time.Time `bson:"visible_from"`
		VisibleUntil *time.Time `bson:"visible_until"`
		Instructions string     `bson:"instructions"`
	}
	if err := cur.Decode(&row); err != nil {
		return nil, err
	}

	return &ResourceAssignment{
		GroupName:    row.GroupName,
		VisibleFrom:  row.VisibleFrom,
		VisibleUntil: row.VisibleUntil,
		Instructions: row.Instructions,
	}, nil
}
