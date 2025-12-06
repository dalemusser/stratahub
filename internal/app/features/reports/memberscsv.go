// internal/app/features/reports/memberscsv.go
package reports

import (
	"context"
	"encoding/csv"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/dalemusser/stratahub/internal/app/system/authz"
	"github.com/dalemusser/stratahub/internal/domain/models"
	textfold "github.com/dalemusser/waffle/toolkit/text/textfold"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo/options"
	"go.uber.org/zap"
)

// ServeMembersCSV handles GET /reports/members.csv and streams a CSV
// of members (and their group memberships) based on the current
// filters. It mirrors the semantics of the old strata_hub handler
// but uses the new Handler shape and authz.UserCtx.
func (h *Handler) ServeMembersCSV(w http.ResponseWriter, r *http.Request) {
	role, userName, uid, ok := authz.UserCtx(r)
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), reportsLongTimeout)
	defer cancel()
	db := h.DB

	// Parse filters
	orgParam := strings.TrimSpace(r.URL.Query().Get("org"))   // "all"|""|hex
	groupHex := strings.TrimSpace(r.URL.Query().Get("group")) // group id or ""
	search := strings.TrimSpace(r.URL.Query().Get("search"))
	status := strings.TrimSpace(r.URL.Query().Get("status"))
	if status == "" {
		// accept member_status from the page form
		status = strings.TrimSpace(r.URL.Query().Get("member_status"))
	}

	// Scope: admin/analyst can pass org, leader is forced to their org
	var scopeOrg *primitive.ObjectID
	switch strings.ToLower(role) {
	case "admin", "analyst":
		if orgParam != "" && orgParam != "all" {
			if oid, err := primitive.ObjectIDFromHex(orgParam); err == nil {
				scopeOrg = &oid
			}
		}
	case "leader":
		var me models.User
		if err := db.Collection("users").FindOne(ctx, bson.M{"_id": uid}).Decode(&me); err != nil || me.OrganizationID == nil {
			http.Error(w, "your account is not linked to an organization", http.StatusForbidden)
			return
		}
		scopeOrg = me.OrganizationID
	default:
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}

	// ----- Load all members in scope (users.role=member) -----
	userFilter := bson.M{"role": "member"}
	if scopeOrg != nil {
		userFilter["organization_id"] = *scopeOrg
	}
	if status == "active" || status == "disabled" {
		userFilter["status"] = status
	}
	if search != "" {
		qFold := textfold.Fold(search)
		hiFold := qFold + "\uffff"
		sLower := strings.ToLower(search)
		hiEmail := sLower + "\uffff"

		emailMode := strings.Contains(search, "@")
		if emailMode && scopeOrg != nil && (status == "active" || status == "disabled") {
			userFilter["email"] = bson.M{"$gte": sLower, "$lt": hiEmail}
		} else {
			userFilter["$or"] = []bson.M{
				{"full_name_ci": bson.M{"$gte": qFold, "$lt": hiFold}},
				{"email": bson.M{"$gte": sLower, "$lt": hiEmail}},
			}
		}
	}

	uCur, err := db.Collection("users").Find(ctx, userFilter, options.Find().
		SetSort(bson.D{{Key: "full_name_ci", Value: 1}, {Key: "_id", Value: 1}}).
		SetProjection(bson.M{
			"full_name":       1,
			"full_name_ci":    1,
			"email":           1,
			"status":          1,
			"organization_id": 1,
		}))
	if err != nil {
		h.Log.Error("find users for CSV failed", zap.Error(err))
		http.Error(w, "database error", http.StatusInternalServerError)
		return
	}
	defer uCur.Close(ctx)

	type userInfo struct {
		FullName string
		Email    string
		Status   string
		OrgID    primitive.ObjectID
	}

	userByID := make(map[primitive.ObjectID]userInfo)
	var userIDs []primitive.ObjectID
	for uCur.Next(ctx) {
		var row struct {
			ID         primitive.ObjectID  `bson:"_id"`
			FullName   string              `bson:"full_name"`
			FullNameCI string              `bson:"full_name_ci"`
			Email      string              `bson:"email"`
			Status     string              `bson:"status"`
			OrgID      *primitive.ObjectID `bson:"organization_id"`
		}
		if err := uCur.Decode(&row); err != nil {
			h.Log.Warn("decode user row failed", zap.Error(err))
			continue
		}
		if row.OrgID == nil {
			// Skip users without an organization; they shouldn't appear in this report.
			continue
		}
		userByID[row.ID] = userInfo{
			FullName: row.FullName,
			Email:    row.Email,
			Status:   row.Status,
			OrgID:    *row.OrgID,
		}
		userIDs = append(userIDs, row.ID)
	}

	if len(userIDs) == 0 {
		// No users in scope: stream an empty CSV with headers.
		writeEmptyCSV(w, r)
		return
	}

	// Preload group names
	groupFilter := bson.M{}
	if scopeOrg != nil {
		groupFilter["organization_id"] = *scopeOrg
	}
	var scopedGroupID *primitive.ObjectID
	if groupHex != "" {
		if gid, err := primitive.ObjectIDFromHex(groupHex); err == nil {
			groupFilter["_id"] = gid
			scopedGroupID = &gid
		}
	}

	groupNames := make(map[primitive.ObjectID]string)
	var groupIDs []primitive.ObjectID
	gcur, err := db.Collection("groups").Find(ctx, groupFilter, options.Find().SetProjection(bson.M{"name": 1}))
	if err != nil {
		h.Log.Error("find groups for CSV failed", zap.Error(err))
		http.Error(w, "database error", http.StatusInternalServerError)
		return
	}
	defer gcur.Close(ctx)

	for gcur.Next(ctx) {
		var g struct {
			ID   primitive.ObjectID `bson:"_id"`
			Name string             `bson:"name"`
		}
		if err := gcur.Decode(&g); err == nil {
			groupNames[g.ID] = g.Name
			groupIDs = append(groupIDs, g.ID)
		}
	}

	// Preload leaders per group (pipe-separated full names)
	leadersByGroup := make(map[primitive.ObjectID]string)
	if len(groupIDs) > 0 {
		groupLeaderIDs := make(map[primitive.ObjectID][]primitive.ObjectID)
		leaderIDSet := make(map[primitive.ObjectID]struct{})

		lmFilter := bson.M{"role": "leader", "group_id": bson.M{"$in": groupIDs}}
		if scopeOrg != nil {
			lmFilter["org_id"] = *scopeOrg
		}

		lmCur, err := db.Collection("group_memberships").Find(ctx, lmFilter, options.Find().SetProjection(bson.M{"group_id": 1, "user_id": 1}))
		if err != nil {
			h.Log.Error("find group memberships for leaders failed", zap.Error(err))
			http.Error(w, "database error", http.StatusInternalServerError)
			return
		}
		defer lmCur.Close(ctx)

		for lmCur.Next(ctx) {
			var row struct {
				GroupID primitive.ObjectID `bson:"group_id"`
				UserID  primitive.ObjectID `bson:"user_id"`
			}
			if err := lmCur.Decode(&row); err == nil {
				groupLeaderIDs[row.GroupID] = append(groupLeaderIDs[row.GroupID], row.UserID)
				leaderIDSet[row.UserID] = struct{}{}
			}
		}

		if len(leaderIDSet) > 0 {
			ids := make([]primitive.ObjectID, 0, len(leaderIDSet))
			for id := range leaderIDSet {
				ids = append(ids, id)
			}

			nameByID := make(map[primitive.ObjectID]string)
			uCur2, err := db.Collection("users").Find(ctx, bson.M{"_id": bson.M{"$in": ids}}, options.Find().SetProjection(bson.M{"full_name": 1}))
			if err != nil {
				h.Log.Error("find leader users failed", zap.Error(err))
				http.Error(w, "database error", http.StatusInternalServerError)
				return
			}
			defer uCur2.Close(ctx)

			for uCur2.Next(ctx) {
				var urow struct {
					ID       primitive.ObjectID `bson:"_id"`
					FullName string             `bson:"full_name"`
				}
				if err := uCur2.Decode(&urow); err == nil {
					nameByID[urow.ID] = urow.FullName
				}
			}

			for gid, uids := range groupLeaderIDs {
				var names []string
				for _, id := range uids {
					if n, ok := nameByID[id]; ok {
						names = append(names, n)
					}
				}
				if len(names) > 0 {
					leadersByGroup[gid] = strings.Join(names, "|")
				}
			}
		}
	}

	// Load memberships for members in scope
	gmFilter := bson.M{
		"role":    "member",
		"user_id": bson.M{"$in": userIDs},
	}
	if scopeOrg != nil {
		gmFilter["org_id"] = *scopeOrg
	}
	if scopedGroupID != nil {
		gmFilter["group_id"] = *scopedGroupID
	}

	mCur, err := db.Collection("group_memberships").Find(ctx, gmFilter, options.Find().
		SetSort(bson.D{{Key: "group_id", Value: 1}, {Key: "_id", Value: 1}}).
		SetProjection(bson.M{"group_id": 1, "user_id": 1}))
	if err != nil {
		h.Log.Error("find group memberships for CSV failed", zap.Error(err))
		http.Error(w, "database error", http.StatusInternalServerError)
		return
	}
	defer mCur.Close(ctx)

	type memDoc struct {
		GroupID primitive.ObjectID `bson:"group_id"`
		UserID  primitive.ObjectID `bson:"user_id"`
	}

	// CSV setup
	filename := csvFilenameFromQuery(r)

	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, url.PathEscape(filename)))

	// UTF-8 BOM for Excel
	_, _ = w.Write([]byte{0xEF, 0xBB, 0xBF})

	cw := csv.NewWriter(w)
	cw.UseCRLF = true
	defer cw.Flush()

	_ = cw.Write([]string{"full_name", "email", "organization", "group", "leaders", "status"})

	// org name cache
	orgName := make(map[primitive.ObjectID]string)
	resolveOrgName := func(orgID primitive.ObjectID) string {
		if orgID == primitive.NilObjectID {
			return ""
		}
		if name, ok := orgName[orgID]; ok {
			return name
		}
		var o models.Organization
		if err := db.Collection("organizations").FindOne(ctx, bson.M{"_id": orgID}).Decode(&o); err == nil {
			orgName[orgID] = o.Name
			return o.Name
		}
		return ""
	}

	// membershipSeen: which members have at least one membership in scope
	membershipSeen := make(map[primitive.ObjectID]bool)
	rowCount := 0

	// 1) Write membership rows
	for mCur.Next(ctx) {
		var m memDoc
		if err := mCur.Decode(&m); err != nil {
			h.Log.Warn("decode membership row failed", zap.Error(err))
			continue
		}
		ui, ok := userByID[m.UserID]
		if !ok {
			continue
		}
		membershipSeen[m.UserID] = true

		org := resolveOrgName(ui.OrgID)
		groupName := groupNames[m.GroupID]
		leaders := leadersByGroup[m.GroupID]

		_ = cw.Write([]string{
			ui.FullName,
			strings.ToLower(ui.Email),
			org,
			groupName,
			leaders,
			ui.Status,
		})
		rowCount++
	}

	// 2) For members with no membership in scope, write one blank-group row
	//    (only when no specific group is selected)
	if scopedGroupID == nil {
		for id, ui := range userByID {
			if membershipSeen[id] {
				continue
			}
			org := resolveOrgName(ui.OrgID)
			_ = cw.Write([]string{
				ui.FullName,
				strings.ToLower(ui.Email),
				org,
				"",
				"",
				ui.Status,
			})
			rowCount++
		}
	}

	h.Log.Info("members CSV exported", zap.String("user", userName), zap.Int("rows", rowCount))
}

// writeEmptyCSV streams an empty CSV file (headers only). It is used when
// there are no members in scope for the current filters.
func writeEmptyCSV(w http.ResponseWriter, r *http.Request) {
	filename := csvFilenameFromQuery(r)

	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, url.PathEscape(filename)))

	// UTF-8 BOM so Excel treats it as Unicode
	_, _ = w.Write([]byte{0xEF, 0xBB, 0xBF})

	cw := csv.NewWriter(w)
	cw.UseCRLF = true
	defer cw.Flush()

	_ = cw.Write([]string{"full_name", "email", "organization", "group", "leaders", "status"})
}

// csvFilenameFromQuery returns a sanitized CSV filename based on the
// "filename" query param, or a default if none is provided.
func csvFilenameFromQuery(r *http.Request) string {
	filename := r.URL.Query().Get("filename")
	if filename == "" {
		filename = "members_" + time.Now().UTC().Format("20060102_150405") + ".csv"
	}
	if !strings.HasSuffix(strings.ToLower(filename), ".csv") {
		filename += ".csv"
	}
	return filename
}
