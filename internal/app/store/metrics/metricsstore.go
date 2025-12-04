package metricsstore

import (
	"context"

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
func FetchDashboardCounts(ctx context.Context, db *mongo.Database) Counts {
	var out Counts

	// organizations
	if n, err := db.Collection("organizations").CountDocuments(ctx, bson.D{}); err == nil {
		out.Organizations = n
	}

	// leaders
	if n, err := db.Collection("users").CountDocuments(ctx, bson.M{"role": "leader"}); err == nil {
		out.Leaders = n
	}

	// groups
	if n, err := db.Collection("groups").CountDocuments(ctx, bson.D{}); err == nil {
		out.Groups = n
	}

	// members
	if n, err := db.Collection("users").CountDocuments(ctx, bson.M{"role": "member"}); err == nil {
		out.Members = n
	}

	// resources
	if n, err := db.Collection("resources").CountDocuments(ctx, bson.D{}); err == nil {
		out.Resources = n
	}

	return out
}
