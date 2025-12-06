// internal/app/features/groups/manage.go
package groups

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strings"

	"github.com/dalemusser/stratahub/internal/app/policy/grouppolicy"
	groupstore "github.com/dalemusser/stratahub/internal/app/store/groups"
	membershipstore "github.com/dalemusser/stratahub/internal/app/store/memberships"
	"github.com/dalemusser/stratahub/internal/app/system/authz"
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
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// ManagePageData holds the full view model for the Manage Group page.
type ManagePageData struct {
	// layout header
	Title      string
	IsLoggedIn bool
	Role       string
	UserName   string

	GroupID          string
	GroupName        string
	GroupDescription string
	OrganizationName string

	CurrentLeaders  []UserItem
	CurrentMembers  []UserItem
	PossibleLeaders []UserItem

	AvailableMembers []UserItem
	AvailableShown   int
	AvailableTotal   int64

	Query         string
	CurrentAfter  string
	CurrentBefore string
	NextCursor    string
	PrevCursor    string
	HasNext       bool
	HasPrev       bool

	// Navigation niceties
	BackURL     string // where "Back" should go
	CurrentPath string // this page's path + query (used to propagate ?return=)
}

// UserItem is a simple view-model for a user row.
type UserItem struct {
	ID       string
	FullName string
	Email    string
}

// ServeManageGroup renders the main Manage Group page.
func (h *Handler) ServeManageGroup(w http.ResponseWriter, r *http.Request) {
	gid := chi.URLParam(r, "id")

	groupOID, err := primitive.ObjectIDFromHex(gid)
	if err != nil {
		http.Error(w, "bad group id", http.StatusBadRequest)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), metaShortTimeout)
	defer cancel()
	db := h.DB

	grpStore := groupstore.New(db)
	grp, err := grpStore.GetByID(ctx, groupOID)
	if err == mongo.ErrNoDocuments {
		http.NotFound(w, r)
		return
	}
	if err != nil {
		http.Error(w, "db error", http.StatusInternalServerError)
		return
	}
	if !grouppolicy.CanManageGroup(ctx, db, r, grp.ID) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}

	data, err := h.buildPageData(r, gid, "", "", "")
	if err != nil {
		http.Error(w, "build error", http.StatusInternalServerError)
		return
	}

	templates.Render(w, r, "group_manage", data)
}

// HandleAddLeader adds a leader to the group.
func (h *Handler) HandleAddLeader(w http.ResponseWriter, r *http.Request) {
	gid := chi.URLParam(r, "id")
	targetHex := r.FormValue("userID")

	ctx, cancel := context.WithTimeout(r.Context(), metaMedTimeout)
	defer cancel()
	db := h.DB

	groupOID, _ := primitive.ObjectIDFromHex(gid)

	group, err := groupstore.New(db).GetByID(ctx, groupOID)
	if err == mongo.ErrNoDocuments {
		http.NotFound(w, r)
		return
	}
	if err != nil {
		http.Error(w, "db error", http.StatusInternalServerError)
		return
	}
	if !grouppolicy.CanManageGroup(ctx, db, r, group.ID) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}

	targetOID, _ := primitive.ObjectIDFromHex(targetHex)
	var u models.User
	if err := db.Collection("users").
		FindOne(ctx, bson.M{"_id": targetOID, "role": "leader", "organization_id": group.OrganizationID}).
		Decode(&u); err != nil {
		http.Error(w, "leader must be from same org", http.StatusBadRequest)
		return
	}

	if err := membershipstore.New(db).Add(ctx, group.ID, targetOID, "leader"); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	h.renderLeadersPartial(w, r, gid)
}

