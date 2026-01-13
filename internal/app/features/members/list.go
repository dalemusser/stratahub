// internal/app/features/members/list.go
package members

// Terminology: User Identifiers
//   - UserID / userID / user_id: The MongoDB ObjectID (_id) that uniquely identifies a user record
//   - LoginID / loginID / login_id: The human-readable string users type to log in

import (
	"context"
	"net/http"

	uierrors "github.com/dalemusser/stratahub/internal/app/features/errors"
	"github.com/dalemusser/stratahub/internal/app/policy/memberpolicy"
	organizationstore "github.com/dalemusser/stratahub/internal/app/store/organizations"
	userstore "github.com/dalemusser/stratahub/internal/app/store/users"
	"github.com/dalemusser/stratahub/internal/app/system/authz"
	"github.com/dalemusser/stratahub/internal/app/system/paging"
	"github.com/dalemusser/stratahub/internal/app/system/timeouts"
	"github.com/dalemusser/stratahub/internal/app/system/viewdata"
	"github.com/dalemusser/waffle/pantry/httpnav"
	"github.com/dalemusser/waffle/pantry/query"
	"github.com/dalemusser/waffle/pantry/templates"
	"github.com/go-chi/chi/v5"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.uber.org/zap"
)

// ServeList renders the main Members screen with org pane + members table.
// Authorization: Admin can list all members; Leader can only list members in their org.
func (h *Handler) ServeList(w http.ResponseWriter, r *http.Request) {
	_, _, _, ok := authz.UserCtx(r)
	if !ok {
		uierrors.RenderUnauthorized(w, r, "/login")
		return
	}

	// Check authorization using policy layer
	listScope := memberpolicy.CanListMembers(r)
	if !listScope.CanList {
		uierrors.RenderForbidden(w, r, "You don't have permission to view members.", httpnav.ResolveBackURL(r, "/"))
		return
	}

	ctx, cancel := timeouts.WithTimeout(r.Context(), timeouts.Batch(), h.Log, "members list")
	defer cancel()
	db := h.DB

	// Parse query parameters
	orgParam := query.Get(r, "org")
	orgQ := query.Search(r, "org_q")
	orgAfter := query.Get(r, "org_after")
	orgBefore := query.Get(r, "org_before")

	searchQuery := query.Search(r, "search")
	loginIDQuery := query.Search(r, "login_id")
	status := query.Get(r, "status")
	after := query.Get(r, "after")
	before := query.Get(r, "before")
	start := paging.ParseStart(r)

	// Determine scope based on policy
	var selectedOrg string
	var scopeOrg *primitive.ObjectID
	var scopeOrgIDs []primitive.ObjectID // For coordinators (multiple orgs)

	if listScope.AllOrgs {
		// Admin can choose org or see all
		if orgParam == "" {
			selectedOrg = "all"
		} else {
			selectedOrg = orgParam
		}
		if selectedOrg != "all" {
			if oid, err := primitive.ObjectIDFromHex(selectedOrg); err == nil {
				scopeOrg = &oid
			} else {
				h.Log.Warn("invalid org parameter, defaulting to all", zap.String("org", selectedOrg), zap.Error(err))
				selectedOrg = "all"
			}
		}
	} else if len(listScope.OrgIDs) > 0 {
		// Coordinator can choose from their assigned orgs or see all their orgs
		scopeOrgIDs = listScope.OrgIDs
		if orgParam == "" {
			selectedOrg = "all"
		} else {
			selectedOrg = orgParam
		}
		if selectedOrg != "all" {
			if oid, err := primitive.ObjectIDFromHex(selectedOrg); err == nil {
				// Verify coordinator has access to this org
				if authz.CanAccessOrg(r, oid) {
					scopeOrg = &oid
				} else {
					h.Log.Warn("coordinator tried to access unauthorized org, defaulting to all", zap.String("org", selectedOrg))
					selectedOrg = "all"
				}
			} else {
				h.Log.Warn("invalid org parameter, defaulting to all", zap.String("org", selectedOrg), zap.Error(err))
				selectedOrg = "all"
			}
		}
	} else {
		// Leader is scoped to their org
		selectedOrg = listScope.OrgID.Hex()
		scopeOrg = &listScope.OrgID
	}

	// Fetch org pane data (admin and coordinator - when they can see multiple orgs)
	showOrgPane := listScope.AllOrgs || len(listScope.OrgIDs) > 0
	var orgPane orgPaneData

	if showOrgPane {
		var err error
		orgPane, err = h.fetchOrgPane(ctx, db, orgQ, orgAfter, orgBefore, scopeOrgIDs)
		if err != nil {
			h.ErrLog.LogServerError(w, r, "database error fetching org pane", err, "A database error occurred.", "/")
			return
		}
	}

	// Fetch members list
	members, err := h.fetchMembersList(ctx, r, db, scopeOrg, searchQuery, loginIDQuery, status, after, before, start, scopeOrgIDs)
	if err != nil {
		h.ErrLog.LogServerError(w, r, "database error fetching members list", err, "A database error occurred.", "/")
		return
	}

	base := viewdata.NewBaseVM(r, db, "Members", "/members")

	templates.RenderAutoMap(w, r, "members_list", nil, listData{
		BaseVM: base,

		// Orgs pane
		ShowOrgPane:   showOrgPane,
		OrgQuery:      orgQ,
		OrgShown:      len(orgPane.Rows),
		OrgTotal:      orgPane.Total,
		OrgHasPrev:    orgPane.HasPrev,
		OrgHasNext:    orgPane.HasNext,
		OrgPrevCur:    orgPane.PrevCursor,
		OrgNextCur:    orgPane.NextCursor,
		OrgRows:       orgPane.Rows,
		SelectedOrg:   selectedOrg,
		AllCount:      orgPane.AllCount,
		OrgRangeStart: orgPane.RangeStart,
		OrgRangeEnd:   orgPane.RangeEnd,

		// Members filter
		SearchQuery:  searchQuery,
		LoginIDQuery: loginIDQuery,
		Status:       status,

		// Members counts + range
		Shown:      members.Shown,
		Total:      members.Total,
		RangeStart: members.RangeStart,
		RangeEnd:   members.RangeEnd,

		// Members keyset cursors + page-index starts
		HasPrev:    members.HasPrev,
		HasNext:    members.HasNext,
		PrevCursor: members.PrevCursor,
		NextCursor: members.NextCursor,
		PrevStart:  members.PrevStart,
		NextStart:  members.NextStart,

		MemberRows: members.Rows,

		AllowUpload: ((base.Role == "superadmin" || base.Role == "admin") && selectedOrg != "all") || base.Role == "leader",
		AllowAdd:    true,
	})
}

