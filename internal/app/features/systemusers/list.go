package systemusers

import (
	"context"
	"net/http"
	"strings"

	"github.com/dalemusser/stratahub/internal/app/system/paging"
	"github.com/dalemusser/stratahub/internal/app/system/search"
	"github.com/dalemusser/waffle/templates"
	mongodb "github.com/dalemusser/waffle/toolkit/db/mongodb"
	textfold "github.com/dalemusser/waffle/toolkit/text/textfold"
	nav "github.com/dalemusser/waffle/toolkit/ui/nav"

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
	role, uname, _, ok := requireAdmin(w, r)
	if !ok {
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), sysUsersLongTimeout)
	defer cancel()
	db := h.DB

	searchQ := strings.TrimSpace(r.URL.Query().Get("search"))
	status := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("status"))) // "", active, disabled
	uRole := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("role")))    // "", admin, analyst
	after := strings.TrimSpace(r.URL.Query().Get("after"))
	before := strings.TrimSpace(r.URL.Query().Get("before"))

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
		qFold := textfold.Fold(searchQ)
		hiFold := qFold + "\uffff"
		sLower := strings.ToLower(strings.TrimSpace(searchQ))
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

	total, _ := db.Collection("users").CountDocuments(ctx, base)

	find := options.Find()
	limit := paging.LimitPlusOne()

	f := bson.M{}
	for k, v := range base {
		f[k] = v
	}

	// Sort field and keyset window
	sortField := "full_name_ci"
	if emailPivot {
		sortField = "email"
	}

	applyWindow := func(direction string, key string, id primitive.ObjectID) {
		win := mongodb.KeysetWindow(sortField, direction, key, id)
		if searchQ != "" {
			// Keep the original OR, but wrap it so we can AND the keyset window.
			if orAny, ok := base["$or"]; ok {
				f["$and"] = []bson.M{
					{"$or": orAny},
					win,
				}
				delete(f, "$or")
			} else {
				for k, v := range win {
					f[k] = v
				}
			}
		} else {
			for k, v := range win {
				f[k] = v
			}
		}
	}

	if before != "" {
		if c, ok := mongodb.DecodeCursor(before); ok {
			applyWindow("lt", c.CI, c.ID)
		}
		find.SetSort(bson.D{{Key: sortField, Value: -1}, {Key: "_id", Value: -1}}).
			SetLimit(limit)
	} else {
		if after != "" {
			if c, ok := mongodb.DecodeCursor(after); ok {
				applyWindow("gt", c.CI, c.ID)
			}
		}
		find.SetSort(bson.D{{Key: sortField, Value: 1}, {Key: "_id", Value: 1}}).
			SetLimit(limit)
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
		http.Error(w, "db error", http.StatusInternalServerError)
		return
	}
	defer cur.Close(ctx)

	var raw []u
	_ = cur.All(ctx, &raw)

	orig := len(raw)
	hasPrev, hasNext := false, false
	pageSize := paging.PageSize

	if before != "" {
		if orig > pageSize {
			raw = raw[1:]
			hasPrev = true
		}
		hasNext = true
	} else {
		if orig > pageSize {
			raw = raw[:pageSize]
			hasNext = true
		}
		hasPrev = after != ""
	}

	shown := len(raw)

	rows := make([]userRow, 0, shown)
	for _, rr := range raw {
		rows = append(rows, userRow{
			ID:       rr.ID,
			FullName: rr.FullName,
			Email:    strings.ToLower(rr.Email),
			Role:     strings.ToLower(rr.Role),
			Auth:     strings.ToLower(rr.Auth),
			Status:   strings.ToLower(rr.Status),
		})
	}

	prevCur, nextCur := "", ""
	if shown > 0 {
		firstKey := raw[0].FullNameCI
		lastKey := raw[shown-1].FullNameCI
		if emailPivot {
			firstKey = strings.ToLower(raw[0].Email)
			lastKey = strings.ToLower(raw[shown-1].Email)
		}
		prevCur = mongodb.EncodeCursor(firstKey, raw[0].ID)
		nextCur = mongodb.EncodeCursor(lastKey, raw[shown-1].ID)
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
		CurrentPath: nav.CurrentPath(r),
	})
}
