// internal/app/features/reports/memberscsv.go
package reports

import (
	"encoding/csv"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	uierrors "github.com/dalemusser/stratahub/internal/app/features/errors"
	"github.com/dalemusser/stratahub/internal/app/policy/reportpolicy"
	"github.com/dalemusser/stratahub/internal/app/system/authz"
	"github.com/dalemusser/stratahub/internal/app/system/orgutil"
	"github.com/dalemusser/waffle/pantry/query"
	"github.com/dalemusser/stratahub/internal/app/system/search"
	"github.com/dalemusser/stratahub/internal/app/system/timeouts"
	"github.com/dalemusser/waffle/pantry/text"

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
	_, userName, _, ok := authz.UserCtx(r)
	if !ok {
		uierrors.RenderUnauthorized(w, r, "/login")
		return
	}

	// Check authorization using policy layer
	reportScope := reportpolicy.CanViewMembersReport(r)
	if !reportScope.CanView {
		uierrors.RenderForbidden(w, r, "You do not have access to this report.", "/")
		return
	}

	ctx, cancel := timeouts.WithTimeout(r.Context(), timeouts.Long(), h.Log, "members CSV export")
	defer cancel()
	db := h.DB

	// Parse filters
	orgParam := query.Get(r, "org")   // "all"|""|hex
	groupHex := query.Get(r, "group") // group id or ""
	searchQuery := query.Search(r, "search")
	status := query.Get(r, "status")
	if status == "" {
		// accept member_status from the page form
		status = query.Get(r, "member_status")
	}

	// Determine scope based on policy
	var scopeOrg *primitive.ObjectID
	if reportScope.AllOrgs {
		// Admin/Analyst can pass org or see all
		if orgParam != "" && orgParam != "all" {
			if oid, err := primitive.ObjectIDFromHex(orgParam); err == nil {
				scopeOrg = &oid
			}
		}
	} else {
		// Leader is scoped to their org
		scopeOrg = &reportScope.OrgID
	}

	// ----- Load all members in scope (users.role=member) -----
	userFilter := bson.M{"role": "member"}
	if scopeOrg != nil {
		userFilter["organization_id"] = *scopeOrg
	}
	if status == "active" || status == "disabled" {
		userFilter["status"] = status
	}
	if searchQuery != "" {
		qFold := text.Fold(searchQuery)
		hiFold := qFold + "\uffff"
		sLower := strings.ToLower(searchQuery)
		hiEmail := sLower + "\uffff"

		if search.EmailPivotOK(searchQuery, status, scopeOrg != nil) {
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
			"login_id":        1,
			"email":           1,
			"status":          1,
			"organization_id": 1,
		}))
	if err != nil {
		h.ErrLog.LogServerError(w, r, "find users for CSV failed", err, "A database error occurred.", "/")
		return
	}
	defer uCur.Close(ctx)

	type userInfo struct {
		FullName string
		LoginID  string
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
			LoginID    *string             `bson:"login_id"`
			Email      *string             `bson:"email"`
			Status     string              `bson:"status"`
			OrgID      *primitive.ObjectID `bson:"organization_id"`
		}
		if err := uCur.Decode(&row); err != nil {
			h.ErrLog.LogServerError(w, r, "database error decoding user row", err, "A database error occurred.", "/")
			return
		}
		if row.OrgID == nil {
			// Skip users without an organization; they shouldn't appear in this report.
			continue
		}
		loginID := ""
		if row.LoginID != nil {
			loginID = *row.LoginID
		}
		email := ""
		if row.Email != nil {
			email = *row.Email
		}
		userByID[row.ID] = userInfo{
			FullName: row.FullName,
			LoginID:  loginID,
			Email:    email,
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

	// Preload org names (batch query to avoid N+1)
	orgIDs := make([]primitive.ObjectID, 0, len(userByID))
	for _, ui := range userByID {
		if ui.OrgID != primitive.NilObjectID {
			orgIDs = append(orgIDs, ui.OrgID)
		}
	}
	orgName, err := orgutil.FetchOrgNames(ctx, db, orgIDs)
	if err != nil {
		h.ErrLog.LogServerError(w, r, "find organizations for CSV failed", err, "A database error occurred.", "/")
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
		h.ErrLog.LogServerError(w, r, "find groups for CSV failed", err, "A database error occurred.", "/")
		return
	}
	defer gcur.Close(ctx)

	for gcur.Next(ctx) {
		var g struct {
			ID   primitive.ObjectID `bson:"_id"`
			Name string             `bson:"name"`
		}
		if err := gcur.Decode(&g); err != nil {
			h.Log.Warn("decode group row for CSV", zap.Error(err))
			continue
		}
		groupNames[g.ID] = g.Name
		groupIDs = append(groupIDs, g.ID)
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
			h.ErrLog.LogServerError(w, r, "find group memberships for leaders failed", err, "A database error occurred.", "/")
			return
		}
		defer lmCur.Close(ctx)

		for lmCur.Next(ctx) {
			var row struct {
				GroupID primitive.ObjectID `bson:"group_id"`
				UserID  primitive.ObjectID `bson:"user_id"`
			}
			if err := lmCur.Decode(&row); err != nil {
				h.Log.Warn("decode leader membership row for CSV", zap.Error(err))
				continue
			}
			groupLeaderIDs[row.GroupID] = append(groupLeaderIDs[row.GroupID], row.UserID)
			leaderIDSet[row.UserID] = struct{}{}
		}

		if len(leaderIDSet) > 0 {
			ids := make([]primitive.ObjectID, 0, len(leaderIDSet))
			for id := range leaderIDSet {
				ids = append(ids, id)
			}

			nameByID := make(map[primitive.ObjectID]string)
			uCur2, err := db.Collection("users").Find(ctx, bson.M{"_id": bson.M{"$in": ids}}, options.Find().SetProjection(bson.M{"full_name": 1}))
			if err != nil {
				h.ErrLog.LogServerError(w, r, "find leader users failed", err, "A database error occurred.", "/")
				return
			}
			defer uCur2.Close(ctx)

			for uCur2.Next(ctx) {
				var urow struct {
					ID       primitive.ObjectID `bson:"_id"`
					FullName string             `bson:"full_name"`
				}
				if err := uCur2.Decode(&urow); err != nil {
					h.Log.Warn("decode leader user row for CSV", zap.Error(err))
					continue
				}
				nameByID[urow.ID] = urow.FullName
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
		h.ErrLog.LogServerError(w, r, "find group memberships for CSV failed", err, "A database error occurred.", "/")
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
	if _, err := w.Write([]byte{0xEF, 0xBB, 0xBF}); err != nil {
		h.Log.Error("CSV write failed (BOM)", zap.Error(err), zap.String("user", userName))
		return
	}

	cw := csv.NewWriter(w)
	cw.UseCRLF = true
	defer cw.Flush()

	if err := cw.Write([]string{"full_name", "login_id", "email", "organization", "group", "leaders", "status"}); err != nil {
		h.Log.Error("CSV write failed (header)", zap.Error(err), zap.String("user", userName))
		return
	}

	// membershipSeen: which members have at least one membership in scope
	membershipSeen := make(map[primitive.ObjectID]bool)
	rowCount := 0
	var writeErr error

	// 1) Write membership rows
	for mCur.Next(ctx) {
		var m memDoc
		if err := mCur.Decode(&m); err != nil {
			h.Log.Error("database error decoding membership row", zap.Error(err))
			return
		}
		ui, ok := userByID[m.UserID]
		if !ok {
			continue
		}
		membershipSeen[m.UserID] = true

		org := orgName[ui.OrgID]
		groupName := groupNames[m.GroupID]
		leaders := leadersByGroup[m.GroupID]

		if writeErr = cw.Write([]string{
			sanitizeCSVField(ui.FullName),
			ui.LoginID,
			ui.Email,
			sanitizeCSVField(org),
			sanitizeCSVField(groupName),
			sanitizeCSVField(leaders),
			ui.Status,
		}); writeErr != nil {
			h.Log.Error("CSV write failed (row)", zap.Error(writeErr), zap.String("user", userName), zap.Int("rows_written", rowCount))
			return
		}
		rowCount++
	}

	// 2) For members with no membership in scope, write one blank-group row
	//    (only when no specific group is selected)
	if scopedGroupID == nil {
		for id, ui := range userByID {
			if membershipSeen[id] {
				continue
			}
			org := orgName[ui.OrgID]
			if writeErr = cw.Write([]string{
				sanitizeCSVField(ui.FullName),
				ui.LoginID,
				ui.Email,
				sanitizeCSVField(org),
				"",
				"",
				ui.Status,
			}); writeErr != nil {
				h.Log.Error("CSV write failed (row)", zap.Error(writeErr), zap.String("user", userName), zap.Int("rows_written", rowCount))
				return
			}
			rowCount++
		}
	}

	h.Log.Info("members CSV exported", zap.String("user", userName), zap.Int("rows", rowCount))
}

// writeEmptyCSV streams an empty CSV file (headers only). It is used when
// there are no members in scope for the current filters.
// Note: Errors are not logged here since this function has no logger access.
// In practice, write failures at this point indicate client disconnect.
func writeEmptyCSV(w http.ResponseWriter, r *http.Request) {
	filename := csvFilenameFromQuery(r)

	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, url.PathEscape(filename)))

	// UTF-8 BOM so Excel treats it as Unicode
	if _, err := w.Write([]byte{0xEF, 0xBB, 0xBF}); err != nil {
		return
	}

	cw := csv.NewWriter(w)
	cw.UseCRLF = true
	defer cw.Flush()

	// Error intentionally not checked - headers only, client likely disconnected
	cw.Write([]string{"full_name", "login_id", "email", "organization", "group", "leaders", "status"})
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

// sanitizeCSVField prevents CSV formula injection by prefixing dangerous
// characters with a single quote. Excel/LibreOffice interpret cells starting
// with =, +, -, or @ as formulas, which can be exploited for code execution.
func sanitizeCSVField(s string) string {
	if len(s) == 0 {
		return s
	}
	switch s[0] {
	case '=', '+', '-', '@':
		return "'" + s
	}
	return s
}
