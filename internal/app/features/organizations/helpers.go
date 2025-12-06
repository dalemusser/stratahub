// internal/app/features/organizations/helpers.go
package organizations

import (
	"context"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// aggregator is a minimal interface satisfied by *mongo.Database.
// It lets us unit-test aggregateCountByOrg with a fake.
type aggregator interface {
	// Matches the signature of (*mongo.Database).Collection.
	Collection(name string, opts ...*options.CollectionOptions) *mongo.Collection
}

// aggregateCountByOrg computes counts grouped by an organization field.
//
//	coll     – collection name (e.g. "users", "groups")
//	match    – base match filter (e.g. {"role":"leader","organization_id":{"$in": ids}}
//	groupKey – field to group on (e.g. "organization_id")
//
// It returns a map keyed by org ObjectID (as hex string) to count.
func aggregateCountByOrg(
	ctx context.Context,
	db aggregator,
	coll string,
	match bson.M,
	groupKey string,
) (map[string]int64, error) {

	pipeline := mongo.Pipeline{
		bson.D{{Key: "$match", Value: match}},
		bson.D{{Key: "$group", Value: bson.D{
			{Key: "_id", Value: "$" + groupKey},
			{Key: "n", Value: bson.D{{Key: "$sum", Value: 1}}},
		}}},
	}

	cur, err := db.Collection(coll).Aggregate(ctx, pipeline)
	if err != nil {
		return nil, err
	}
	defer cur.Close(ctx)

	out := make(map[string]int64)
	for cur.Next(ctx) {
		var row struct {
			ID primitive.ObjectID `bson:"_id"`
			N  int64              `bson:"n"`
		}
		if err := cur.Decode(&row); err != nil {
			return nil, err
		}
		out[row.ID.Hex()] = row.N
	}
	return out, cur.Err()
}
