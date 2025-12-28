// internal/app/features/members/listmembers.go
package members

import (
	"context"
	"maps"
	"strings"

	userstore "github.com/dalemusser/stratahub/internal/app/store/users"
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

// memberListResult holds the result of fetching a paginated members list.
type memberListResult struct {
	Rows       []memberRow
	Total      int64
	Shown      int
	HasPrev    bool
	HasNext    bool
	PrevCursor string
	NextCursor string
	RangeStart int
	RangeEnd   int
	PrevStart  int
	NextStart  int
}

// fetchMembersList fetches a paginated list of members with optional filtering.
func (h *Handler) fetchMembersList(
	ctx context.Context,
	db *mongo.Database,
	scopeOrg *primitive.ObjectID,
	searchQuery, status, after, before string,
	start int,
) (memberListResult, error) {
	var result memberListResult

	qFold := text.Fold(strings.TrimSpace(searchQuery))
	hiFold := qFold + "\uffff"
	sLower := strings.ToLower(strings.TrimSpace(searchQuery))
	hiEmail := sLower + "\uffff"

	// Email-pivot when searching by email and org+status are constrained
	emailPivot := search.EmailPivotOK(searchQuery, status, scopeOrg != nil)

	// Build base filter
	pbase := bson.M{"role": "member"}
	if status == "active" || status == "disabled" {
		pbase["status"] = status
	}
	if scopeOrg != nil {
		pbase["organization_id"] = *scopeOrg
	}

	var searchOr []bson.M
	if searchQuery != "" {
		if emailPivot {
			searchOr = []bson.M{{"email": bson.M{"$gte": sLower, "$lt": hiEmail}}}
		} else {
			searchOr = []bson.M{
				{"full_name_ci": bson.M{"$gte": qFold, "$lt": hiFold}},
				{"email": bson.M{"$gte": sLower, "$lt": hiEmail}},
			}
		}
		pbase["$or"] = searchOr
	}

	// Count total via store
	usrStore := userstore.New(db)
	total, err := usrStore.Count(ctx, pbase)
	if err != nil {
		h.Log.Error("database error counting users", zap.Error(err))
		return result, err
	}
	result.Total = total

	// Build pagination filter (clone base filter, then add cursor conditions)
	f := maps.Clone(pbase)
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

	// Fetch users via store
	urows, err := usrStore.Find(ctx, f, find)
	if err != nil {
		h.Log.Error("database error finding users", zap.Error(err))
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

	// Fetch org names for the table
	orgNames, err := orgutil.FetchOrgNames(ctx, db, orgIDs)
	if err != nil {
		return result, err
	}

	// Build member rows
	result.Rows = make([]memberRow, 0, len(urows))
	for _, r := range urows {
		on := ""
		if r.OrganizationID != nil {
			on = orgNames[*r.OrganizationID]
		}
		result.Rows = append(result.Rows, memberRow{
			ID:       r.ID,
			FullName: r.FullName,
			Email:    strings.ToLower(r.Email),
			OrgName:  on,
			Status:   r.Status,
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
