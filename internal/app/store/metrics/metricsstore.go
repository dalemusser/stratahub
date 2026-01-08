package metricsstore

import (
	"context"

	"github.com/dalemusser/stratahub/internal/app/system/workspace"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
)

// Counts is the set of totals used by dashboards (admin, analyst, etc.).
type Counts struct {
	Organizations int64
	Leaders       int64
	Groups        int64
	Members       int64
	Resources     int64
}

// FetchDashboardCounts returns the high-level counts used by dashboards.
// Intentionally tolerant: on error it returns 0 for that counter.
// All counts are workspace-scoped when running in a workspace context.
func FetchDashboardCounts(ctx context.Context, db *mongo.Database) Counts {
	var out Counts

	// organizations
	orgFilter := bson.M{}
	workspace.FilterCtx(ctx, orgFilter)
	if n, err := db.Collection("organizations").CountDocuments(ctx, orgFilter); err == nil {
		out.Organizations = n
	}

	// leaders
	leaderFilter := bson.M{"role": "leader"}
	workspace.FilterCtx(ctx, leaderFilter)
	if n, err := db.Collection("users").CountDocuments(ctx, leaderFilter); err == nil {
		out.Leaders = n
	}

	// groups
	groupFilter := bson.M{}
	workspace.FilterCtx(ctx, groupFilter)
	if n, err := db.Collection("groups").CountDocuments(ctx, groupFilter); err == nil {
		out.Groups = n
	}

	// members
	memberFilter := bson.M{"role": "member"}
	workspace.FilterCtx(ctx, memberFilter)
	if n, err := db.Collection("users").CountDocuments(ctx, memberFilter); err == nil {
		out.Members = n
	}

	// resources
	resourceFilter := bson.M{}
	workspace.FilterCtx(ctx, resourceFilter)
	if n, err := db.Collection("resources").CountDocuments(ctx, resourceFilter); err == nil {
		out.Resources = n
	}

	return out
}
