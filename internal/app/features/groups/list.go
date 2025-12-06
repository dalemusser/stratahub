// internal/app/features/groups/list.go
package groups

import (
	"context"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/dalemusser/stratahub/internal/app/system/authz"
	"github.com/dalemusser/stratahub/internal/app/system/paging"
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
	"go.uber.org/zap"
)

const (
	listTimeout      = 10 * time.Second
	listShortTimeout = 5 * time.Second
)

// View model for the group manage modal snippet.
type groupManageModalData struct {
	GroupID          string
	GroupName        string
	OrganizationName string
	BackURL          string
}

type orgRow struct {
	ID    primitive.ObjectID
	Name  string
	Count int64
}

type groupListItem struct {
	ID                     primitive.ObjectID
	Name                   string
	OrganizationName       string
	LeadersCount           int
	MembersCount           int
	AssignedResourcesCount int
}

type groupListData struct {
	Title, SearchQuery string
	IsLoggedIn         bool
	Role, UserName     string

	// Left org pane (admins only)
	ShowOrgPane   bool
	OrgQuery      string
	OrgShown      int
	OrgTotal      int64
	OrgHasPrev    bool
	OrgHasNext    bool
	OrgPrevCur    string
	OrgNextCur    string
	OrgRangeStart int
	OrgRangeEnd   int
	SelectedOrg   string // "all" or hex
	OrgRows       []orgRow
	AllCount      int64

	// Right table
	Shown      int
	Total      int64
	HasPrev    bool
	HasNext    bool
	PrevCursor string
	NextCursor string
	Groups     []groupListItem

	// groups range + page-index starts
	RangeStart int
	RangeEnd   int
	PrevStart  int
	NextStart  int

	CurrentPath string
}

// andify composes clauses into a single bson.M with optional $and.
func andify(clauses []bson.M) bson.M {
	switch len(clauses) {
	case 0:
		return bson.M{}
	case 1:
		return clauses[0]
	default:
		return bson.M{"$and": clauses}
	}
}

// dedupObjectIDs removes duplicates while preserving order.
func dedupObjectIDs(in []primitive.ObjectID) []primitive.ObjectID {
	seen := make(map[primitive.ObjectID]struct{}, len(in))
	out := make([]primitive.ObjectID, 0, len(in))
	for _, id := range in {
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		out = append(out, id)
	}
	return out
}

