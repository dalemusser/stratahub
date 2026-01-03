// internal/app/system/orgutil/orgpane.go
package orgutil

import (
	"context"
	"maps"

	"github.com/dalemusser/stratahub/internal/app/system/paging"
	wafflemongo "github.com/dalemusser/waffle/pantry/mongo"
	"github.com/dalemusser/waffle/pantry/text"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"go.uber.org/zap"
)

// OrgPaneData holds the paginated organization list with user counts.
// Used by members, leaders, and groups list pages for the org filter pane.
type OrgPaneData struct {
	Rows       []OrgRow
	Total      int64
	HasPrev    bool
	HasNext    bool
	PrevCursor string
	NextCursor string
	RangeStart int
	RangeEnd   int
	AllCount   int64
}

// FetchOrgPane fetches paginated organizations with user counts for a specific role.
// The role parameter determines which users are counted (e.g., "member", "leader").
// scopeOrgIDs limits the orgs to those in the list (for coordinators); nil means all orgs.
func FetchOrgPane(
	ctx context.Context,
	db *mongo.Database,
	log *zap.Logger,
	role string,
	orgQ, orgAfter, orgBefore string,
	scopeOrgIDs []primitive.ObjectID,
) (OrgPaneData, error) {
	var result OrgPaneData

	// Build base filter for search
	orgBase := bson.M{}
	if orgQ != "" {
		q := text.Fold(orgQ)
		hi := q + "\uffff"
		orgBase["name_ci"] = bson.M{"$gte": q, "$lt": hi}
	}

	// If scoped to specific orgs (coordinator), filter by those org IDs
	if len(scopeOrgIDs) > 0 {
		orgBase["_id"] = bson.M{"$in": scopeOrgIDs}
	}

	// Count total orgs matching search
	total, err := db.Collection("organizations").CountDocuments(ctx, orgBase)
	if err != nil {
		log.Error("database error counting organizations", zap.Error(err))
		return result, err
	}
	result.Total = total

	// Count all users with the specified role (for "All" row), respecting scope
	allFilter := bson.M{"role": role}
	if len(scopeOrgIDs) > 0 {
		allFilter["organization_id"] = bson.M{"$in": scopeOrgIDs}
	}
	allCount, err := db.Collection("users").CountDocuments(ctx, allFilter)
	if err != nil {
		log.Error("database error counting all users by role",
			zap.Error(err),
			zap.String("role", role))
		return result, err
	}
	result.AllCount = allCount

	// Build pagination filter (clone base filter, then add cursor conditions)
	orgFilter := maps.Clone(orgBase)

	findOpts := options.Find()
	limit := paging.LimitPlusOne()

	if orgBefore != "" {
		if c, ok := wafflemongo.DecodeCursor(orgBefore); ok {
			orgFilter["$or"] = []bson.M{
				{"name_ci": bson.M{"$lt": c.CI}},
				{"name_ci": c.CI, "_id": bson.M{"$lt": c.ID}},
			}
		}
		findOpts.SetSort(bson.D{{Key: "name_ci", Value: -1}, {Key: "_id", Value: -1}}).SetLimit(limit)
	} else {
		if orgAfter != "" {
			if c, ok := wafflemongo.DecodeCursor(orgAfter); ok {
				orgFilter["$or"] = []bson.M{
					{"name_ci": bson.M{"$gt": c.CI}},
					{"name_ci": c.CI, "_id": bson.M{"$gt": c.ID}},
				}
			}
		}
		findOpts.SetSort(bson.D{{Key: "name_ci", Value: 1}, {Key: "_id", Value: 1}}).SetLimit(limit)
	}

	// Fetch orgs
	type orgView struct {
		ID     primitive.ObjectID `bson:"_id"`
		Name   string             `bson:"name"`
		NameCI string             `bson:"name_ci"`
	}

	cur, err := db.Collection("organizations").Find(ctx, orgFilter, findOpts)
	if err != nil {
		log.Error("database error finding organizations", zap.Error(err))
		return result, err
	}
	defer cur.Close(ctx)

	var orows []orgView
	if err := cur.All(ctx, &orows); err != nil {
		log.Error("database error decoding organizations", zap.Error(err))
		return result, err
	}

	// Reverse if paging backwards
	if orgBefore != "" {
		for i, j := 0, len(orows)-1; i < j; i, j = i+1, j-1 {
			orows[i], orows[j] = orows[j], orows[i]
		}
	}

	// Apply pagination trimming
	orgPage := paging.TrimPage(&orows, orgBefore, orgAfter)
	result.HasPrev = orgPage.HasPrev
	result.HasNext = orgPage.HasNext

	// Compute range
	orgRange := paging.ComputeRange(1, len(orows))
	result.RangeStart = orgRange.Start
	result.RangeEnd = orgRange.End

	// Collect org IDs for user count lookup
	orgIDs := make([]primitive.ObjectID, 0, len(orows))
	for _, o := range orows {
		orgIDs = append(orgIDs, o.ID)
	}

	// Fetch user counts per org for the specified role
	byOrg, err := fetchOrgUserCounts(ctx, db, log, role, orgIDs)
	if err != nil {
		return result, err
	}

	// Build org rows with counts
	result.Rows = make([]OrgRow, 0, len(orows))
	for _, o := range orows {
		result.Rows = append(result.Rows, OrgRow{ID: o.ID, Name: o.Name, Count: byOrg[o.ID]})
	}

	// Build cursors
	if len(result.Rows) > 0 {
		result.PrevCursor = wafflemongo.EncodeCursor(text.Fold(result.Rows[0].Name), result.Rows[0].ID)
		result.NextCursor = wafflemongo.EncodeCursor(text.Fold(result.Rows[len(result.Rows)-1].Name), result.Rows[len(result.Rows)-1].ID)
	}

	return result, nil
}

// fetchOrgUserCounts fetches user counts per organization for a specific role.
func fetchOrgUserCounts(
	ctx context.Context,
	db *mongo.Database,
	log *zap.Logger,
	role string,
	orgIDs []primitive.ObjectID,
) (map[primitive.ObjectID]int64, error) {
	if len(orgIDs) == 0 {
		return make(map[primitive.ObjectID]int64), nil
	}

	counts, err := AggregateCountByField(ctx, db, "users", bson.M{
		"role":            role,
		"organization_id": bson.M{"$in": orgIDs},
	}, "organization_id")
	if err != nil {
		log.Error("database error aggregating user counts by org",
			zap.Error(err),
			zap.String("role", role))
		return nil, err
	}
	return counts, nil
}
