// internal/app/features/members/listmembers.go
package members

// Terminology: User Identifiers
//   - UserID / userID / user_id: The MongoDB ObjectID (_id) that uniquely identifies a user record
//   - LoginID / loginID / login_id: The human-readable string users type to log in

import (
	"context"
	"maps"
	"net/http"
	"strings"

	userstore "github.com/dalemusser/stratahub/internal/app/store/users"
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
// scopeOrgIDs limits members to those in the specified orgs (for coordinators); nil means all orgs.
func (h *Handler) fetchMembersList(
	ctx context.Context,
	r *http.Request,
	db *mongo.Database,
	scopeOrg *primitive.ObjectID,
	searchQuery, status, after, before string,
	start int,
	scopeOrgIDs []primitive.ObjectID,
) (memberListResult, error) {
	var result memberListResult

	// Build base filter with workspace scoping
	pbase := bson.M{"role": "member"}
	workspace.Filter(r, pbase)
	if status == "active" || status == "disabled" {
		pbase["status"] = status
	}
	if scopeOrg != nil {
		pbase["organization_id"] = *scopeOrg
	} else if len(scopeOrgIDs) > 0 {
		// Coordinator scope: limit to assigned orgs
		pbase["organization_id"] = bson.M{"$in": scopeOrgIDs}
	}

	// Search clause - search by name only
	if searchQuery != "" {
		qFold := text.Fold(strings.TrimSpace(searchQuery))
		hiFold := qFold + "\uffff"
		pbase["full_name_ci"] = bson.M{"$gte": qFold, "$lt": hiFold}
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

	// Configure keyset pagination
	cfg := paging.ConfigureKeyset(before, after)
	cfg.ApplyToFind(find, sortField)

	// Apply cursor conditions
	if ks := cfg.KeysetWindow(sortField); ks != nil {
		maps.Copy(f, ks)
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
		loginID := ""
		if r.LoginID != nil {
			loginID = *r.LoginID
		}
		result.Rows = append(result.Rows, memberRow{
			ID:       r.ID,
			FullName: r.FullName,
			LoginID:  loginID,
			OrgName:  on,
			Status:   r.Status,
		})
	}

	// Build cursors
	if len(urows) > 0 {
		firstKey := urows[0].FullNameCI
		lastKey := urows[len(urows)-1].FullNameCI
		result.PrevCursor = wafflemongo.EncodeCursor(firstKey, urows[0].ID)
		result.NextCursor = wafflemongo.EncodeCursor(lastKey, urows[len(urows)-1].ID)
	}

	return result, nil
}
