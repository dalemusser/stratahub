// internal/app/features/reports/membersreportgroups.go
package reports

import (
	"context"

	"github.com/dalemusser/stratahub/internal/domain/models"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"go.uber.org/zap"
)

// fetchReportGroupsPane fetches the groups pane data for a selected org.
func (h *Handler) fetchReportGroupsPane(
	ctx context.Context,
	db *mongo.Database,
	scopeOrg primitive.ObjectID,
	memberStatus string,
) (groupsPaneResult, error) {
	var result groupsPaneResult

	// Fetch org name
	{
		var org models.Organization
		if err := db.Collection("organizations").FindOne(ctx, bson.M{"_id": scopeOrg}).Decode(&org); err != nil {
			if err == mongo.ErrNoDocuments {
				result.OrgName = "(Deleted)"
			} else {
				h.Log.Error("database error loading organization", zap.Error(err), zap.String("org_id", scopeOrg.Hex()))
				return result, err
			}
		} else {
			result.OrgName = org.Name
		}
	}

	// Fetch groups in selected org
	gf := bson.M{"organization_id": scopeOrg}
	gcur, err := db.Collection("groups").Find(ctx, gf, options.Find().
		SetSort(bson.D{{Key: "name_ci", Value: 1}, {Key: "_id", Value: 1}}).
		SetProjection(bson.M{"name": 1}))
	if err != nil {
		h.Log.Error("database error finding groups", zap.Error(err), zap.String("org_id", scopeOrg.Hex()))
		return result, err
	}
	defer gcur.Close(ctx)

	type grow struct {
		ID   primitive.ObjectID `bson:"_id"`
		Name string             `bson:"name"`
	}
	var glist []grow
	if err := gcur.All(ctx, &glist); err != nil {
		h.Log.Error("database error decoding groups", zap.Error(err))
		return result, err
	}

	// Collect group IDs for member count lookup
	gids := make([]primitive.ObjectID, 0, len(glist))
	for _, g := range glist {
		gids = append(gids, g.ID)
	}

	// Fetch member counts per group
	byGroup, err := h.fetchGroupMemberCounts(ctx, db, gids, memberStatus)
	if err != nil {
		return result, err
	}

	// Build group rows
	result.Rows = make([]groupRow, 0, len(glist))
	for _, g := range glist {
		result.Rows = append(result.Rows, groupRow{ID: g.ID, Name: g.Name, Count: byGroup[g.ID]})
	}

	// Count total members in this org (respecting memberStatus)
	ocond := bson.M{"role": "member", "organization_id": scopeOrg}
	if memberStatus == "active" || memberStatus == "disabled" {
		ocond["status"] = memberStatus
	}
	cnt, err := db.Collection("users").CountDocuments(ctx, ocond)
	if err != nil {
		h.Log.Error("database error counting org members", zap.Error(err), zap.String("org_id", scopeOrg.Hex()))
		return result, err
	}
	result.OrgMembersCount = cnt

	return result, nil
}

// fetchGroupMemberCounts fetches member counts for each group.
func (h *Handler) fetchGroupMemberCounts(ctx context.Context, db *mongo.Database, gids []primitive.ObjectID, memberStatus string) (map[primitive.ObjectID]int64, error) {
	byGroup := make(map[primitive.ObjectID]int64)

	if len(gids) == 0 {
		return byGroup, nil
	}

	gmMatch := bson.M{
		"group_id": bson.M{"$in": gids},
		"role":     "member",
	}

	userMatch := bson.M{"user.role": "member"}
	if memberStatus == "active" || memberStatus == "disabled" {
		userMatch["user.status"] = memberStatus
	}

	pipeline := []bson.M{
		{"$match": gmMatch},
		{"$lookup": bson.M{
			"from":         "users",
			"localField":   "user_id",
			"foreignField": "_id",
			"as":           "user",
		}},
		{"$unwind": "$user"},
		{"$match": userMatch},
		{"$group": bson.M{"_id": "$group_id", "count": bson.M{"$sum": 1}}},
	}

	agg, err := db.Collection("group_memberships").Aggregate(ctx, pipeline)
	if err != nil {
		h.Log.Error("database error aggregating group member counts", zap.Error(err))
		return nil, err
	}
	defer agg.Close(ctx)

	for agg.Next(ctx) {
		var row struct {
			ID    primitive.ObjectID `bson:"_id"`
			Count int64              `bson:"count"`
		}
		if err := agg.Decode(&row); err != nil {
			h.Log.Error("database error decoding group member count", zap.Error(err))
			return nil, err
		}
		byGroup[row.ID] = row.Count
	}

	return byGroup, nil
}