// HandleRemoveLeader removes a leader from the group.
func (h *Handler) HandleRemoveLeader(w http.ResponseWriter, r *http.Request) {
	_, _, uid, ok := authz.UserCtx(r)
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	gid := chi.URLParam(r, "id")
	targetHex := r.FormValue("userID")

	ctx, cancel := context.WithTimeout(r.Context(), metaMedTimeout)
	defer cancel()
	db := h.DB

	groupOID, _ := primitive.ObjectIDFromHex(gid)

	group, err := groupstore.New(db).GetByID(ctx, groupOID)
	if err == mongo.ErrNoDocuments {
		http.NotFound(w, r)
		return
	}
	if err != nil {
		http.Error(w, "db error", http.StatusInternalServerError)
		return
	}
	if !grouppolicy.CanManageGroup(ctx, db, r, group.ID) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}

	targetOID, _ := primitive.ObjectIDFromHex(targetHex)
	if uid == targetOID {
		http.Error(w, "cannot remove yourself as leader", http.StatusBadRequest)
		return
	}

	cnt, _ := db.Collection("group_memberships").
		CountDocuments(ctx, bson.M{"group_id": group.ID, "role": "leader"})
	if cnt <= 1 {
		http.Error(w, "cannot remove last leader", http.StatusBadRequest)
		return
	}

	if err := membershipstore.New(db).Remove(ctx, group.ID, targetOID); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	h.renderLeadersPartial(w, r, gid)
}

// HandleAddMember adds a member to the group (via search list).
func (h *Handler) HandleAddMember(w http.ResponseWriter, r *http.Request) {
	gid := chi.URLParam(r, "id")
	q := r.FormValue("q")
	after := r.FormValue("after")
	before := r.FormValue("before")

	groupOID, err := primitive.ObjectIDFromHex(gid)
	if err != nil {
		http.Error(w, "bad group id", http.StatusBadRequest)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), metaMedTimeout)
	defer cancel()
	db := h.DB

	group, err := groupstore.New(db).GetByID(ctx, groupOID)
	if err == mongo.ErrNoDocuments {
		http.NotFound(w, r)
		return
	}
	if err != nil {
		http.Error(w, "db error", http.StatusInternalServerError)
		return
	}
	if !grouppolicy.CanManageGroup(ctx, db, r, group.ID) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}

	targetOID, err := primitive.ObjectIDFromHex(r.FormValue("userID"))
	if err != nil {
		http.Error(w, "bad user id", http.StatusBadRequest)
		return
	}

	var u struct {
		ID             primitive.ObjectID `bson:"_id"`
		OrganizationID primitive.ObjectID `bson:"organization_id"`
		Role           string             `bson:"role"`
		Status         string             `bson:"status"`
	}
	if err := db.Collection("users").FindOne(ctx, bson.M{
		"_id":             targetOID,
		"organization_id": group.OrganizationID,
		"role":            "member",
		"status":          "active",
	}).Decode(&u); err != nil {
		http.Error(w, "member must exist in same organization and be active", http.StatusBadRequest)
		return
	}

	if err := membershipstore.New(db).Add(ctx, group.ID, targetOID, "member"); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	data, err := h.buildPageData(r, gid, q, after, before)
	if err != nil {
		http.Error(w, "build error", http.StatusInternalServerError)
		return
	}

	// If the current page is now empty, adjust paging backwards or to first page.
	if data.AvailableShown == 0 {
		if after != "" {
			if p, shown, total, nextCur, prevCur, hasNext, hasPrev, err2 :=
				h.fetchAvailablePrevInclusive(ctx, groupOID, q, after); err2 == nil {

				data.AvailableMembers = p
				data.AvailableShown = shown
				data.AvailableTotal = total
				data.NextCursor = nextCur
				data.PrevCursor = prevCur
				data.HasNext = hasNext
				data.HasPrev = hasPrev
				if !hasPrev {
					data.CurrentAfter, data.CurrentBefore = "", ""
				} else {
					data.CurrentAfter, data.CurrentBefore = "", prevCur
				}
			} else if d2, err3 := h.buildPageData(r, gid, q, "", ""); err3 == nil {
				data = d2
			}
		} else if d2, err2 := h.buildPageData(r, gid, q, "", ""); err2 == nil {
			data = d2
		}
	}

	// Re-render the members and available lists.
	templates.RenderSnippet(w, "group_members_list_inner", data)
	templates.RenderSnippet(w, "group_members_header_oob", data)
	templates.RenderSnippet(w, "group_available_members_block_oob", data)
}

