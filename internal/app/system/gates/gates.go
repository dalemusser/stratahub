// Package gates provides authorization gate functions for HTTP handlers.
// Gates check authentication and authorization, rendering appropriate error
// pages when checks fail.
//
// # Three-Tier Authorization Pattern
//
// StrataHub uses a three-tier authorization approach:
//
//  1. Route-Level Middleware (auth.RequireSignedIn, auth.RequireRole)
//     Applied in routes.go files for coarse-grained access control.
//     Example: sm.RequireRole("admin") ensures all routes in a group require admin.
//     When middleware handles role checking, handlers don't need gates.
//
//  2. Handler-Level Gates (this package)
//     Used in handlers that need role checks WITHOUT route-level middleware,
//     or need different role requirements than the route group.
//     Gates render error pages and return user context (role, name, userID).
//     Example: gates.RequireAdminOrLeader for a handler in a mixed-access route.
//
//  3. Policy Layer (internal/app/policy/*)
//     Used for resource-specific authorization requiring database lookups.
//     Example: grouppolicy.CanManageGroup checks if user can manage a specific group.
//     Policies return (bool, error) - callers handle error rendering.
//
// # When to Use Each Tier
//
// Use middleware when: All routes in a group have the same role requirements.
// Use gates when: Individual handlers need different role checks than the route.
// Use policies when: Authorization depends on the specific resource being accessed.
//
// # Avoiding Redundancy
//
// Don't use gates in handlers that are behind role-specific middleware.
// If routes.go has RequireRole("admin"), handlers don't need gates.RequireAdmin.
// Instead, use authz.UserCtx(r) to get user context without re-checking role.
package gates

import (
	"net/http"

	uierrors "github.com/dalemusser/stratahub/internal/app/features/errors"
	"github.com/dalemusser/stratahub/internal/app/system/authz"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

// Result contains the result of an authorization gate check.
type Result struct {
	Role   string
	Name   string
	UserID primitive.ObjectID
	OK     bool
}

// RequireAuth ensures a user is authenticated.
// If not authenticated, it renders an unauthorized error and returns OK=false.
// The loginURL parameter specifies where to redirect for login.
func RequireAuth(w http.ResponseWriter, r *http.Request, loginURL string) Result {
	role, name, uid, ok := authz.UserCtx(r)
	if !ok {
		uierrors.RenderUnauthorized(w, r, loginURL)
		return Result{OK: false}
	}
	return Result{Role: role, Name: name, UserID: uid, OK: true}
}

// RequireAdmin ensures the user is authenticated and has the admin role.
// If not authenticated, renders unauthorized error.
// If authenticated but not admin, renders forbidden error with the provided message and fallback URL.
func RequireAdmin(w http.ResponseWriter, r *http.Request, forbiddenMsg, fallbackURL string) Result {
	role, name, uid, ok := authz.UserCtx(r)
	if !ok {
		uierrors.RenderUnauthorized(w, r, "/login")
		return Result{OK: false}
	}
	if role != "admin" {
		uierrors.RenderForbidden(w, r, forbiddenMsg, fallbackURL)
		return Result{OK: false}
	}
	return Result{Role: role, Name: name, UserID: uid, OK: true}
}

// RequireAdminOrAnalyst ensures the user is authenticated and has the admin or analyst role.
// If not authenticated, renders unauthorized error.
// If authenticated but not admin/analyst, renders forbidden error.
func RequireAdminOrAnalyst(w http.ResponseWriter, r *http.Request, forbiddenMsg, fallbackURL string) Result {
	role, name, uid, ok := authz.UserCtx(r)
	if !ok {
		uierrors.RenderUnauthorized(w, r, "/login")
		return Result{OK: false}
	}
	if role != "admin" && role != "analyst" {
		uierrors.RenderForbidden(w, r, forbiddenMsg, fallbackURL)
		return Result{OK: false}
	}
	return Result{Role: role, Name: name, UserID: uid, OK: true}
}

// RequireAdminOrLeader ensures the user is authenticated and has the admin or leader role.
// If not authenticated, renders unauthorized error.
// If authenticated but not admin/leader, renders forbidden error.
func RequireAdminOrLeader(w http.ResponseWriter, r *http.Request, forbiddenMsg, fallbackURL string) Result {
	role, name, uid, ok := authz.UserCtx(r)
	if !ok {
		uierrors.RenderUnauthorized(w, r, "/login")
		return Result{OK: false}
	}
	if role != "admin" && role != "leader" {
		uierrors.RenderForbidden(w, r, forbiddenMsg, fallbackURL)
		return Result{OK: false}
	}
	return Result{Role: role, Name: name, UserID: uid, OK: true}
}

// RequireAnyRole ensures the user is authenticated and has one of the specified roles.
// If not authenticated, renders unauthorized error.
// If authenticated but role not in allowed list, renders forbidden error.
func RequireAnyRole(w http.ResponseWriter, r *http.Request, forbiddenMsg, fallbackURL string, allowedRoles ...string) Result {
	role, name, uid, ok := authz.UserCtx(r)
	if !ok {
		uierrors.RenderUnauthorized(w, r, "/login")
		return Result{OK: false}
	}

	for _, allowed := range allowedRoles {
		if role == allowed {
			return Result{Role: role, Name: name, UserID: uid, OK: true}
		}
	}

	uierrors.RenderForbidden(w, r, forbiddenMsg, fallbackURL)
	return Result{OK: false}
}
