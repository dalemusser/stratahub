// internal/app/features/organizations/picker.go
package organizations

import (
	"context"
	"net/http"

	uierrors "github.com/dalemusser/stratahub/internal/app/features/errors"
	"github.com/dalemusser/stratahub/internal/app/system/authz"
	"github.com/dalemusser/stratahub/internal/app/system/normalize"
	"github.com/dalemusser/stratahub/internal/app/system/paging"
	"github.com/dalemusser/stratahub/internal/app/system/timeouts"
	"github.com/dalemusser/waffle/pantry/query"
	"github.com/dalemusser/waffle/pantry/templates"
	"github.com/dalemusser/waffle/pantry/text"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// orgPickerData is the view model for the organization picker modal.
type orgPickerData struct {
	Query      string
	SelectedID string // Currently selected org ID (for highlighting)

	Orgs []orgPickerItem

	Total      int64
	RangeStart int
	RangeEnd   int
	HasPrev    bool
	HasNext    bool
	PrevCursor string
	NextCursor string
	PrevStart  int
	NextStart  int
}

// orgPickerItem represents an organization in the picker list.
type orgPickerItem struct {
	ID       string
	Name     string
	Selected bool
}

// ServeOrgPicker serves the organization picker modal or just the list portion (for HTMX updates).
func (h *Handler) ServeOrgPicker(w http.ResponseWriter, r *http.Request) {
	role, _, _, ok := authz.UserCtx(r)
	if !ok {
		uierrors.RenderUnauthorized(w, r, "/login")
		return
	}
	if role != "admin" && role != "coordinator" && role != "leader" {
		uierrors.RenderForbidden(w, r, "Access denied.", "/dashboard")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), timeouts.Short())
	defer cancel()
	db := h.DB

	// Parse parameters
	q := normalize.QueryParam(query.Get(r, "q"))
	after := normalize.QueryParam(query.Get(r, "after"))
	before := normalize.QueryParam(query.Get(r, "before"))
	selectedID := normalize.QueryParam(query.Get(r, "selected"))
	start := paging.ParseStart(r)

	// Build filter for active organizations
	filter := bson.M{
		"status": "active",
	}

	// Coordinators can only see their assigned organizations
	if role == "coordinator" {
		orgIDs := authz.UserOrgIDs(r)
		if len(orgIDs) == 0 {
			filter["_id"] = primitive.NilObjectID // Will match nothing
		} else {
			filter["_id"] = bson.M{"$in": orgIDs}
		}
	}

	// Add search filter if query provided (match from start of name)
	if q != "" {
		qFold := text.Fold(q)
		filter["name_ci"] = bson.M{"$regex": "^" + qFold, "$options": "i"}
	}

	// Configure keyset pagination
	cfg := paging.ConfigureKeyset(before, after)
	if window := cfg.KeysetWindow("name_ci"); window != nil {
		for k, v := range window {
			filter[k] = v
		}
	}

	// Count total (without pagination)
	countFilter := bson.M{
		"status": "active",
	}
	// Coordinators can only see their assigned organizations
	if role == "coordinator" {
		orgIDs := authz.UserOrgIDs(r)
		if len(orgIDs) == 0 {
			countFilter["_id"] = primitive.NilObjectID // Will match nothing
		} else {
			countFilter["_id"] = bson.M{"$in": orgIDs}
		}
	}
	if q != "" {
		qFold := text.Fold(q)
		countFilter["name_ci"] = bson.M{"$regex": "^" + qFold, "$options": "i"}
	}
	total, err := db.Collection("organizations").CountDocuments(ctx, countFilter)
	if err != nil {
		h.ErrLog.LogServerError(w, r, "database error counting organizations", err, "A database error occurred.", "/dashboard")
		return
	}

	// Fetch organizations (use modal page size for picker)
	findOpts := options.Find().
		SetProjection(bson.M{"_id": 1, "name": 1, "name_ci": 1})
	cfg.ApplyToFindModal(findOpts, "name_ci")

	cur, err := db.Collection("organizations").Find(ctx, filter, findOpts)
	if err != nil {
		h.ErrLog.LogServerError(w, r, "database error finding organizations", err, "A database error occurred.", "/dashboard")
		return
	}
	defer cur.Close(ctx)

	type orgDoc struct {
		ID     primitive.ObjectID `bson:"_id"`
		Name   string             `bson:"name"`
		NameCI string             `bson:"name_ci"`
	}
	var docs []orgDoc
	if err := cur.All(ctx, &docs); err != nil {
		h.ErrLog.LogServerError(w, r, "database error decoding organizations", err, "A database error occurred.", "/dashboard")
		return
	}

	// Apply pagination trimming (use modal page size)
	result := paging.TrimPageModal(&docs, before, after)
	if cfg.Direction == paging.Backward {
		paging.Reverse(docs)
	}

	// Build cursors
	var prevCursor, nextCursor string
	if len(docs) > 0 {
		prevCursor, nextCursor = paging.BuildCursors(docs,
			func(d orgDoc) string { return d.NameCI },
			func(d orgDoc) primitive.ObjectID { return d.ID },
		)
	}

	// Compute range (use modal page size)
	rng := paging.ComputeRangeModal(start, len(docs))

	// Convert to view model
	orgs := make([]orgPickerItem, len(docs))
	for i, d := range docs {
		idHex := d.ID.Hex()
		orgs[i] = orgPickerItem{
			ID:       idHex,
			Name:     d.Name,
			Selected: idHex == selectedID,
		}
	}

	data := orgPickerData{
		Query:      q,
		SelectedID: selectedID,
		Orgs:       orgs,
		Total:      total,
		RangeStart: rng.Start,
		RangeEnd:   rng.End,
		HasPrev:    result.HasPrev,
		HasNext:    result.HasNext,
		PrevCursor: prevCursor,
		NextCursor: nextCursor,
		PrevStart:  rng.PrevStart,
		NextStart:  rng.NextStart,
	}

	// Check if this is an HTMX request for just the list
	if r.Header.Get("HX-Request") == "true" && r.Header.Get("HX-Target") == "org-picker-list" {
		templates.Render(w, r, "org_picker_list", data)
		return
	}

	// Full modal
	templates.Render(w, r, "org_picker_modal", data)
}
