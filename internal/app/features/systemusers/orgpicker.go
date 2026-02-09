// internal/app/features/systemusers/orgpicker.go
package systemusers

import (
	"maps"
	"net/http"
	"strconv"
	"strings"

	orgstore "github.com/dalemusser/stratahub/internal/app/store/organizations"
	"github.com/dalemusser/stratahub/internal/app/system/normalize"
	"github.com/dalemusser/stratahub/internal/app/system/paging"
	"github.com/dalemusser/stratahub/internal/app/system/timeouts"
	"github.com/dalemusser/stratahub/internal/app/system/workspace"
	wafflemongo "github.com/dalemusser/waffle/pantry/mongo"
	"github.com/dalemusser/waffle/pantry/templates"
	"github.com/dalemusser/waffle/pantry/text"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// orgPickerRow represents an organization in the picker.
type orgPickerRow struct {
	ID       string
	Name     string
	Selected bool
}

// orgPickerData is the view model for the org picker modal.
type orgPickerData struct {
	Orgs        []orgPickerRow
	Query       string
	SelectedIDs string // Comma-separated IDs
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

// ServeOrgPicker serves the multi-select organization picker modal for coordinators.
// GET /system-users/org-picker
func (h *Handler) ServeOrgPicker(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := timeouts.WithTimeout(r.Context(), timeouts.Medium(), h.Log, "org picker")
	defer cancel()

	query := normalize.QueryParam(r.URL.Query().Get("q"))
	selectedStr := r.URL.Query().Get("selected")
	after := normalize.QueryParam(r.URL.Query().Get("after"))
	before := normalize.QueryParam(r.URL.Query().Get("before"))

	// Parse selected IDs into a set for quick lookup
	selectedSet := make(map[string]bool)
	if selectedStr != "" {
		for _, id := range strings.Split(selectedStr, ",") {
			id = strings.TrimSpace(id)
			if id != "" {
				selectedSet[id] = true
			}
		}
	}

	// Track range start for display
	rangeStart := 1
	if startStr := r.URL.Query().Get("start"); startStr != "" {
		if s, err := strconv.Atoi(startStr); err == nil && s > 0 {
			rangeStart = s
		}
	}

	// Base filter: active organizations with workspace scoping
	base := bson.M{"status": "active"}
	workspace.FilterCtx(ctx, base)

	// Search clause
	if query != "" {
		qFold := text.Fold(query)
		hiFold := qFold + "\uffff"
		base["name_ci"] = bson.M{"$gte": qFold, "$lt": hiFold}
	}

	// Count total
	oStore := orgstore.New(h.DB)
	total, err := oStore.Count(ctx, base)
	if err != nil {
		h.ErrLog.LogServerError(w, r, "count orgs failed", err, "Failed to load organizations.", "/system-users")
		return
	}

	// Clone for cursor conditions
	f := maps.Clone(base)

	// Configure pagination
	find := options.Find()
	cfg := paging.ConfigureKeyset(before, after)
	cfg.ApplyToFind(find, "name_ci")

	if ks := cfg.KeysetWindow("name_ci"); ks != nil {
		maps.Copy(f, ks)
	}

	// Fetch organizations
	orgs, err := oStore.Find(ctx, f, find)
	if err != nil {
		h.ErrLog.LogServerError(w, r, "find orgs failed", err, "Failed to load organizations.", "/system-users")
		return
	}

	// Reverse if paging backwards
	if cfg.Direction == paging.Backward {
		paging.Reverse(orgs)
	}

	// Apply pagination trimming
	page := paging.TrimPage(&orgs, before, after)

	// Build rows
	rows := make([]orgPickerRow, 0, len(orgs))
	for _, org := range orgs {
		rows = append(rows, orgPickerRow{
			ID:       org.ID.Hex(),
			Name:     org.Name,
			Selected: selectedSet[org.ID.Hex()],
		})
	}

	// Pagination cursors
	prevCur, nextCur := "", ""
	shown := len(rows)
	if shown > 0 {
		prevCur = wafflemongo.EncodeCursor(orgs[0].NameCI, orgs[0].ID)
		nextCur = wafflemongo.EncodeCursor(orgs[shown-1].NameCI, orgs[shown-1].ID)
	}

	rangeEnd := rangeStart + shown - 1
	if rangeEnd < rangeStart {
		rangeEnd = rangeStart
	}

	data := orgPickerData{
		Orgs:        rows,
		Query:       query,
		SelectedIDs: selectedStr,
		Total:       total,
		RangeStart:  rangeStart,
		RangeEnd:    rangeEnd,
		HasPrev:     page.HasPrev,
		HasNext:     page.HasNext,
		PrevCursor:  prevCur,
		NextCursor:  nextCur,
		PrevStart:   max(1, rangeStart-shown),
		NextStart:   rangeStart + shown,
	}

	// Determine which template to render
	// If this is an HTMX request targeting the list, render just the list
	if r.Header.Get("HX-Target") == "org-picker-list" {
		templates.Render(w, r, "system_user_org_picker_list", data)
		return
	}

	// Otherwise render the full modal
	templates.Render(w, r, "system_user_org_picker_modal", data)
}

// parseOrgIDs parses a slice of org ID strings into ObjectIDs.
func parseOrgIDs(ids []string) []primitive.ObjectID {
	result := make([]primitive.ObjectID, 0, len(ids))
	for _, idStr := range ids {
		if oid, err := primitive.ObjectIDFromHex(idStr); err == nil {
			result = append(result, oid)
		}
	}
	return result
}