// HandleRemoveMember removes a member from the group.
func (h *Handler) HandleRemoveMember(w http.ResponseWriter, r *http.Request) {
	gid := chi.URLParam(r, "id")
	q := r.FormValue("q")
	after := r.FormValue("after")
	before := r.FormValue("before")

	ctx, cancel := context.WithTimeout(r.Context(), metaMedTimeout)
	defer cancel()
	db := h.DB

	groupOID, _ := primitive.ObjectIDFromHex(gid)

	group, err := groupstore.New(db).GetByID(ctx, groupOID)
	if err != nil {
		http.Error(w, "db error", http.StatusInternalServerError)
		return
	}
	if !grouppolicy.CanManageGroup(ctx, db, r, group.ID) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}

	targetHex := r.FormValue("userID")
	targetOID, _ := primitive.ObjectIDFromHex(targetHex)
	if err := membershipstore.New(db).Remove(ctx, group.ID, targetOID); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	data, err := h.buildPageData(r, gid, q, after, before)
	if err != nil {
		http.Error(w, "build error", http.StatusInternalServerError)
		return
	}

	// guard stale before anchor -> first page
	if before != "" && !data.HasPrev {
		if d2, err2 := h.buildPageData(r, gid, q, "", ""); err2 == nil {
			data = d2
		}
	}

	templates.RenderSnippet(w, "group_members_list_inner", data)
	templates.RenderSnippet(w, "group_members_header_oob", data)
	templates.RenderSnippet(w, "group_available_members_block_oob", data)

	// Also emit a "recently removed" chip snippet if we can load the user.
	var u models.User
	if err := db.Collection("users").FindOne(ctx, bson.M{"_id": targetOID}).Decode(&u); err == nil {
		chip := struct {
			FullName string
			GroupID  string
			UserID   string
		}{
			FullName: u.FullName,
			GroupID:  gid,
			UserID:   targetOID.Hex(),
		}
		templates.RenderSnippet(w, "group_recent_chip_oob", chip)
	}
}

// HandleSearchMembers handles HTMX search + paging for available members.
func (h *Handler) HandleSearchMembers(w http.ResponseWriter, r *http.Request) {
	gid := chi.URLParam(r, "id")
	q := r.URL.Query().Get("q")
	after := r.URL.Query().Get("after")
	before := r.URL.Query().Get("before")

	data, err := h.buildPageData(r, gid, q, after, before)
	if err != nil {
		http.Error(w, "build error", http.StatusForbidden)
		return
	}

	// If we paged backwards and there's no previous page, snap to first page and update the URL.
	if before != "" && !data.HasPrev {
		if d2, err2 := h.buildPageData(r, gid, q, "", ""); err2 == nil {
			data = d2
		}
		base := "/groups/" + gid + "/manage/search-members"
		v := url.Values{}
		if q != "" {
			v.Set("q", q)
		}
		if ret := r.URL.Query().Get("return"); ret != "" {
			v.Set("return", ret)
		}
		if enc := v.Encode(); enc != "" {
			base += "?" + enc
		}
		w.Header().Set("HX-Push-Url", base)
	}

	templates.RenderSnippet(w, "group_available_members_block", data)
}

