// internal/app/features/members/list.go
package members

import (
	"context"
	"net/http"
	"strconv"
	"strings"

	"github.com/dalemusser/stratahub/internal/app/system/paging"
	"github.com/dalemusser/stratahub/internal/app/system/search"
	"github.com/dalemusser/stratahub/internal/domain/models"
	"github.com/dalemusser/waffle/templates"
	mongodb "github.com/dalemusser/waffle/toolkit/db/mongodb"
	textfold "github.com/dalemusser/waffle/toolkit/text/textfold"
	nav "github.com/dalemusser/waffle/toolkit/ui/nav"

	"github.com/go-chi/chi/v5"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo/options"
	"go.uber.org/zap"
)

// ServeList renders the main Members screen with org pane + members table.
func (h *Handler) ServeList(w http.ResponseWriter, r *http.Request) {
	role, uname, uid, ok := userCtx(r)
	if !ok {
		renderUnauthorized(w, r)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), membersLongTimeout)
	defer cancel()
	db := h.DB

	orgParam := strings.TrimSpace(r.URL.Query().Get("org"))
	orgQ := strings.TrimSpace(r.URL.Query().Get("org_q"))
	orgAfter := strings.TrimSpace(r.URL.Query().Get("org_after"))
	orgBefore := strings.TrimSpace(r.URL.Query().Get("org_before"))

	searchQuery := strings.TrimSpace(r.URL.Query().Get("search"))
	status := strings.TrimSpace(r.URL.Query().Get("status"))
	after := strings.TrimSpace(r.URL.Query().Get("after"))
	before := strings.TrimSpace(r.URL.Query().Get("before"))

	// Human-friendly start index for the MEMBERS range (defaults to 1)
	startParam := strings.TrimSpace(r.URL.Query().Get("start"))
	start := 1
	if startParam != "" {
		if n, err := strconv.Atoi(startParam); err == nil && n > 0 {
			start = n
		}
	}

	// Org range (we default to 1 on each org page)
	orgRangeStart := 1
	orgRangeEnd := 0 // computed later when we know how many orgs are shown

	// scope: admin = choose; leader = fixed org
	var selectedOrg string
	var scopeOrg *primitive.ObjectID
	if role == "admin" {
		if orgParam == "" {
			selectedOrg = "all"
		} else {
			selectedOrg = orgParam
		}
		if selectedOrg != "all" {
			if oid, err := primitive.ObjectIDFromHex(selectedOrg); err == nil {
				scopeOrg = &oid
			} else {
				selectedOrg = "all"
			}
		}
	} else {
		var u models.User
		if err := db.Collection("users").
			FindOne(ctx, bson.M{"_id": uid}).
			Decode(&u); err == nil && u.OrganizationID != nil {
			selectedOrg = (*u.OrganizationID).Hex()
			scopeOrg = u.OrganizationID
		} else {
			renderForbidden(w, r, "Your account is not linked to an organization.")
			return
		}
	}

	// ------- Left orgs pane (admin only) -------
	showOrgPane := role == "admin"
	var orgRows []orgRow
	var orgShown int
	var orgTotal int64
	var orgHasPrev, orgHasNext bool
	var orgPrevCur, orgNextCur string

	if showOrgPane {
		orgBase := bson.M{}
		if orgQ != "" {
			q := textfold.Fold(strings.TrimSpace(orgQ))
			hi := q + "\uffff"
			orgBase["name_ci"] = bson.M{"$gte": q, "$lt": hi}
		}
		tot, err := db.Collection("organizations").CountDocuments(ctx, orgBase)
		if err != nil {
			h.Log.Warn("count orgs", zap.Error(err))
			renderForbidden(w, r, "A database error occurred.")
			return
		}
		orgTotal = tot

		orgFilter := bson.M{}
		for k, v := range orgBase {
			orgFilter[k] = v
		}
		orgFind := options.Find()
		limit := paging.LimitPlusOne()

		if orgBefore != "" {
			if c, ok := mongodb.DecodeCursor(orgBefore); ok {
				orgFilter["$or"] = []bson.M{
					{"name_ci": bson.M{"$lt": c.CI}},
					{"name_ci": c.CI, "_id": bson.M{"$lt": c.ID}},
				}
			}
			orgFind.SetSort(bson.D{{Key: "name_ci", Value: -1}, {Key: "_id", Value: -1}}).SetLimit(limit)
		} else {
			if orgAfter != "" {
				if c, ok := mongodb.DecodeCursor(orgAfter); ok {
					orgFilter["$or"] = []bson.M{
						{"name_ci": bson.M{"$gt": c.CI}},
						{"name_ci": c.CI, "_id": bson.M{"$gt": c.ID}},
					}
				}
			}
			orgFind.SetSort(bson.D{{Key: "name_ci", Value: 1}, {Key: "_id", Value: 1}}).SetLimit(limit)
		}

		type oview struct {
			ID     primitive.ObjectID `bson:"_id"`
			Name   string             `bson:"name"`
			NameCI string             `bson:"name_ci"`
		}
		cur, err := db.Collection("organizations").Find(ctx, orgFilter, orgFind)
		if err != nil {
			h.Log.Warn("find orgs", zap.Error(err))
			renderForbidden(w, r, "A database error occurred.")
			return
		}
		defer cur.Close(ctx)

		var orows []oview
		if err := cur.All(ctx, &orows); err != nil {
			h.Log.Warn("decode orgs", zap.Error(err))
			renderForbidden(w, r, "A database error occurred.")
			return
		}

		if orgBefore != "" {
			for i, j := 0, len(orows)-1; i < j; i, j = i+1, j-1 {
				orows[i], orows[j] = orows[j], orows[i]
			}
		}

		orig := len(orows)
		if orgBefore != "" {
			if orig > paging.PageSize {
				orows = orows[1:]
				orgHasPrev = true
			}
			orgHasNext = true
		} else {
			if orig > paging.PageSize {
				orows = orows[:paging.PageSize]
				orgHasNext = true
			}
			orgHasPrev = orgAfter != ""
		}

		// Compute org range
		orgShown = len(orows)
		if orgShown > 0 {
			orgRangeEnd = orgRangeStart + orgShown - 1
		} else {
			orgRangeStart, orgRangeEnd = 0, 0
		}

		// counts per org (TOTAL members)
		orgIDs := make([]primitive.ObjectID, 0, len(orows))
		for _, o := range orows {
			orgIDs = append(orgIDs, o.ID)
		}
		byOrg := map[primitive.ObjectID]int64{}
		if len(orgIDs) > 0 {
			agg, _ := db.Collection("users").Aggregate(ctx, []bson.M{
				{"$match": bson.M{"role": "member", "organization_id": bson.M{"$in": orgIDs}}},
				{"$group": bson.M{"_id": "$organization_id", "count": bson.M{"$sum": 1}}},
			})
			if agg != nil {
				defer agg.Close(ctx)
				for agg.Next(ctx) {
					var row struct {
						ID    primitive.ObjectID `bson:"_id"`
						Count int64              `bson:"count"`
					}
					if err := agg.Decode(&row); err != nil {
						h.Log.Warn("decode org count", zap.Error(err))
						continue
					}
					byOrg[row.ID] = row.Count
				}
			}
		}

		orgRows = make([]orgRow, 0, len(orows))
		for _, o := range orows {
			orgRows = append(orgRows, orgRow{ID: o.ID, Name: o.Name, Count: byOrg[o.ID]})
		}
		if orgShown > 0 {
			orgPrevCur = mongodb.EncodeCursor(textfold.Fold(orgRows[0].Name), orgRows[0].ID)
			orgNextCur = mongodb.EncodeCursor(textfold.Fold(orgRows[orgShown-1].Name), orgRows[orgShown-1].ID)
		}
	}

	// ------- Members table -------
	qFold := textfold.Fold(strings.TrimSpace(searchQuery))
	hiFold := qFold + "\uffff"
	sLower := strings.ToLower(strings.TrimSpace(searchQuery))
	hiEmail := sLower + "\uffff"

	// Email-pivot when searching by email and org+status are constrained
	emailPivot := search.EmailPivotOK(searchQuery, status, scopeOrg != nil)

	pbase := bson.M{"role": "member"}
	if status == "active" || status == "disabled" {
		pbase["status"] = status
	}
	if scopeOrg != nil {
		pbase["organization_id"] = *scopeOrg
	}
	var searchOr []bson.M
	if searchQuery != "" {
		if emailPivot {
			searchOr = []bson.M{{"email": bson.M{"$gte": sLower, "$lt": hiEmail}}}
		} else {
			searchOr = []bson.M{
				{"full_name_ci": bson.M{"$gte": qFold, "$lt": hiFold}},
				{"email": bson.M{"$gte": sLower, "$lt": hiEmail}},
			}
		}
		pbase["$or"] = searchOr
	}

	total, err := db.Collection("users").CountDocuments(ctx, pbase)
	if err != nil {
		h.Log.Warn("count users", zap.Error(err))
		renderForbidden(w, r, "A database error occurred.")
		return
	}

	f := bson.M{}
	for k, v := range pbase {
		f[k] = v
	}
	find := options.Find()
	limit := paging.LimitPlusOne()
	sortField := "full_name_ci"
	if emailPivot {
		sortField = "email"
	}

	applyKeyset := func(field, direction, key string, id primitive.ObjectID) {
		ks := mongodb.KeysetWindow(field, direction, key, id)
		if searchQuery != "" {
			f["$and"] = []bson.M{{"$or": searchOr}, ks}
			delete(f, "$or")
		} else {
			for k, v := range ks {
				f[k] = v
			}
		}
	}

	if before != "" {
		if c, ok := mongodb.DecodeCursor(before); ok {
			applyKeyset(sortField, "lt", c.CI, c.ID)
		}
		find.SetSort(bson.D{{Key: sortField, Value: -1}, {Key: "_id", Value: -1}}).SetLimit(limit)
	} else {
		if after != "" {
			if c, ok := mongodb.DecodeCursor(after); ok {
				applyKeyset(sortField, "gt", c.CI, c.ID)
			}
		}
		find.SetSort(bson.D{{Key: sortField, Value: 1}, {Key: "_id", Value: 1}}).SetLimit(limit)
	}

	type urow struct {
		ID             primitive.ObjectID  `bson:"_id"`
		FullName       string              `bson:"full_name"`
		FullNameCI     string              `bson:"full_name_ci"`
		Email          string              `bson:"email"`
		Status         string              `bson:"status"`
		OrganizationID *primitive.ObjectID `bson:"organization_id"`
	}
	ucur, err := db.Collection("users").Find(ctx, f, find)
	if err != nil {
		h.Log.Warn("find users", zap.Error(err))
		renderForbidden(w, r, "A database error occurred.")
		return
	}
	defer ucur.Close(ctx)

	var urows []urow
	if err := ucur.All(ctx, &urows); err != nil {
		h.Log.Warn("decode users", zap.Error(err))
		renderForbidden(w, r, "A database error occurred.")
		return
	}

	if before != "" {
		for i, j := 0, len(urows)-1; i < j; i, j = i+1, j-1 {
			urows[i], urows[j] = urows[j], urows[i]
		}
	}

	orig := len(urows)
	hasPrev, hasNext := false, false
	if before != "" {
		if orig > paging.PageSize {
			urows = urows[1:]
			hasPrev = true
		}
		hasNext = true
	} else {
		if orig > paging.PageSize {
			urows = urows[:paging.PageSize]
			hasNext = true
		}
		hasPrev = after != ""
	}
	shown := len(urows)

	// Compute members' range and next/prev start values
	rangeStart := 0
	rangeEnd := 0
	if shown > 0 {
		rangeStart = start
		rangeEnd = start + shown - 1
	}
	prevStart := start - paging.PageSize
	if prevStart < 1 {
		prevStart = 1
	}
	nextStart := start + shown

	// Map org IDs to names for the table
	orgIDs := make([]primitive.ObjectID, 0, shown)
	for _, r := range urows {
		if r.OrganizationID != nil {
			orgIDs = append(orgIDs, *r.OrganizationID)
		}
	}
	orgNames := map[primitive.ObjectID]string{}
	if len(orgIDs) > 0 {
		oc2, _ := db.Collection("organizations").Find(ctx, bson.M{"_id": bson.M{"$in": orgIDs}})
		defer oc2.Close(ctx)
		for oc2.Next(ctx) {
			var o models.Organization
			if err := oc2.Decode(&o); err != nil {
				h.Log.Warn("decode org failed", zap.Error(err))
				continue
			}
			orgNames[o.ID] = o.Name
		}
	}

	rows := make([]memberRow, 0, shown)
	for _, r := range urows {
		on := ""
		if r.OrganizationID != nil {
			on = orgNames[*r.OrganizationID]
		}
		rows = append(rows, memberRow{
			ID:       r.ID,
			FullName: r.FullName,
			Email:    strings.ToLower(r.Email),
			OrgName:  on,
			Status:   r.Status,
		})
	}

	prevCur, nextCur := "", ""
	if shown > 0 {
		firstKey := urows[0].FullNameCI
		lastKey := urows[shown-1].FullNameCI
		if emailPivot {
			firstKey = strings.ToLower(urows[0].Email)
			lastKey = strings.ToLower(urows[shown-1].Email)
		}
		prevCur = mongodb.EncodeCursor(firstKey, urows[0].ID)
		nextCur = mongodb.EncodeCursor(lastKey, urows[shown-1].ID)
	}

	// total members (All row)
	var all int64
	if showOrgPane {
		acur, _ := db.Collection("users").CountDocuments(ctx, bson.M{"role": "member"})
		all = acur
	}

	templates.RenderAutoMap(w, r, "members_list", nil, listData{
		Title:      "Members",
		IsLoggedIn: true,
		Role:       role,
		UserName:   uname,

		// Orgs pane
		ShowOrgPane:   showOrgPane,
		OrgQuery:      orgQ,
		OrgShown:      orgShown,
		OrgTotal:      orgTotal,
		OrgHasPrev:    orgHasPrev,
		OrgHasNext:    orgHasNext,
		OrgPrevCur:    orgPrevCur,
		OrgNextCur:    orgNextCur,
		OrgRows:       orgRows,
		SelectedOrg:   selectedOrg,
		AllCount:      all,
		OrgRangeStart: orgRangeStart,
		OrgRangeEnd:   orgRangeEnd,

		// Members filter
		SearchQuery: searchQuery,
		Status:      status,

		// Members counts + range
		Shown:      shown,
		Total:      total,
		RangeStart: rangeStart,
		RangeEnd:   rangeEnd,

		// Members keyset cursors + page-index starts
		HasPrev:    hasPrev,
		HasNext:    hasNext,
		PrevCursor: prevCur,
		NextCursor: nextCur,
		PrevStart:  prevStart,
		NextStart:  nextStart,

		MemberRows: rows,

		AllowUpload: (role == "admin" && selectedOrg != "all") || role == "leader",
		AllowAdd:    true,

		BackURL:     nav.ResolveBackURL(r, "/members"),
		CurrentPath: nav.CurrentPath(r),
	})
}

