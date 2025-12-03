// internal/app/system/authz/roles.go
package authz

import (
	"net/http"
	"strings"
)

// HasAnyRole reports whether the current request's user has any of the given roles.
// Returns false if no user is present (i.e., not signed in).
func HasAnyRole(r *http.Request, roles ...string) bool {
	role, _, _, ok := UserCtx(r)
	if !ok {
		return false
	}
	cur := strings.ToLower(role)
	for _, want := range roles {
		if cur == strings.ToLower(strings.TrimSpace(want)) {
			return true
		}
	}
	return false
}

// HasRole is a convenience wrapper for a single role.
func HasRole(r *http.Request, role string) bool {
	return HasAnyRole(r, role)
}

// Role returns the current user's role (lowercased) and whether a user is present.
func Role(r *http.Request) (string, bool) {
	role, _, _, ok := UserCtx(r)
	return strings.ToLower(role), ok
}
