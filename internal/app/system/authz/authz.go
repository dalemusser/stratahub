// internal/app/system/authz/authz.go
package authz

import (
	"net/http"
	"strings"

	"github.com/dalemusser/stratahub/internal/app/system/auth"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

// UserCtx returns the user's role (lowercased), name, Mongo ObjectID, and a found flag.
// If no user is present in context or the user ID is malformed, it returns
// "visitor", "", NilObjectID, false. This ensures callers can trust that
// ok=true means a valid, authenticated user with a valid ObjectID.
// The role is normalized to lowercase for consistent comparison.
func UserCtx(r *http.Request) (role string, name string, userID primitive.ObjectID, ok bool) {
	user, ok := auth.CurrentUser(r)
	if !ok {
		return "visitor", "", primitive.NilObjectID, false
	}
	userID, err := primitive.ObjectIDFromHex(user.ID)
	if err != nil {
		// Malformed user ID in session - fail closed for security.
		// This should not happen in normal operation; indicates session corruption.
		return "visitor", "", primitive.NilObjectID, false
	}
	return strings.ToLower(user.Role), user.Name, userID, true
}

// IsSuperAdmin reports whether the current request's user is a superadmin.
func IsSuperAdmin(r *http.Request) bool {
	user, ok := auth.CurrentUser(r)
	return ok && user.IsSuperAdmin
}

// IsAdmin reports whether the current request's user is an admin.
// Note: Superadmins are also considered admins for permission purposes.
func IsAdmin(r *http.Request) bool {
	role, _, _, ok := UserCtx(r)
	return ok && (role == "admin" || role == "superadmin")
}

// IsAdminOnly reports whether the current request's user is specifically an admin (not superadmin).
func IsAdminOnly(r *http.Request) bool {
	role, _, _, ok := UserCtx(r)
	return ok && role == "admin"
}

// IsAnalyst reports whether the current request's user is an analyst.
func IsAnalyst(r *http.Request) bool {
	role, _, _, ok := UserCtx(r)
	return ok && role == "analyst"
}

// IsLeader reports whether the current request's user is a leader.
func IsLeader(r *http.Request) bool {
	role, _, _, ok := UserCtx(r)
	return ok && role == "leader"
}

// IsMember reports whether the current request's user is a member.
func IsMember(r *http.Request) bool {
	role, _, _, ok := UserCtx(r)
	return ok && role == "member"
}

// IsCoordinator reports whether the current request's user is a coordinator.
func IsCoordinator(r *http.Request) bool {
	role, _, _, ok := UserCtx(r)
	return ok && role == "coordinator"
}

// UserOrgID returns the current user's organization ID as an ObjectID.
// Returns NilObjectID if user is not logged in or has no organization.
func UserOrgID(r *http.Request) primitive.ObjectID {
	user, ok := auth.CurrentUser(r)
	if !ok || user.OrganizationID == "" {
		return primitive.NilObjectID
	}
	oid, err := primitive.ObjectIDFromHex(user.OrganizationID)
	if err != nil {
		return primitive.NilObjectID
	}
	return oid
}

// UserOrgIDs returns the coordinator's assigned organization IDs.
// Returns nil if user is not logged in or has no assigned organizations.
func UserOrgIDs(r *http.Request) []primitive.ObjectID {
	user, ok := auth.CurrentUser(r)
	if !ok || len(user.OrganizationIDs) == 0 {
		return nil
	}
	result := make([]primitive.ObjectID, 0, len(user.OrganizationIDs))
	for _, idHex := range user.OrganizationIDs {
		oid, err := primitive.ObjectIDFromHex(idHex)
		if err == nil {
			result = append(result, oid)
		}
	}
	return result
}

// CanManageMaterials reports whether the current user can create/edit/delete materials.
// Superadmins and admins always can. Coordinators can only if they have the CanManageMaterials permission.
func CanManageMaterials(r *http.Request) bool {
	user, ok := auth.CurrentUser(r)
	if !ok {
		return false
	}

	role := strings.ToLower(user.Role)

	// Superadmins and admins can always manage materials
	if role == "admin" || role == "superadmin" {
		return true
	}

	// Coordinators can manage if they have the permission
	if role == "coordinator" {
		return user.CanManageMaterials
	}

	return false
}

// CanManageResources reports whether the current user can create/edit/delete resources.
// Superadmins and admins always can. Coordinators can only if they have the CanManageResources permission.
func CanManageResources(r *http.Request) bool {
	user, ok := auth.CurrentUser(r)
	if !ok {
		return false
	}

	role := strings.ToLower(user.Role)

	// Superadmins and admins can always manage resources
	if role == "admin" || role == "superadmin" {
		return true
	}

	// Coordinators can manage if they have the permission
	if role == "coordinator" {
		return user.CanManageResources
	}

	return false
}

// CanAccessOrg reports whether the current user can access the given organization.
// Superadmins and admins can access all organizations.
// Coordinators can access their assigned organizations.
// Leaders/members can access only their single organization.
// Analysts cannot access organizations directly.
func CanAccessOrg(r *http.Request, orgID primitive.ObjectID) bool {
	user, ok := auth.CurrentUser(r)
	if !ok {
		return false
	}

	role := strings.ToLower(user.Role)

	// Superadmins and admins can access all organizations
	if role == "admin" || role == "superadmin" {
		return true
	}

	// Coordinators can access their assigned organizations
	if role == "coordinator" {
		for _, idHex := range user.OrganizationIDs {
			if oid, err := primitive.ObjectIDFromHex(idHex); err == nil && oid == orgID {
				return true
			}
		}
		return false
	}

	// Leaders/members can access their single organization
	if role == "leader" || role == "member" {
		if user.OrganizationID == "" {
			return false
		}
		userOrgID, err := primitive.ObjectIDFromHex(user.OrganizationID)
		if err != nil {
			return false
		}
		return userOrgID == orgID
	}

	return false
}
