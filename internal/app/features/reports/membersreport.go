// internal/app/features/reports/membersreport.go
package reports

import (
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	uierrors "github.com/dalemusser/stratahub/internal/app/features/errors"
	"github.com/dalemusser/stratahub/internal/app/policy/reportpolicy"
	"github.com/dalemusser/stratahub/internal/app/system/authz"
	"github.com/dalemusser/stratahub/internal/app/system/timeouts"
	"github.com/dalemusser/waffle/pantry/httpnav"
	"github.com/dalemusser/waffle/pantry/query"
	"github.com/dalemusser/waffle/pantry/templates"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

// ServeMembersReport renders the HTML Members Report UI.
func (h *Handler) ServeMembersReport(w http.ResponseWriter, r *http.Request) {
	role, uname, _, ok := authz.UserCtx(r)
	if !ok {
		uierrors.RenderUnauthorized(w, r, "/login")
		return
	}

	// Check authorization using policy layer
	reportScope := reportpolicy.CanViewMembersReport(r)
	if !reportScope.CanView {
		uierrors.RenderForbidden(w, r, "You don't have permission to view this page.", httpnav.ResolveBackURL(r, "/"))
		return
	}

	ctx, cancel := timeouts.WithTimeout(r.Context(), timeouts.Long(), h.Log, "members report")
	defer cancel()
	db := h.DB

	// Parse query parameters
	orgParam := query.Get(r, "org")
	orgQ := query.Search(r, "org_q")
	orgAfter := query.Get(r, "org_after")
	orgBefore := query.Get(r, "org_before")

	groupStatus := query.Get(r, "group_status")
	memberStatus := query.Get(r, "member_status")
	if memberStatus == "" {
		memberStatus = query.Get(r, "status")
	}
	selectedGroup := query.Get(r, "group")

	// Optional back link
	ret := query.Get(r, "return")
	showBack := ret != ""
	returnQS := ""
	if showBack {
		returnQS = "&return=" + url.QueryEscape(ret)
	}

	// Determine scope based on policy
	var scopeOrg *primitive.ObjectID
	selectedOrg := "all"

	if reportScope.AllOrgs {
		// Admin/Analyst can choose org or see all
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
	} else {
		// Leader is scoped to their org
		scopeOrg = &reportScope.OrgID
		selectedOrg = reportScope.OrgID.Hex()
	}

	// Fetch org pane data
	orgPane, err := h.fetchReportOrgPane(ctx, db, orgQ, orgAfter, orgBefore, memberStatus)
	if err != nil {
		h.ErrLog.LogServerError(w, r, "database error fetching org pane", err, "A database error occurred.", "/")
		return
	}

	// Fetch groups pane data (only when an org is selected)
	var groupsPane groupsPaneResult
	if scopeOrg != nil {
		groupsPane, err = h.fetchReportGroupsPane(ctx, db, *scopeOrg, memberStatus)
		if err != nil {
			h.ErrLog.LogServerError(w, r, "database error fetching groups pane", err, "A database error occurred.", "/")
			return
		}
	}

	// Calculate export counts
	exportCounts := h.fetchExportCounts(ctx, db, scopeOrg, selectedGroup, memberStatus, orgPane.AllCount, groupsPane.OrgMembersCount)

	// Build download filename
	label := "All"
	if selectedGroup != "" {
		for _, g := range groupsPane.Rows {
			if g.ID.Hex() == selectedGroup {
				label = g.Name
				break
			}
		}
	} else if selectedOrg != "" && selectedOrg != "all" && groupsPane.OrgName != "" {
		label = groupsPane.OrgName
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
		OrgShown:        len(orgPane.Rows),
		OrgTotal:        orgPane.Total,
		OrgHasPrev:      orgPane.HasPrev,
		OrgHasNext:      orgPane.HasNext,
		OrgPrevCur:      orgPane.PrevCursor,
		OrgNextCur:      orgPane.NextCursor,
		SelectedOrg:     selectedOrg,
		SelectedOrgName: groupsPane.OrgName,
		OrgRows:         orgPane.Rows,
		AllCount:        orgPane.AllCount,

		SelectedGroup:        selectedGroup,
		GroupRows:            groupsPane.Rows,
		OrgMembersCount:      groupsPane.OrgMembersCount,
		MembersInGroupsCount: exportCounts.MembersInGroupsCount,
		ExportRecordCount:    exportCounts.ExportRecordCount,

		GroupStatus:      groupStatus,
		MemberStatus:     memberStatus,
		DownloadFilename: downloadFilename,

		CurrentPath: r.URL.Path,
	}

	templates.RenderAutoMap(w, r, "reports_members", nil, data)
}
