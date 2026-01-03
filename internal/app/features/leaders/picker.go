// internal/app/features/leaders/picker.go
package leaders

import (
	"context"
	"net/http"
	"strings"

	uierrors "github.com/dalemusser/stratahub/internal/app/features/errors"
	"github.com/dalemusser/stratahub/internal/app/system/authz"
	"github.com/dalemusser/stratahub/internal/app/system/normalize"
	"github.com/dalemusser/stratahub/internal/app/system/orgutil"
	"github.com/dalemusser/stratahub/internal/app/system/paging"
	"github.com/dalemusser/stratahub/internal/app/system/timeouts"
	"github.com/dalemusser/waffle/pantry/query"
	"github.com/dalemusser/waffle/pantry/templates"
	"github.com/dalemusser/waffle/pantry/text"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// leaderPickerData is the view model for the leader picker modal.
type leaderPickerData struct {
	OrgHex      string
	OrgName     string
	Query       string
	SelectedIDs string // Comma-separated list of selected leader IDs

	Leaders []leaderPickerItem

	Total       int64
	RangeStart  int
	RangeEnd    int
	HasPrev     bool
	HasNext     bool
	PrevCursor  string
	NextCursor  string
	PrevStart   int
	NextStart   int
}

// leaderPickerItem represents a leader in the picker list.
type leaderPickerItem struct {
	ID       string
	FullName string
	Email    string
	Selected bool
}

// ServeLeaderPicker serves the leader picker modal or just the list portion (for HTMX updates).
func (h *Handler) ServeLeaderPicker(w http.ResponseWriter, r *http.Request) {
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
	orgHex := normalize.QueryParam(r.URL.Query().Get("org"))
	q := normalize.QueryParam(query.Get(r, "q"))
	after := normalize.QueryParam(query.Get(r, "after"))
	before := normalize.QueryParam(query.Get(r, "before"))
	selectedIDs := normalize.QueryParam(query.Get(r, "selected"))
	start := paging.ParseStart(r)

	// Validate org
	if orgHex == "" {
		uierrors.RenderBadRequest(w, r, "Organization is required.", "/dashboard")
		return
	}
	orgID, orgName, err := orgutil.ResolveActiveOrgFromHex(ctx, db, orgHex)
	if err == nil && role == "coordinator" {
		// Verify coordinator has access to this org
		if !authz.CanAccessOrg(r, orgID) {
			uierrors.RenderForbidden(w, r, "You don't have access to this organization.", "/dashboard")
			return
		}
	}
	if err != nil {
		if orgutil.IsExpectedOrgError(err) {
			uierrors.RenderBadRequest(w, r, "Organization not found.", "/dashboard")
			return
		}
		h.ErrLog.LogServerError(w, r, "database error loading organization", err, "A database error occurred.", "/dashboard")
		return
	}

	// Parse selected IDs into a set for quick lookup
	selectedSet := make(map[string]bool)
	if selectedIDs != "" {
		for _, id := range strings.Split(selectedIDs, ",") {
			id = strings.TrimSpace(id)
			if id != "" {
				selectedSet[id] = true
			}
		}
	}

	// Build filter for active leaders in this org
	filter := bson.M{
		"organization_id": orgID,
		"role":            "leader",
		"status":          "active",
	}

	// Add search filter if query provided (match from start of name/email)
	if q != "" {
		qFold := text.Fold(q)
		filter["$or"] = []bson.M{
			{"full_name_ci": bson.M{"$regex": "^" + qFold, "$options": "i"}},
			{"email": bson.M{"$regex": "^" + q, "$options": "i"}},
		}
	}

	// Configure keyset pagination
	cfg := paging.ConfigureKeyset(before, after)
	if window := cfg.KeysetWindow("full_name_ci"); window != nil {
		for k, v := range window {
			filter[k] = v
		}
	}

	// Count total (without pagination)
	countFilter := bson.M{
		"organization_id": orgID,
		"role":            "leader",
		"status":          "active",
	}
	if q != "" {
		qFold := text.Fold(q)
		countFilter["$or"] = []bson.M{
			{"full_name_ci": bson.M{"$regex": "^" + qFold, "$options": "i"}},
			{"email": bson.M{"$regex": "^" + q, "$options": "i"}},
		}
	}
	total, err := db.Collection("users").CountDocuments(ctx, countFilter)
	if err != nil {
		h.ErrLog.LogServerError(w, r, "database error counting leaders", err, "A database error occurred.", "/dashboard")
		return
	}

	// Fetch leaders (use modal page size for picker)
	findOpts := options.Find().
		SetProjection(bson.M{"_id": 1, "full_name": 1, "full_name_ci": 1, "email": 1})
	cfg.ApplyToFindModal(findOpts, "full_name_ci")

	cur, err := db.Collection("users").Find(ctx, filter, findOpts)
	if err != nil {
		h.ErrLog.LogServerError(w, r, "database error finding leaders", err, "A database error occurred.", "/dashboard")
		return
	}
	defer cur.Close(ctx)

	type leaderDoc struct {
		ID         primitive.ObjectID `bson:"_id"`
		FullName   string             `bson:"full_name"`
		FullNameCI string             `bson:"full_name_ci"`
		Email      string             `bson:"email"`
	}
	var docs []leaderDoc
	if err := cur.All(ctx, &docs); err != nil {
		h.ErrLog.LogServerError(w, r, "database error decoding leaders", err, "A database error occurred.", "/dashboard")
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
			func(d leaderDoc) string { return d.FullNameCI },
			func(d leaderDoc) primitive.ObjectID { return d.ID },
		)
	}

	// Compute range (use modal page size)
	rng := paging.ComputeRangeModal(start, len(docs))

	// Convert to view model
	leaders := make([]leaderPickerItem, len(docs))
	for i, d := range docs {
		idHex := d.ID.Hex()
		leaders[i] = leaderPickerItem{
			ID:       idHex,
			FullName: d.FullName,
			Email:    d.Email,
			Selected: selectedSet[idHex],
		}
	}

	data := leaderPickerData{
		OrgHex:      orgHex,
		OrgName:     orgName,
		Query:       q,
		SelectedIDs: selectedIDs,
		Leaders:     leaders,
		Total:       total,
		RangeStart:  rng.Start,
		RangeEnd:    rng.End,
		HasPrev:     result.HasPrev,
		HasNext:     result.HasNext,
		PrevCursor:  prevCursor,
		NextCursor:  nextCursor,
		PrevStart:   rng.PrevStart,
		NextStart:   rng.NextStart,
	}

	// Check if this is an HTMX request for just the list
	if r.Header.Get("HX-Request") == "true" && r.Header.Get("HX-Target") == "leader-picker-list" {
		templates.Render(w, r, "leader_picker_list", data)
		return
	}

	// Full modal
	templates.Render(w, r, "leader_picker_modal", data)
}
