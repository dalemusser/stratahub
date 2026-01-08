// internal/app/features/leaders/list.go
package leaders

import (
	"context"
	"net/http"

	uierrors "github.com/dalemusser/stratahub/internal/app/features/errors"
	"github.com/dalemusser/stratahub/internal/app/system/authz"
	"github.com/dalemusser/stratahub/internal/app/system/paging"
	"github.com/dalemusser/stratahub/internal/app/system/timeouts"
	"github.com/dalemusser/stratahub/internal/app/system/viewdata"
	"github.com/dalemusser/waffle/pantry/query"
	"github.com/dalemusser/waffle/pantry/templates"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

// ServeList renders the main Leaders screen with org pane + leaders table.
func (h *Handler) ServeList(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), timeouts.Medium())
	defer cancel()
	db := h.DB

	// Parse query parameters
	orgParam := query.Get(r, "org")
	orgQ := query.Search(r, "org_q")
	orgAfter := query.Get(r, "org_after")
	orgBefore := query.Get(r, "org_before")

	search := query.Search(r, "search")
	status := query.Get(r, "status")
	after := query.Get(r, "after")
	before := query.Get(r, "before")
	start := paging.ParseStart(r)

	// Determine coordinator scope (if coordinator, limit to assigned orgs)
	var scopeOrgIDs []primitive.ObjectID
	if authz.IsCoordinator(r) {
		scopeOrgIDs = authz.UserOrgIDs(r)
	}

	// Determine scope
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

	// Fetch org pane data
	orgPane, err := h.fetchOrgPane(ctx, db, orgQ, orgAfter, orgBefore, scopeOrgIDs)
	if err != nil {
		uierrors.RenderServerError(w, r, "A database error occurred.", "/leaders")
		return
	}

	// Fetch leaders list
	leaders, err := h.fetchLeadersList(r, ctx, db, scopeOrg, search, status, after, before, start, scopeOrgIDs)
	if err != nil {
		uierrors.RenderServerError(w, r, "A database error occurred.", "/leaders")
		return
	}

	data := listData{
		BaseVM: viewdata.NewBaseVM(r, h.DB, "Leaders", "/leaders"),

		OrgQuery:      orgQ,
		OrgShown:      len(orgPane.Rows),
		OrgTotal:      orgPane.Total,
		OrgHasPrev:    orgPane.HasPrev,
		OrgHasNext:    orgPane.HasNext,
		OrgPrevCur:    orgPane.PrevCursor,
		OrgNextCur:    orgPane.NextCursor,
		OrgRangeStart: orgPane.RangeStart,
		OrgRangeEnd:   orgPane.RangeEnd,
		SelectedOrg:   selectedOrg,
		OrgRows:       orgPane.Rows,
		AllCount:      orgPane.AllCount,

		SearchQuery: search,
		Status:      status,
		Shown:       leaders.Shown,
		Total:       leaders.Total,
		RangeStart:  leaders.RangeStart,
		RangeEnd:    leaders.RangeEnd,
		HasPrev:     leaders.HasPrev,
		HasNext:     leaders.HasNext,
		PrevCursor:  leaders.PrevCursor,
		NextCursor:  leaders.NextCursor,
		PrevStart:   leaders.PrevStart,
		NextStart:   leaders.NextStart,
		Rows:        leaders.Rows,
	}

	templates.RenderAutoMap(w, r, "admin_leaders_list", nil, data)
}
