// internal/app/features/groups/manageleaders.go
package groups

import (
	"context"
	"net/http"

	uierrors "github.com/dalemusser/stratahub/internal/app/features/errors"
	"github.com/dalemusser/stratahub/internal/app/policy/grouppolicy"
	groupstore "github.com/dalemusser/stratahub/internal/app/store/groups"
	membershipstore "github.com/dalemusser/stratahub/internal/app/store/memberships"
	userstore "github.com/dalemusser/stratahub/internal/app/store/users"
	"github.com/dalemusser/stratahub/internal/app/system/authz"
	"github.com/dalemusser/stratahub/internal/app/system/timeouts"
	"github.com/dalemusser/waffle/pantry/templates"
	"github.com/go-chi/chi/v5"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
)

// HandleAddLeader adds a leader to the group.
func (h *Handler) HandleAddLeader(w http.ResponseWriter, r *http.Request) {
	gid := chi.URLParam(r, "id")
	targetHex := r.FormValue("userID")

	groupOID, err := primitive.ObjectIDFromHex(gid)
	if err != nil {
		uierrors.HTMXBadRequest(w, r, "Invalid group ID.", "/groups")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), timeouts.Medium())
	defer cancel()
	db := h.DB

	group, err := groupstore.New(db).GetByID(ctx, groupOID)
	if err == mongo.ErrNoDocuments {
		uierrors.HTMXError(w, r, http.StatusNotFound, "Group not found.", func() {
			uierrors.RenderNotFound(w, r, "Group not found.", "/groups")
		})
		return
	}
	if err != nil {
		h.ErrLog.HTMXLogServerError(w, r, "database error loading group", err, "A database error occurred.", "/groups")
		return
	}
	canManage, policyErr := grouppolicy.CanManageGroup(ctx, db, r, group.ID, group.OrganizationID)
	if policyErr != nil {
		h.ErrLog.HTMXLogServerError(w, r, "database error checking group access", policyErr, "A database error occurred.", "/groups")
		return
	}
	if !canManage {
		uierrors.HTMXForbidden(w, r, "You don't have permission to manage this group.", "/groups")
		return
	}

	targetOID, err := primitive.ObjectIDFromHex(targetHex)
	if err != nil {
		uierrors.HTMXBadRequest(w, r, "Invalid leader ID.", "/groups")
		return
	}

	// Verify leader exists and belongs to same organization
	usrStore := userstore.New(db)
	if _, err := usrStore.GetLeaderInOrg(ctx, targetOID, group.OrganizationID); err != nil {
		uierrors.HTMXBadRequest(w, r, "Leader must be from the same organization.", "/groups")
		return
	}

	if err := membershipstore.New(db).Add(ctx, group.ID, targetOID, "leader"); err != nil {
		h.ErrLog.HTMXLogBadRequest(w, r, "database error adding leader to group", err, "Failed to add leader.", "/groups")
		return
	}

	h.renderLeadersPartial(w, r, gid)
}

// HandleRemoveLeader removes a leader from the group.
func (h *Handler) HandleRemoveLeader(w http.ResponseWriter, r *http.Request) {
	_, _, uid, ok := authz.UserCtx(r)
	if !ok {
		uierrors.HTMXError(w, r, http.StatusUnauthorized, "Unauthorized.", func() {
			uierrors.RenderUnauthorized(w, r, "/login")
		})
		return
	}
	gid := chi.URLParam(r, "id")
	targetHex := r.FormValue("userID")

	groupOID, err := primitive.ObjectIDFromHex(gid)
	if err != nil {
		uierrors.HTMXBadRequest(w, r, "Invalid group ID.", "/groups")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), timeouts.Medium())
	defer cancel()
	db := h.DB

	group, err := groupstore.New(db).GetByID(ctx, groupOID)
	if err == mongo.ErrNoDocuments {
		uierrors.HTMXError(w, r, http.StatusNotFound, "Group not found.", func() {
			uierrors.RenderNotFound(w, r, "Group not found.", "/groups")
		})
		return
	}
	if err != nil {
		h.ErrLog.HTMXLogServerError(w, r, "database error loading group", err, "A database error occurred.", "/groups")
		return
	}
	canManage, policyErr := grouppolicy.CanManageGroup(ctx, db, r, group.ID, group.OrganizationID)
	if policyErr != nil {
		h.ErrLog.HTMXLogServerError(w, r, "database error checking group access", policyErr, "A database error occurred.", "/groups")
		return
	}
	if !canManage {
		uierrors.HTMXForbidden(w, r, "You don't have permission to manage this group.", "/groups")
		return
	}

	targetOID, err := primitive.ObjectIDFromHex(targetHex)
	if err != nil {
		uierrors.HTMXBadRequest(w, r, "Invalid leader ID.", "/groups")
		return
	}

	if uid == targetOID {
		uierrors.HTMXBadRequest(w, r, "You cannot remove yourself as leader.", "/groups")
		return
	}

	memStore := membershipstore.New(db)
	cnt, cntErr := memStore.CountByGroup(ctx, group.ID, "leader")
	if cntErr != nil {
		h.ErrLog.HTMXLogServerError(w, r, "database error counting group leaders", cntErr, "A database error occurred.", "/groups")
		return
	}
	if cnt <= 1 {
		uierrors.HTMXBadRequest(w, r, "Cannot remove the last leader.", "/groups")
		return
	}

	if err := membershipstore.New(db).Remove(ctx, group.ID, targetOID); err != nil {
		h.ErrLog.HTMXLogBadRequest(w, r, "database error removing leader from group", err, "Failed to remove leader.", "/groups")
		return
	}
	h.renderLeadersPartial(w, r, gid)
}

// renderLeadersPartial re-renders just the leaders block as a snippet.
func (h *Handler) renderLeadersPartial(w http.ResponseWriter, r *http.Request, gid string) {
	data, err := h.buildPageData(r, gid, "", "", "")
	if err != nil {
		h.ErrLog.HTMXLogServerError(w, r, "error building group page data", err, "Failed to load group data.", "/groups")
		return
	}
	templates.RenderSnippet(w, "group_leaders_contents", data)
}