// ServeManageMemberModal renders the HTMX modal to manage a single member:
// View / Edit / Delete.
// Authorization: Admin can manage any member; Leader can only manage members in their org.
func (h *Handler) ServeManageMemberModal(w http.ResponseWriter, r *http.Request) {
	_, _, _, ok := authz.UserCtx(r)
	if !ok {
		uierrors.RenderUnauthorized(w, r, "/login")
		return
	}

	idHex := chi.URLParam(r, "id")
	memberID, err := primitive.ObjectIDFromHex(idHex)
	if err != nil {
		uierrors.HTMXBadRequest(w, r, "Invalid member ID.", "/members")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), timeouts.Short())
	defer cancel()
	db := h.DB

	// Load the member via store
	usrStore := userstore.New(db)
	uptr, err := usrStore.GetMemberByID(ctx, memberID)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			uierrors.HTMXNotFound(w, r, "Member not found.", "/members")
		} else {
			h.ErrLog.HTMXLogServerError(w, r, "database error loading member", err, "A database error occurred.", "/members")
		}
		return
	}
	u := *uptr

	// Check authorization: can this user manage this member?
	canManage, policyErr := memberpolicy.CanManageMember(ctx, db, r, u.OrganizationID)
	if policyErr != nil {
		h.ErrLog.HTMXLogServerError(w, r, "policy check failed", policyErr, "A database error occurred.", "/members")
		return
	}
	if !canManage {
		uierrors.HTMXForbidden(w, r, "You don't have permission to manage this member.", "/members")
		return
	}

	// Resolve organization name if present
	orgName := ""
	if u.OrganizationID != nil {
		orgStore := organizationstore.New(db)
		org, err := orgStore.GetByID(ctx, *u.OrganizationID)
		if err != nil {
			if err == mongo.ErrNoDocuments {
				h.Log.Warn("organization not found for member (may have been deleted)",
					zap.String("user_id", u.ID.Hex()),
					zap.String("org_id", u.OrganizationID.Hex()))
				orgName = "(Deleted)"
			} else {
				h.ErrLog.HTMXLogServerError(w, r, "database error loading organization for member modal", err, "A database error occurred.", "/members")
				return
			}
		} else {
			orgName = org.Name
		}
	}

	back := r.URL.Query().Get("return")
	if back == "" {
		back = httpnav.ResolveBackURL(r, "/members")
	}

	loginID := ""
	if u.LoginID != nil {
		loginID = *u.LoginID
	}

	data := memberManageModalData{
		MemberID: u.ID.Hex(),
		FullName: u.FullName,
		LoginID:  loginID,
		OrgName:  orgName,
		BackURL:  back,
	}

	// Render the modal snippet
	templates.RenderSnippet(w, "members_manage_member_modal", data)
}

