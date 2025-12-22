// internal/app/features/groups/list.go
package groups

import (
	"context"
	"errors"
	"net/http"

	uierrors "github.com/dalemusser/stratahub/internal/app/features/errors"
	"github.com/dalemusser/stratahub/internal/app/system/authz"
	"github.com/dalemusser/stratahub/internal/app/system/normalize"
	"github.com/dalemusser/stratahub/internal/app/system/orgutil"
	"github.com/dalemusser/stratahub/internal/app/system/paging"
	"github.com/dalemusser/stratahub/internal/app/system/timeouts"
	"github.com/dalemusser/stratahub/internal/domain/models"
	"github.com/dalemusser/waffle/pantry/httpnav"
	wafflemongo "github.com/dalemusser/waffle/pantry/mongo"
	"github.com/dalemusser/waffle/pantry/templates"
	"github.com/dalemusser/waffle/pantry/text"
	"github.com/go-chi/chi/v5"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.uber.org/zap"
)

// andify composes clauses into a single bson.M with optional $and.
func andify(clauses []bson.M) bson.M {
	switch len(clauses) {
	case 0:
		return bson.M{}
	case 1:
		return clauses[0]
	default:
		return bson.M{"$and": clauses}
	}
}

// dedupObjectIDs removes duplicates while preserving order.
func dedupObjectIDs(in []primitive.ObjectID) []primitive.ObjectID {
	seen := make(map[primitive.ObjectID]struct{}, len(in))
	out := make([]primitive.ObjectID, 0, len(in))
	for _, id := range in {
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		out = append(out, id)
	}
	return out
}

// ServeGroupsList handles GET /groups (relative to mount) with admin org
// filtering and leader scoping.
func (h *Handler) ServeGroupsList(w http.ResponseWriter, r *http.Request) {
	role, uname, uid, ok := authz.UserCtx(r)
	if !ok {
		uierrors.RenderUnauthorized(w, r, "/login")
		return
	}

	// Only admins and leaders may view Groups
	if !(authz.IsAdmin(r) || authz.IsLeader(r)) {
		uierrors.RenderForbidden(w, r, "You don't have permission to view this page.", httpnav.ResolveBackURL(r, "/"))
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), timeouts.Medium())
	defer cancel()
	db := h.DB

	// --- read query params ---
	selectedOrg := normalize.QueryParam(r.URL.Query().Get("org")) // "all" or org hex
	orgQ := normalize.QueryParam(r.URL.Query().Get("org_q"))
	orgAfter := normalize.QueryParam(r.URL.Query().Get("org_after"))
	orgBefore := normalize.QueryParam(r.URL.Query().Get("org_before"))

	search := normalize.QueryParam(r.URL.Query().Get("search"))
	after := normalize.QueryParam(r.URL.Query().Get("after"))
	before := normalize.QueryParam(r.URL.Query().Get("before"))
	start := paging.ParseStart(r)

	// Leaders are forced to their own org; no org pane.
	showOrgPane := authz.IsAdmin(r)
	if authz.IsLeader(r) {
		oid, _, err := orgutil.ResolveLeaderOrg(ctx, db, uid)
		if errors.Is(err, orgutil.ErrUserNotFound) || errors.Is(err, orgutil.ErrNoOrganization) {
			uierrors.RenderForbidden(w, r, "Your account is not linked to an organization.", httpnav.ResolveBackURL(r, "/"))
			return
		}
		if err != nil {
			h.ErrLog.LogServerError(w, r, "database error resolving leader org", err, "A database error occurred.", "/groups")
			return
		}
		selectedOrg = oid.Hex()
		showOrgPane = false
	} else {
		// For admins, default to "all" when empty.
		if selectedOrg == "" {
			selectedOrg = "all"
		}
	}

	// --- build org pane (admins only) ---
	var orgPane orgPaneData
	if showOrgPane {
		var err error
		orgPane, err = h.fetchOrgPane(ctx, db, orgQ, orgAfter, orgBefore)
		if err != nil {
			h.ErrLog.LogServerError(w, r, "database error fetching org pane", err, "A database error occurred.", "/groups")
			return
		}
	}

	// --- build right-side groups table ---
	groups, shown, total, prevCur, nextCur, hasPrev, hasNext, err := h.fetchGroupsList(ctx, db, selectedOrg, search, after, before)
	if err != nil {
		h.ErrLog.LogServerError(w, r, "database error fetching groups list", err, "A database error occurred.", "/groups")
		return
	}

	rng := paging.ComputeRange(start, shown)

	data := groupListData{
		Title:      "Groups",
		IsLoggedIn: true,
		Role:       role,
		UserName:   uname,

		ShowOrgPane:   showOrgPane,
		OrgQuery:      orgQ,
		OrgShown:      len(orgPane.Rows),
		OrgTotal:      orgPane.Total,
		OrgHasPrev:    orgPane.HasPrev,
		OrgHasNext:    orgPane.HasNext,
		OrgPrevCur:    orgPane.PrevCursor,
		OrgNextCur:    orgPane.NextCursor,
		OrgRangeStart: orgPane.RangeStart,
		OrgRangeEnd:   orgPane.RangeEnd,
		SelectedOrg:   selectedOrg,
		OrgRows:       orgPane.Rows,
		AllCount:      orgPane.AllCount,

		SearchQuery: search,
		Shown:       shown,
		Total:       total,
		HasPrev:     hasPrev,
		HasNext:     hasNext,
		PrevCursor:  prevCur,
		NextCursor:  nextCur,
		Groups:      groups,

		RangeStart: rng.Start,
		RangeEnd:   rng.End,
		PrevStart:  rng.PrevStart,
		NextStart:  rng.NextStart,

		CurrentPath: httpnav.CurrentPath(r),
	}

	templates.RenderAutoMap(w, r, "groups_list", nil, data)
}

