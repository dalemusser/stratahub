package leaders

import (
	"context"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/dalemusser/stratahub/internal/app/system/auth"
	"github.com/dalemusser/stratahub/internal/app/system/paging"
	"github.com/dalemusser/stratahub/internal/domain/models"
	"github.com/dalemusser/waffle/templates"
	"github.com/dalemusser/waffle/toolkit/db/mongodb"
	"github.com/dalemusser/waffle/toolkit/text/textfold"
	"github.com/dalemusser/waffle/toolkit/ui/nav"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo/options"
)

const listTimeout = 10 * time.Second

type orgRow struct {
	ID    primitive.ObjectID
	Name  string
	Count int64
}

type leaderRow struct {
	ID          primitive.ObjectID
	FullName    string
	Email       string
	OrgName     string
	GroupsCount int
	Auth        string
	Status      string
}

type listData struct {
	Title, Role, UserName string
	IsLoggedIn            bool

	// left pane
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
	AllCount      int64 // total leaders across all orgs

	// leaders table
	SearchQuery string
	Status      string
	Shown       int
	Total       int64
	RangeStart  int
	RangeEnd    int
	HasPrev     bool
	HasNext     bool
	PrevCursor  string
	NextCursor  string
	PrevStart   int
	NextStart   int
	Rows        []leaderRow

	BackURL     string
	CurrentPath string
}

