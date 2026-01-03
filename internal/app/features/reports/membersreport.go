// internal/app/features/reports/membersreport.go
package reports

import (
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	uierrors "github.com/dalemusser/stratahub/internal/app/features/errors"
	"github.com/dalemusser/stratahub/internal/app/policy/reportpolicy"
	"github.com/dalemusser/stratahub/internal/app/system/authz"
	"github.com/dalemusser/stratahub/internal/app/system/paging"
	"github.com/dalemusser/stratahub/internal/app/system/timeouts"
	"github.com/dalemusser/stratahub/internal/app/system/viewdata"
	"github.com/dalemusser/waffle/pantry/httpnav"
	"github.com/dalemusser/waffle/pantry/query"
	"github.com/dalemusser/waffle/pantry/templates"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

// ServeMembersReport renders the HTML Members Report UI.
func (h *Handler) ServeMembersReport(w http.ResponseWriter, r *http.Request) {
	_, _, _, ok := authz.UserCtx(r)
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
	orgStart := parseStart(r, "org_start")

	groupsAfter := query.Get(r, "groups_after")
	groupsBefore := query.Get(r, "groups_before")
	groupsStart := parseStart(r, "groups_start")

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
	backURL := "/"
	if showBack {
		returnQS = "&return=" + url.QueryEscape(ret)
		backURL = ret
	}

	// Determine scope based on policy
	var scopeOrg *primitive.ObjectID
	var scopeOrgIDs []primitive.ObjectID // For coordinators: list of allowed org IDs
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
	} else if len(reportScope.OrgIDs) > 0 {
		// Coordinator is scoped to multiple orgs
		scopeOrgIDs = reportScope.OrgIDs
		if orgParam != "" {
			selectedOrg = orgParam
		}
		if selectedOrg != "all" {
			if oid, err := primitive.ObjectIDFromHex(selectedOrg); err == nil {
				// Verify the selected org is in the coordinator's allowed list
				for _, allowedOrgID := range scopeOrgIDs {
					if allowedOrgID == oid {
						scopeOrg = &oid
						break
					}
				}
				// If not found in allowed list, reset to "all"
				if scopeOrg == nil {
					selectedOrg = "all"
				}
			} else {
				selectedOrg = "all"
			}
		}
	} else {
		// Leader is scoped to their single org
		scopeOrg = &reportScope.OrgID
		selectedOrg = reportScope.OrgID.Hex()
	}

	// Fetch org pane data
	orgPane, err := h.fetchReportOrgPane(ctx, db, orgQ, orgAfter, orgBefore, memberStatus, scopeOrgIDs)
	if err != nil {
		h.ErrLog.LogServerError(w, r, "database error fetching org pane", err, "A database error occurred.", "/")
		return
	}

	// Fetch groups pane data (only when an org is selected)
	var groupsPane groupsPaneResult
	if scopeOrg != nil {
		groupsPane, err = h.fetchReportGroupsPane(ctx, db, *scopeOrg, groupsAfter, groupsBefore, memberStatus)
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

	// Compute ranges for pagination display
	orgRange := paging.ComputeRange(orgStart, len(orgPane.Rows))
	groupsRange := paging.ComputeRange(groupsStart, len(groupsPane.Rows))

	data := pageData{
		BaseVM:   viewdata.NewBaseVM(r, h.DB, "Members Report", backURL),
		ShowBack: showBack,
		ReturnQS: returnQS,

		OrgQuery:      orgQ,
		OrgShown:      len(orgPane.Rows),
		OrgTotal:      orgPane.Total,
		OrgRangeStart: orgRange.Start,
		OrgRangeEnd:   orgRange.End,
		OrgHasPrev:    orgPane.HasPrev,
		OrgHasNext:    orgPane.HasNext,
		OrgPrevCur:    orgPane.PrevCursor,
		OrgNextCur:    orgPane.NextCursor,
		OrgPrevStart:  orgRange.PrevStart,
		OrgNextStart:  orgRange.NextStart,
		SelectedOrg:     selectedOrg,
		SelectedOrgName: groupsPane.OrgName,
		OrgRows:         orgPane.Rows,
		AllCount:        orgPane.AllCount,

		SelectedGroup:    selectedGroup,
		GroupRows:        groupsPane.Rows,
		GroupsShown:      len(groupsPane.Rows),
		GroupsTotal:      groupsPane.Total,
		GroupsRangeStart: groupsRange.Start,
		GroupsRangeEnd:   groupsRange.End,
		GroupsHasPrev:    groupsPane.HasPrev,
		GroupsHasNext:    groupsPane.HasNext,
		GroupsPrevCur:    groupsPane.PrevCursor,
		GroupsNextCur:    groupsPane.NextCursor,
		GroupsPrevStart:  groupsRange.PrevStart,
		GroupsNextStart:  groupsRange.NextStart,
		OrgMembersCount:      groupsPane.OrgMembersCount,
		MembersInGroupsCount: exportCounts.MembersInGroupsCount,
		ExportRecordCount:    exportCounts.ExportRecordCount,

		GroupStatus:      groupStatus,
		MemberStatus:     memberStatus,
		DownloadFilename: downloadFilename,
	}

	templates.RenderAutoMap(w, r, "reports_members", nil, data)
}

// parseStart extracts a start parameter from the request.
func parseStart(r *http.Request, name string) int {
	s := query.Get(r, name)
	if s == "" {
		return 1
	}
	n, err := strconv.Atoi(s)
	if err != nil || n < 1 {
		return 1
	}
	return n
}