// fetchExportCounts calculates the export record count and members in groups count.
func (h *Handler) fetchExportCounts(
	ctx context.Context,
	db *mongo.Database,
	scopeOrg *primitive.ObjectID,
	selectedGroup, memberStatus string,
	allMembersTotal, orgMembersCount int64,
) exportCountsResult {
	var result exportCountsResult

	gmMatch := bson.M{"role": "member"}
	if scopeOrg != nil {
		gmMatch["org_id"] = *scopeOrg
	}
	if selectedGroup != "" {
		if gid, err := primitive.ObjectIDFromHex(selectedGroup); err == nil {
			gmMatch["group_id"] = gid
		}
	}

	userMatch := bson.M{"user.role": "member"}
	if memberStatus == "active" || memberStatus == "disabled" {
		userMatch["user.status"] = memberStatus
	}

	// Count memberships
	var membershipCount int64
	pipeline := []bson.M{
		{"$match": gmMatch},
		{"$lookup": bson.M{
			"from":         "users",
			"localField":   "user_id",
			"foreignField": "_id",
			"as":           "user",
		}},
		{"$unwind": "$user"},
		{"$match": userMatch},
		{"$count": "count"},
	}
	agg, err := db.Collection("group_memberships").Aggregate(ctx, pipeline)
	if err != nil {
		h.Log.Warn("failed to aggregate membership counts", zap.Error(err))
	} else {
		defer agg.Close(ctx)
		if agg.Next(ctx) {
			var row struct {
				Count int64 `bson:"count"`
			}
			if err := agg.Decode(&row); err != nil {
				h.Log.Warn("failed to decode membership count", zap.Error(err))
			} else {
				membershipCount = row.Count
			}
		}
	}

	// Count distinct members with memberships
	var membersWithMembership int64
	pipeline2 := []bson.M{
		{"$match": gmMatch},
		{"$lookup": bson.M{
			"from":         "users",
			"localField":   "user_id",
			"foreignField": "_id",
			"as":           "user",
		}},
		{"$unwind": "$user"},
		{"$match": userMatch},
		{"$group": bson.M{"_id": "$user._id"}},
		{"$count": "count"},
	}
	agg2, err := db.Collection("group_memberships").Aggregate(ctx, pipeline2)
	if err != nil {
		h.Log.Warn("failed to aggregate distinct member counts", zap.Error(err))
	} else {
		defer agg2.Close(ctx)
		if agg2.Next(ctx) {
			var row struct {
				Count int64 `bson:"count"`
			}
			if err := agg2.Decode(&row); err != nil {
				h.Log.Warn("failed to decode distinct member count", zap.Error(err))
			} else {
				membersWithMembership = row.Count
			}
		}
	}

	result.MembersInGroupsCount = membersWithMembership

	if selectedGroup != "" {
		result.ExportRecordCount = membershipCount
	} else {
		var membersInScope int64
		if scopeOrg == nil {
			membersInScope = allMembersTotal
		} else {
			membersInScope = orgMembersCount
		}
		if membersInScope < membersWithMembership {
			membersWithMembership = membersInScope
		}
		result.ExportRecordCount = membershipCount + (membersInScope - membersWithMembership)
	}

	return result
}
