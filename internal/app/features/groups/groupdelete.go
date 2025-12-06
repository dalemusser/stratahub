// internal/app/features/groups/groupdelete.go
package groups

import (
	"context"
	"net/http"

	uierrors "github.com/dalemusser/stratahub/internal/app/features/errors"
	"github.com/dalemusser/stratahub/internal/app/policy/grouppolicy"
	groupstore "github.com/dalemusser/stratahub/internal/app/store/groups"
	"github.com/dalemusser/stratahub/internal/app/system/authz"
	nav "github.com/dalemusser/waffle/toolkit/ui/nav"
	"github.com/go-chi/chi/v5"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.uber.org/zap"
)

// HandleDeleteGroup handles deleting a group (admin only).
func (h *Handler) HandleDeleteGroup(w http.ResponseWriter, r *http.Request) {
	role, _, _, ok := authz.UserCtx(r)
	if !ok {
		uierrors.RenderUnauthorized(w, r, "/login")
		return
	}
	// Admin only delete (safer). Change later if you want leader-of-group delete.
	if role != "admin" {
		uierrors.RenderForbidden(w, r, "You do not have access to delete groups.", nav.ResolveBackURL(r, "/groups"))
		return
	}

	gid := chi.URLParam(r, "id")
	groupOID, err := primitive.ObjectIDFromHex(gid)
	if err != nil {
		uierrors.RenderForbidden(w, r, "Bad group id.", nav.ResolveBackURL(r, "/groups"))
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), metaMedTimeout)
	defer cancel()
	db := h.DB

	// Verify group exists and that policy would allow manage (defensive).
	grpStore := groupstore.New(db)
	group, err := grpStore.GetByID(ctx, groupOID)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			uierrors.RenderForbidden(w, r, "Group not found.", nav.ResolveBackURL(r, "/groups"))
			return
		}
		h.Log.Warn("GetByID(delete)", zap.Error(err))
		uierrors.RenderForbidden(w, r, "A database error occurred.", nav.ResolveBackURL(r, "/groups"))
		return
	}

	if !grouppolicy.CanManageGroup(ctx, db, r, group.ID) {
		uierrors.RenderForbidden(w, r, "You do not have access to this group.", nav.ResolveBackURL(r, "/groups"))
		return
	}

	// Remove all memberships and assignments for this group.
	_, _ = db.Collection("group_memberships").DeleteMany(ctx, bson.M{"group_id": groupOID})
	_, _ = db.Collection("group_resource_assignments").DeleteMany(ctx, bson.M{"group_id": groupOID})

	// Finally delete the group itself.
	if _, err := db.Collection("groups").DeleteOne(ctx, bson.M{"_id": groupOID}); err != nil {
		h.Log.Warn("delete group failed", zap.Error(err))
		uierrors.RenderForbidden(w, r, "Delete failed.", nav.ResolveBackURL(r, "/groups"))
		return
	}

	// Redirect back to caller or to /groups.
	if ret := r.FormValue("return"); ret != "" && len(ret) > 0 && ret[0] == '/' {
		http.Redirect(w, r, ret, http.StatusSeeOther)
		return
	}
	http.Redirect(w, r, "/groups", http.StatusSeeOther)
}
