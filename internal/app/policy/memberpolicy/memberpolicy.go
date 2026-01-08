// Package memberpolicy provides authorization policies for member management.
//
// Authorization rules:
//   - Admins can view and manage all members across all organizations
//   - Coordinators can view and manage members within their assigned organizations
//   - Leaders can only view and manage members within their own organization
//   - Other roles (analyst, member) cannot access member management
package memberpolicy

import (
	"context"
	"net/http"

	"github.com/dalemusser/stratahub/internal/app/system/authz"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// MemberInfo contains the minimal member data needed for authorization checks.
type MemberInfo struct {
	ID             primitive.ObjectID
	OrganizationID *primitive.ObjectID
}

// ListScope represents the scope of members a user can list.
type ListScope struct {
	// CanList indicates whether the user can list members at all.
	CanList bool
	// AllOrgs indicates whether the user can see members from all organizations.
	// If false, check OrgID (single org) or OrgIDs (multiple orgs).
	AllOrgs bool
	// OrgID is the organization ID the user is restricted to (for leaders).
	OrgID primitive.ObjectID
	// OrgIDs is the list of organization IDs the user can access (for coordinators).
	OrgIDs []primitive.ObjectID
}

// CanListMembers determines what scope of members the current user can list.
// Returns a ListScope indicating access level.
//
// Authorization:
//   - Admin: can list all members from all organizations
//   - Coordinator: can list members from their assigned organizations
//   - Leader: can only list members from their own organization
//   - Others: cannot list members
func CanListMembers(r *http.Request) ListScope {
	role, _, _, ok := authz.UserCtx(r)
	if !ok {
		return ListScope{CanList: false}
	}

	switch role {
	case "superadmin", "admin":
		return ListScope{CanList: true, AllOrgs: true}
	case "coordinator":
		orgIDs := authz.UserOrgIDs(r)
		if len(orgIDs) == 0 {
			return ListScope{CanList: false}
		}
		return ListScope{CanList: true, AllOrgs: false, OrgIDs: orgIDs}
	case "leader":
		orgID := authz.UserOrgID(r)
		if orgID == primitive.NilObjectID {
			return ListScope{CanList: false}
		}
		return ListScope{CanList: true, AllOrgs: false, OrgID: orgID}
	default:
		return ListScope{CanList: false}
	}
}

// CanViewMember reports whether the current user can view the specified member.
//
// Authorization:
//   - Admin: can view any member
//   - Coordinator: can view members in their assigned organizations
//   - Leader: can only view members in their organization
//   - Others: cannot view members
//
// Returns an error only if a database operation fails.
func CanViewMember(ctx context.Context, db *mongo.Database, r *http.Request, memberOrgID *primitive.ObjectID) (bool, error) {
	role, _, _, ok := authz.UserCtx(r)
	if !ok {
		return false, nil
	}

	switch role {
	case "superadmin", "admin":
		return true, nil
	case "coordinator":
		if memberOrgID == nil {
			return false, nil
		}
		// Coordinator can view members in any of their assigned organizations
		return authz.CanAccessOrg(r, *memberOrgID), nil
	case "leader":
		userOrgID := authz.UserOrgID(r)
		if userOrgID == primitive.NilObjectID {
			return false, nil
		}
		// Leader can only view members in their organization
		if memberOrgID == nil {
			return false, nil
		}
		return userOrgID == *memberOrgID, nil
	default:
		return false, nil
	}
}

// CanManageMember reports whether the current user can edit or delete the specified member.
// This has the same authorization rules as CanViewMember.
//
// Authorization:
//   - Admin: can manage any member
//   - Coordinator: can manage members in their assigned organizations
//   - Leader: can only manage members in their organization
//   - Others: cannot manage members
//
// Returns an error only if a database operation fails.
func CanManageMember(ctx context.Context, db *mongo.Database, r *http.Request, memberOrgID *primitive.ObjectID) (bool, error) {
	return CanViewMember(ctx, db, r, memberOrgID)
}

// FetchMemberInfo retrieves the minimal member information needed for authorization.
// Returns nil if the member is not found or is not a member role user.
func FetchMemberInfo(ctx context.Context, db *mongo.Database, memberID primitive.ObjectID) (*MemberInfo, error) {
	var result struct {
		ID             primitive.ObjectID  `bson:"_id"`
		OrganizationID *primitive.ObjectID `bson:"organization_id"`
	}

	proj := options.FindOne().SetProjection(bson.M{
		"_id":             1,
		"organization_id": 1,
	})

	err := db.Collection("users").FindOne(ctx, bson.M{
		"_id":  memberID,
		"role": "member",
	}, proj).Decode(&result)

	if err == mongo.ErrNoDocuments {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	return &MemberInfo{
		ID:             result.ID,
		OrganizationID: result.OrganizationID,
	}, nil
}

// CheckMemberAccess is a convenience function that fetches member info and checks
// if the current user can view/manage them. It combines FetchMemberInfo and CanViewMember.
//
// Returns:
//   - (memberInfo, true, nil) if user can access the member
//   - (memberInfo, false, nil) if member exists but user cannot access
//   - (nil, false, nil) if member not found
//   - (nil, false, err) if database error
func CheckMemberAccess(ctx context.Context, db *mongo.Database, r *http.Request, memberID primitive.ObjectID) (*MemberInfo, bool, error) {
	info, err := FetchMemberInfo(ctx, db, memberID)
	if err != nil {
		return nil, false, err
	}
	if info == nil {
		return nil, false, nil
	}

	canAccess, err := CanViewMember(ctx, db, r, info.OrganizationID)
	if err != nil {
		return nil, false, err
	}

	return info, canAccess, nil
}