// fetchGroupsList fetches the paginated groups list with counts using a single
// aggregation pipeline for optimal performance. Combines org name lookup,
// membership counts, and assignment counts into one database round-trip.
func (h *Handler) fetchGroupsList(
	ctx context.Context,
	db *mongo.Database,
	selectedOrg, search, after, before string,
) ([]groupListItem, int, int64, string, string, bool, bool, error) {
	// Build base filter (without keyset window for total count)
	var baseClauses []bson.M
	if selectedOrg != "" && selectedOrg != "all" {
		if oid, err := primitive.ObjectIDFromHex(selectedOrg); err == nil {
			baseClauses = append(baseClauses, bson.M{"organization_id": oid})
		}
	}
	if search != "" {
		q := text.Fold(search)
		hi := q + "\uffff"
		baseClauses = append(baseClauses, bson.M{"name_ci": bson.M{"$gte": q, "$lt": hi}})
	}
	baseFilter := andify(baseClauses)

	// Configure keyset pagination
	cfg := paging.ConfigureKeyset(before, after)

	// Build full filter including keyset window
	fullClauses := append([]bson.M{}, baseClauses...)
	if ks := cfg.KeysetWindow("name_ci"); ks != nil {
		fullClauses = append(fullClauses, ks)
	}
	fullFilter := andify(fullClauses)

	// Build aggregation pipeline with $facet to get data and count in one query
	pipe := mongo.Pipeline{
		// Match base filter first (for accurate total count)
		bson.D{{Key: "$match", Value: baseFilter}},
		// Use $facet to run count and data queries in parallel
		bson.D{{Key: "$facet", Value: bson.M{
			"totalCount": []bson.M{
				{"$count": "count"},
			},
			"data": h.buildGroupsDataPipeline(fullFilter, baseFilter, cfg),
		}}},
	}

	cur, err := db.Collection("groups").Aggregate(ctx, pipe)
	if err != nil {
		return nil, 0, 0, "", "", false, false, err
	}
	defer cur.Close(ctx)

	// Parse aggregation result
	var result struct {
		TotalCount []struct {
			Count int64 `bson:"count"`
		} `bson:"totalCount"`
		Data []groupAggRow `bson:"data"`
	}
	if cur.Next(ctx) {
		if err := cur.Decode(&result); err != nil {
			return nil, 0, 0, "", "", false, false, err
		}
	}

	total := int64(0)
	if len(result.TotalCount) > 0 {
		total = result.TotalCount[0].Count
	}

	graw := result.Data

	// Reverse if paging backwards
	if cfg.Direction == paging.Backward {
		paging.Reverse(graw)
	}

	page := paging.TrimPage(&graw, before, after)
	hasPrev, hasNext := page.HasPrev, page.HasNext
	shown := len(graw)

	// Build result rows from aggregation data
	rows := make([]groupListItem, 0, shown)
	for _, g := range graw {
		rows = append(rows, groupListItem{
			ID:                     g.ID,
			Name:                   g.Name,
			OrganizationName:       g.OrgName,
			LeadersCount:           g.LeadersCount,
			MembersCount:           g.MembersCount,
			AssignedResourcesCount: g.AssignmentCount,
		})
	}

	// Build cursors
	prevCur, nextCur := "", ""
	if shown > 0 {
		prevCur = wafflemongo.EncodeCursor(graw[0].NameCI, graw[0].ID)
		nextCur = wafflemongo.EncodeCursor(graw[shown-1].NameCI, graw[shown-1].ID)
	}

	return rows, shown, total, prevCur, nextCur, hasPrev, hasNext, nil
}

