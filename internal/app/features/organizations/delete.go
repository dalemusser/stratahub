// internal/app/features/organizations/delete.go
package organizations

import (
	"context"
	"net/http"

	uierrors "github.com/dalemusser/stratahub/internal/app/features/errors"
	groupstore "github.com/dalemusser/stratahub/internal/app/store/groups"
	materialassignstore "github.com/dalemusser/stratahub/internal/app/store/materialassign"
	membershipstore "github.com/dalemusser/stratahub/internal/app/store/memberships"
	organizationstore "github.com/dalemusser/stratahub/internal/app/store/organizations"
	resourceassignstore "github.com/dalemusser/stratahub/internal/app/store/resourceassign"
	userstore "github.com/dalemusser/stratahub/internal/app/store/users"
	"github.com/dalemusser/stratahub/internal/app/system/navigation"
	"github.com/dalemusser/stratahub/internal/app/system/timeouts"
	"github.com/dalemusser/stratahub/internal/app/system/txn"
	"github.com/go-chi/chi/v5"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.uber.org/zap"
)

// HandleDelete deletes an organization and all related documents (users, groups,
// memberships, assignments) in a transaction. If transactions are unavailable
// (no replica set), falls back to sequential deletes with best-effort cleanup.
// Authorization: RequireRole("admin") middleware in routes.go ensures only admins reach this handler.
func (h *Handler) HandleDelete(w http.ResponseWriter, r *http.Request) {
	idHex := chi.URLParam(r, "id")
	oid, err := primitive.ObjectIDFromHex(idHex)
	if err != nil {
		uierrors.RenderBadRequest(w, r, "Invalid organization ID.", "/organizations")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), timeouts.Long())
	defer cancel()

	// Use txn.Run for atomic cascading delete with automatic fallback.
	if err := txn.Run(ctx, h.DB, h.Log, func(ctx context.Context) error {
		return h.cascadeDeleteOrg(ctx, oid)
	}); err != nil {
		h.ErrLog.LogServerError(w, r, "delete organization failed", err, "Failed to delete organization. Please try again.", "/organizations")
		return
	}

	h.Log.Info("organization deleted with cascading cleanup", zap.String("org_id", idHex))

	ret := navigation.SafeBackURL(r, navigation.OrganizationsBackURL)
	http.Redirect(w, r, ret, http.StatusSeeOther)
}

// cascadeDeleteOrg deletes all documents related to an organization in the correct order:
// 1. group_resource_assignments (references groups)
// 2. material_assignments (references organization)
// 3. group_memberships (references groups and users)
// 4. groups (references organization)
// 5. users (references organization)
// 6. organization itself
func (h *Handler) cascadeDeleteOrg(ctx context.Context, orgID primitive.ObjectID) error {
	db := h.DB
	idHex := orgID.Hex()

	rasStore := resourceassignstore.New(db)
	masStore := materialassignstore.New(db)
	memStore := membershipstore.New(db)
	grpStore := groupstore.New(db)
	usrStore := userstore.New(db)
	orgStore := organizationstore.New(db)

	// 1. Delete all resource assignments for this organization.
	if cnt, err := rasStore.DeleteByOrg(ctx, orgID); err != nil {
		h.Log.Error("failed to delete group_resource_assignments", zap.Error(err), zap.String("org_id", idHex))
		return err
	} else if cnt > 0 {
		h.Log.Debug("deleted group_resource_assignments", zap.Int64("count", cnt), zap.String("org_id", idHex))
	}

	// 2. Delete all material assignments for this organization.
	if cnt, err := masStore.DeleteByOrg(ctx, orgID); err != nil {
		h.Log.Error("failed to delete material_assignments", zap.Error(err), zap.String("org_id", idHex))
		return err
	} else if cnt > 0 {
		h.Log.Debug("deleted material_assignments", zap.Int64("count", cnt), zap.String("org_id", idHex))
	}

	// 3. Delete all group memberships for this organization.
	if cnt, err := memStore.DeleteByOrg(ctx, orgID); err != nil {
		h.Log.Error("failed to delete group_memberships", zap.Error(err), zap.String("org_id", idHex))
		return err
	} else if cnt > 0 {
		h.Log.Debug("deleted group_memberships", zap.Int64("count", cnt), zap.String("org_id", idHex))
	}

	// 4. Delete all groups for this organization.
	if cnt, err := grpStore.DeleteByOrg(ctx, orgID); err != nil {
		h.Log.Error("failed to delete groups", zap.Error(err), zap.String("org_id", idHex))
		return err
	} else if cnt > 0 {
		h.Log.Debug("deleted groups", zap.Int64("count", cnt), zap.String("org_id", idHex))
	}

	// 5. Delete all users for this organization.
	// Note: Only deletes users with role "member" or "leader" that belong to this org.
	// System users (admin/analyst) don't have organization_id set.
	if cnt, err := usrStore.DeleteByOrg(ctx, orgID); err != nil {
		h.Log.Error("failed to delete users", zap.Error(err), zap.String("org_id", idHex))
		return err
	} else if cnt > 0 {
		h.Log.Debug("deleted users", zap.Int64("count", cnt), zap.String("org_id", idHex))
	}

	// 6. Delete the organization itself.
	cnt, err := orgStore.Delete(ctx, orgID)
	if err != nil {
		h.Log.Error("failed to delete organization", zap.Error(err), zap.String("org_id", idHex))
		return err
	}
	if cnt == 0 {
		h.Log.Info("organization delete: no document found (idempotent)", zap.String("org_id", idHex))
	}

	return nil
}
