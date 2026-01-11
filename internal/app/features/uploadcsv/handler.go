// internal/app/features/uploadcsv/handler.go
package uploadcsv

// Terminology: User Identifiers
//   - UserID / userID / user_id: The MongoDB ObjectID (_id) that uniquely identifies a user record
//   - LoginID / loginID / login_id: The human-readable string users type to log in

import (
	"context"
	"errors"
	"net/http"

	uierrors "github.com/dalemusser/stratahub/internal/app/features/errors"
	"github.com/dalemusser/stratahub/internal/app/policy/grouppolicy"
	groupstore "github.com/dalemusser/stratahub/internal/app/store/groups"
	"github.com/dalemusser/stratahub/internal/app/system/authz"
	"github.com/dalemusser/stratahub/internal/app/system/normalize"
	"github.com/dalemusser/stratahub/internal/app/system/orgutil"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.uber.org/zap"
)

// Handler provides HTTP handlers for CSV upload operations.
type Handler struct {
	DB     *mongo.Database
	Log    *zap.Logger
	ErrLog *uierrors.ErrorLogger
}

// Authorization errors
var (
	ErrNotAuthenticated = errors.New("not authenticated")
	ErrForbiddenOrg     = errors.New("no access to organization")
	ErrForbiddenGroup   = errors.New("no access to group")
	ErrGroupOrgMismatch = errors.New("group does not belong to organization")
	ErrNoOrganization   = errors.New("no organization linked")
	ErrBadOrgID         = errors.New("invalid organization ID")
	ErrOrgNotFound      = errors.New("organization not found")
	ErrBadGroupID       = errors.New("invalid group ID")
	ErrGroupNotFound    = errors.New("group not found")
)

// resolveContext validates org/group access for the current user.
// It enforces authorization rules to prevent URL hacking:
//   - Leaders: org is always their org (orgHex param ignored), can only manage groups where they're a leader
//   - Coordinators: can only access orgs in their coordinator_assignments
//   - Admins: can access any org/group
func (h *Handler) resolveContext(ctx context.Context, r *http.Request, role string, uid primitive.ObjectID, orgHex, groupHex string, groupMode bool) (*UploadContext, error) {
	db := h.DB
	uc := &UploadContext{
		Role:      role,
		UserID:    uid,
		GroupMode: groupMode,
	}

	// 1. Resolve and validate organization
	if role == "leader" {
		// Leaders: force their org, ignore orgHex param
		orgID, orgName, err := orgutil.ResolveLeaderOrg(ctx, db, uid)
		if errors.Is(err, orgutil.ErrUserNotFound) || errors.Is(err, orgutil.ErrNoOrganization) {
			return nil, ErrNoOrganization
		}
		if err != nil {
			return nil, err
		}
		uc.OrgID = orgID
		uc.OrgName = orgName
		uc.OrgLocked = true
	} else {
		// Admin/Coordinator: use provided orgHex
		orgHex = normalize.OrgID(orgHex)
		if orgHex == "" || orgHex == "all" {
			// Org not specified - this is OK for initial form display
			// but will be rejected when submitting
			return uc, nil
		}

		orgID, orgName, err := orgutil.ResolveActiveOrgFromHex(ctx, db, orgHex)
		if errors.Is(err, orgutil.ErrBadOrgID) {
			return nil, ErrBadOrgID
		}
		if errors.Is(err, orgutil.ErrOrgNotFound) || errors.Is(err, orgutil.ErrOrgNotActive) {
			return nil, ErrOrgNotFound
		}
		if err != nil {
			return nil, err
		}

		// Coordinator access check: verify org is in their assigned list
		if role == "coordinator" && !authz.CanAccessOrg(r, orgID) {
			return nil, ErrForbiddenOrg
		}

		uc.OrgID = orgID
		uc.OrgName = orgName
		uc.OrgLocked = false
	}

	// 2. Resolve and validate group (if group mode with group specified)
	if groupMode && groupHex != "" {
		groupOID, err := primitive.ObjectIDFromHex(groupHex)
		if err != nil {
			return nil, ErrBadGroupID
		}

		group, err := groupstore.New(db).GetByID(ctx, groupOID)
		if err == mongo.ErrNoDocuments {
			return nil, ErrGroupNotFound
		}
		if err != nil {
			return nil, err
		}

		// Verify group belongs to the resolved org
		if uc.OrgID != primitive.NilObjectID && group.OrganizationID != uc.OrgID {
			return nil, ErrGroupOrgMismatch
		}

		// If org wasn't set yet (shouldn't happen but be safe), set it from group
		if uc.OrgID == primitive.NilObjectID {
			orgName, err := orgutil.GetOrgName(ctx, db, group.OrganizationID)
			if err != nil {
				return nil, err
			}
			uc.OrgID = group.OrganizationID
			uc.OrgName = orgName
		}

		// Verify user can manage this group
		canManage, err := grouppolicy.CanManageGroup(ctx, db, r, group.ID, group.OrganizationID)
		if err != nil {
			return nil, err
		}
		if !canManage {
			return nil, ErrForbiddenGroup
		}

		uc.GroupID = group.ID
		uc.GroupName = group.Name
		uc.GroupLocked = true // Group is specified, so it's locked
	}

	return uc, nil
}
