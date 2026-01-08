// internal/app/features/organizations/list.go
package organizations

import (
	"context"
	"maps"
	"net/http"

	organizationstore "github.com/dalemusser/stratahub/internal/app/store/organizations"
	"github.com/dalemusser/stratahub/internal/app/system/authz"
	"github.com/dalemusser/stratahub/internal/app/system/orgutil"
	"github.com/dalemusser/stratahub/internal/app/system/paging"
	"github.com/dalemusser/stratahub/internal/app/system/timeouts"
	"github.com/dalemusser/stratahub/internal/app/system/viewdata"
	"github.com/dalemusser/stratahub/internal/app/system/workspace"
	wafflemongo "github.com/dalemusser/waffle/pantry/mongo"
	"github.com/dalemusser/waffle/pantry/query"
	"github.com/dalemusser/waffle/pantry/templates"
	"github.com/dalemusser/waffle/pantry/text"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// ServeList handles GET /organizations (with optional ?q= search).
// It supports HTMX partial refresh of the table when HX-Target="orgs-table-wrap".
// Authorization: RequireRole("admin", "coordinator") middleware in routes.go.
// Admins see all organizations; coordinators see only their assigned organizations.
func (h *Handler) ServeList(w http.ResponseWriter, r *http.Request) {
	role, _, _, _ := authz.UserCtx(r)

	q := query.Search(r, "q")
	after := query.Get(r, "after")
	before := query.Get(r, "before")
	start := paging.ParseStart(r)

	ctx, cancel := context.WithTimeout(r.Context(), timeouts.Medium())
	defer cancel()

	db := h.DB

	// Build base filter with workspace scoping
	base := bson.M{}
	workspace.Filter(r, base)

	// Coordinators can only see their assigned organizations
	if role == "coordinator" {
		orgIDs := authz.UserOrgIDs(r)
		if len(orgIDs) == 0 {
			// No assigned orgs - show empty list
			base["_id"] = primitive.NilObjectID // Will match nothing
		} else {
			base["_id"] = bson.M{"$in": orgIDs}
		}
	}

	var searchOr []bson.M
	if q != "" {
		fq := text.Fold(q)
		if fq != "" {
			hi := fq + "\uffff"
			searchOr = []bson.M{
				{"name_ci": bson.M{"$gte": fq, "$lt": hi}},
				{"city_ci": bson.M{"$gte": fq, "$lt": hi}},
				{"state_ci": bson.M{"$gte": fq, "$lt": hi}},
			}
			base["$or"] = searchOr
		}
	}

	// Count total via store
	orgStore := organizationstore.New(db)
	total, err := orgStore.Count(ctx, base)
	if err != nil {
		h.ErrLog.LogServerError(w, r, "count organizations failed", err, "Unable to load organizations.", "")
		return
	}

	// Clone base filter for pagination query
	f := maps.Clone(base)
	find := options.Find()
	sortField := "name_ci"

	// Configure keyset pagination
	cfg := paging.ConfigureKeyset(before, after)
	cfg.ApplyToFind(find, sortField)

	// Apply cursor conditions (handle $or clause specially)
	if ks := cfg.KeysetWindow(sortField); ks != nil {
		if q != "" && len(searchOr) > 0 {
			f["$and"] = []bson.M{{"$or": searchOr}, ks}
			delete(f, "$or")
		} else {
			maps.Copy(f, ks)
		}
	}

	// Fetch organizations via store
	orgs, err := orgStore.Find(ctx, f, find)
	if err != nil {
		h.ErrLog.LogServerError(w, r, "find organizations failed", err, "Unable to load organizations.", "")
		return
	}

	// Reverse if paging backwards
	if cfg.Direction == paging.Backward {
		paging.Reverse(orgs)
	}

	// Apply pagination trimming
	page := paging.TrimPage(&orgs, before, after)

	// Compute range
	shown := len(orgs)
	rng := paging.ComputeRange(start, shown)

	// Build ID list for aggregates
	orgIDs := make([]primitive.ObjectID, 0, len(orgs))
	for _, o := range orgs {
		orgIDs = append(orgIDs, o.ID)
	}

	leadersFilter := bson.M{
		"role":            "leader",
		"organization_id": bson.M{"$in": orgIDs},
	}
	workspace.Filter(r, leadersFilter)
	leadersMap, err := orgutil.AggregateCountByField(ctx, db, "users", leadersFilter, "organization_id")
	if err != nil {
		h.ErrLog.LogServerError(w, r, "aggregate leader counts failed", err, "Unable to load organization data.", "")
		return
	}

	groupsFilter := bson.M{
		"organization_id": bson.M{"$in": orgIDs},
	}
	workspace.Filter(r, groupsFilter)
	groupsMap, err := orgutil.AggregateCountByField(ctx, db, "groups", groupsFilter, "organization_id")
	if err != nil {
		h.ErrLog.LogServerError(w, r, "aggregate group counts failed", err, "Unable to load organization data.", "")
		return
	}

	items := make([]listItem, 0, len(orgs))
	for _, o := range orgs {
		items = append(items, listItem{
			ID:           o.ID,
			Name:         o.Name,
			NameCI:       o.NameCI,
			City:         o.City,
			State:        o.State,
			LeadersCount: leadersMap[o.ID],
			GroupsCount:  groupsMap[o.ID],
		})
	}

	// Build cursors
	prevCur, nextCur := "", ""
	if len(orgs) > 0 {
		prevCur = wafflemongo.EncodeCursor(orgs[0].NameCI, orgs[0].ID)
		nextCur = wafflemongo.EncodeCursor(orgs[len(orgs)-1].NameCI, orgs[len(orgs)-1].ID)
	}

	data := listData{
		BaseVM: viewdata.NewBaseVM(r, h.DB, "Organizations", "/organizations"),
		Q:      q,
		Items:  items,

		Shown:      shown,
		Total:      total,
		HasPrev:    page.HasPrev,
		HasNext:    page.HasNext,
		PrevCursor: prevCur,
		NextCursor: nextCur,
		RangeStart: rng.Start,
		RangeEnd:   rng.End,
		PrevStart:  rng.PrevStart,
		NextStart:  rng.NextStart,
	}

	// HTMX partial: just the table
	if r.Header.Get("HX-Request") != "" && r.Header.Get("HX-Target") == "orgs-table-wrap" {
		templates.RenderSnippet(w, "organizations_table", data)
		return
	}

	templates.Render(w, r, "organizations_list", data)
}
