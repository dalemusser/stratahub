// internal/app/features/organizations/delete.go
package organizations

import (
	"context"
	"net/http"

	uierrors "github.com/dalemusser/stratahub/internal/app/features/errors"
	"github.com/dalemusser/stratahub/internal/app/system/navigation"
	"github.com/dalemusser/stratahub/internal/app/system/timeouts"
	"github.com/dalemusser/stratahub/internal/app/system/txn"
	"github.com/go-chi/chi/v5"
	"go.mongodb.org/mongo-driver/bson"
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
// 2. group_memberships (references groups and users)
// 3. groups (references organization)
// 4. users (references organization)
// 5. organization itself
func (h *Handler) cascadeDeleteOrg(ctx context.Context, orgID primitive.ObjectID) error {
	db := h.DB
	idHex := orgID.Hex()

	// 1. Delete all resource assignments for this organization.
	if res, err := db.Collection("group_resource_assignments").DeleteMany(ctx, bson.M{"organization_id": orgID}); err != nil {
		h.Log.Error("failed to delete group_resource_assignments", zap.Error(err), zap.String("org_id", idHex))
		return err
	} else if res.DeletedCount > 0 {
		h.Log.Debug("deleted group_resource_assignments", zap.Int64("count", res.DeletedCount), zap.String("org_id", idHex))
	}

	// 2. Delete all group memberships for this organization.
	if res, err := db.Collection("group_memberships").DeleteMany(ctx, bson.M{"org_id": orgID}); err != nil {
		h.Log.Error("failed to delete group_memberships", zap.Error(err), zap.String("org_id", idHex))
		return err
	} else if res.DeletedCount > 0 {
		h.Log.Debug("deleted group_memberships", zap.Int64("count", res.DeletedCount), zap.String("org_id", idHex))
	}

	// 3. Delete all groups for this organization.
	if res, err := db.Collection("groups").DeleteMany(ctx, bson.M{"organization_id": orgID}); err != nil {
		h.Log.Error("failed to delete groups", zap.Error(err), zap.String("org_id", idHex))
		return err
	} else if res.DeletedCount > 0 {
		h.Log.Debug("deleted groups", zap.Int64("count", res.DeletedCount), zap.String("org_id", idHex))
	}

	// 4. Delete all users for this organization.
	// Note: Only deletes users with role "member" or "leader" that belong to this org.
	// System users (admin/analyst) don't have organization_id set.
	if res, err := db.Collection("users").DeleteMany(ctx, bson.M{"organization_id": orgID}); err != nil {
		h.Log.Error("failed to delete users", zap.Error(err), zap.String("org_id", idHex))
		return err
	} else if res.DeletedCount > 0 {
		h.Log.Debug("deleted users", zap.Int64("count", res.DeletedCount), zap.String("org_id", idHex))
	}

	// 5. Delete the organization itself.
	res, err := db.Collection("organizations").DeleteOne(ctx, bson.M{"_id": orgID})
	if err != nil {
		h.Log.Error("failed to delete organization", zap.Error(err), zap.String("org_id", idHex))
		return err
	}
	if res.DeletedCount == 0 {
		h.Log.Info("organization delete: no document found (idempotent)", zap.String("org_id", idHex))
	}

	return nil
}
