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
	"github.com/dalemusser/stratahub/internal/app/system/authz"
	"github.com/dalemusser/stratahub/internal/domain/models"
	"github.com/dalemusser/waffle/templates"
	nav "github.com/dalemusser/waffle/toolkit/ui/nav"
	"github.com/go-chi/chi/v5"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.uber.org/zap"
)

// ServeGroupView renders a read-only view of a group.
func (h *Handler) ServeGroupView(w http.ResponseWriter, r *http.Request) {
	role, uname, _, ok := authz.UserCtx(r)
	if !ok {
		uierrors.RenderUnauthorized(w, r, "/login")
		return
	}

	gid := chi.URLParam(r, "id")
	groupOID, err := primitive.ObjectIDFromHex(gid)
	if err != nil {
		uierrors.RenderForbidden(w, r, "Bad group id.", nav.ResolveBackURL(r, "/groups"))
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), metaShortTimeout)
	defer cancel()
	db := h.DB

	grpStore := groupstore.New(db)
	group, err := grpStore.GetByID(ctx, groupOID)
	if err == mongo.ErrNoDocuments {
		uierrors.RenderForbidden(w, r, "Group not found.", nav.ResolveBackURL(r, "/groups"))
		return
	}
	if err != nil {
		h.Log.Warn("group GetByID(view)", zap.Error(err))
		uierrors.RenderForbidden(w, r, "A database error occurred.", nav.ResolveBackURL(r, "/groups"))
		return
	}

	if !grouppolicy.CanManageGroup(ctx, db, r, group.ID) {
		uierrors.RenderForbidden(w, r, "You do not have access to this group.", nav.ResolveBackURL(r, "/groups"))
		return
	}

	var org models.Organization
	_ = db.Collection("organizations").FindOne(ctx, bson.M{"_id": group.OrganizationID}).Decode(&org)

	assigned, _ := fetchAssignedResourcesForGroup(ctx, db, group.ID)

	leadersCount, _ := db.Collection("group_memberships").CountDocuments(ctx, bson.M{
		"group_id": group.ID,
		"role":     "leader",
	})
	membersCount, _ := db.Collection("group_memberships").CountDocuments(ctx, bson.M{
		"group_id": group.ID,
		"role":     "member",
	})

	templates.Render(w, r, "group_view", groupViewData{
		Title:             "View Group",
		IsLoggedIn:        true,
		Role:              role,
		UserName:          uname,
		GroupID:           group.ID.Hex(),
		Name:              group.Name,
		Description:       group.Description,
		OrganizationName:  org.Name,
		LeadersCount:      int(leadersCount),
		MembersCount:      int(membersCount),
		CreatedAt:         group.CreatedAt,
		UpdatedAt:         group.UpdatedAt,
		AssignedResources: assigned,
		BackURL:           nav.ResolveBackURL(r, "/groups"),
		CurrentPath:       nav.CurrentPath(r),
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
	_ = cur.All(ctx, &assigns)
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
		_ = cg.Decode(&r)
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