// ServeGroupsList handles GET /groups (relative to mount) with admin org
// filtering and leader scoping.
func (h *Handler) ServeGroupsList(w http.ResponseWriter, r *http.Request) {
	role, uname, uid, ok := authz.UserCtx(r)
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	// Only admins and leaders may view Groups
	if !(authz.IsAdmin(r) || authz.IsLeader(r)) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), listTimeout)
	defer cancel()
	db := h.DB

	// --- read query params ---
	selectedOrg := strings.TrimSpace(r.URL.Query().Get("org")) // "all" or org hex
	orgQ := strings.TrimSpace(r.URL.Query().Get("org_q"))
	orgAfter := strings.TrimSpace(r.URL.Query().Get("org_after"))
	orgBefore := strings.TrimSpace(r.URL.Query().Get("org_before"))

	search := strings.TrimSpace(r.URL.Query().Get("search"))
	after := strings.TrimSpace(r.URL.Query().Get("after"))
	before := strings.TrimSpace(r.URL.Query().Get("before"))

	// human-friendly start index for the RIGHT table range
	startParam := strings.TrimSpace(r.URL.Query().Get("start"))
	start := 1
	if startParam != "" {
		if n, err := strconv.Atoi(startParam); err == nil && n > 0 {
			start = n
		}
	}

	// Leaders are forced to their own org; no org pane.
	showOrgPane := authz.IsAdmin(r)
	if authz.IsLeader(r) {
		var u struct {
			OrganizationID *primitive.ObjectID `bson:"organization_id"`
		}
		if err := db.Collection("users").FindOne(ctx, bson.M{"_id": uid}).Decode(&u); err != nil || u.OrganizationID == nil {
			http.Error(w, "your account is not linked to an organization", http.StatusForbidden)
			return
		}
		selectedOrg = u.OrganizationID.Hex()
		showOrgPane = false
	} else {
		// For admins, default to "all" when empty.
		if selectedOrg == "" {
			selectedOrg = "all"
		}
	}

	// --- build org pane (admins only) ---
	var orgRows []orgRow
	var orgTotal int64
	var orgHasPrev, orgHasNext bool
	var orgPrevCur, orgNextCur string
	var allCount int64
	orgRangeStart := 1
	orgRangeEnd := 0

	if showOrgPane {
		orgBase := bson.M{}
		if orgQ != "" {
			q := textfold.Fold(orgQ)
			hi := q + "\uffff"
			orgBase["name_ci"] = bson.M{"$gte": q, "$lt": hi}
		}

		var err error
		orgTotal, err = db.Collection("organizations").CountDocuments(ctx, orgBase)
		if err != nil {
			h.Log.Warn("count orgs failed", zap.Error(err))
			http.Error(w, "db error", http.StatusInternalServerError)
			return
		}

		orgFilter := bson.M{}
		for k, v := range orgBase {
			orgFilter[k] = v
		}

		find := options.Find()
		limit := paging.LimitPlusOne()
		if orgBefore != "" {
			if c, ok := mongodb.DecodeCursor(orgBefore); ok {
				orgFilter["$or"] = []bson.M{
					{"name_ci": bson.M{"$lt": c.CI}},
					{"name_ci": c.CI, "_id": bson.M{"$lt": c.ID}},
				}
			}
			find.SetSort(bson.D{{Key: "name_ci", Value: -1}, {Key: "_id", Value: -1}}).SetLimit(limit)
		} else {
			if orgAfter != "" {
				if c, ok := mongodb.DecodeCursor(orgAfter); ok {
					orgFilter["$or"] = []bson.M{
						{"name_ci": bson.M{"$gt": c.CI}},
						{"name_ci": c.CI, "_id": bson.M{"$gt": c.ID}},
					}
				}
			}
			find.SetSort(bson.D{{Key: "name_ci", Value: 1}, {Key: "_id", Value: 1}}).SetLimit(limit)
		}

		cur, err := db.Collection("organizations").Find(ctx, orgFilter, find)
		if err != nil {
			h.Log.Warn("find orgs failed", zap.Error(err))
			http.Error(w, "db error", http.StatusInternalServerError)
			return
		}
		defer cur.Close(ctx)

		var oraw []struct {
			ID     primitive.ObjectID `bson:"_id"`
			Name   string             `bson:"name"`
			NameCI string             `bson:"name_ci"`
		}
		if err := cur.All(ctx, &oraw); err != nil {
			h.Log.Warn("decode orgs failed", zap.Error(err))
			http.Error(w, "db error", http.StatusInternalServerError)
			return
		}

		shown := len(oraw)
		if orgBefore != "" {
			if shown > paging.PageSize {
				oraw = oraw[1:]
				shown = len(oraw)
				orgHasPrev = true
			}
			orgHasNext = true
		} else {
			if shown > paging.PageSize {
				oraw = oraw[:paging.PageSize]
				shown = paging.PageSize
				orgHasNext = true
			}
			orgHasPrev = orgAfter != ""
		}

		orgRows = make([]orgRow, 0, shown)
		for _, o := range oraw {
			orgRows = append(orgRows, orgRow{ID: o.ID, Name: o.Name})
		}

		if shown > 0 {
			orgRangeEnd = orgRangeStart + shown - 1
			first := oraw[0]
			last := oraw[shown-1]
			orgPrevCur = mongodb.EncodeCursor(first.NameCI, first.ID)
			orgNextCur = mongodb.EncodeCursor(last.NameCI, last.ID)
		} else {
			orgRangeStart, orgRangeEnd = 0, 0
		}

		allCount, _ = db.Collection("groups").CountDocuments(ctx, bson.M{})

		orgIDs := make([]primitive.ObjectID, 0, len(orgRows))
		for _, o := range orgRows {
			orgIDs = append(orgIDs, o.ID)
		}
		orgIDs = dedupObjectIDs(orgIDs)

		byOrg := map[primitive.ObjectID]int64{}
		if len(orgIDs) > 0 {
			pipeline := []bson.M{
				{"$match": bson.M{"organization_id": bson.M{"$in": orgIDs}}},
				{"$group": bson.M{"_id": "$organization_id", "count": bson.M{"$sum": 1}}},
			}
			cc, _ := db.Collection("groups").Aggregate(ctx, pipeline)
			if cc != nil {
				defer cc.Close(ctx)
				for cc.Next(ctx) {
					var row struct {
						ID    primitive.ObjectID `bson:"_id"`
						Count int64              `bson:"count"`
					}
					_ = cc.Decode(&row)
					byOrg[row.ID] = row.Count
				}
			}
		}
		for i := range orgRows {
			orgRows[i].Count = byOrg[orgRows[i].ID]
		}
	}

	// --- build right-side groups table ---

	var clauses []bson.M

	if selectedOrg != "" && selectedOrg != "all" {
		if oid, err := primitive.ObjectIDFromHex(selectedOrg); err == nil {
			clauses = append(clauses, bson.M{"organization_id": oid})
		}
	}

	if search != "" {
		q := textfold.Fold(search)
		hi := q + "\uffff"
		clauses = append(clauses, bson.M{"name_ci": bson.M{"$gte": q, "$lt": hi}})
	}

	find := options.Find()
	limit := paging.LimitPlusOne()

	if before != "" {
		if c, ok := mongodb.DecodeCursor(before); ok {
			clauses = append(clauses, mongodb.KeysetWindow("name_ci", "lt", c.CI, c.ID))
		}
		find.SetSort(bson.D{{Key: "name_ci", Value: -1}, {Key: "_id", Value: -1}}).SetLimit(limit)
	} else {
		if after != "" {
			if c, ok := mongodb.DecodeCursor(after); ok {
				clauses = append(clauses, mongodb.KeysetWindow("name_ci", "gt", c.CI, c.ID))
			}
		}
		find.SetSort(bson.D{{Key: "name_ci", Value: 1}, {Key: "_id", Value: 1}}).SetLimit(limit)
	}

	filter := andify(clauses)

	curG, err := db.Collection("groups").Find(ctx, filter, find)
	if err != nil {
		http.Error(w, "db error", http.StatusInternalServerError)
		return
	}
	defer curG.Close(ctx)

	var graw []struct {
		ID             primitive.ObjectID `bson:"_id"`
		Name           string             `bson:"name"`
		NameCI         string             `bson:"name_ci"`
		OrganizationID primitive.ObjectID `bson:"organization_id"`
	}
	if err := curG.All(ctx, &graw); err != nil {
		http.Error(w, "db error", http.StatusInternalServerError)
		return
	}

	orig := len(graw)
	hasPrev, hasNext := false, false
	if before != "" {
		if orig > paging.PageSize {
			graw = graw[1:]
			hasPrev = true
		}
		hasNext = true
	} else {
		if orig > paging.PageSize {
			graw = graw[:paging.PageSize]
			hasNext = true
		}
		hasPrev = after != ""
	}
	shown := len(graw)

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

	// org id -> name map
	orgIDs := make([]primitive.ObjectID, 0, shown)
	for _, g := range graw {
		orgIDs = append(orgIDs, g.OrganizationID)
	}
	orgNames := map[primitive.ObjectID]string{}
	if len(orgIDs) > 0 {
		oc2, _ := db.Collection("organizations").Find(ctx, bson.M{"_id": bson.M{"$in": orgIDs}})
		defer oc2.Close(ctx)
		for oc2.Next(ctx) {
			var o models.Organization
			_ = oc2.Decode(&o)
			orgNames[o.ID] = o.Name
		}
	}

	// leaders/members counts via group_memberships
	groupIDs := make([]primitive.ObjectID, 0, shown)
	for _, g := range graw {
		groupIDs = append(groupIDs, g.ID)
	}

	leaderByGroup := map[primitive.ObjectID]int{}
	memberByGroup := map[primitive.ObjectID]int{}
	if len(groupIDs) > 0 {
		curLM, _ := db.Collection("group_memberships").Aggregate(ctx, []bson.M{
			{"$match": bson.M{"group_id": bson.M{"$in": groupIDs}}},
			{"$group": bson.M{"_id": bson.M{"g": "$group_id", "r": "$role"}, "n": bson.M{"$sum": 1}}},
		})
		defer curLM.Close(ctx)
		for curLM.Next(ctx) {
			var row struct {
				ID struct {
					G primitive.ObjectID `bson:"g"`
					R string             `bson:"r"`
				} `bson:"_id"`
				N int `bson:"n"`
			}
			if err := curLM.Decode(&row); err == nil {
				if row.ID.R == "leader" {
					leaderByGroup[row.ID.G] = row.N
				} else if row.ID.R == "member" {
					memberByGroup[row.ID.G] = row.N
				}
			}
		}
	}

	assignByGroup := map[primitive.ObjectID]int{}
	if len(groupIDs) > 0 {
		curA, _ := db.Collection("group_resource_assignments").Aggregate(ctx, []bson.M{
			{"$match": bson.M{"group_id": bson.M{"$in": groupIDs}}},
			{"$group": bson.M{"_id": "$group_id", "n": bson.M{"$sum": 1}}},
		})
		defer curA.Close(ctx)
		for curA.Next(ctx) {
			var row struct {
				ID primitive.ObjectID `bson:"_id"`
				N  int                `bson:"n"`
			}
			if err := curA.Decode(&row); err == nil {
				assignByGroup[row.ID] = row.N
			}
		}
	}

	rows := make([]groupListItem, 0, shown)
	for _, g := range graw {
		rows = append(rows, groupListItem{
			ID:                     g.ID,
			Name:                   g.Name,
			OrganizationName:       orgNames[g.OrganizationID],
			LeadersCount:           leaderByGroup[g.ID],
			MembersCount:           memberByGroup[g.ID],
			AssignedResourcesCount: assignByGroup[g.ID],
		})
	}

	prevCur, nextCur := "", ""
	if shown > 0 {
		firstKey := graw[0].NameCI
		lastKey := graw[shown-1].NameCI
		prevCur = mongodb.EncodeCursor(firstKey, graw[0].ID)
		nextCur = mongodb.EncodeCursor(lastKey, graw[shown-1].ID)
	}

	data := groupListData{
		Title:      "Groups",
		IsLoggedIn: true,
		Role:       role,
		UserName:   uname,

		ShowOrgPane:   showOrgPane,
		OrgQuery:      orgQ,
		OrgShown:      len(orgRows),
		OrgTotal:      orgTotal,
		OrgHasPrev:    orgHasPrev,
		OrgHasNext:    orgHasNext,
		OrgPrevCur:    orgPrevCur,
		OrgNextCur:    orgNextCur,
		OrgRangeStart: orgRangeStart,
		OrgRangeEnd:   orgRangeEnd,
		SelectedOrg:   selectedOrg,
		OrgRows:       orgRows,
		AllCount:      allCount,

		SearchQuery: search,
		Shown:       shown,
		Total:       countGroups(ctx, db, filter),
		HasPrev:     hasPrev,
		HasNext:     hasNext,
		PrevCursor:  prevCur,
		NextCursor:  nextCur,
		Groups:      rows,

		RangeStart: rangeStart,
		RangeEnd:   rangeEnd,
		PrevStart:  prevStart,
		NextStart:  nextStart,

		CurrentPath: nav.CurrentPath(r),
	}

	templates.RenderAutoMap(w, r, "groups_list", nil, data)
}

