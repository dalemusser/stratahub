// internal/app/system/orgutil/orgs.go
package orgutil

import (
	"context"

	"github.com/dalemusser/stratahub/internal/domain/models"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
)

// ListActiveOrgs returns all active organizations.
func ListActiveOrgs(ctx context.Context, db *mongo.Database) ([]models.Organization, error) {
	cur, err := db.Collection("organizations").Find(ctx, bson.M{"status": "active"})
	if err != nil {
		return nil, err
	}
	defer cur.Close(ctx)
	var out []models.Organization
	for cur.Next(ctx) {
		var o models.Organization
		if err := cur.Decode(&o); err == nil {
			out = append(out, o)
		}
	}
	return out, nil
}
