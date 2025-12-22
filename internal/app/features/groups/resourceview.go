// internal/app/features/groups/resourceview.go
package groups

import (
	"context"
	"net/http"

	uierrors "github.com/dalemusser/stratahub/internal/app/features/errors"
	"github.com/dalemusser/stratahub/internal/app/policy/grouppolicy"
	groupstore "github.com/dalemusser/stratahub/internal/app/store/groups"
	"github.com/dalemusser/stratahub/internal/app/system/authz"
	"github.com/dalemusser/stratahub/internal/app/system/timeouts"
	"github.com/dalemusser/stratahub/internal/domain/models"
	"github.com/dalemusser/waffle/pantry/templates"
	"github.com/dalemusser/waffle/pantry/httpnav"
	"github.com/go-chi/chi/v5"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.uber.org/zap"
)

// ServeGroupResourceView renders a read-only view of a resource in the
// context of a group.
func (h *Handler) ServeGroupResourceView(w http.ResponseWriter, r *http.Request) {
	role, uname, _, ok := authz.UserCtx(r)
	if !ok {
		uierrors.RenderUnauthorized(w, r, "/login")
		return
	}

	gid := chi.URLParam(r, "id")
	resourceID := chi.URLParam(r, "resourceID")

	ctx, cancel := context.WithTimeout(r.Context(), timeouts.Short())
	defer cancel()
	db := h.DB

	groupOID, err := primitive.ObjectIDFromHex(gid)
	if err != nil {
		uierrors.RenderForbidden(w, r, "Bad group id.", httpnav.ResolveBackURL(r, "/groups"))
		return
	}

	grpStore := groupstore.New(db)
	group, err := grpStore.GetByID(ctx, groupOID)
	if err == mongo.ErrNoDocuments {
		uierrors.RenderForbidden(w, r, "Group not found.", httpnav.ResolveBackURL(r, "/groups"))
		return
	}
	if err != nil {
		h.Log.Warn("group GetByID(resource view)", zap.Error(err))
		uierrors.RenderForbidden(w, r, "A database error occurred.", httpnav.ResolveBackURL(r, "/groups"))
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

	resourceOID, err := primitive.ObjectIDFromHex(resourceID)
	if err != nil {
		uierrors.RenderForbidden(w, r, "Bad resource id.", httpnav.ResolveBackURL(r, "/groups"))
		return
	}

	var res models.Resource
	if err := db.Collection("resources").FindOne(ctx, bson.M{"_id": resourceOID}).Decode(&res); err != nil {
		if err == mongo.ErrNoDocuments {
			uierrors.RenderForbidden(w, r, "Resource not found.", httpnav.ResolveBackURL(r, "/groups"))
			return
		}
		h.Log.Warn("resource FindOne(resource view)", zap.Error(err))
		uierrors.RenderForbidden(w, r, "A database error occurred.", httpnav.ResolveBackURL(r, "/groups"))
		return
	}

	templates.Render(w, r, "group_resource_view", groupResourceViewData{
		Title:         "View Resource",
		IsLoggedIn:    true,
		Role:          role,
		UserName:      uname,
		GroupID:       group.ID.Hex(),
		GroupName:     group.Name,
		ResourceID:    res.ID.Hex(),
		ResourceTitle: res.Title,
		Subject:       res.Subject,
		Description:   res.Description,
		Status:        res.Status,
		LaunchURL:     res.LaunchURL,
		BackURL:       httpnav.ResolveBackURL(r, "/groups/"+gid+"/assign_resources"),
		CurrentPath:   httpnav.CurrentPath(r),
	})
}
