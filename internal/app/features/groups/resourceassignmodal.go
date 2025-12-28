// internal/app/features/groups/resourceassignmodal.go
package groups

import (
	"context"
	"net/http"

	uierrors "github.com/dalemusser/stratahub/internal/app/features/errors"
	"github.com/dalemusser/stratahub/internal/app/policy/grouppolicy"
	groupstore "github.com/dalemusser/stratahub/internal/app/store/groups"
	resourceassignstore "github.com/dalemusser/stratahub/internal/app/store/resourceassign"
	resourcestore "github.com/dalemusser/stratahub/internal/app/store/resources"
	"github.com/dalemusser/stratahub/internal/app/system/authz"
	"github.com/dalemusser/stratahub/internal/app/system/timeouts"
	"github.com/dalemusser/waffle/pantry/httpnav"
	"github.com/dalemusser/waffle/pantry/templates"
	"github.com/dalemusser/waffle/pantry/urlutil"
	"github.com/go-chi/chi/v5"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.uber.org/zap"
)

// ServeAssignResourceModal renders the modal fragment used to configure a
// single resource assignment before it is added to the group.
func (h *Handler) ServeAssignResourceModal(w http.ResponseWriter, r *http.Request) {
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

	group, err := groupstore.New(db).GetByID(ctx, groupOID)
	if err == mongo.ErrNoDocuments {
		uierrors.RenderForbidden(w, r, "Group not found.", httpnav.ResolveBackURL(r, "/groups"))
		return
	}
	if err != nil {
		h.Log.Warn("group GetByID(assign modal)", zap.Error(err))
		uierrors.RenderForbidden(w, r, "A database error occurred.", httpnav.ResolveBackURL(r, "/groups"))
		return
	}

	canManage, policyErr := grouppolicy.CanManageGroup(ctx, db, r, group.ID, group.OrganizationID)
	if policyErr != nil {
		h.ErrLog.HTMXLogServerError(w, r, "database error checking group access", policyErr, "A database error occurred.", "/groups")
		return
	}
	if !canManage {
		uierrors.RenderForbidden(w, r, "You do not have access to this group.", httpnav.ResolveBackURL(r, "/groups"))
		return
	}

	back := urlutil.SafeReturn(r.FormValue("return"), "", httpnav.ResolveBackURL(r, "/groups/"+group.ID.Hex()+"/assign_resources"))

	mode := r.FormValue("mode")
	if mode != "manage" {
		uierrors.RenderForbidden(w, r, "Unsupported operation.", back)
		return
	}

	// Manage an existing assignment: load by assignmentID.
	assignHex := r.FormValue("assignmentID")
	assignID, err := primitive.ObjectIDFromHex(assignHex)
	if err != nil {
		uierrors.RenderForbidden(w, r, "Bad assignment id.", back)
		return
	}

	asn, err := resourceassignstore.New(db).GetByID(ctx, assignID)
	if err == mongo.ErrNoDocuments {
		uierrors.RenderForbidden(w, r, "Assignment not found.", back)
		return
	}
	if err != nil {
		h.Log.Warn("assignment GetByID(manage modal)", zap.Error(err))
		uierrors.RenderForbidden(w, r, "A database error occurred.", back)
		return
	}
	if asn.GroupID != group.ID {
		uierrors.RenderForbidden(w, r, "Assignment does not belong to this group.", back)
		return
	}

	resStore := resourcestore.New(db)
	res, err := resStore.GetByID(ctx, asn.ResourceID)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			uierrors.RenderForbidden(w, r, "Resource not found.", back)
			return
		}
		h.Log.Warn("resource FindOne(manage modal)", zap.Error(err))
		uierrors.RenderForbidden(w, r, "A database error occurred.", back)
		return
	}

	manageVM := manageAssignmentModalVM{
		AssignmentID:  asn.ID.Hex(),
		GroupID:       group.ID.Hex(),
		GroupName:     group.Name,
		ResourceID:    res.ID.Hex(),
		ResourceTitle: res.Title,
		ResourceType:  res.Type,
		BackURL:       back,
	}

	templates.RenderSnippet(w, "group_manage_assignment_modal", manageVM)
}
