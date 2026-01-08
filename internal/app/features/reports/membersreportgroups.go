// internal/app/features/reports/membersreportgroups.go
package reports

import (
	"context"

	"github.com/dalemusser/stratahub/internal/app/store/queries/reportqueries"
	"github.com/dalemusser/stratahub/internal/app/system/paging"
	"github.com/dalemusser/stratahub/internal/app/system/workspace"
	"github.com/dalemusser/stratahub/internal/domain/models"
	wafflemongo "github.com/dalemusser/waffle/pantry/mongo"
	"github.com/dalemusser/waffle/pantry/text"
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
	groupsAfter, groupsBefore, memberStatus string,
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

	// Build base filter
	gf := bson.M{"organization_id": scopeOrg}

	// Count total groups in org
	total, err := db.Collection("groups").CountDocuments(ctx, gf)
	if err != nil {
		h.Log.Error("database error counting groups", zap.Error(err), zap.String("org_id", scopeOrg.Hex()))
		return result, err
	}
	result.Total = total

	// Configure keyset pagination
	cfg := paging.ConfigureKeyset(groupsBefore, groupsAfter)
	findOpts := options.Find().SetProjection(bson.M{"name": 1, "name_ci": 1})
	cfg.ApplyToFind(findOpts, "name_ci")

	// Add cursor condition if present
	if window := cfg.KeysetWindow("name_ci"); window != nil {
		gf["$or"] = window["$or"]
	}

	gcur, err := db.Collection("groups").Find(ctx, gf, findOpts)
	if err != nil {
		h.Log.Error("database error finding groups", zap.Error(err), zap.String("org_id", scopeOrg.Hex()))
		return result, err
	}
	defer gcur.Close(ctx)

	type grow struct {
		ID     primitive.ObjectID `bson:"_id"`
		Name   string             `bson:"name"`
		NameCI string             `bson:"name_ci"`
	}
	var glist []grow
	if err := gcur.All(ctx, &glist); err != nil {
		h.Log.Error("database error decoding groups", zap.Error(err))
		return result, err
	}

	// Apply pagination trimming
	page := paging.TrimPage(&glist, groupsBefore, groupsAfter)
	result.HasPrev = page.HasPrev
	result.HasNext = page.HasNext

	// Reverse if paging backwards
	if groupsBefore != "" {
		paging.Reverse(glist)
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

	// Build cursors
	if len(glist) > 0 {
		result.PrevCursor = wafflemongo.EncodeCursor(text.Fold(glist[0].Name), glist[0].ID)
		result.NextCursor = wafflemongo.EncodeCursor(text.Fold(glist[len(glist)-1].Name), glist[len(glist)-1].ID)
	}

	// Count total members in this org (respecting memberStatus, with workspace scoping)
	ocond := bson.M{"role": "member", "organization_id": scopeOrg}
	workspace.FilterCtx(ctx, ocond)
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
	counts, err := reportqueries.CountGroupMembersPerGroup(ctx, db, gids, memberStatus)
	if err != nil {
		h.Log.Error("database error aggregating group member counts", zap.Error(err))
		return nil, err
	}
	return counts, nil
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

	membershipCount, membersWithMembership, err := reportqueries.CountMembershipStats(
		ctx, db, scopeOrg, selectedGroup, memberStatus,
	)
	if err != nil {
		h.Log.Warn("failed to aggregate membership stats", zap.Error(err))
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
