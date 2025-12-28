// Package leadermaterials provides queries to fetch materials assigned to a leader.
//
// Materials are assigned via the material_assignments collection. An assignment
// can target either:
//   - An organization: all leaders in that organization see the material
//   - A specific leader: only that leader sees the material
//
// This package provides aggregation queries to fetch the combined list of
// materials available to a leader based on their ID and organization.
package leadermaterials

import (
	"context"
	"time"

	"github.com/dalemusser/stratahub/internal/app/system/status"
	"github.com/dalemusser/stratahub/internal/domain/models"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
)

// LeaderMaterial represents a material that a leader can access, including
// assignment details like the visibility window and directions.
//
// Fields:
//   - Material:       the joined material document from the materials collection
//   - AssignmentID:   the ID of the material_assignment granting access
//   - OrgName:        if org-wide, the organization name; empty for direct assignments
//   - IsOrgWide:      true if assigned to organization, false if assigned to leader directly
//   - VisibleFrom:    start of visibility window, or nil if not set
//   - VisibleUntil:   end of visibility window, or nil if not set
//   - Directions:     custom directions for this assignment (from DefaultInstructions)
type LeaderMaterial struct {
	Material     models.Material    `bson:"material" json:"material"`
	AssignmentID primitive.ObjectID `bson:"assignment_id" json:"assignment_id"`
	OrgName      string             `bson:"org_name" json:"org_name,omitempty"`
	IsOrgWide    bool               `bson:"is_org_wide" json:"is_org_wide"`
	VisibleFrom  *time.Time         `bson:"visible_from" json:"visible_from,omitempty"`
	VisibleUntil *time.Time         `bson:"visible_until" json:"visible_until,omitempty"`
	Directions   string             `bson:"directions" json:"directions"`
}

// StatusFilter controls filtering on materials.
// Leave a field empty ("") to include all statuses.
type StatusFilter struct {
	Material string // "active" | ""
}

