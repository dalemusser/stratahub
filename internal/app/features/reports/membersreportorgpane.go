// internal/app/features/reports/membersreportorgpane.go
package reports

import (
	"context"
	"maps"

	"github.com/dalemusser/stratahub/internal/app/system/orgutil"
	"github.com/dalemusser/stratahub/internal/app/system/paging"
	"github.com/dalemusser/stratahub/internal/app/system/workspace"
	wafflemongo "github.com/dalemusser/waffle/pantry/mongo"
	"github.com/dalemusser/waffle/pantry/text"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"go.uber.org/zap"
)

// fetchReportOrgPane fetches the org pane data for the members report.
// scopeOrgIDs limits the orgs to those in the list (for coordinators); nil means all orgs.
// memberStatus is used to filter the member counts per org.
func (h *Handler) fetchReportOrgPane(
	ctx context.Context,
	db *mongo.Database,
	orgQ, orgAfter, orgBefore, memberStatus string,
	scopeOrgIDs []primitive.ObjectID,
) (orgPaneResult, error) {
	var result orgPaneResult

	// Build base filter for search with workspace scoping
	orgBase := bson.M{}
	workspace.FilterCtx(ctx, orgBase)

	if orgQ != "" {
		q := text.Fold(orgQ)
		hi := q + text.High
		orgBase["name_ci"] = bson.M{"$gte": q, "$lt": hi}
	}

	// If scoped to specific orgs (coordinator), filter by those org IDs
	if len(scopeOrgIDs) > 0 {
		orgBase["_id"] = bson.M{"$in": scopeOrgIDs}
	}

	// Count total orgs matching search
	total, err := db.Collection("organizations").CountDocuments(ctx, orgBase)
	if err != nil {
		h.Log.Error("database error counting organizations", zap.Error(err))
		return result, err
	}
	result.Total = total

	// Count all members (for "All" row), respecting memberStatus and scope
	allFilter := bson.M{"role": "member"}
	workspace.FilterCtx(ctx, allFilter)

	if memberStatus == "active" || memberStatus == "disabled" {
		allFilter["status"] = memberStatus
	}
	// If scoped to specific orgs (coordinator), only count members in those orgs
	if len(scopeOrgIDs) > 0 {
		allFilter["organization_id"] = bson.M{"$in": scopeOrgIDs}
	}
	allCount, err := db.Collection("users").CountDocuments(ctx, allFilter)
	if err != nil {
		h.Log.Error("database error counting all members", zap.Error(err))
		return result, err
	}
	result.AllCount = allCount

	// Build pagination filter (clone base filter, then add cursor conditions)
	orgFilter := maps.Clone(orgBase)

	findOrg := options.Find().SetLimit(paging.LimitPlusOne())
	if orgBefore != "" {
		if c, ok := wafflemongo.DecodeCursor(orgBefore); ok {
			orgFilter["$or"] = []bson.M{
				{"name_ci": bson.M{"$lt": c.CI}},
				{"name_ci": c.CI, "_id": bson.M{"$lt": c.ID}},
			}
		}
		findOrg.SetSort(bson.D{{Key: "name_ci", Value: -1}, {Key: "_id", Value: -1}})
	} else {
		if orgAfter != "" {
			if c, ok := wafflemongo.DecodeCursor(orgAfter); ok {
				orgFilter["$or"] = []bson.M{
					{"name_ci": bson.M{"$gt": c.CI}},
					{"name_ci": c.CI, "_id": bson.M{"$gt": c.ID}},
				}
			}
		}
		findOrg.SetSort(bson.D{{Key: "name_ci", Value: 1}, {Key: "_id", Value: 1}})
	}

	// Fetch orgs
	type oview struct {
		ID     primitive.ObjectID `bson:"_id"`
		Name   string             `bson:"name"`
		NameCI string             `bson:"name_ci"`
	}

	oc, err := db.Collection("organizations").Find(ctx, orgFilter, findOrg)
	if err != nil {
		h.Log.Error("database error finding organizations", zap.Error(err))
		return result, err
	}
	defer oc.Close(ctx)

	var orows []oview
	if err := oc.All(ctx, &orows); err != nil {
		h.Log.Error("database error decoding organizations", zap.Error(err))
		return result, err
	}

	// Apply pagination trimming
	orgPage := paging.TrimPage(&orows, orgBefore, orgAfter)
	result.HasPrev = orgPage.HasPrev
	result.HasNext = orgPage.HasNext

	// Collect org IDs for member count lookup
	orgIDs := make([]primitive.ObjectID, 0, len(orows))
	for _, o := range orows {
		orgIDs = append(orgIDs, o.ID)
	}

	// Fetch member counts per org
	byOrg, err := h.fetchReportOrgMemberCounts(ctx, db, orgIDs, memberStatus)
	if err != nil {
		return result, err
	}

	// Build org rows with counts
	result.Rows = make([]orgutil.OrgRow, 0, len(orows))
	for _, o := range orows {
		result.Rows = append(result.Rows, orgutil.OrgRow{ID: o.ID, Name: o.Name, Count: byOrg[o.ID]})
	}

	// Build cursors
	if len(result.Rows) > 0 {
		result.PrevCursor = wafflemongo.EncodeCursor(text.Fold(result.Rows[0].Name), result.Rows[0].ID)
		result.NextCursor = wafflemongo.EncodeCursor(text.Fold(result.Rows[len(result.Rows)-1].Name), result.Rows[len(result.Rows)-1].ID)
	}

	return result, nil
}

// fetchReportOrgMemberCounts fetches member counts for each org, respecting memberStatus filter.
func (h *Handler) fetchReportOrgMemberCounts(ctx context.Context, db *mongo.Database, orgIDs []primitive.ObjectID, memberStatus string) (map[primitive.ObjectID]int64, error) {
	if len(orgIDs) == 0 {
		return make(map[primitive.ObjectID]int64), nil
	}

	match := bson.M{"role": "member", "organization_id": bson.M{"$in": orgIDs}}
	if memberStatus == "active" || memberStatus == "disabled" {
		match["status"] = memberStatus
	}

	counts, err := orgutil.AggregateCountByField(ctx, db, "users", match, "organization_id")
	if err != nil {
		h.Log.Error("database error aggregating member counts by org", zap.Error(err))
		return nil, err
	}
	return counts, nil
}