// groupAggRow holds the result of the combined aggregation query.
type groupAggRow struct {
	ID              primitive.ObjectID `bson:"_id"`
	Name            string             `bson:"name"`
	NameCI          string             `bson:"name_ci"`
	OrgName         string             `bson:"org_name"`
	LeadersCount    int                `bson:"leaders_count"`
	MembersCount    int                `bson:"members_count"`
	AssignmentCount int                `bson:"assignment_count"`
}

// buildGroupsDataPipeline constructs the data portion of the $facet pipeline.
// It applies keyset pagination, joins org names, and computes membership/assignment counts.
func (h *Handler) buildGroupsDataPipeline(fullFilter, baseFilter bson.M, cfg paging.KeysetConfig) []bson.M {
	pipeline := []bson.M{}

	// Apply keyset window filter if present (re-match after facet's base match)
	if ks := cfg.KeysetWindow("name_ci"); ks != nil {
		pipeline = append(pipeline, bson.M{"$match": ks})
	}

	// Sort and limit for pagination
	pipeline = append(pipeline,
		bson.M{"$sort": bson.D{
			{Key: "name_ci", Value: cfg.SortOrder},
			{Key: "_id", Value: cfg.SortOrder},
		}},
		bson.M{"$limit": paging.LimitPlusOne()},
	)

	// Lookup organization name
	pipeline = append(pipeline,
		bson.M{"$lookup": bson.M{
			"from":         "organizations",
			"localField":   "organization_id",
			"foreignField": "_id",
			"as":           "org",
		}},
	)

	// Lookup and aggregate membership counts (leaders and members)
	pipeline = append(pipeline,
		bson.M{"$lookup": bson.M{
			"from": "group_memberships",
			"let":  bson.M{"gid": "$_id"},
			"pipeline": []bson.M{
				{"$match": bson.M{"$expr": bson.M{"$eq": []string{"$group_id", "$$gid"}}}},
				{"$group": bson.M{"_id": "$role", "count": bson.M{"$sum": 1}}},
			},
			"as": "memberships",
		}},
	)

	// Lookup and count resource assignments
	pipeline = append(pipeline,
		bson.M{"$lookup": bson.M{
			"from": "group_resource_assignments",
			"let":  bson.M{"gid": "$_id"},
			"pipeline": []bson.M{
				{"$match": bson.M{"$expr": bson.M{"$eq": []string{"$group_id", "$$gid"}}}},
				{"$count": "count"},
			},
			"as": "assignments",
		}},
	)

	// Project final fields with computed counts
	pipeline = append(pipeline,
		bson.M{"$project": bson.M{
			"_id":     1,
			"name":    1,
			"name_ci": 1,
			"org_name": bson.M{"$ifNull": []interface{}{
				bson.M{"$arrayElemAt": []interface{}{"$org.name", 0}},
				"",
			}},
			"leaders_count": bson.M{"$ifNull": []interface{}{
				bson.M{"$arrayElemAt": []interface{}{
					bson.M{"$map": bson.M{
						"input": bson.M{"$filter": bson.M{
							"input": "$memberships",
							"as":    "m",
							"cond":  bson.M{"$eq": []interface{}{"$$m._id", "leader"}},
						}},
						"as": "m",
						"in": "$$m.count",
					}},
					0,
				}},
				0,
			}},
			"members_count": bson.M{"$ifNull": []interface{}{
				bson.M{"$arrayElemAt": []interface{}{
					bson.M{"$map": bson.M{
						"input": bson.M{"$filter": bson.M{
							"input": "$memberships",
							"as":    "m",
							"cond":  bson.M{"$eq": []interface{}{"$$m._id", "member"}},
						}},
						"as": "m",
						"in": "$$m.count",
					}},
					0,
				}},
				0,
			}},
			"assignment_count": bson.M{"$ifNull": []interface{}{
				bson.M{"$arrayElemAt": []interface{}{"$assignments.count", 0}},
				0,
			}},
		}},
	)

	return pipeline
}

