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

// IsAdmin reports whether the current request's user is an admin.
func IsAdmin(r *http.Request) bool {
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
