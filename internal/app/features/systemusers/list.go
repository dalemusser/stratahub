package systemusers

import (
	"maps"
	"net/http"
	"strings"

	"github.com/dalemusser/stratahub/internal/app/system/normalize"
	"github.com/dalemusser/stratahub/internal/app/system/paging"
	"github.com/dalemusser/stratahub/internal/app/system/search"
	"github.com/dalemusser/stratahub/internal/app/system/timeouts"
	"github.com/dalemusser/waffle/pantry/httpnav"
	wafflemongo "github.com/dalemusser/waffle/pantry/mongo"
	"github.com/dalemusser/waffle/pantry/templates"
	"github.com/dalemusser/waffle/pantry/text"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// ServeList handles GET /system-users.
//
// It lists system users (admins/analysts) with search, status/role
// filters, and keyset pagination. Admin-only access is enforced via
// requireAdmin.
func (h *Handler) ServeList(w http.ResponseWriter, r *http.Request) {
	role, uname, _, ok := userContext(r)
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

	// Base filter: system users (admin/analyst).
	roleSet := []string{"admin", "analyst"}
	base := bson.M{"role": bson.M{"$in": roleSet}}
	if status == "active" || status == "disabled" {
		base["status"] = status
	}
	if uRole == "admin" || uRole == "analyst" {
		base["role"] = uRole
	}

	// Decide whether to pivot to email sorting:
	// reuse the shared helper for "no-org" email pivot.
	emailPivot := search.EmailPivotNoOrgOK(searchQ, status)

	// Search clause
	if searchQ != "" {
		qFold := text.Fold(searchQ)
		hiFold := qFold + "\uffff"
		sLower := strings.ToLower(searchQ)
		hiEmail := sLower + "\uffff"

		if emailPivot {
			base["$or"] = []bson.M{
				{"email": bson.M{"$gte": sLower, "$lt": hiEmail}},
			}
		} else {
			base["$or"] = []bson.M{
				{"full_name_ci": bson.M{"$gte": qFold, "$lt": hiFold}},
				{"email": bson.M{"$gte": sLower, "$lt": hiEmail}},
			}
		}
	}

	total, countErr := db.Collection("users").CountDocuments(ctx, base)
	if countErr != nil {
		h.ErrLog.LogServerError(w, r, "database error counting system users", countErr, "A database error occurred.", "/")
		return
	}

	// Clone base filter, then add cursor conditions
	f := maps.Clone(base)

	// Sort field
	sortField := "full_name_ci"
	if emailPivot {
		sortField = "email"
	}

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

	type u struct {
		ID         primitive.ObjectID `bson:"_id"`
		FullName   string             `bson:"full_name"`
		FullNameCI string             `bson:"full_name_ci"`
		Email      string             `bson:"email"`
		Auth       string             `bson:"auth_method"`
		Role       string             `bson:"role"`
		Status     string             `bson:"status"`
	}

	cur, err := db.Collection("users").Find(ctx, f, find)
	if err != nil {
		h.ErrLog.LogServerError(w, r, "database error querying system users", err, "A database error occurred.", "/")
		return
	}
	defer cur.Close(ctx)

	var raw []u
	if err := cur.All(ctx, &raw); err != nil {
		h.ErrLog.LogServerError(w, r, "database error decoding system users", err, "A database error occurred.", "/")
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
		rows = append(rows, userRow{
			ID:       rr.ID,
			FullName: rr.FullName,
			Email:    normalize.Email(rr.Email),
			Role:     normalize.Role(rr.Role),
			Auth:     normalize.AuthMethod(rr.Auth),
			Status:   normalize.Status(rr.Status),
		})
	}

	prevCur, nextCur := "", ""
	if shown > 0 {
		firstKey := raw[0].FullNameCI
		lastKey := raw[shown-1].FullNameCI
		if emailPivot {
			firstKey = normalize.Email(raw[0].Email)
			lastKey = normalize.Email(raw[shown-1].Email)
		}
		prevCur = wafflemongo.EncodeCursor(firstKey, raw[0].ID)
		nextCur = wafflemongo.EncodeCursor(lastKey, raw[shown-1].ID)
	}

	templates.RenderAutoMap(w, r, "system-users_list", nil, listData{
		Title:       "System Users",
		IsLoggedIn:  true,
		Role:        role,
		UserName:    uname,
		SearchQuery: searchQ,
		Status:      status,
		URole:       uRole,
		UserRole:    uRole,
		Shown:       shown,
		Total:       total,
		HasPrev:     hasPrev,
		HasNext:     hasNext,
		PrevCursor:  prevCur,
		NextCursor:  nextCur,
		Rows:        rows,
		CurrentPath: httpnav.CurrentPath(r),
	})
}
