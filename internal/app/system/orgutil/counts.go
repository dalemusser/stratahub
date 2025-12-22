// internal/app/system/orgutil/counts.go
package orgutil

import (
	"context"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// Aggregator is a minimal interface satisfied by *mongo.Database.
// It allows unit-testing aggregation helpers with a fake.
type Aggregator interface {
	Collection(name string, opts ...*options.CollectionOptions) *mongo.Collection
}

// AggregateCountByField computes counts grouped by a field.
//
//	coll     – collection name (e.g. "users", "groups")
//	match    – base match filter (e.g. {"role":"leader","organization_id":{"$in": ids}})
//	groupKey – field to group on (e.g. "organization_id")
//
// Returns a map keyed by ObjectID to count.
func AggregateCountByField(
	ctx context.Context,
	db Aggregator,
	coll string,
	match bson.M,
	groupKey string,
) (map[primitive.ObjectID]int64, error) {
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

	out := make(map[primitive.ObjectID]int64)
	for cur.Next(ctx) {
		var row struct {
			ID primitive.ObjectID `bson:"_id"`
			N  int64              `bson:"n"`
		}
		if err := cur.Decode(&row); err != nil {
			return nil, err
		}
		out[row.ID] = row.N
	}
	return out, cur.Err()
}