// countGroups counts documents matching filter, logging errors.
func countGroups(ctx context.Context, db *mongo.Database, filter bson.M) int64 {
	n, err := db.Collection("groups").CountDocuments(ctx, filter)
	if err != nil {
		// best-effort; log via global zap if needed
		return 0
	}
	return n
}

// ServeGroupManageModal handles GET /groups/{id}/manage_modal and returns
// the snippet for the Manage Group modal.
func (h *Handler) ServeGroupManageModal(w http.ResponseWriter, r *http.Request) {
	_, _, _, ok := authz.UserCtx(r)
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	if !(authz.IsAdmin(r) || authz.IsLeader(r)) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}

	gid := chi.URLParam(r, "id")
	if gid == "" {
		http.Error(w, "bad group id", http.StatusBadRequest)
		return
	}
	groupOID, err := primitive.ObjectIDFromHex(gid)
	if err != nil {
		http.Error(w, "bad group id", http.StatusBadRequest)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), listShortTimeout)
	defer cancel()
	db := h.DB

	var g struct {
		ID             primitive.ObjectID `bson:"_id"`
		Name           string             `bson:"name"`
		OrganizationID primitive.ObjectID `bson:"organization_id"`
	}
	if err := db.Collection("groups").FindOne(ctx, bson.M{"_id": groupOID}).Decode(&g); err != nil {
		if err == mongo.ErrNoDocuments {
			http.NotFound(w, r)
		} else {
			http.Error(w, "db error", http.StatusInternalServerError)
		}
		return
	}

	orgName := ""
	if !g.OrganizationID.IsZero() {
		var o models.Organization
		if err := db.Collection("organizations").FindOne(ctx, bson.M{"_id": g.OrganizationID}).Decode(&o); err == nil {
			orgName = o.Name
		}
	}

	back := r.URL.Query().Get("return")
	if back == "" {
		back = nav.ResolveBackURL(r, "/groups")
	}

	data := groupManageModalData{
		GroupID:          gid,
		GroupName:        g.Name,
		OrganizationName: orgName,
		BackURL:          back,
	}

	templates.RenderSnippet(w, "group_manage_group_modal", data)
}
