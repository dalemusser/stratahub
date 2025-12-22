// internal/app/features/leaders/listleaders.go
package leaders

import (
	"context"
	"maps"
	"strings"

	"github.com/dalemusser/stratahub/internal/app/system/orgutil"
	"github.com/dalemusser/stratahub/internal/app/system/paging"
	"github.com/dalemusser/stratahub/internal/app/system/search"
	wafflemongo "github.com/dalemusser/waffle/pantry/mongo"
	"github.com/dalemusser/waffle/pantry/text"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"go.uber.org/zap"
)

// fetchLeadersList fetches a paginated list of leaders with optional filtering.
func (h *Handler) fetchLeadersList(
	ctx context.Context,
	db *mongo.Database,
	scopeOrg *primitive.ObjectID,
	searchQuery, status, after, before string,
	start int,
) (leaderListResult, error) {
	var result leaderListResult

	// Build base filter
	base := bson.M{"role": "leader"}
	if status == "active" || status == "disabled" {
		base["status"] = status
	}
	if scopeOrg != nil {
		base["organization_id"] = *scopeOrg
	}

	// Email pivot detection
	emailPivot := search.EmailPivotOK(searchQuery, status, scopeOrg != nil)

	var searchOr []bson.M
	if searchQuery != "" {
		sName := text.Fold(searchQuery)
		hiName := sName + "\uffff"
		sEmail := strings.ToLower(searchQuery)
		hiEmail := sEmail + "\uffff"

		if emailPivot {
			searchOr = []bson.M{{"email": bson.M{"$gte": sEmail, "$lt": hiEmail}}}
		} else {
			searchOr = []bson.M{
				{"full_name_ci": bson.M{"$gte": sName, "$lt": hiName}},
				{"email": bson.M{"$gte": sEmail, "$lt": hiEmail}},
			}
		}
		base["$or"] = searchOr
	}

	// Count total
	total, err := db.Collection("users").CountDocuments(ctx, base)
	if err != nil {
		h.Log.Error("database error counting leaders", zap.Error(err))
		return result, err
	}
	result.Total = total

	// Build pagination filter (clone base filter, then add cursor conditions)
	f := maps.Clone(base)
	find := options.Find()
	sortField := "full_name_ci"
	if emailPivot {
		sortField = "email"
	}

	// Configure keyset pagination
	cfg := paging.ConfigureKeyset(before, after)
	cfg.ApplyToFind(find, sortField)

	// Apply cursor conditions (handle $or clause specially)
	if ks := cfg.KeysetWindow(sortField); ks != nil {
		if searchQuery != "" {
			f["$and"] = []bson.M{{"$or": searchOr}, ks}
			delete(f, "$or")
		} else {
			maps.Copy(f, ks)
		}
	}

	// Fetch leaders
	type urow struct {
		ID             primitive.ObjectID  `bson:"_id"`
		FullName       string              `bson:"full_name"`
		FullNameCI     string              `bson:"full_name_ci"`
		Email          string              `bson:"email"`
		Status         string              `bson:"status"`
		Auth           string              `bson:"auth_method"`
		OrganizationID *primitive.ObjectID `bson:"organization_id"`
	}

	uc, err := db.Collection("users").Find(ctx, f, find)
	if err != nil {
		h.Log.Error("database error finding leaders", zap.Error(err))
		return result, err
	}
	defer uc.Close(ctx)

	var urows []urow
	if err := uc.All(ctx, &urows); err != nil {
		h.Log.Error("database error decoding leaders", zap.Error(err))
		return result, err
	}

	// Reverse if paging backwards
	if cfg.Direction == paging.Backward {
		paging.Reverse(urows)
	}

	// Apply pagination trimming
	page := paging.TrimPage(&urows, before, after)
	result.HasPrev = page.HasPrev
	result.HasNext = page.HasNext
	result.Shown = len(urows)

	// Compute range
	rng := paging.ComputeRange(start, result.Shown)
	result.RangeStart = rng.Start
	result.RangeEnd = rng.End
	result.PrevStart = rng.PrevStart
	result.NextStart = rng.NextStart

	// Collect org IDs for name lookup
	orgIDs := make([]primitive.ObjectID, 0, len(urows))
	for _, r := range urows {
		if r.OrganizationID != nil {
			orgIDs = append(orgIDs, *r.OrganizationID)
		}
	}

	// Fetch org names
	orgNames, err := orgutil.FetchOrgNames(ctx, db, orgIDs)
	if err != nil {
		return result, err
	}

	// Collect leader IDs for group count lookup
	leaderIDs := make([]primitive.ObjectID, 0, len(urows))
	for _, r := range urows {
		leaderIDs = append(leaderIDs, r.ID)
	}

	// Fetch groups per leader
	groupsByLeader, err := h.fetchLeaderGroupCounts(ctx, db, leaderIDs)
	if err != nil {
		return result, err
	}

	// Build leader rows
	result.Rows = make([]leaderRow, 0, len(urows))
	for _, r := range urows {
		on := ""
		if r.OrganizationID != nil {
			on = orgNames[*r.OrganizationID]
		}
		result.Rows = append(result.Rows, leaderRow{
			ID:          r.ID,
			FullName:    r.FullName,
			Email:       strings.ToLower(r.Email),
			OrgName:     on,
			GroupsCount: groupsByLeader[r.ID],
			Auth:        r.Auth,
			Status:      r.Status,
		})
	}

	// Build cursors
	if len(urows) > 0 {
		firstKey := urows[0].FullNameCI
		lastKey := urows[len(urows)-1].FullNameCI
		if emailPivot {
			firstKey = strings.ToLower(urows[0].Email)
			lastKey = strings.ToLower(urows[len(urows)-1].Email)
		}
		result.PrevCursor = wafflemongo.EncodeCursor(firstKey, urows[0].ID)
		result.NextCursor = wafflemongo.EncodeCursor(lastKey, urows[len(urows)-1].ID)
	}

	return result, nil
}

// fetchLeaderGroupCounts fetches group counts for each leader.
func (h *Handler) fetchLeaderGroupCounts(ctx context.Context, db *mongo.Database, leaderIDs []primitive.ObjectID) (map[primitive.ObjectID]int, error) {
	groupsByLeader := make(map[primitive.ObjectID]int)

	if len(leaderIDs) == 0 {
		return groupsByLeader, nil
	}

	curGM, err := db.Collection("group_memberships").Aggregate(ctx, []bson.M{
		{"$match": bson.M{"role": "leader", "user_id": bson.M{"$in": leaderIDs}}},
		{"$group": bson.M{"_id": "$user_id", "n": bson.M{"$sum": 1}}},
	})
	if err != nil {
		h.Log.Error("database error aggregating groups per leader", zap.Error(err))
		return nil, err
	}
	defer curGM.Close(ctx)

	for curGM.Next(ctx) {
		var row struct {
			ID primitive.ObjectID `bson:"_id"`
			N  int                `bson:"n"`
		}
		if err := curGM.Decode(&row); err != nil {
			h.Log.Error("database error decoding group count", zap.Error(err))
			return nil, err
		}
		groupsByLeader[row.ID] = row.N
	}

	return groupsByLeader, nil
}
