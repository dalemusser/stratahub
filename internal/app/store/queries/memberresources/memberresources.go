package memberresources

import (
	"context"
	"time"

	"github.com/dalemusser/stratahub/internal/domain/models"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
)

// MemberResource represents a resource that a member can access, including
// the originating group and the optional visibility window from the
// corresponding group_resource_assignments document.
//
// VisibleFrom / VisibleUntil are stored in UTC in MongoDB and are surfaced as
// *time.Time so callers (handlers) can perform any necessary timezone
// conversion and availability calculations.
//
// Fields:
//   - Resource:     the joined resource document from the `resources` collection
//   - GroupID:      the ID of the group that granted access
//   - GroupName:    the name of that group
//   - VisibleFrom:  start of the assignment window, or nil if not set
//   - VisibleUntil: end of the assignment window, or nil if not set
//
// Callers should interpret a nil VisibleFrom / VisibleUntil according to the
// business rules (e.g., nil VisibleFrom means "not yet released" or
// "always available" depending on how the assignment was created).

type MemberResource struct {
	Resource     models.Resource    `bson:"resource" json:"resource"`
	GroupID      primitive.ObjectID `bson:"group_id" json:"group_id"`
	GroupName    string             `bson:"group_name" json:"group_name"`
	VisibleFrom  *time.Time         `bson:"visible_from" json:"visible_from,omitempty"`
	VisibleUntil *time.Time         `bson:"visible_until" json:"visible_until,omitempty"`
}

// StatusFilter controls filtering on related documents.
// Leave a field empty ("") to include all statuses.
//
//   - Resource: if "active", only resources with status == "active" are
//               returned; if empty, all resource statuses are allowed.
//   - Group:    if "active", only memberships whose group.status == "active"
//               are returned; if empty, all group statuses are allowed.

type StatusFilter struct {
	Resource string // "active" | ""
	Group    string // "active" | ""
}

// ListResourcesForMember returns resources for a member with optional
// resource and group status filters. It performs the following joins:
//
//  1. `group_memberships` (this collection) filtered by user_id/role
//  2. `$lookup` into `group_resource_assignments` on group_id
//  3. `$lookup` into `resources` on `asg.resource_id`
//  4. `$lookup` into `groups` on `group_id` to obtain the group name
//
// The aggregation projects the joined resource document, the granting group
// ID/name, and the `visible_from` / `visible_until` fields from the
// group_resource_assignments row so that callers can compute availability.
func ListResourcesForMember(ctx context.Context, db *mongo.Database, userID primitive.ObjectID, f StatusFilter) ([]MemberResource, error) {
	pipe := mongo.Pipeline{
		// Start from group_memberships for this member in the "member" role.
		bson.D{{Key: "$match", Value: bson.M{
			"user_id": userID,
			"role":    "member",
		}}},
		// Join to group_resource_assignments on group_id.
		bson.D{{Key: "$lookup", Value: bson.M{
			"from":         "group_resource_assignments",
			"localField":   "group_id",
			"foreignField": "group_id",
			"as":           "asg",
		}}},
		bson.D{{Key: "$unwind", Value: "$asg"}},
		// Join to resources collection via asg.resource_id.
		bson.D{{Key: "$lookup", Value: bson.M{
			"from":         "resources",
			"localField":   "asg.resource_id",
			"foreignField": "_id",
			"as":           "resource",
		}}},
		bson.D{{Key: "$unwind", Value: "$resource"}},
	}

	// Optionally filter by resource status (currently we only support
	// filtering for "active" resources).
	if f.Resource == "active" {
		pipe = append(pipe, bson.D{{Key: "$match", Value: bson.M{"resource.status": "active"}}})
	}

	// Join to groups to get the group name.
	pipe = append(pipe,
		bson.D{{Key: "$lookup", Value: bson.M{
			"from":         "groups",
			"localField":   "group_id",
			"foreignField": "_id",
			"as":           "g",
		}}},
		bson.D{{Key: "$unwind", Value: "$g"}},
	)

	// Optionally filter by group.status
	if f.Group == "active" {
		pipe = append(pipe, bson.D{{Key: "$match", Value: bson.M{"g.status": "active"}}})
	}

	// Project only the fields we care about, including the visibility window
	// from the assignment document.
	pipe = append(pipe, bson.D{{Key: "$project", Value: bson.M{
		"resource":      "$resource",
		"group_id":      "$group_id",
		"group_name":    "$g.name",
		"visible_from":  "$asg.visible_from",
		"visible_until": "$asg.visible_until",
	}}})

	cur, err := db.Collection("group_memberships").Aggregate(ctx, pipe)
	if err != nil {
		return nil, err
	}
	defer cur.Close(ctx)

	var out []MemberResource
	if err := cur.All(ctx, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// ListActiveResourcesForMember is a convenience wrapper that returns all
// resources for which the member has a membership in any group and the
// resource status is "active". Group status is not filtered.
func ListActiveResourcesForMember(ctx context.Context, db *mongo.Database, userID primitive.ObjectID) ([]MemberResource, error) {
	return ListResourcesForMember(ctx, db, userID, StatusFilter{Resource: "active", Group: ""})
}