// ServeGroupManageModal handles GET /groups/{id}/manage_modal and returns
// the snippet for the Manage Group modal.
func (h *Handler) ServeGroupManageModal(w http.ResponseWriter, r *http.Request) {
	_, _, _, ok := authz.UserCtx(r)
	if !ok {
		uierrors.RenderUnauthorized(w, r, "/login")
		return
	}
	if !(authz.IsAdmin(r) || authz.IsLeader(r)) {
		uierrors.RenderForbidden(w, r, "You don't have permission to view this page.", httpnav.ResolveBackURL(r, "/groups"))
		return
	}

	gid := chi.URLParam(r, "id")
	if gid == "" {
		uierrors.HTMXBadRequest(w, r, "Invalid group ID.", "/groups")
		return
	}
	groupOID, err := primitive.ObjectIDFromHex(gid)
	if err != nil {
		uierrors.HTMXBadRequest(w, r, "Invalid group ID.", "/groups")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), timeouts.Short())
	defer cancel()
	db := h.DB

	var g struct {
		ID             primitive.ObjectID `bson:"_id"`
		Name           string             `bson:"name"`
		OrganizationID primitive.ObjectID `bson:"organization_id"`
	}
	if err := db.Collection("groups").FindOne(ctx, bson.M{"_id": groupOID}).Decode(&g); err != nil {
		if err == mongo.ErrNoDocuments {
			uierrors.HTMXError(w, r, http.StatusNotFound, "Group not found.", func() {
				uierrors.RenderNotFound(w, r, "Group not found.", "/groups")
			})
		} else {
			h.ErrLog.HTMXLogServerError(w, r, "database error loading group for modal", err, "A database error occurred.", "/groups")
		}
		return
	}

	orgName := ""
	if !g.OrganizationID.IsZero() {
		var o models.Organization
		if err := db.Collection("organizations").FindOne(ctx, bson.M{"_id": g.OrganizationID}).Decode(&o); err != nil {
			if err == mongo.ErrNoDocuments {
				h.Log.Warn("organization not found for group (may have been deleted)",
					zap.String("group_id", gid),
					zap.String("org_id", g.OrganizationID.Hex()))
				orgName = "(Deleted)"
			} else {
				h.ErrLog.HTMXLogServerError(w, r, "database error loading organization for group", err, "A database error occurred.", "/groups")
				return
			}
		} else {
			orgName = o.Name
		}
	}

	back := r.URL.Query().Get("return")
	if back == "" {
		back = httpnav.ResolveBackURL(r, "/groups")
	}

	data := groupManageModalData{
		GroupID:          gid,
		GroupName:        g.Name,
		OrganizationName: orgName,
		BackURL:          back,
	}

	templates.RenderSnippet(w, "group_manage_group_modal", data)
}