/* ---------- UI error helpers (use shared error templates) ---------- */

func renderUnauthorized(w http.ResponseWriter, r *http.Request) {
	role, name, _, signed := userCtx(r)
	data := struct {
		Title      string
		IsLoggedIn bool
		Role       string
		UserName   string
		Message    string
		BackURL    string
	}{
		Title:      "Sign in required",
		IsLoggedIn: signed,
		Role:       role,
		UserName:   name,
		Message:    "Please sign in to continue.",
		BackURL:    "/login",
	}
	templates.Render(w, r, "error_unauthorized", data)
}

func renderForbidden(w http.ResponseWriter, r *http.Request, msg string) {
	role, name, _, signed := userCtx(r)
	data := struct {
		Title      string
		IsLoggedIn bool
		Role       string
		UserName   string
		Message    string
		BackURL    string
	}{
		Title:      "Access denied",
		IsLoggedIn: signed,
		Role:       role,
		UserName:   name,
		Message:    msg,
		BackURL:    nav.ResolveBackURL(r, "/members"),
	}
	templates.Render(w, r, "error_forbidden", data)
}

// ServeManageMemberModal renders the HTMX modal to manage a single member:
// View / Edit / Delete.
func (h *Handler) ServeManageMemberModal(w http.ResponseWriter, r *http.Request) {
	// Require a signed-in user; you can tighten this to admin/leader if desired.
	role, _, _, ok := userCtx(r)
	if !ok {
		renderUnauthorized(w, r)
		return
	}
	if role != "admin" && role != "leader" {
		renderForbidden(w, r, "You do not have access to manage members.")
		return
	}

	idHex := chi.URLParam(r, "id")
	memberID, err := primitive.ObjectIDFromHex(idHex)
	if err != nil {
		http.Error(w, "bad member id", http.StatusBadRequest)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), membersShortTimeout)
	defer cancel()
	db := h.DB

	// Load the member
	var u models.User
	if err := db.Collection("users").
		FindOne(ctx, bson.M{"_id": memberID, "role": "member"}).
		Decode(&u); err != nil {

		http.NotFound(w, r)
		return
	}

	// Resolve organization name if present
	orgName := ""
	if u.OrganizationID != nil {
		var org models.Organization
		if err := db.Collection("organizations").
			FindOne(ctx, bson.M{"_id": *u.OrganizationID}).
			Decode(&org); err == nil {
			orgName = org.Name
		}
	}

	back := r.URL.Query().Get("return")
	if back == "" {
		back = nav.ResolveBackURL(r, "/members")
	}

	data := memberManageModalData{
		MemberID: u.ID.Hex(),
		FullName: u.FullName,
		Email:    strings.ToLower(u.Email),
		OrgName:  orgName,
		BackURL:  back,
	}

	// Render the modal snippet
	templates.RenderSnippet(w, "members_manage_member_modal", data)
}

// trivial helpers retained for non-UI code paths
func httpError(w http.ResponseWriter, msg string, code int) { http.Error(w, msg, code) }
func httpIfErr(w http.ResponseWriter, err error, code int) bool {
	if err != nil {
		http.Error(w, "db error", code)
		return true
	}
	return false
}