// ListMaterialsForLeader returns all materials assigned to a leader, either
// directly (leader_id match) or through their organization (organization_id match).
//
// Parameters:
//   - ctx:      context for the database operation
//   - db:       the MongoDB database handle
//   - leaderID: the leader's user ID
//   - orgID:    the leader's organization ID (used for org-wide assignments)
//   - f:        optional status filters
//
// The query performs:
//  1. Match material_assignments where leader_id == leaderID OR organization_id == orgID
//  2. Lookup materials on material_id
//  3. Optionally filter by material status
//  4. Lookup organizations on organization_id for org name
//  5. Project the combined result
func ListMaterialsForLeader(
	ctx context.Context,
	db *mongo.Database,
	leaderID primitive.ObjectID,
	orgID primitive.ObjectID,
	f StatusFilter,
) ([]LeaderMaterial, error) {
	pipe := mongo.Pipeline{
		// Match assignments for this leader (direct or org-wide)
		bson.D{{Key: "$match", Value: bson.M{
			"$or": []bson.M{
				{"leader_id": leaderID},
				{"organization_id": orgID},
			},
		}}},
		// Join to materials collection
		bson.D{{Key: "$lookup", Value: bson.M{
			"from":         "materials",
			"localField":   "material_id",
			"foreignField": "_id",
			"as":           "mat",
		}}},
		bson.D{{Key: "$unwind", Value: "$mat"}},
	}

	// Optionally filter by material status
	if f.Material == status.Active {
		pipe = append(pipe, bson.D{{Key: "$match", Value: bson.M{"mat.status": status.Active}}})
	}

	// Join to organizations to get org name for org-wide assignments
	pipe = append(pipe,
		bson.D{{Key: "$lookup", Value: bson.M{
			"from":         "organizations",
			"localField":   "organization_id",
			"foreignField": "_id",
			"as":           "org",
		}}},
		// Use preserveNullAndEmptyArrays since not all assignments have org
		bson.D{{Key: "$unwind", Value: bson.M{
			"path":                       "$org",
			"preserveNullAndEmptyArrays": true,
		}}},
	)

	// Project the fields we need
	pipe = append(pipe, bson.D{{Key: "$project", Value: bson.M{
		"material":      "$mat",
		"assignment_id": "$_id",
		"org_name":      bson.M{"$ifNull": []any{"$org.name", ""}},
		// is_org_wide is true when leader_id is null/missing (meaning assigned to org, not individual leader)
		"is_org_wide": bson.M{"$eq": []any{
			bson.M{"$ifNull": []any{"$leader_id", "missing"}},
			"missing",
		}},
		"visible_from":  "$visible_from",
		"visible_until": "$visible_until",
		"directions":    "$directions",
	}}})

	// Sort by material title
	pipe = append(pipe, bson.D{{Key: "$sort", Value: bson.M{"material.title_ci": 1}}})

	cur, err := db.Collection("material_assignments").Aggregate(ctx, pipe)
	if err != nil {
		return nil, err
	}
	defer cur.Close(ctx)

	var out []LeaderMaterial
	if err := cur.All(ctx, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// ListActiveMaterialsForLeader is a convenience wrapper that returns all
// materials for which the leader has an assignment and the material status
// is "active".
func ListActiveMaterialsForLeader(
	ctx context.Context,
	db *mongo.Database,
	leaderID primitive.ObjectID,
	orgID primitive.ObjectID,
) ([]LeaderMaterial, error) {
	return ListMaterialsForLeader(ctx, db, leaderID, orgID, StatusFilter{Material: status.Active})
}

// GetMaterialForLeader returns a specific material for a leader if they have access.
// Returns nil if the leader doesn't have access to this material.
func GetMaterialForLeader(
	ctx context.Context,
	db *mongo.Database,
	leaderID primitive.ObjectID,
	orgID primitive.ObjectID,
	materialID primitive.ObjectID,
) (*LeaderMaterial, error) {
	pipe := mongo.Pipeline{
		// Match assignments for this leader (direct or org-wide) for this specific material
		bson.D{{Key: "$match", Value: bson.M{
			"material_id": materialID,
			"$or": []bson.M{
				{"leader_id": leaderID},
				{"organization_id": orgID},
			},
		}}},
		// Join to materials collection
		bson.D{{Key: "$lookup", Value: bson.M{
			"from":         "materials",
			"localField":   "material_id",
			"foreignField": "_id",
			"as":           "mat",
		}}},
		bson.D{{Key: "$unwind", Value: "$mat"}},
		// Only active materials
		bson.D{{Key: "$match", Value: bson.M{"mat.status": status.Active}}},
		// Join to organizations
		bson.D{{Key: "$lookup", Value: bson.M{
			"from":         "organizations",
			"localField":   "organization_id",
			"foreignField": "_id",
			"as":           "org",
		}}},
		bson.D{{Key: "$unwind", Value: bson.M{
			"path":                       "$org",
			"preserveNullAndEmptyArrays": true,
		}}},
		// Project
		bson.D{{Key: "$project", Value: bson.M{
			"material":      "$mat",
			"assignment_id": "$_id",
			"org_name":      bson.M{"$ifNull": []any{"$org.name", ""}},
			// is_org_wide is true when leader_id is null/missing (meaning assigned to org, not individual leader)
			"is_org_wide": bson.M{"$eq": []any{
				bson.M{"$ifNull": []any{"$leader_id", "missing"}},
				"missing",
			}},
			"visible_from":  "$visible_from",
			"visible_until": "$visible_until",
			"directions":    "$directions",
		}}},
		// Limit to first match (there should only be one anyway)
		bson.D{{Key: "$limit", Value: 1}},
	}

	cur, err := db.Collection("material_assignments").Aggregate(ctx, pipe)
	if err != nil {
		return nil, err
	}
	defer cur.Close(ctx)

	var results []LeaderMaterial
	if err := cur.All(ctx, &results); err != nil {
		return nil, err
	}

	if len(results) == 0 {
		return nil, nil
	}
	return &results[0], nil
}