// buildPageData assembles the ManagePageData for a given group and search window.
func (h *Handler) buildPageData(r *http.Request, gid, q, after, before string) (ManagePageData, error) {
	role, uname, _, ok := authz.UserCtx(r)
	if !ok {
		return ManagePageData{}, fmt.Errorf("unauthorized")
	}

	groupOID, err := primitive.ObjectIDFromHex(gid)
	if err != nil {
		return ManagePageData{}, fmt.Errorf("invalid id")
	}

	ctx, cancel := context.WithTimeout(r.Context(), metaMedTimeout)
	defer cancel()
	db := h.DB

	group, err := groupstore.New(db).GetByID(ctx, groupOID)
	if err != nil {
		return ManagePageData{}, fmt.Errorf("not found")
	}

	usersColl := db.Collection("users")

	leadIDs := h.fetchMemberIDs(ctx, db, group.ID, "leader")
	memberIDs := h.fetchMemberIDs(ctx, db, group.ID, "member")

	currentLeads := h.fetchUserItemsByIDs(ctx, usersColl, leadIDs)
	currentMembers := h.fetchUserItemsByIDs(ctx, usersColl, memberIDs)

	// Possible leaders: active leaders in org not already in leads.
	leadFilter := bson.M{
		"organization_id": group.OrganizationID,
		"role":            "leader",
		"status":          "active",
	}
	if len(leadIDs) > 0 {
		leadFilter["_id"] = bson.M{"$nin": leadIDs}
	}
	possibleLeads := h.fetchUserItems(ctx, usersColl, leadFilter)

	sortUsers := func(s []UserItem) {
		sort.SliceStable(s, func(i, j int) bool {
			ni, nj := strings.ToLower(s[i].FullName), strings.ToLower(s[j].FullName)
			if ni == nj {
				if s[i].FullName == s[j].FullName {
					return s[i].Email < s[j].Email
				}
				return s[i].FullName < s[j].FullName
			}
			return ni < nj
		})
	}

	sortUsers(currentLeads)
	sortUsers(currentMembers)
	sortUsers(possibleLeads)

	avail, shown, total, nextCur, prevCur, hasNext, hasPrev, err :=
		h.fetchAvailablePaged(ctx, group.OrganizationID, group.ID, q, after, before)
	if err != nil {
		return ManagePageData{}, err
	}

	var orgName string
	{
		var o models.Organization
		_ = db.Collection("organizations").
			FindOne(ctx, bson.M{"_id": group.OrganizationID}).
			Decode(&o)
		orgName = o.Name
	}

	return ManagePageData{
		Title:            "Manage Group",
		IsLoggedIn:       true,
		Role:             role,
		UserName:         uname,
		GroupID:          group.ID.Hex(),
		GroupName:        group.Name,
		GroupDescription: group.Description,
		OrganizationName: orgName,
		CurrentLeaders:   currentLeads,
		CurrentMembers:   currentMembers,
		PossibleLeaders:  possibleLeads,
		AvailableMembers: avail,
		AvailableShown:   shown,
		AvailableTotal:   total,
		Query:            q,
		CurrentAfter:     after,
		CurrentBefore:    before,
		NextCursor:       nextCur,
		PrevCursor:       prevCur,
		HasNext:          hasNext,
		HasPrev:          hasPrev,
		BackURL:          nav.ResolveBackURL(r, "/groups"),
		CurrentPath:      nav.CurrentPath(r),
	}, nil
}

