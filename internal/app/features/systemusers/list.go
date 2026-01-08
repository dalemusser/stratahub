package systemusers

import (
	"maps"
	"net/http"
	"strconv"

	userstore "github.com/dalemusser/stratahub/internal/app/store/users"
	"github.com/dalemusser/stratahub/internal/app/system/normalize"
	"github.com/dalemusser/stratahub/internal/app/system/paging"
	"github.com/dalemusser/stratahub/internal/app/system/timeouts"
	"github.com/dalemusser/stratahub/internal/app/system/viewdata"
	"github.com/dalemusser/stratahub/internal/app/system/workspace"
	wafflemongo "github.com/dalemusser/waffle/pantry/mongo"
	"github.com/dalemusser/waffle/pantry/templates"
	"github.com/dalemusser/waffle/pantry/text"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// ServeList handles GET /system-users.
//
// It lists system users (admins/analysts) with search, status/role
// filters, and keyset pagination. Admin-only access is enforced via
// requireAdmin.
func (h *Handler) ServeList(w http.ResponseWriter, r *http.Request) {
	_, _, _, ok := userContext(r)
	if !ok {
		return
	}

	ctx, cancel := timeouts.WithTimeout(r.Context(), timeouts.Long(), h.Log, "system users list")
	defer cancel()
	db := h.DB

	searchQ := normalize.QueryParam(r.URL.Query().Get("search"))
	status := normalize.Status(r.URL.Query().Get("status"))  // "", active, disabled
	uRole := normalize.Role(r.URL.Query().Get("role"))       // "", admin, analyst
	after := normalize.QueryParam(r.URL.Query().Get("after"))
	before := normalize.QueryParam(r.URL.Query().Get("before"))

	// Track range start for display (defaults to 1)
	rangeStart := 1
	if startStr := r.URL.Query().Get("start"); startStr != "" {
		if s, err := strconv.Atoi(startStr); err == nil && s > 0 {
			rangeStart = s
		}
	}

	// Base filter: system users (admin/analyst/coordinator) with workspace scoping.
	roleSet := []string{"admin", "analyst", "coordinator"}
	base := bson.M{"role": bson.M{"$in": roleSet}}
	workspace.Filter(r, base)

	if status == "active" || status == "disabled" {
		base["status"] = status
	}
	if uRole == "admin" || uRole == "analyst" || uRole == "coordinator" {
		base["role"] = uRole
	}

	// Search clause - search by name only
	if searchQ != "" {
		qFold := text.Fold(searchQ)
		hiFold := qFold + "\uffff"
		base["full_name_ci"] = bson.M{"$gte": qFold, "$lt": hiFold}
	}

	// Count and find via store
	usrStore := userstore.New(db)
	total, countErr := usrStore.Count(ctx, base)
	if countErr != nil {
		h.ErrLog.LogServerError(w, r, "database error counting system users", countErr, "A database error occurred.", "/")
		return
	}

	// Clone base filter, then add cursor conditions
	f := maps.Clone(base)

	// Sort field - always by name
	sortField := "full_name_ci"

	// Configure keyset pagination
	find := options.Find()
	cfg := paging.ConfigureKeyset(before, after)
	cfg.ApplyToFind(find, sortField)

	// Apply cursor conditions (handle $or clause specially)
	if ks := cfg.KeysetWindow(sortField); ks != nil {
		if searchQ != "" {
			if orAny, ok := base["$or"]; ok {
				f["$and"] = []bson.M{{"$or": orAny}, ks}
				delete(f, "$or")
			} else {
				maps.Copy(f, ks)
			}
		} else {
			maps.Copy(f, ks)
		}
	}

	// Fetch users via store
	raw, err := usrStore.Find(ctx, f, find)
	if err != nil {
		h.ErrLog.LogServerError(w, r, "database error querying system users", err, "A database error occurred.", "/")
		return
	}

	// Reverse if paging backwards
	if cfg.Direction == paging.Backward {
		paging.Reverse(raw)
	}

	// Apply pagination trimming
	page := paging.TrimPage(&raw, before, after)
	hasPrev, hasNext := page.HasPrev, page.HasNext
	shown := len(raw)

	rows := make([]userRow, 0, shown)
	for _, rr := range raw {
		loginID := ""
		if rr.LoginID != nil {
			loginID = *rr.LoginID
		}
		rows = append(rows, userRow{
			ID:       rr.ID,
			FullName: rr.FullName,
			LoginID:  loginID,
			Role:     normalize.Role(rr.Role),
			Auth:     normalize.AuthMethod(rr.AuthMethod),
			Status:   normalize.Status(rr.Status),
		})
	}

	prevCur, nextCur := "", ""
	if shown > 0 {
		firstKey := raw[0].FullNameCI
		lastKey := raw[shown-1].FullNameCI
		prevCur = wafflemongo.EncodeCursor(firstKey, raw[0].ID)
		nextCur = wafflemongo.EncodeCursor(lastKey, raw[shown-1].ID)
	}

	// Calculate range end
	rangeEnd := rangeStart + shown - 1
	if rangeEnd < rangeStart {
		rangeEnd = rangeStart
	}

	// Calculate next/prev start positions for pagination
	nextStart := rangeStart + shown
	prevStart := rangeStart - shown
	if prevStart < 1 {
		prevStart = 1
	}

	templates.RenderAutoMap(w, r, "system-users_list", nil, listData{
		BaseVM:      viewdata.NewBaseVM(r, h.DB, "System Users", "/"),
		SearchQuery: searchQ,
		Status:      status,
		URole:       uRole,
		UserRole:    uRole,
		Shown:       shown,
		Total:       total,
		RangeStart:  rangeStart,
		RangeEnd:    rangeEnd,
		PrevStart:   prevStart,
		NextStart:   nextStart,
		HasPrev:     hasPrev,
		HasNext:     hasNext,
		PrevCursor:  prevCur,
		NextCursor:  nextCur,
		Rows:        rows,
	})
}
