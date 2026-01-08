// Package groupqueries provides complex read-only queries for groups.
package groupqueries

import (
	"context"

	"github.com/dalemusser/stratahub/internal/app/system/paging"
	"github.com/dalemusser/waffle/pantry/text"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
)

// GroupListItem holds the result of a group list query with computed counts.
type GroupListItem struct {
	ID              primitive.ObjectID `bson:"_id"`
	Name            string             `bson:"name"`
	NameCI          string             `bson:"name_ci"`
	OrgName         string             `bson:"org_name"`
	LeadersCount    int                `bson:"leaders_count"`
	MembersCount    int                `bson:"members_count"`
	AssignmentCount int                `bson:"assignment_count"`
}

// GroupListResult contains the paginated results and metadata.
type GroupListResult struct {
	Items []GroupListItem
	Total int64
}

// ListFilter defines the filter options for listing groups.
type ListFilter struct {
	WorkspaceID *primitive.ObjectID  // workspace scoping; nil means no workspace filter
	OrgID       *primitive.ObjectID  // nil means all orgs (single org filter)
	OrgIDs      []primitive.ObjectID // filter to these orgs (for coordinators); takes precedence if non-empty
	GroupIDs    []primitive.ObjectID // filter to these specific groups (for leaders); takes precedence if non-empty
	SearchQuery string               // prefix search on name_ci
}

// ListGroupsWithCounts fetches a paginated list of groups with org names,
// membership counts, and assignment counts.
// Uses DocumentDB-compatible queries (no $facet, no expressive $lookup).
func ListGroupsWithCounts(
	ctx context.Context,
	db *mongo.Database,
	filter ListFilter,
	cfg paging.KeysetConfig,
) (GroupListResult, error) {
	var result GroupListResult

	// Build base filter (without keyset window for total count)
	baseClauses := buildBaseClauses(filter)
	baseFilter := andify(baseClauses)

	// Get total count using CountDocuments
	total, err := db.Collection("groups").CountDocuments(ctx, baseFilter)
	if err != nil {
		return result, err
	}
	result.Total = total

	// Build aggregation pipeline for data (keyset filter, sort, limit, lookups)
	pipe := mongo.Pipeline{
		bson.D{{Key: "$match", Value: baseFilter}},
	}

	// Add keyset window filter if present
	if ks := cfg.KeysetWindow("name_ci"); ks != nil {
		pipe = append(pipe, bson.D{{Key: "$match", Value: ks}})
	}

	// Sort and limit for pagination
	pipe = append(pipe,
		bson.D{{Key: "$sort", Value: bson.D{
			{Key: "name_ci", Value: cfg.SortOrder},
			{Key: "_id", Value: cfg.SortOrder},
		}}},
		bson.D{{Key: "$limit", Value: paging.LimitPlusOne()}},
	)

	// Lookup organization name (basic $lookup - DocumentDB compatible)
	pipe = append(pipe,
		bson.D{{Key: "$lookup", Value: bson.M{
			"from":         "organizations",
			"localField":   "organization_id",
			"foreignField": "_id",
			"as":           "org",
		}}},
	)

	// Lookup all memberships for each group (basic $lookup)
	pipe = append(pipe,
		bson.D{{Key: "$lookup", Value: bson.M{
			"from":         "group_memberships",
			"localField":   "_id",
			"foreignField": "group_id",
			"as":           "memberships",
		}}},
	)

	// Lookup all resource assignments for each group (basic $lookup)
	pipe = append(pipe,
		bson.D{{Key: "$lookup", Value: bson.M{
			"from":         "group_resource_assignments",
			"localField":   "_id",
			"foreignField": "group_id",
			"as":           "assignments",
		}}},
	)

	// Project final fields - compute counts using $size and $filter
	pipe = append(pipe,
		bson.D{{Key: "$project", Value: bson.M{
			"_id":     1,
			"name":    1,
			"name_ci": 1,
			"org_name": bson.M{"$ifNull": []interface{}{
				bson.M{"$arrayElemAt": []interface{}{"$org.name", 0}},
				"",
			}},
			// Count leaders: filter memberships where role == "leader", then get size
			"leaders_count": bson.M{"$size": bson.M{"$filter": bson.M{
				"input": "$memberships",
				"as":    "m",
				"cond":  bson.M{"$eq": []interface{}{"$$m.role", "leader"}},
			}}},
			// Count members: filter memberships where role == "member", then get size
			"members_count": bson.M{"$size": bson.M{"$filter": bson.M{
				"input": "$memberships",
				"as":    "m",
				"cond":  bson.M{"$eq": []interface{}{"$$m.role", "member"}},
			}}},
			// Count assignments: just get size of assignments array
			"assignment_count": bson.M{"$size": "$assignments"},
		}}},
	)

	cur, err := db.Collection("groups").Aggregate(ctx, pipe)
	if err != nil {
		return result, err
	}
	defer cur.Close(ctx)

	// Decode results
	if err := cur.All(ctx, &result.Items); err != nil {
		return result, err
	}

	return result, nil
}

// buildBaseClauses builds the base filter clauses from the ListFilter.
func buildBaseClauses(filter ListFilter) []bson.M {
	var clauses []bson.M
	// Workspace scoping first (highest priority)
	if filter.WorkspaceID != nil {
		clauses = append(clauses, bson.M{"workspace_id": *filter.WorkspaceID})
	}
	// GroupIDs takes highest precedence (for leaders scoped to specific groups)
	if len(filter.GroupIDs) > 0 {
		clauses = append(clauses, bson.M{"_id": bson.M{"$in": filter.GroupIDs}})
	} else if len(filter.OrgIDs) > 0 {
		// OrgIDs next (for coordinators scoped to multiple orgs)
		clauses = append(clauses, bson.M{"organization_id": bson.M{"$in": filter.OrgIDs}})
	} else if filter.OrgID != nil {
		clauses = append(clauses, bson.M{"organization_id": *filter.OrgID})
	}
	if filter.SearchQuery != "" {
		q := text.Fold(filter.SearchQuery)
		hi := q + "\uffff"
		clauses = append(clauses, bson.M{"name_ci": bson.M{"$gte": q, "$lt": hi}})
	}
	return clauses
}

// andify composes clauses into a single bson.M with optional $and.
func andify(clauses []bson.M) bson.M {
	switch len(clauses) {
	case 0:
		return bson.M{}
	case 1:
		return clauses[0]
	default:
		return bson.M{"$and": clauses}
	}
}
