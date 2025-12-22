// internal/app/system/handlers/organizations/list.go
package organizations

import (
	"context"
	"maps"
	"net/http"

	"github.com/dalemusser/stratahub/internal/app/system/authz"
	"github.com/dalemusser/stratahub/internal/app/system/orgutil"
	"github.com/dalemusser/stratahub/internal/app/system/paging"
	"github.com/dalemusser/stratahub/internal/app/system/timeouts"
	"github.com/dalemusser/waffle/pantry/httpnav"
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
// Authorization: RequireRole("admin") middleware in routes.go ensures only admins reach this handler.
func (h *Handler) ServeList(w http.ResponseWriter, r *http.Request) {
	role, uname, _, _ := authz.UserCtx(r)

	q := query.Search(r, "q")
	after := query.Get(r, "after")
	before := query.Get(r, "before")
	start := paging.ParseStart(r)

	ctx, cancel := context.WithTimeout(r.Context(), timeouts.Medium())
	defer cancel()

	db := h.DB

	// Build base filter
	base := bson.M{}
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

	// Count total
	total, err := db.Collection("organizations").CountDocuments(ctx, base)
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

	// Fetch organizations
	type orgRow struct {
		ID     primitive.ObjectID `bson:"_id"`
		Name   string             `bson:"name"`
		NameCI string             `bson:"name_ci"`
		City   string             `bson:"city"`
		State  string             `bson:"state"`
	}

	cur, err := db.Collection("organizations").Find(ctx, f, find)
	if err != nil {
		h.ErrLog.LogServerError(w, r, "find organizations failed", err, "Unable to load organizations.", "")
		return
	}
	defer cur.Close(ctx)

	var orgs []orgRow
	if err := cur.All(ctx, &orgs); err != nil {
		h.ErrLog.LogServerError(w, r, "decode organizations failed", err, "Unable to load organizations.", "")
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

	leadersMap, err := orgutil.AggregateCountByField(ctx, db, "users", bson.M{
		"role":            "leader",
		"organization_id": bson.M{"$in": orgIDs},
	}, "organization_id")
	if err != nil {
		h.ErrLog.LogServerError(w, r, "aggregate leader counts failed", err, "Unable to load organization data.", "")
		return
	}

	groupsMap, err := orgutil.AggregateCountByField(ctx, db, "groups", bson.M{
		"organization_id": bson.M{"$in": orgIDs},
	}, "organization_id")
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
		Title:       "Organizations",
		IsLoggedIn:  true,
		Role:        role,
		UserName:    uname,
		Q:           q,
		Items:       items,
		CurrentPath: httpnav.CurrentPath(r),

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