// fetchAvailablePaged returns a page of available members (not yet in group),
// with email-pivot logic for search.
func (h *Handler) fetchAvailablePaged(
	ctx context.Context,
	orgOID, groupOID primitive.ObjectID,
	qRaw, after, before string,
) (members []UserItem, shown int, total int64, nextCursor, prevCursor string, hasNext, hasPrev bool, err error) {

	db := h.DB
	users := db.Collection("users")

	memberIDs := h.fetchMemberIDs(ctx, db, groupOID, "member")

	filter := bson.M{
		"organization_id": orgOID,
		"role":            "member",
		"status":          "active",
	}
	if len(memberIDs) > 0 {
		filter["_id"] = bson.M{"$nin": memberIDs}
	}

	q := textfold.Fold(qRaw)
	status := "active"
	hasOrg := true

	emailPivot := search.EmailPivotOK(qRaw, status, hasOrg)

	if q != "" {
		sName := q
		hiName := sName + "\uffff"
		sEmail := strings.ToLower(strings.TrimSpace(qRaw))
		hiEmail := sEmail + "\uffff"

		if emailPivot {
			filter["$or"] = []bson.M{
				{"email": bson.M{"$gte": sEmail, "$lt": hiEmail}},
			}
		} else {
			filter["$or"] = []bson.M{
				{"full_name_ci": bson.M{"$gte": sName, "$lt": hiName}},
				{"email": bson.M{"$gte": sEmail, "$lt": hiEmail}},
			}
		}
	}

	total, _ = users.CountDocuments(ctx, filter)

	findOpts := options.Find()
	limit := paging.LimitPlusOne()
	sortField := "full_name_ci"
	if emailPivot {
		sortField = "email"
	}

	applyWin := func(dir, key string, id primitive.ObjectID) {
		ks := mongodb.KeysetWindow(sortField, dir, key, id)
		if q != "" && filter["$or"] != nil {
			filter["$and"] = []bson.M{{"$or": filter["$or"].([]bson.M)}, ks}
			delete(filter, "$or")
		} else {
			for k, v := range ks {
				filter[k] = v
			}
		}
	}

	if before != "" {
		if c, ok := mongodb.DecodeCursor(before); ok {
			applyWin("lt", c.CI, c.ID)
		}
		findOpts.SetSort(bson.D{{Key: sortField, Value: -1}, {Key: "_id", Value: -1}}).SetLimit(limit)
	} else {
		if after != "" {
			if c, ok := mongodb.DecodeCursor(after); ok {
				applyWin("gt", c.CI, c.ID)
			}
		}
		findOpts.SetSort(bson.D{{Key: sortField, Value: 1}, {Key: "_id", Value: 1}}).SetLimit(limit)
	}

	cur, e := users.Find(ctx, filter, findOpts)
	if e != nil {
		err = e
		return
	}
	defer cur.Close(ctx)

	var rows []struct {
		ID         primitive.ObjectID `bson:"_id"`
		FullName   string             `bson:"full_name"`
		Email      string             `bson:"email"`
		FullNameCI string             `bson:"full_name_ci"`
		OrgID      primitive.ObjectID `bson:"organization_id"`
	}
	if err = cur.All(ctx, &rows); err != nil {
		return
	}

	if before != "" {
		for i, j := 0, len(rows)-1; i < j; i, j = i+1, j-1 {
			rows[i], rows[j] = rows[j], rows[i]
		}
	}

	orig := len(rows)
	if before != "" {
		if orig > paging.PageSize {
			rows = rows[1:]
			hasPrev = true
		} else {
			hasPrev = false
		}
		hasNext = true
	} else {
		if orig > paging.PageSize {
			rows = rows[:paging.PageSize]
			hasNext = true
		} else {
			hasNext = false
		}
		hasPrev = after != ""
	}

	members = make([]UserItem, 0, len(rows))
	for _, r := range rows {
		members = append(members, UserItem{
			ID:       r.ID.Hex(),
			FullName: r.FullName,
			Email:    r.Email,
		})
	}
	shown = len(members)
	if shown > 0 {
		first := rows[0]
		last := rows[shown-1]
		prevCursor = mongodb.EncodeCursor(first.FullNameCI, first.ID)
		nextCursor = mongodb.EncodeCursor(last.FullNameCI, last.ID)
	}

	return
}

