// internal/app/features/groups/list.go
package groups

import (
	"context"
	"net/http"

	uierrors "github.com/dalemusser/stratahub/internal/app/features/errors"
	"github.com/dalemusser/stratahub/internal/app/store/queries/groupqueries"
	"github.com/dalemusser/stratahub/internal/app/system/authz"
	"github.com/dalemusser/stratahub/internal/app/system/normalize"
	"github.com/dalemusser/stratahub/internal/app/system/paging"
	"github.com/dalemusser/stratahub/internal/app/system/timeouts"
	"github.com/dalemusser/stratahub/internal/app/system/viewdata"
	"github.com/dalemusser/stratahub/internal/domain/models"
	"github.com/dalemusser/waffle/pantry/httpnav"
	wafflemongo "github.com/dalemusser/waffle/pantry/mongo"
	"github.com/dalemusser/waffle/pantry/templates"
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
// filtering, coordinator org filtering, and leader scoping.
func (h *Handler) ServeGroupsList(w http.ResponseWriter, r *http.Request) {
	_, _, uid, ok := authz.UserCtx(r)
	if !ok {
		uierrors.RenderUnauthorized(w, r, "/login")
		return
	}

	// Only admins, coordinators, and leaders may view Groups
	if !(authz.IsAdmin(r) || authz.IsCoordinator(r) || authz.IsLeader(r)) {
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

	// Determine org pane visibility and scope based on role
	var showOrgPane bool
	var scopeOrgIDs []primitive.ObjectID   // For coordinators: limit org pane to these orgs
	var leaderGroupIDs []primitive.ObjectID // For leaders: limit to their assigned groups

	if authz.IsLeader(r) {
		// Leaders see only groups they are assigned to as leaders; no org pane
		// Initialize to non-nil empty slice to distinguish "leader with no groups" from "not a leader"
		leaderGroupIDs = []primitive.ObjectID{}

		cur, err := db.Collection("group_memberships").Find(ctx, bson.M{
			"user_id": uid,
			"role":    "leader",
		})
		if err != nil {
			h.ErrLog.LogServerError(w, r, "database error fetching leader groups", err, "A database error occurred.", "/groups")
			return
		}
		defer cur.Close(ctx)

		for cur.Next(ctx) {
			var gm struct {
				GroupID primitive.ObjectID `bson:"group_id"`
			}
			if err := cur.Decode(&gm); err == nil {
				leaderGroupIDs = append(leaderGroupIDs, gm.GroupID)
			}
		}
		if err := cur.Err(); err != nil {
			h.ErrLog.LogServerError(w, r, "database error iterating leader groups", err, "A database error occurred.", "/groups")
			return
		}

		showOrgPane = false
	} else if authz.IsCoordinator(r) {
		// Coordinators see org pane filtered to their assigned orgs
		scopeOrgIDs = authz.UserOrgIDs(r)
		if len(scopeOrgIDs) == 0 {
			uierrors.RenderForbidden(w, r, "You are not assigned to any organizations.", httpnav.ResolveBackURL(r, "/"))
			return
		}
		showOrgPane = true
		if selectedOrg == "" {
			selectedOrg = "all"
		}
		// Validate selected org is in coordinator's allowed list
		if selectedOrg != "all" {
			if oid, err := primitive.ObjectIDFromHex(selectedOrg); err == nil {
				allowed := false
				for _, allowedOrgID := range scopeOrgIDs {
					if allowedOrgID == oid {
						allowed = true
						break
					}
				}
				if !allowed {
					selectedOrg = "all"
				}
			} else {
				selectedOrg = "all"
			}
		}
	} else {
		// Admins see all orgs
		showOrgPane = true
		if selectedOrg == "" {
			selectedOrg = "all"
		}
	}

	// --- build org pane (admins and coordinators) ---
	var orgPane orgPaneData
	if showOrgPane {
		var err error
		orgPane, err = h.fetchOrgPane(ctx, db, orgQ, orgAfter, orgBefore, scopeOrgIDs)
		if err != nil {
			h.ErrLog.LogServerError(w, r, "database error fetching org pane", err, "A database error occurred.", "/groups")
			return
		}
	}

	// --- build right-side groups table ---
	groups, shown, total, prevCur, nextCur, hasPrev, hasNext, err := h.fetchGroupsList(ctx, db, selectedOrg, search, after, before, scopeOrgIDs, leaderGroupIDs)
	if err != nil {
		h.ErrLog.LogServerError(w, r, "database error fetching groups list", err, "A database error occurred.", "/groups")
		return
	}

	rng := paging.ComputeRange(start, shown)

	data := groupListData{
		BaseVM: viewdata.NewBaseVM(r, h.DB, "Groups", "/groups"),

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
	}

	templates.RenderAutoMap(w, r, "groups_list", nil, data)
}

// fetchGroupsList fetches the paginated groups list with counts using groupqueries.
// scopeOrgIDs limits the results to groups in those orgs (for coordinators); nil means no restriction.
// leaderGroupIDs limits the results to those specific groups (for leaders).
// nil = not a leader (use other filters), non-nil empty = leader with no groups (return empty result).
func (h *Handler) fetchGroupsList(
	ctx context.Context,
	db *mongo.Database,
	selectedOrg, search, after, before string,
	scopeOrgIDs []primitive.ObjectID,
	leaderGroupIDs []primitive.ObjectID,
) ([]groupListItem, int, int64, string, string, bool, bool, error) {
	// Build filter
	var filter groupqueries.ListFilter

	// Handle leader-specific filtering
	if leaderGroupIDs != nil {
		// This is a leader - they can only see their assigned groups
		if len(leaderGroupIDs) == 0 {
			// Leader with no assigned groups - return empty result
			return nil, 0, 0, "", "", false, false, nil
		}
		// Leader with assigned groups - filter to those groups
		filter.GroupIDs = leaderGroupIDs
	} else if selectedOrg != "" && selectedOrg != "all" {
		if oid, err := primitive.ObjectIDFromHex(selectedOrg); err == nil {
			filter.OrgID = &oid
		}
	} else if len(scopeOrgIDs) > 0 {
		// Coordinator viewing "all" - scope to their assigned orgs
		filter.OrgIDs = scopeOrgIDs
	}
	filter.SearchQuery = search

	// Configure keyset pagination
	cfg := paging.ConfigureKeyset(before, after)

	// Fetch groups with counts using query module
	result, err := groupqueries.ListGroupsWithCounts(ctx, db, filter, cfg)
	if err != nil {
		return nil, 0, 0, "", "", false, false, err
	}

	graw := result.Items

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

	return rows, shown, result.Total, prevCur, nextCur, hasPrev, hasNext, nil
}

// ServeGroupManageModal handles GET /groups/{id}/manage_modal and returns
// the snippet for the Manage Group modal.
func (h *Handler) ServeGroupManageModal(w http.ResponseWriter, r *http.Request) {
	role, _, _, ok := authz.UserCtx(r)
	if !ok {
		uierrors.RenderUnauthorized(w, r, "/login")
		return
	}
	if !(authz.IsAdmin(r) || authz.IsCoordinator(r) || authz.IsLeader(r)) {
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
		OrganizationID:   g.OrganizationID.Hex(),
		OrganizationName: orgName,
		BackURL:          back,
		Role:             role,
	}

	templates.RenderSnippet(w, "group_manage_group_modal", data)
}
