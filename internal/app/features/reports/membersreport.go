// internal/app/features/reports/membersreport.go
package reports

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/dalemusser/stratahub/internal/app/system/authz"
	"github.com/dalemusser/stratahub/internal/app/system/paging"
	"github.com/dalemusser/stratahub/internal/domain/models"
	"github.com/dalemusser/waffle/templates"
	mongodb "github.com/dalemusser/waffle/toolkit/db/mongodb"
	textfold "github.com/dalemusser/waffle/toolkit/text/textfold"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// ServeMembersReport renders the HTML Members Report UI.
// It mirrors the original strata_hub behavior but uses the stratahub
// Handler wiring, WAFFLE templates, and shared helper packages.
func (h *Handler) ServeMembersReport(w http.ResponseWriter, r *http.Request) {
	role, uname, uid, ok := authz.UserCtx(r)
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	role = strings.ToLower(strings.TrimSpace(role))

	ctx, cancel := context.WithTimeout(r.Context(), reportsLongTimeout)
	defer cancel()
	db := h.DB

	// --- Parse org-pane controls -------------------------------------------------
	orgParam := strings.TrimSpace(r.URL.Query().Get("org"))       // "all"|""|hex
	orgQ := strings.TrimSpace(r.URL.Query().Get("org_q"))         // org search
	orgAfter := strings.TrimSpace(r.URL.Query().Get("org_after")) // keyset after
	orgBefore := strings.TrimSpace(r.URL.Query().Get("org_before"))

	// Right side filters
	groupStatus := strings.TrimSpace(r.URL.Query().Get("group_status")) // currently unused but kept for parity
	_ = groupStatus
	memberStatus := strings.TrimSpace(r.URL.Query().Get("member_status"))
	if memberStatus == "" {
		// legacy param name from CSV endpoint
		memberStatus = strings.TrimSpace(r.URL.Query().Get("status"))
	}
	selectedGroup := strings.TrimSpace(r.URL.Query().Get("group"))

	// Optional back link
	ret := strings.TrimSpace(r.URL.Query().Get("return"))
	showBack := ret != ""
	returnQS := ""
	if showBack {
		returnQS = "&return=" + url.QueryEscape(ret)
	}

	// --- Scope: admin/analyst can browse all orgs; leader locks to their org ---
	var scopeOrg *primitive.ObjectID
	selectedOrg := "all"

	switch role {
	case "admin", "analyst":
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

	case "leader":
		var me models.User
		if err := db.Collection("users").FindOne(ctx, bson.M{"_id": uid}).Decode(&me); err != nil || me.OrganizationID == nil {
			http.Error(w, "your account is not linked to an organization", http.StatusForbidden)
			return
		}
		scopeOrg = me.OrganizationID
		selectedOrg = scopeOrg.Hex()

	default:
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}

	// --- Build org list (left pane) --------------------------------------------
	orgBase := bson.M{}
	if orgQ != "" {
		q := textfold.Fold(orgQ)
		hi := q + textfold.High
		orgBase["name_ci"] = bson.M{"$gte": q, "$lt": hi}
	}

	orgTotal, _ := db.Collection("organizations").CountDocuments(ctx, orgBase)

	// Total members across all orgs (for the All row), respecting memberStatus.
	allFilter := bson.M{"role": "member"}
	if memberStatus == "active" || memberStatus == "disabled" {
		allFilter["status"] = memberStatus
	}
	allMembersTotal, _ := db.Collection("users").CountDocuments(ctx, allFilter)

	orgFilter := bson.M{}
	for k, v := range orgBase {
		orgFilter[k] = v
	}

	findOrg := options.Find().SetLimit(paging.LimitPlusOne())
	if orgBefore != "" {
		if c, ok := mongodb.DecodeCursor(orgBefore); ok {
			orgFilter["$or"] = []bson.M{
				{"name_ci": bson.M{"$lt": c.CI}},
				{"name_ci": c.CI, "_id": bson.M{"$lt": c.ID}},
			}
		}
		findOrg.SetSort(bson.D{{Key: "name_ci", Value: -1}, {Key: "_id", Value: -1}})
	} else {
		if orgAfter != "" {
			if c, ok := mongodb.DecodeCursor(orgAfter); ok {
				orgFilter["$or"] = []bson.M{
					{"name_ci": bson.M{"$gt": c.CI}},
					{"name_ci": c.CI, "_id": bson.M{"$gt": c.ID}},
				}
			}
		}
		findOrg.SetSort(bson.D{{Key: "name_ci", Value: 1}, {Key: "_id", Value: 1}})
	}

	type oview struct {
		ID     primitive.ObjectID `bson:"_id"`
		Name   string             `bson:"name"`
		NameCI string             `bson:"name_ci"`
	}

	oc, _ := db.Collection("organizations").Find(ctx, orgFilter, findOrg)
	defer oc.Close(ctx)

	var orows []oview
	_ = oc.All(ctx, &orows)

	orgHasPrev, orgHasNext := false, false
	if orgBefore != "" {
		if len(orows) > paging.PageSize {
			orows = orows[1:]
			orgHasPrev = true
		}
		orgHasNext = true
	} else {
		if len(orows) > paging.PageSize {
			orows = orows[:paging.PageSize]
			orgHasNext = true
		}
		orgHasPrev = orgAfter != ""
	}

	// Counts of members per visible org
	idSet := make([]primitive.ObjectID, 0, len(orows))
	for _, o := range orows {
		idSet = append(idSet, o.ID)
	}

	byOrg := map[primitive.ObjectID]int64{}
	if len(idSet) > 0 {
		match := bson.M{"role": "member", "organization_id": bson.M{"$in": idSet}}
		if memberStatus == "active" || memberStatus == "disabled" {
			match["status"] = memberStatus
		}
		pipeline := []bson.M{
			{"$match": match},
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

	orgRows := make([]orgRow, 0, len(orows))
	for _, o := range orows {
		orgRows = append(orgRows, orgRow{ID: o.ID, Name: o.Name, Count: byOrg[o.ID]})
	}
	orgShown := len(orgRows)

	orgPrevCur, orgNextCur := "", ""
	if orgShown > 0 {
		orgPrevCur = mongodb.EncodeCursor(textfold.Fold(orgRows[0].Name), orgRows[0].ID)
		orgNextCur = mongodb.EncodeCursor(textfold.Fold(orgRows[orgShown-1].Name), orgRows[orgShown-1].ID)
	}

	// Derive selected org name if any
	selectedOrgName := ""
	if selectedOrg != "" && selectedOrg != "all" && scopeOrg != nil {
		var org models.Organization
		_ = db.Collection("organizations").FindOne(ctx, bson.M{"_id": *scopeOrg}).Decode(&org)
		selectedOrgName = org.Name
	}

	// --- Middle pane: groups for selected org ----------------------------------
	var groupRows []groupRow
	var orgMembersCount int64

	if scopeOrg != nil {
		// Groups in selected org
		gf := bson.M{"organization_id": *scopeOrg}
		gcur, _ := db.Collection("groups").Find(ctx, gf, options.Find().
			SetSort(bson.D{{Key: "name_ci", Value: 1}, {Key: "_id", Value: 1}}).
			SetProjection(bson.M{"name": 1}))
		defer gcur.Close(ctx)

		type grow struct {
			ID   primitive.ObjectID `bson:"_id"`
			Name string             `bson:"name"`
		}
		var glist []grow
		_ = gcur.All(ctx, &glist)

		gids := make([]primitive.ObjectID, 0, len(glist))
		for _, g := range glist {
			gids = append(gids, g.ID)
		}

		byGroup := map[primitive.ObjectID]int64{}
		if len(gids) > 0 {
			gmMatch := bson.M{
				"group_id": bson.M{"$in": gids},
				"role":     "member",
			}

			userMatch := bson.M{"user.role": "member"}
			if memberStatus == "active" || memberStatus == "disabled" {
				userMatch["user.status"] = memberStatus
			}

			pipeline := []bson.M{
				{"$match": gmMatch},
				{"$lookup": bson.M{
					"from":         "users",
					"localField":   "user_id",
					"foreignField": "_id",
					"as":           "user",
				}},
				{"$unwind": "$user"},
				{"$match": userMatch},
				{"$group": bson.M{"_id": "$group_id", "count": bson.M{"$sum": 1}}},
			}

			agg, _ := db.Collection("group_memberships").Aggregate(ctx, pipeline)
			if agg != nil {
				defer agg.Close(ctx)
				for agg.Next(ctx) {
					var row struct {
						ID    primitive.ObjectID `bson:"_id"`
						Count int64              `bson:"count"`
					}
					if err := agg.Decode(&row); err == nil {
						byGroup[row.ID] = row.Count
					}
				}
			}
		}

		groupRows = make([]groupRow, 0, len(glist))
		for _, g := range glist {
			groupRows = append(groupRows, groupRow{ID: g.ID, Name: g.Name, Count: byGroup[g.ID]})
		}

		// Total members in this org (respecting memberStatus)
		ocond := bson.M{"role": "member", "organization_id": *scopeOrg}
		if memberStatus == "active" || memberStatus == "disabled" {
			ocond["status"] = memberStatus
		}
		orgMembersCount, _ = db.Collection("users").CountDocuments(ctx, ocond)
	}

	// --- Export counts (records, members in groups) ---------------------------
	var exportRecordCount int64
	var membersInGroupsCount int64

	{
		gmMatch := bson.M{"role": "member"}
		if scopeOrg != nil {
			gmMatch["org_id"] = *scopeOrg
		}
		if selectedGroup != "" {
			if gid, err := primitive.ObjectIDFromHex(selectedGroup); err == nil {
				gmMatch["group_id"] = gid
			}
		}

		userMatch := bson.M{"user.role": "member"}
		if memberStatus == "active" || memberStatus == "disabled" {
			userMatch["user.status"] = memberStatus
		}

		// 1) membershipCount
		var membershipCount int64
		pipeline := []bson.M{
			{"$match": gmMatch},
			{"$lookup": bson.M{
				"from":         "users",
				"localField":   "user_id",
				"foreignField": "_id",
				"as":           "user",
			}},
			{"$unwind": "$user"},
			{"$match": userMatch},
			{"$count": "count"},
		}
		agg, _ := db.Collection("group_memberships").Aggregate(ctx, pipeline)
		if agg != nil {
			defer agg.Close(ctx)
			if agg.Next(ctx) {
				var row struct {
					Count int64 `bson:"count"`
				}
				if err := agg.Decode(&row); err == nil {
					membershipCount = row.Count
				}
			}
		}

		// 2) membersWithMembership (distinct user_ids)
		var membersWithMembership int64
		pipeline2 := []bson.M{
			{"$match": gmMatch},
			{"$lookup": bson.M{
				"from":         "users",
				"localField":   "user_id",
				"foreignField": "_id",
				"as":           "user",
			}},
			{"$unwind": "$user"},
			{"$match": userMatch},
			{"$group": bson.M{"_id": "$user._id"}},
			{"$count": "count"},
		}
		agg2, _ := db.Collection("group_memberships").Aggregate(ctx, pipeline2)
		if agg2 != nil {
			defer agg2.Close(ctx)
			if agg2.Next(ctx) {
				var row struct {
					Count int64 `bson:"count"`
				}
				if err := agg2.Decode(&row); err == nil {
					membersWithMembership = row.Count
				}
			}
		}

		membersInGroupsCount = membersWithMembership

		if selectedGroup != "" {
			exportRecordCount = membershipCount
		} else {
			var membersInScope int64
			if selectedOrg == "all" || selectedOrg == "" || scopeOrg == nil {
				membersInScope = allMembersTotal
			} else {
				membersInScope = orgMembersCount
			}
			if membersInScope < membersWithMembership {
				membersWithMembership = membersInScope
			}
			exportRecordCount = membershipCount + (membersInScope - membersWithMembership)
		}
	}

	// Default filename for CSV export (editable in UI)
	label := "All"
	if selectedGroup != "" {
		for _, g := range groupRows {
			if g.ID.Hex() == selectedGroup {
				label = g.Name
				break
			}
		}
	} else if selectedOrg != "" && selectedOrg != "all" && selectedOrgName != "" {
		label = selectedOrgName
	}
	safeLabel := strings.TrimSpace(label)
	if safeLabel == "" {
		safeLabel = "All"
	}
	safeLabel = strings.ReplaceAll(safeLabel, " ", "_")
	downloadFilename := fmt.Sprintf("%s_%s.csv", safeLabel, time.Now().UTC().Format("2006-01-02_1504"))

	data := pageData{
		Title:      "Members Report",
		IsLoggedIn: true,
		Role:       role,
		UserName:   uname,

		ShowBack: showBack,
		BackURL:  ret,
		ReturnQS: returnQS,

		OrgQuery:        orgQ,
		OrgShown:        orgShown,
		OrgTotal:        orgTotal,
		OrgHasPrev:      orgHasPrev,
		OrgHasNext:      orgHasNext,
		OrgPrevCur:      orgPrevCur,
		OrgNextCur:      orgNextCur,
		SelectedOrg:     selectedOrg,
		SelectedOrgName: selectedOrgName,
		OrgRows:         orgRows,
		AllCount:        allMembersTotal,

		SelectedGroup:        selectedGroup,
		GroupRows:            groupRows,
		OrgMembersCount:      orgMembersCount,
		MembersInGroupsCount: membersInGroupsCount,
		ExportRecordCount:    exportRecordCount,

		GroupStatus:      groupStatus,
		MemberStatus:     memberStatus,
		DownloadFilename: downloadFilename,

		CurrentPath: r.URL.Path,
	}

	templates.RenderAutoMap(w, r, "reports_members", nil, data)
}
