// internal/app/features/groups/groupview.go
package groups

import (
	"context"
	"net/http"
	"sort"
	"strings"

	uierrors "github.com/dalemusser/stratahub/internal/app/features/errors"
	"github.com/dalemusser/stratahub/internal/app/policy/grouppolicy"
	groupstore "github.com/dalemusser/stratahub/internal/app/store/groups"
	membershipstore "github.com/dalemusser/stratahub/internal/app/store/memberships"
	organizationstore "github.com/dalemusser/stratahub/internal/app/store/organizations"
	"github.com/dalemusser/stratahub/internal/app/system/authz"
	"github.com/dalemusser/stratahub/internal/app/system/timeouts"
	"github.com/dalemusser/stratahub/internal/app/system/viewdata"
	"github.com/dalemusser/stratahub/internal/app/system/workspace"
	"github.com/dalemusser/stratahub/internal/domain/models"
	"github.com/dalemusser/waffle/pantry/httpnav"
	"github.com/dalemusser/waffle/pantry/templates"
	"github.com/go-chi/chi/v5"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
)

// ServeGroupView renders a read-only view of a group.
func (h *Handler) ServeGroupView(w http.ResponseWriter, r *http.Request) {
	_, _, _, ok := authz.UserCtx(r)
	if !ok {
		uierrors.RenderUnauthorized(w, r, "/login")
		return
	}

	gid := chi.URLParam(r, "id")
	groupOID, err := primitive.ObjectIDFromHex(gid)
	if err != nil {
		uierrors.RenderForbidden(w, r, "Bad group id.", httpnav.ResolveBackURL(r, "/groups"))
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), timeouts.Short())
	defer cancel()
	db := h.DB

	grpStore := groupstore.New(db)
	group, err := grpStore.GetByID(ctx, groupOID)
	if err == mongo.ErrNoDocuments {
		uierrors.RenderForbidden(w, r, "Group not found.", httpnav.ResolveBackURL(r, "/groups"))
		return
	}
	if err != nil {
		h.ErrLog.LogForbidden(w, r, "database error loading group", err, "A database error occurred.", httpnav.ResolveBackURL(r, "/groups"))
		return
	}

	// Verify workspace ownership (prevent cross-workspace access)
	wsID := workspace.IDFromRequest(r)
	if wsID != primitive.NilObjectID && group.WorkspaceID != wsID {
		uierrors.RenderNotFound(w, r, "Group not found.", "/groups")
		return
	}

	canManage, policyErr := grouppolicy.CanManageGroup(ctx, db, r, group.ID, group.OrganizationID)
	if policyErr != nil {
		h.ErrLog.LogServerError(w, r, "database error checking group access", policyErr, "A database error occurred.", "/groups")
		return
	}
	if !canManage {
		uierrors.RenderForbidden(w, r, "You do not have access to this group.", httpnav.ResolveBackURL(r, "/groups"))
		return
	}

	orgName := ""
	{
		orgStore := organizationstore.New(db)
		org, err := orgStore.GetByID(ctx, group.OrganizationID)
		if err != nil {
			if err == mongo.ErrNoDocuments {
				orgName = "(Deleted)"
			} else {
				h.ErrLog.LogServerError(w, r, "database error loading organization for group", err, "A database error occurred.", "/groups")
				return
			}
		} else {
			orgName = org.Name
		}
	}

	assigned, assignErr := fetchAssignedResourcesForGroup(ctx, db, group.ID)
	if assignErr != nil {
		h.ErrLog.LogServerError(w, r, "database error fetching assigned resources", assignErr, "A database error occurred.", "/groups")
		return
	}

	memStore := membershipstore.New(db)
	leadersCount, lcErr := memStore.CountByGroup(ctx, group.ID, "leader")
	if lcErr != nil {
		h.ErrLog.LogServerError(w, r, "database error counting group leaders", lcErr, "A database error occurred.", "/groups")
		return
	}
	membersCount, mcErr := memStore.CountByGroup(ctx, group.ID, "member")
	if mcErr != nil {
		h.ErrLog.LogServerError(w, r, "database error counting group members", mcErr, "A database error occurred.", "/groups")
		return
	}

	templates.Render(w, r, "group_view", groupViewData{
		BaseVM:            viewdata.NewBaseVM(r, h.DB, "View Group", "/groups"),
		GroupID:           group.ID.Hex(),
		Name:              group.Name,
		Description:       group.Description,
		OrganizationID:    group.OrganizationID.Hex(),
		OrganizationName:  orgName,
		LeadersCount:      int(leadersCount),
		MembersCount:      int(membersCount),
		CreatedAt:         group.CreatedAt,
		UpdatedAt:         group.UpdatedAt,
		AssignedResources: assigned,
	})
}

// fetchAssignedResourcesForGroup loads resource titles for a group's assignments.
func fetchAssignedResourcesForGroup(ctx context.Context, db *mongo.Database, groupID primitive.ObjectID) ([]assignedResourceViewItem, error) {
	cur, err := db.Collection("group_resource_assignments").Find(ctx, bson.M{"group_id": groupID})
	if err != nil {
		return nil, err
	}
	defer cur.Close(ctx)

	var assigns []models.GroupResourceAssignment
	if err := cur.All(ctx, &assigns); err != nil {
		return nil, err
	}
	if len(assigns) == 0 {
		return nil, nil
	}

	ids := make([]primitive.ObjectID, 0, len(assigns))
	for _, a := range assigns {
		ids = append(ids, a.ResourceID)
	}

	cg, err := db.Collection("resources").Find(ctx, bson.M{"_id": bson.M{"$in": ids}})
	if err != nil {
		return nil, err
	}
	defer cg.Close(ctx)

	items := make([]assignedResourceViewItem, 0, len(ids))
	for cg.Next(ctx) {
		var r models.Resource
		if err := cg.Decode(&r); err != nil {
			return nil, err
		}
		items = append(items, assignedResourceViewItem{
			ResourceID:    r.ID.Hex(),
			ResourceTitle: r.Title,
		})
	}

	// Sort A→Z by resource title (case-insensitive).
	sort.SliceStable(items, func(i, j int) bool {
		ti := strings.ToLower(items[i].ResourceTitle)
		tj := strings.ToLower(items[j].ResourceTitle)
		if ti == tj {
			return items[i].ResourceTitle < items[j].ResourceTitle
		}
		return ti < tj
	})

	return items, nil
}
