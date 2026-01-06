// internal/app/features/groups/groupdelete.go
package groups

import (
	"context"
	"net/http"

	uierrors "github.com/dalemusser/stratahub/internal/app/features/errors"
	"github.com/dalemusser/stratahub/internal/app/policy/grouppolicy"
	groupstore "github.com/dalemusser/stratahub/internal/app/store/groups"
	membershipstore "github.com/dalemusser/stratahub/internal/app/store/memberships"
	resourceassignstore "github.com/dalemusser/stratahub/internal/app/store/resourceassign"
	"github.com/dalemusser/stratahub/internal/app/system/authz"
	"github.com/dalemusser/stratahub/internal/app/system/navigation"
	"github.com/dalemusser/stratahub/internal/app/system/timeouts"
	"github.com/dalemusser/stratahub/internal/app/system/txn"
	"github.com/dalemusser/waffle/pantry/httpnav"
	"github.com/go-chi/chi/v5"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.uber.org/zap"
)

// HandleDeleteGroup handles deleting a group (admin and coordinator).
func (h *Handler) HandleDeleteGroup(w http.ResponseWriter, r *http.Request) {
	role, _, actorID, ok := authz.UserCtx(r)
	if !ok {
		uierrors.RenderUnauthorized(w, r, "/login")
		return
	}
	// Admin and coordinator can delete groups (coordinators are restricted to their orgs via policy check below)
	if role != "admin" && role != "coordinator" {
		uierrors.RenderForbidden(w, r, "You do not have access to delete groups.", httpnav.ResolveBackURL(r, "/groups"))
		return
	}

	gid := chi.URLParam(r, "id")
	groupOID, err := primitive.ObjectIDFromHex(gid)
	if err != nil {
		uierrors.RenderForbidden(w, r, "Bad group id.", httpnav.ResolveBackURL(r, "/groups"))
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), timeouts.Medium())
	defer cancel()
	db := h.DB

	// Verify group exists and that policy would allow manage (defensive).
	grpStore := groupstore.New(db)
	group, err := grpStore.GetByID(ctx, groupOID)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			uierrors.RenderForbidden(w, r, "Group not found.", httpnav.ResolveBackURL(r, "/groups"))
			return
		}
		h.Log.Warn("GetByID(delete)", zap.Error(err))
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

	// Use transaction for atomic deletion of group and related data.
	memStore := membershipstore.New(db)
	rasStore := resourceassignstore.New(db)
	if err := txn.Run(ctx, db, h.Log, func(ctx context.Context) error {
		// 1) Remove all memberships for this group.
		if _, err := memStore.DeleteByGroup(ctx, groupOID); err != nil {
			return err
		}
		// 2) Remove all resource assignments for this group.
		if _, err := rasStore.DeleteByGroup(ctx, groupOID); err != nil {
			return err
		}
		// 3) Delete the group itself.
		if _, err := grpStore.Delete(ctx, groupOID); err != nil {
			return err
		}
		return nil
	}); err != nil {
		h.ErrLog.LogServerError(w, r, "database error deleting group", err, "Failed to delete group.", "/groups")
		return
	}

	// Audit log: group deleted
	h.AuditLog.GroupDeleted(ctx, r, actorID, groupOID, &group.OrganizationID, role, group.Name)

	// Redirect back to caller or to /groups.
	ret := navigation.SafeBackURL(r, navigation.GroupsBackURL)
	http.Redirect(w, r, ret, http.StatusSeeOther)
}
