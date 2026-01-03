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
	OrgID       *primitive.ObjectID  // nil means all orgs (single org filter)
	OrgIDs      []primitive.ObjectID // filter to these orgs (for coordinators); takes precedence if non-empty
	GroupIDs    []primitive.ObjectID // filter to these specific groups (for leaders); takes precedence if non-empty
	SearchQuery string               // prefix search on name_ci
}

// ListGroupsWithCounts fetches a paginated list of groups with org names,
// membership counts, and assignment counts using a single aggregation pipeline
// with $facet for optimal performance.
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

	// Build full filter including keyset window
	fullClauses := append([]bson.M{}, baseClauses...)
	if ks := cfg.KeysetWindow("name_ci"); ks != nil {
		fullClauses = append(fullClauses, ks)
	}

	// Build aggregation pipeline with $facet to get data and count in one query
	pipe := mongo.Pipeline{
		// Match base filter first (for accurate total count)
		bson.D{{Key: "$match", Value: baseFilter}},
		// Use $facet to run count and data queries in parallel
		bson.D{{Key: "$facet", Value: bson.M{
			"totalCount": []bson.M{
				{"$count": "count"},
			},
			"data": buildDataPipeline(cfg),
		}}},
	}

	cur, err := db.Collection("groups").Aggregate(ctx, pipe)
	if err != nil {
		return result, err
	}
	defer cur.Close(ctx)

	// Parse aggregation result
	var aggResult struct {
		TotalCount []struct {
			Count int64 `bson:"count"`
		} `bson:"totalCount"`
		Data []GroupListItem `bson:"data"`
	}
	if cur.Next(ctx) {
		if err := cur.Decode(&aggResult); err != nil {
			return result, err
		}
	}

	if len(aggResult.TotalCount) > 0 {
		result.Total = aggResult.TotalCount[0].Count
	}
	result.Items = aggResult.Data

	return result, nil
}

// buildBaseClauses builds the base filter clauses from the ListFilter.
func buildBaseClauses(filter ListFilter) []bson.M {
	var clauses []bson.M
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

// buildDataPipeline constructs the data portion of the $facet pipeline.
// It applies keyset pagination, joins org names, and computes membership/assignment counts.
func buildDataPipeline(cfg paging.KeysetConfig) []bson.M {
	pipeline := []bson.M{}

	// Apply keyset window filter if present (re-match after facet's base match)
	if ks := cfg.KeysetWindow("name_ci"); ks != nil {
		pipeline = append(pipeline, bson.M{"$match": ks})
	}

	// Sort and limit for pagination
	pipeline = append(pipeline,
		bson.M{"$sort": bson.D{
			{Key: "name_ci", Value: cfg.SortOrder},
			{Key: "_id", Value: cfg.SortOrder},
		}},
		bson.M{"$limit": paging.LimitPlusOne()},
	)

	// Lookup organization name
	pipeline = append(pipeline,
		bson.M{"$lookup": bson.M{
			"from":         "organizations",
			"localField":   "organization_id",
			"foreignField": "_id",
			"as":           "org",
		}},
	)

	// Lookup and aggregate membership counts (leaders and members)
	pipeline = append(pipeline,
		bson.M{"$lookup": bson.M{
			"from": "group_memberships",
			"let":  bson.M{"gid": "$_id"},
			"pipeline": []bson.M{
				{"$match": bson.M{"$expr": bson.M{"$eq": []string{"$group_id", "$$gid"}}}},
				{"$group": bson.M{"_id": "$role", "count": bson.M{"$sum": 1}}},
			},
			"as": "memberships",
		}},
	)

	// Lookup and count resource assignments
	pipeline = append(pipeline,
		bson.M{"$lookup": bson.M{
			"from": "group_resource_assignments",
			"let":  bson.M{"gid": "$_id"},
			"pipeline": []bson.M{
				{"$match": bson.M{"$expr": bson.M{"$eq": []string{"$group_id", "$$gid"}}}},
				{"$count": "count"},
			},
			"as": "assignments",
		}},
	)

	// Project final fields with computed counts
	pipeline = append(pipeline,
		bson.M{"$project": bson.M{
			"_id":     1,
			"name":    1,
			"name_ci": 1,
			"org_name": bson.M{"$ifNull": []interface{}{
				bson.M{"$arrayElemAt": []interface{}{"$org.name", 0}},
				"",
			}},
			"leaders_count": bson.M{"$ifNull": []interface{}{
				bson.M{"$arrayElemAt": []interface{}{
					bson.M{"$map": bson.M{
						"input": bson.M{"$filter": bson.M{
							"input": "$memberships",
							"as":    "m",
							"cond":  bson.M{"$eq": []interface{}{"$$m._id", "leader"}},
						}},
						"as": "m",
						"in": "$$m.count",
					}},
					0,
				}},
				0,
			}},
			"members_count": bson.M{"$ifNull": []interface{}{
				bson.M{"$arrayElemAt": []interface{}{
					bson.M{"$map": bson.M{
						"input": bson.M{"$filter": bson.M{
							"input": "$memberships",
							"as":    "m",
							"cond":  bson.M{"$eq": []interface{}{"$$m._id", "member"}},
						}},
						"as": "m",
						"in": "$$m.count",
					}},
					0,
				}},
				0,
			}},
			"assignment_count": bson.M{"$ifNull": []interface{}{
				bson.M{"$arrayElemAt": []interface{}{"$assignments.count", 0}},
				0,
			}},
		}},
	)

	return pipeline
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