// fetchAvailablePrevInclusive is used when paging backwards after removals
// and we need to pull a page that includes the anchor row.
func (h *Handler) fetchAvailablePrevInclusive(
	ctx context.Context,
	groupOID primitive.ObjectID,
	qRaw, after string,
) (members []UserItem, shown int, total int64, nextCursor, prevCursor string, hasNext, hasPrev bool, err error) {

	db := h.DB
	users := db.Collection("users")
	groupsColl := db.Collection("groups")

	var grp models.Group
	if err = groupsColl.FindOne(ctx, bson.M{"_id": groupOID}).Decode(&grp); err != nil {
		return
	}
	orgOID := grp.OrganizationID

	memberIDs := h.fetchMemberIDs(ctx, db, groupOID, "member")

	filter := bson.M{
		"organization_id": orgOID,
		"role":            "member",
		"status":          "active",
	}
	if len(memberIDs) > 0 {
		filter["_id"] = bson.M{"$nin": memberIDs}
	}

	total, _ = users.CountDocuments(ctx, filter)

	q := textfold.Fold(qRaw)
	if q != "" {
		high := q + "\uffff"
		filter["full_name_ci"] = bson.M{"$gte": q, "$lt": high}
	}

	anchor, ok := mongodb.DecodeCursor(after)
	if !ok {
		cur, e := users.Find(ctx, filter,
			options.Find().SetSort(bson.D{{Key: "full_name_ci", Value: 1}, {Key: "_id", Value: 1}}).
				SetLimit(int64(paging.PageSize)))
		if e != nil {
			err = e
			return
		}
		defer cur.Close(ctx)

		var rows []struct {
			ID         primitive.ObjectID `bson:"_id"`
			FullName   string             `bson:"full_name"`
			Email      string             `bson:"email"`
			FullNameCI string             `bson:"full_name_ci"`
		}
		if err = cur.All(ctx, &rows); err != nil {
			return
		}

		members = make([]UserItem, 0, len(rows))
		for _, r := range rows {
			members = append(members, UserItem{
				ID:       r.ID.Hex(),
				FullName: r.FullName,
				Email:    r.Email,
			})
		}
		shown = len(members)
		if shown > 0 {
			prevCursor = mongodb.EncodeCursor(rows[0].FullNameCI, rows[0].ID)
			nextCursor = mongodb.EncodeCursor(rows[shown-1].FullNameCI, rows[shown-1].ID)

			last := rows[shown-1]
			fwd := bson.M{
				"organization_id": orgOID,
				"role":            "member",
				"$or": []bson.M{
					{"full_name_ci": bson.M{"$gt": last.FullNameCI}},
					{"full_name_ci": last.FullNameCI, "_id": bson.M{"$gt": last.ID}},
				},
			}
			if len(memberIDs) > 0 {
				fwd["_id"] = bson.M{"$nin": memberIDs}
			}
			fc, _ := users.Find(ctx, fwd,
				options.Find().SetSort(bson.D{{Key: "full_name_ci", Value: 1}, {Key: "_id", Value: 1}}).
					SetLimit(1))
			defer fc.Close(ctx)
			hasNext = fc.Next(ctx)
		}
		hasPrev = false
		return
	}

	// Backwards from anchor (inclusive).
	filter["$or"] = []bson.M{
		{"full_name_ci": bson.M{"$lt": anchor.CI}},
		{"full_name_ci": anchor.CI, "_id": bson.M{"$lte": anchor.ID}},
	}

	cur, e := users.Find(ctx, filter,
		options.Find().SetSort(bson.D{{Key: "full_name_ci", Value: -1}, {Key: "_id", Value: -1}}).
			SetLimit(paging.LimitPlusOne()))
	if e != nil {
		err = e
		return
	}
	defer cur.Close(ctx)

	var rowsDesc []struct {
		ID         primitive.ObjectID `bson:"_id"`
		FullName   string             `bson:"full_name"`
		Email      string             `bson:"email"`
		FullNameCI string             `bson:"full_name_ci"`
	}
	if err = cur.All(ctx, &rowsDesc); err != nil {
		return
	}

	for i, j := 0, len(rowsDesc)-1; i < j; i, j = i+1, j-1 {
		rowsDesc[i], rowsDesc[j] = rowsDesc[j], rowsDesc[i]
	}

	if len(rowsDesc) > paging.PageSize {
		rowsDesc = rowsDesc[1:]
		hasPrev = true
	} else {
		hasPrev = false
	}

	members = make([]UserItem, 0, len(rowsDesc))
	for _, r := range rowsDesc {
		members = append(members, UserItem{
			ID:       r.ID.Hex(),
			FullName: r.FullName,
			Email:    r.Email,
		})
	}
	shown = len(members)
	if shown > 0 {
		first := rowsDesc[0]
		last := rowsDesc[shown-1]
		prevCursor = mongodb.EncodeCursor(first.FullNameCI, first.ID)
		nextCursor = mongodb.EncodeCursor(last.FullNameCI, last.ID)

		fwd := bson.M{
			"organization_id": orgOID,
			"role":            "member",
			"$or": []bson.M{
				{"full_name_ci": bson.M{"$gt": last.FullNameCI}},
				{"full_name_ci": last.FullNameCI, "_id": bson.M{"$gt": last.ID}},
			},
		}
		if len(memberIDs) > 0 {
			fwd["_id"] = bson.M{"$nin": memberIDs}
		}
		fc, _ := users.Find(ctx, fwd,
			options.Find().SetSort(bson.D{{Key: "full_name_ci", Value: 1}, {Key: "_id", Value: 1}}).
				SetLimit(1))
		defer fc.Close(ctx)
		hasNext = fc.Next(ctx)
	} else {
		hasNext = false
	}

	return
}

