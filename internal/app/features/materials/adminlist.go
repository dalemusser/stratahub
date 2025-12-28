// internal/app/features/materials/adminlist.go
package materials

import (
	"context"
	"maps"
	"net/http"

	materialstore "github.com/dalemusser/stratahub/internal/app/store/materials"
	"github.com/dalemusser/stratahub/internal/app/system/paging"
	"github.com/dalemusser/stratahub/internal/app/system/timeouts"
	"github.com/dalemusser/stratahub/internal/app/system/viewdata"
	wafflemongo "github.com/dalemusser/waffle/pantry/mongo"
	"github.com/dalemusser/waffle/pantry/query"
	"github.com/dalemusser/waffle/pantry/templates"
	"github.com/dalemusser/waffle/pantry/text"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// ServeList displays the admin list of materials.
// Supports live HTMX search and prefix queries on *_ci columns.
// Authorization: RequireRole("admin") middleware in routes.go ensures only admins reach this handler.
func (h *AdminHandler) ServeList(w http.ResponseWriter, r *http.Request) {
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
	if lo, hi := text.PrefixRange(q); lo != "" {
		searchOr = []bson.M{
			{"title_ci": bson.M{"$gte": lo, "$lt": hi}},
			{"subject_ci": bson.M{"$gte": lo, "$lt": hi}},
		}
		base["$or"] = searchOr
	}

	// Count total via store
	matStore := materialstore.New(db)
	total, err := matStore.Count(ctx, base)
	if err != nil {
		h.ErrLog.LogServerError(w, r, "count materials failed", err, "A database error occurred.", "/")
		return
	}

	// Clone base filter for pagination query
	f := maps.Clone(base)
	find := options.Find()
	sortField := "title_ci"

	// Configure keyset pagination
	cfg := paging.ConfigureKeyset(before, after)
	cfg.ApplyToFind(find, sortField)

	// Add projection
	find.SetProjection(bson.M{
		"_id":         1,
		"title":       1,
		"title_ci":    1,
		"subject":     1,
		"type":        1,
		"status":      1,
		"file_path":   1,
		"description": 1,
	})

	// Apply cursor conditions (handle $or clause specially)
	if ks := cfg.KeysetWindow(sortField); ks != nil {
		if q != "" && len(searchOr) > 0 {
			f["$and"] = []bson.M{{"$or": searchOr}, ks}
			delete(f, "$or")
		} else {
			maps.Copy(f, ks)
		}
	}

	// Fetch materials via store
	rows, err := matStore.Find(ctx, f, find)
	if err != nil {
		h.ErrLog.LogServerError(w, r, "find materials failed", err, "A database error occurred.", "/")
		return
	}

	// Reverse if paging backwards
	if cfg.Direction == paging.Backward {
		paging.Reverse(rows)
	}

	// Apply pagination trimming
	page := paging.TrimPage(&rows, before, after)

	// Compute range
	shown := len(rows)
	rng := paging.ComputeRange(start, shown)

	items := make([]listItem, 0, len(rows))
	for _, mat := range rows {
		items = append(items, listItem{
			ID:          mat.ID,
			Title:       mat.Title,
			TitleCI:     mat.TitleCI,
			Subject:     mat.Subject,
			Type:        mat.Type,
			Status:      mat.Status,
			HasFile:     mat.HasFile(),
			Description: mat.Description,
		})
	}

	// Build cursors
	prevCur, nextCur := "", ""
	if len(rows) > 0 {
		prevCur = wafflemongo.EncodeCursor(rows[0].TitleCI, rows[0].ID)
		nextCur = wafflemongo.EncodeCursor(rows[len(rows)-1].TitleCI, rows[len(rows)-1].ID)
	}

	data := listData{
		BaseVM: viewdata.NewBaseVM(r, h.DB, "Materials", "/"),
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

	// HTMX partial table refresh
	if r.Header.Get("HX-Request") != "" && r.Header.Get("HX-Target") == "materials-table-wrap" {
		templates.RenderSnippet(w, "materials_table", data)
		return
	}

	templates.Render(w, r, "materials_list", data)
}