func (h *Handler) ServeList(w http.ResponseWriter, r *http.Request) {
	u, _ := auth.CurrentUser(r)
	role := "admin"
	userName := ""
	if u != nil {
		userName = u.Name
	}

	ctx, cancel := context.WithTimeout(r.Context(), listTimeout)
	defer cancel()

	db := h.DB

	// org pane params
	orgParam := strings.TrimSpace(r.URL.Query().Get("org")) // "all" or hex
	orgQ := strings.TrimSpace(r.URL.Query().Get("org_q"))
	orgAfter := strings.TrimSpace(r.URL.Query().Get("org_after"))
	orgBefore := strings.TrimSpace(r.URL.Query().Get("org_before"))

	// leaders table params
	search := strings.TrimSpace(r.URL.Query().Get("search"))
	status := strings.TrimSpace(r.URL.Query().Get("status"))
	after := strings.TrimSpace(r.URL.Query().Get("after"))
	before := strings.TrimSpace(r.URL.Query().Get("before"))

	// human-friendly start index (defaults to 1)
	startParam := strings.TrimSpace(r.URL.Query().Get("start"))
	start := 1
	if startParam != "" {
		if n, err := strconv.Atoi(startParam); err == nil && n > 0 {
			start = n
		}
	}

	// selected org
	selectedOrg := "all"
	var scopeOrg *primitive.ObjectID
	if orgParam != "" {
		selectedOrg = orgParam
	}
	if selectedOrg != "all" {
		if oid, err := primitive.ObjectIDFromHex(selectedOrg); err == nil {
			scopeOrg = &oid
		} else {
			selectedOrg = "all"
		}
	}

	// ---- Left pane: org list (paged + counts) ----
	var (
		orgRows                []orgRow
		orgShown               int
		orgTotal               int64
		orgHasPrev, orgHasNext bool
		orgPrevCur, orgNextCur string
		orgRangeStart          = 1
		orgRangeEnd            = 0
	)

	orgBase := bson.M{}
	if orgQ != "" {
		q := textfold.Fold(orgQ)
		hi := q + "\uffff"
		orgBase["name_ci"] = bson.M{"$gte": q, "$lt": hi}
	}

	orgTotal, _ = db.Collection("organizations").CountDocuments(ctx, orgBase)
	allLeadersTotal, _ := db.Collection("users").CountDocuments(ctx, bson.M{"role": "leader"})

	orgFilter := bson.M{}
	for k, v := range orgBase {
		orgFilter[k] = v
	}
	findOrg := options.Find()
	limit := paging.LimitPlusOne()

	if orgBefore != "" {
		if c, ok := mongodb.DecodeCursor(orgBefore); ok {
			orgFilter["$or"] = []bson.M{
				{"name_ci": bson.M{"$lt": c.CI}},
				{"name_ci": c.CI, "_id": bson.M{"$lt": c.ID}},
			}
		}
		findOrg.SetSort(bson.D{{Key: "name_ci", Value: -1}, {Key: "_id", Value: -1}}).SetLimit(limit)
	} else {
		if orgAfter != "" {
			if c, ok := mongodb.DecodeCursor(orgAfter); ok {
				orgFilter["$or"] = []bson.M{
					{"name_ci": bson.M{"$gt": c.CI}},
					{"name_ci": c.CI, "_id": bson.M{"$gt": c.ID}},
				}
			}
		}
		findOrg.SetSort(bson.D{{Key: "name_ci", Value: 1}, {Key: "_id", Value: 1}}).SetLimit(limit)
	}

	type oview struct {
		ID     primitive.ObjectID `bson:"_id"`
		Name   string             `bson:"name"`
		NameCI string             `bson:"name_ci"`
	}

	oc, err := db.Collection("organizations").Find(ctx, orgFilter, findOrg)
	if err != nil {
		http.Error(w, "db error", http.StatusInternalServerError)
		return
	}
	defer oc.Close(ctx)

	var orows []oview
	_ = oc.All(ctx, &orows)

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

	// counts per org
	idSet := make([]primitive.ObjectID, 0, len(orows))
	for _, o := range orows {
		idSet = append(idSet, o.ID)
	}
	byOrg := map[primitive.ObjectID]int64{}
	if len(idSet) > 0 {
		pipeline := []bson.M{
			{"$match": bson.M{"role": "leader", "organization_id": bson.M{"$in": idSet}}},
			{"$group": bson.M{"_id": "$organization_id", "count": bson.M{"$sum": 1}}},
		}
		cc, _ := db.Collection("users").Aggregate(ctx, pipeline)
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

	orgRows = make([]orgRow, 0, len(orows))
	for _, o := range orows {
		orgRows = append(orgRows, orgRow{ID: o.ID, Name: o.Name, Count: byOrg[o.ID]})
	}
	orgShown = len(orgRows)
	if orgShown > 0 {
		orgPrevCur = mongodb.EncodeCursor(textfold.Fold(orgRows[0].Name), orgRows[0].ID)
		orgNextCur = mongodb.EncodeCursor(textfold.Fold(orgRows[orgShown-1].Name), orgRows[orgShown-1].ID)
		orgRangeEnd = orgRangeStart + orgShown - 1
	} else {
		orgRangeStart, orgRangeEnd = 0, 0
	}

	// ---- Leaders table with email-pivot ----

	base := bson.M{"role": "leader"}
	if status == "active" || status == "disabled" {
		base["status"] = status
	}
	if scopeOrg != nil {
		base["organization_id"] = *scopeOrg
	}

	emailMode := strings.Contains(search, "@")
	emailPivot := emailMode && (status == "active" || status == "disabled") && (scopeOrg != nil)

	if search != "" {
		sName := textfold.Fold(search)
		hiName := sName + "\uffff"
		sEmail := strings.ToLower(search)
		hiEmail := sEmail + "\uffff"

		if emailPivot {
			base["$or"] = []bson.M{{"email": bson.M{"$gte": sEmail, "$lt": hiEmail}}}
		} else {
			base["$or"] = []bson.M{
				{"full_name_ci": bson.M{"$gte": sName, "$lt": hiName}},
				{"email": bson.M{"$gte": sEmail, "$lt": hiEmail}},
			}
		}
	}

	total, _ := db.Collection("users").CountDocuments(ctx, base)

	f := bson.M{}
	for k, v := range base {
		f[k] = v
	}

	find := options.Find()
	limit = paging.LimitPlusOne()
	sortField := "full_name_ci"
	if emailPivot {
		sortField = "email"
	}

	applyWin := func(dir, key string, id primitive.ObjectID) {
		ks := mongodb.KeysetWindow(sortField, dir, key, id)
		if search != "" {
			f["$and"] = []bson.M{{"$or": base["$or"]}, ks}
			delete(f, "$or")
		} else {
			for k, v := range ks {
				f[k] = v
			}
		}
	}

	if before != "" {
		if c, ok := mongodb.DecodeCursor(before); ok {
			applyWin("lt", c.CI, c.ID)
		}
		find.SetSort(bson.D{{Key: sortField, Value: -1}, {Key: "_id", Value: -1}}).SetLimit(limit)
	} else {
		if after != "" {
			if c, ok := mongodb.DecodeCursor(after); ok {
				applyWin("gt", c.CI, c.ID)
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
		Auth           string              `bson:"auth_method"`
		OrganizationID *primitive.ObjectID `bson:"organization_id"`
	}

	uc, err := db.Collection("users").Find(ctx, f, find)
	if err != nil {
		http.Error(w, "db error", http.StatusInternalServerError)
		return
	}
	defer uc.Close(ctx)

	var urows []urow
	_ = uc.All(ctx, &urows)

	if before != "" {
		for i, j := 0, len(urows)-1; i < j; i, j = i+1, j-1 {
			urows[i], urows[j] = urows[j], urows[i]
		}
	}

	orig = len(urows)
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

	// org id -> name map
	orgIds := make([]primitive.ObjectID, 0, shown)
	for _, r := range urows {
		if r.OrganizationID != nil {
			orgIds = append(orgIds, *r.OrganizationID)
		}
	}
	orgNames := map[primitive.ObjectID]string{}
	if len(orgIds) > 0 {
		oc2, _ := db.Collection("organizations").Find(ctx, bson.M{"_id": bson.M{"$in": orgIds}})
		defer oc2.Close(ctx)
		for oc2.Next(ctx) {
			var o models.Organization
			_ = oc2.Decode(&o)
			orgNames[o.ID] = o.Name
		}
	}

	// groups per leader (role=leader in group_memberships)
	leaderIDs := make([]primitive.ObjectID, 0, shown)
	for _, r := range urows {
		leaderIDs = append(leaderIDs, r.ID)
	}
	groupsByLeader := map[primitive.ObjectID]int{}
	if len(leaderIDs) > 0 {
		curGM, _ := db.Collection("group_memberships").Aggregate(ctx, []bson.M{
			{"$match": bson.M{"role": "leader", "user_id": bson.M{"$in": leaderIDs}}},
			{"$group": bson.M{"_id": "$user_id", "n": bson.M{"$sum": 1}}},
		})
		defer curGM.Close(ctx)
		for curGM.Next(ctx) {
			var row struct {
				ID primitive.ObjectID `bson:"_id"`
				N  int                `bson:"n"`
			}
			if err := curGM.Decode(&row); err == nil {
				groupsByLeader[row.ID] = row.N
			}
		}
	}

	rows := make([]leaderRow, 0, shown)
	for _, r := range urows {
		on := ""
		if r.OrganizationID != nil {
			on = orgNames[*r.OrganizationID]
		}
		rows = append(rows, leaderRow{
			ID:          r.ID,
			FullName:    r.FullName,
			Email:       strings.ToLower(r.Email),
			OrgName:     on,
			GroupsCount: groupsByLeader[r.ID],
			Auth:        r.Auth,
			Status:      r.Status,
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

	// compute range + next/prev start values
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

	data := listData{
		Title:      "Leaders",
		IsLoggedIn: true,
		Role:       role,
		UserName:   userName,

		OrgQuery:      orgQ,
		OrgShown:      orgShown,
		OrgTotal:      orgTotal,
		OrgHasPrev:    orgHasPrev,
		OrgHasNext:    orgHasNext,
		OrgPrevCur:    orgPrevCur,
		OrgNextCur:    orgNextCur,
		OrgRangeStart: orgRangeStart,
		OrgRangeEnd:   orgRangeEnd,
		SelectedOrg:   selectedOrg,
		OrgRows:       orgRows,
		AllCount:      allLeadersTotal,

		SearchQuery: search,
		Status:      status,
		Shown:       shown,
		Total:       total,
		RangeStart:  rangeStart,
		RangeEnd:    rangeEnd,
		HasPrev:     hasPrev,
		HasNext:     hasNext,
		PrevCursor:  prevCur,
		NextCursor:  nextCur,
		PrevStart:   prevStart,
		NextStart:   nextStart,
		Rows:        rows,

		BackURL:     nav.ResolveBackURL(r, "/leaders"),
		CurrentPath: nav.CurrentPath(r),
	}

	templates.RenderAutoMap(w, r, "admin_leaders_list", nil, data)
}