// fetchMemberIDs returns all user IDs in group_memberships for a given group/role.
func (h *Handler) fetchMemberIDs(ctx context.Context, db *mongo.Database, groupID primitive.ObjectID, role string) []primitive.ObjectID {
	cur, _ := db.Collection("group_memberships").Find(
		ctx,
		bson.M{"group_id": groupID, "role": role},
		options.Find().SetProjection(bson.M{"user_id": 1}),
	)
	defer cur.Close(ctx)

	var ids []primitive.ObjectID
	for cur.Next(ctx) {
		var row struct {
			UserID primitive.ObjectID `bson:"user_id"`
		}
		if cur.Decode(&row) == nil {
			ids = append(ids, row.UserID)
		}
	}
	return ids
}

// renderLeadersPartial re-renders just the leaders block as a snippet.
func (h *Handler) renderLeadersPartial(w http.ResponseWriter, r *http.Request, gid string) {
	data, err := h.buildPageData(r, gid, "", "", "")
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	templates.RenderSnippet(w, "group_leaders_contents", data)
}

// fetchUserItemsByIDs returns basic user info for a set of IDs.
func (h *Handler) fetchUserItemsByIDs(ctx context.Context, col *mongo.Collection, ids []primitive.ObjectID) []UserItem {
	if len(ids) == 0 {
		return nil
	}
	cur, _ := col.Find(
		ctx,
		bson.M{"_id": bson.M{"$in": ids}},
		options.Find().SetProjection(bson.M{"full_name": 1, "email": 1}),
	)
	defer cur.Close(ctx)

	var users []struct {
		ID       primitive.ObjectID `bson:"_id"`
		FullName string             `bson:"full_name"`
		Email    string             `bson:"email"`
	}
	_ = cur.All(ctx, &users)

	out := make([]UserItem, len(users))
	for i, u := range users {
		out[i] = UserItem{
			ID:       u.ID.Hex(),
			FullName: u.FullName,
			Email:    u.Email,
		}
	}
	return out
}

// fetchUserItems returns basic user info matching a filter.
func (h *Handler) fetchUserItems(ctx context.Context, col *mongo.Collection, filter bson.M) []UserItem {
	cur, _ := col.Find(
		ctx,
		filter,
		options.Find().SetProjection(bson.M{"_id": 1, "full_name": 1, "email": 1}),
	)
	defer cur.Close(ctx)

	var users []models.User
	_ = cur.All(ctx, &users)

	out := make([]UserItem, len(users))
	for i, u := range users {
		out[i] = UserItem{
			ID:       u.ID.Hex(),
			FullName: u.FullName,
			Email:    u.Email,
		}
	}
	return out
}
