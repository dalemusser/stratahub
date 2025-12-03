// internal/app/system/authz/authz.go
package authz

import (
	"net/http"

	"github.com/dalemusser/stratahub/internal/app/system/auth"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

// UserCtx returns the user's role, name, Mongo ObjectID, and a found flag.
// If no user is present in context, it returns "visitor", "", NilObjectID, false.
func UserCtx(r *http.Request) (role string, name string, userID primitive.ObjectID, ok bool) {
	user, ok := auth.CurrentUser(r)
	if !ok {
		return "visitor", "", primitive.NilObjectID, false
	}
	userID, _ = primitive.ObjectIDFromHex(user.ID)

	return user.Role, user.Name, userID, true
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
