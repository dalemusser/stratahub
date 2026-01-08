package leaders

import (
	"context"
	"net/http"

	uierrors "github.com/dalemusser/stratahub/internal/app/features/errors"
	materialassignstore "github.com/dalemusser/stratahub/internal/app/store/materialassign"
	"github.com/dalemusser/stratahub/internal/app/system/authz"
	"github.com/dalemusser/stratahub/internal/app/system/navigation"
	"github.com/dalemusser/stratahub/internal/app/system/timeouts"
	"github.com/dalemusser/stratahub/internal/app/system/txn"
	"github.com/dalemusser/stratahub/internal/app/system/workspace"
	"github.com/dalemusser/stratahub/internal/domain/models"
	"github.com/go-chi/chi/v5"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

// HandleDelete removes a leader and all of their group memberships and
// material assignments. It is mounted on POST /leaders/{id}/delete.
func (h *Handler) HandleDelete(w http.ResponseWriter, r *http.Request) {
	actorRole, _, actorID, ok := authz.UserCtx(r)
	if !ok {
		uierrors.RenderUnauthorized(w, r, "/login")
		return
	}

	uidHex := chi.URLParam(r, "id")
	uid, err := primitive.ObjectIDFromHex(uidHex)
	if err != nil {
		uierrors.RenderBadRequest(w, r, "Invalid leader ID.", "/leaders")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), timeouts.Medium())
	defer cancel()

	// Load leader to check organization for coordinator access
	var usr models.User
	if err := h.DB.Collection("users").FindOne(ctx, bson.M{"_id": uid, "role": "leader"}).Decode(&usr); err != nil {
		uierrors.RenderNotFound(w, r, "Leader not found.", "/leaders")
		return
	}

	// Verify workspace ownership (prevent cross-workspace deletion)
	wsID := workspace.IDFromRequest(r)
	if wsID != primitive.NilObjectID && (usr.WorkspaceID == nil || *usr.WorkspaceID != wsID) {
		uierrors.RenderNotFound(w, r, "Leader not found.", "/leaders")
		return
	}

	// Coordinator access check: verify access to leader's organization
	if authz.IsCoordinator(r) && usr.OrganizationID != nil {
		if !authz.CanAccessOrg(r, *usr.OrganizationID) {
			uierrors.RenderForbidden(w, r, "You don't have access to this leader.", "/leaders")
			return
		}
	}

	masStore := materialassignstore.New(h.DB)

	// Use transaction for atomic deletion of memberships, material assignments, and leader.
	if err := txn.Run(ctx, h.DB, h.Log, func(ctx context.Context) error {
		// 1) Remove ALL memberships for this user (defensive: leader/member)
		if _, err := h.DB.Collection("group_memberships").DeleteMany(ctx, bson.M{"user_id": uid}); err != nil {
			return err
		}
		// 2) Remove material assignments for this leader
		if _, err := masStore.DeleteByLeader(ctx, uid); err != nil {
			return err
		}
		// 3) Delete the user (role: leader)
		if _, err := h.DB.Collection("users").DeleteOne(ctx, bson.M{"_id": uid, "role": "leader"}); err != nil {
			return err
		}
		return nil
	}); err != nil {
		uierrors.RenderServerError(w, r, "Failed to delete leader.", "/leaders")
		return
	}

	// Audit log: leader deleted
	h.AuditLog.UserDeleted(ctx, r, actorID, uid, usr.OrganizationID, actorRole, "leader")

	// Optional return parameter, otherwise send back to leaders list.
	ret := navigation.SafeBackURL(r, navigation.LeadersBackURL)
	http.Redirect(w, r, ret, http.StatusSeeOther)
}
