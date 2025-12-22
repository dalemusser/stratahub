// internal/app/features/groups/listorgpane.go
package groups

import (
	"context"
	"maps"

	"github.com/dalemusser/stratahub/internal/app/system/orgutil"
	"github.com/dalemusser/stratahub/internal/app/system/paging"
	wafflemongo "github.com/dalemusser/waffle/pantry/mongo"
	"github.com/dalemusser/waffle/pantry/text"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"go.uber.org/zap"
)

// fetchOrgPane fetches the org pane data including paginated orgs with group counts.
func (h *Handler) fetchOrgPane(
	ctx context.Context,
	db *mongo.Database,
	orgQ, orgAfter, orgBefore string,
) (orgPaneData, error) {
	var result orgPaneData

	// Build base filter for search
	orgBase := bson.M{}
	if orgQ != "" {
		q := text.Fold(orgQ)
		hi := q + "\uffff"
		orgBase["name_ci"] = bson.M{"$gte": q, "$lt": hi}
	}

	// Count total orgs matching search
	total, err := db.Collection("organizations").CountDocuments(ctx, orgBase)
	if err != nil {
		h.Log.Warn("count orgs failed", zap.Error(err))
		return result, err
	}
	result.Total = total

	// Build pagination filter (clone base filter, then add cursor conditions)
	orgFilter := maps.Clone(orgBase)

	find := options.Find()
	limit := paging.LimitPlusOne()

	if orgBefore != "" {
		if c, ok := wafflemongo.DecodeCursor(orgBefore); ok {
			orgFilter["$or"] = []bson.M{
				{"name_ci": bson.M{"$lt": c.CI}},
				{"name_ci": c.CI, "_id": bson.M{"$lt": c.ID}},
			}
		}
		find.SetSort(bson.D{{Key: "name_ci", Value: -1}, {Key: "_id", Value: -1}}).SetLimit(limit)
	} else {
		if orgAfter != "" {
			if c, ok := wafflemongo.DecodeCursor(orgAfter); ok {
				orgFilter["$or"] = []bson.M{
					{"name_ci": bson.M{"$gt": c.CI}},
					{"name_ci": c.CI, "_id": bson.M{"$gt": c.ID}},
				}
			}
		}
		find.SetSort(bson.D{{Key: "name_ci", Value: 1}, {Key: "_id", Value: 1}}).SetLimit(limit)
	}

	// Fetch orgs
	cur, err := db.Collection("organizations").Find(ctx, orgFilter, find)
	if err != nil {
		h.Log.Warn("find orgs failed", zap.Error(err))
		return result, err
	}
	defer cur.Close(ctx)

	var oraw []struct {
		ID     primitive.ObjectID `bson:"_id"`
		Name   string             `bson:"name"`
		NameCI string             `bson:"name_ci"`
	}
	if err := cur.All(ctx, &oraw); err != nil {
		h.Log.Warn("decode orgs failed", zap.Error(err))
		return result, err
	}

	// Apply pagination trimming
	orgPage := paging.TrimPage(&oraw, orgBefore, orgAfter)
	result.HasPrev = orgPage.HasPrev
	result.HasNext = orgPage.HasNext

	// Build org rows
	result.Rows = make([]orgutil.OrgRow, 0, len(oraw))
	for _, o := range oraw {
		result.Rows = append(result.Rows, orgutil.OrgRow{ID: o.ID, Name: o.Name})
	}

	// Compute range
	orgRange := paging.ComputeRange(1, len(oraw))
	result.RangeStart = orgRange.Start
	result.RangeEnd = orgRange.End

	// Build cursors
	if len(oraw) > 0 {
		first := oraw[0]
		last := oraw[len(oraw)-1]
		result.PrevCursor = wafflemongo.EncodeCursor(first.NameCI, first.ID)
		result.NextCursor = wafflemongo.EncodeCursor(last.NameCI, last.ID)
	}

	// Count all groups (for "All" row)
	allCount, err := db.Collection("groups").CountDocuments(ctx, bson.M{})
	if err != nil {
		h.Log.Error("database error counting all groups", zap.Error(err))
		return result, err
	}
	result.AllCount = allCount

	// Fetch group counts per org
	if err := h.fetchOrgGroupCounts(ctx, db, result.Rows); err != nil {
		return result, err
	}

	return result, nil
}

// fetchOrgGroupCounts populates the Count field for each org row.
func (h *Handler) fetchOrgGroupCounts(ctx context.Context, db *mongo.Database, rows []orgutil.OrgRow) error {
	if len(rows) == 0 {
		return nil
	}

	orgIDs := make([]primitive.ObjectID, 0, len(rows))
	for _, o := range rows {
		orgIDs = append(orgIDs, o.ID)
	}
	orgIDs = dedupObjectIDs(orgIDs)

	byOrg, err := orgutil.AggregateCountByField(ctx, db, "groups", bson.M{
		"organization_id": bson.M{"$in": orgIDs},
	}, "organization_id")
	if err != nil {
		h.Log.Error("database error aggregating group counts", zap.Error(err))
		return err
	}

	for i := range rows {
		rows[i].Count = byOrg[rows[i].ID]
	}

	return nil
}
