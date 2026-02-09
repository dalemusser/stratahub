// Package workspace provides workspace context extraction and validation for multi-tenant operations.
package workspace

// Terminology: User Identifiers
//   - UserID / userID / user_id: The MongoDB ObjectID (_id) that uniquely identifies a user record
//   - LoginID / loginID / login_id: The human-readable string users type to log in

import (
	"context"
	"net/http"
	"strings"

	"github.com/dalemusser/stratahub/internal/app/system/auth"
	"github.com/dalemusser/stratahub/internal/app/system/timeouts"
	"github.com/dalemusser/stratahub/internal/domain/models"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.uber.org/zap"
)

type ctxKey string

const workspaceKey ctxKey = "workspace"

// Info holds workspace context for the current request.
type Info struct {
	ID                primitive.ObjectID // Workspace ObjectID
	Subdomain         string             // Workspace subdomain (e.g., "mhs")
	Name              string             // Workspace display name
	Status            string             // Workspace status: active, suspended, archived
	IsApex            bool               // true if request is to apex domain (no subdomain)
	IsSingleWorkspace bool               // true if running in single-workspace mode
}

// WorkspaceStore defines the interface for workspace lookups.
type WorkspaceStore interface {
	GetBySubdomain(ctx context.Context, subdomain string) (models.Workspace, error)
	GetFirst(ctx context.Context) (models.Workspace, error)
}

// Middleware creates middleware that extracts workspace from the request host.
//
// In multi-workspace mode:
//   - Requests to apex domain (e.g., adroit.games) set IsApex=true
//   - Requests to subdomains (e.g., mhs.adroit.games) lookup the workspace
//   - Invalid subdomains return 404
//   - Suspended/archived workspaces return 403
//
// In single-workspace mode:
//   - Uses the first (default) workspace for all requests
func Middleware(primaryDomain string, store WorkspaceStore, multiWorkspace bool, logger *zap.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx, cancel := context.WithTimeout(r.Context(), timeouts.Short())
			defer cancel()

			if !multiWorkspace {
				// Single workspace mode - use default workspace
				ws, err := store.GetFirst(ctx)
				if err != nil {
					// No workspace exists yet - allow request to proceed (setup flow)
					logger.Debug("no workspace found in single-workspace mode, proceeding without workspace context")
					next.ServeHTTP(w, r)
					return
				}

				r = withWorkspace(r, &Info{
					ID:                ws.ID,
					Subdomain:         ws.Subdomain,
					Name:              ws.Name,
					Status:            ws.Status,
					IsApex:            false,
					IsSingleWorkspace: true,
				})
				next.ServeHTTP(w, r)
				return
			}

			// Multi-workspace mode - extract from host
			host := r.Host
			if idx := strings.Index(host, ":"); idx != -1 {
				host = host[:idx] // Remove port
			}

			// Check if apex domain
			if host == primaryDomain {
				// For API requests on apex domain, fall back to default workspace
				// This supports legacy game clients that use apex domain for /api/* endpoints
				if strings.HasPrefix(r.URL.Path, "/api/") {
					ws, err := store.GetFirst(ctx)
					if err == nil {
						r = withWorkspace(r, &Info{
							ID:        ws.ID,
							Subdomain: ws.Subdomain,
							Name:      ws.Name,
							Status:    ws.Status,
							IsApex:    false, // Treat as workspace request for API purposes
						})
						next.ServeHTTP(w, r)
						return
					}
					// Fall through to apex handling if no workspace found
				}
				r = withWorkspace(r, &Info{IsApex: true})
				next.ServeHTTP(w, r)
				return
			}

			// Extract subdomain
			suffix := "." + primaryDomain
			if !strings.HasSuffix(host, suffix) {
				// Not our domain - could be localhost in dev
				if strings.HasPrefix(host, "localhost") || strings.HasPrefix(host, "127.0.0.1") {
					// Development mode - use default workspace
					ws, err := store.GetFirst(ctx)
					if err != nil {
						logger.Debug("no workspace found for localhost, proceeding without workspace context")
						next.ServeHTTP(w, r)
						return
					}
					r = withWorkspace(r, &Info{
						ID:        ws.ID,
						Subdomain: ws.Subdomain,
						Name:      ws.Name,
						Status:    ws.Status,
						IsApex:    false,
					})
					next.ServeHTTP(w, r)
					return
				}

				logger.Warn("request to unknown domain",
					zap.String("host", host),
					zap.String("primary_domain", primaryDomain))
				http.Error(w, "Invalid domain", http.StatusBadRequest)
				return
			}

			subdomain := strings.TrimSuffix(host, suffix)
			if subdomain == "" {
				// Edge case: host is ".adroit.games" - treat as apex
				r = withWorkspace(r, &Info{IsApex: true})
				next.ServeHTTP(w, r)
				return
			}

			// Look up workspace by subdomain
			ws, err := store.GetBySubdomain(ctx, subdomain)
			if err != nil {
				logger.Debug("workspace not found",
					zap.String("subdomain", subdomain),
					zap.Error(err))
				http.NotFound(w, r)
				return
			}

			// Check workspace status
			if ws.Status != "active" {
				logger.Info("request to non-active workspace",
					zap.String("subdomain", subdomain),
					zap.String("status", ws.Status))
				http.Error(w, "Workspace unavailable", http.StatusForbidden)
				return
			}

			r = withWorkspace(r, &Info{
				ID:                ws.ID,
				Subdomain:         ws.Subdomain,
				Name:              ws.Name,
				Status:            ws.Status,
				IsApex:            false,
				IsSingleWorkspace: true,
			})

			next.ServeHTTP(w, r)
		})
	}
}

// FromRequest returns the workspace info from the request context.
// Returns nil if no workspace context is set.
func FromRequest(r *http.Request) *Info {
	if ws, ok := r.Context().Value(workspaceKey).(*Info); ok {
		return ws
	}
	return nil
}

// FromContext returns the workspace info from the context.
// Returns nil if no workspace context is set.
func FromContext(ctx context.Context) *Info {
	if ws, ok := ctx.Value(workspaceKey).(*Info); ok {
		return ws
	}
	return nil
}

// IDFromRequest returns the workspace ID from the request context.
// Returns primitive.NilObjectID if no workspace is set or if on apex domain.
func IDFromRequest(r *http.Request) primitive.ObjectID {
	ws := FromRequest(r)
	if ws == nil || ws.IsApex {
		return primitive.NilObjectID
	}
	return ws.ID
}

// CheckerFromRequest returns the workspace ID (as hex string) and whether this is the apex domain.
// This function can be used as an auth.WorkspaceChecker callback.
// Returns ("", true) for apex domain or if no workspace context.
// Returns (workspaceIDHex, false) for workspace subdomain requests.
func CheckerFromRequest(r *http.Request) (string, bool) {
	ws := FromRequest(r)
	if ws == nil || ws.IsApex {
		return "", true
	}
	return ws.ID.Hex(), false
}

// withWorkspace adds workspace info to the request context.
func withWorkspace(r *http.Request, ws *Info) *http.Request {
	return r.WithContext(context.WithValue(r.Context(), workspaceKey, ws))
}

// RequireWorkspace returns middleware that ensures a workspace context exists.
// Requests without workspace context (apex domain in multi-workspace mode) are rejected.
// For HTML requests, redirects to /workspaces. For API/HTMX requests, returns an error.
func RequireWorkspace(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ws := FromRequest(r)
		if ws == nil || ws.IsApex {
			// For HTMX requests, use HX-Redirect
			if r.Header.Get("HX-Request") == "true" {
				w.Header().Set("HX-Redirect", "/workspaces")
				w.WriteHeader(http.StatusForbidden)
				return
			}

			// For HTML requests, redirect to workspaces
			accept := r.Header.Get("Accept")
			if strings.Contains(accept, "text/html") || accept == "" {
				http.Redirect(w, r, "/workspaces", http.StatusSeeOther)
				return
			}

			// For API requests, return error
			http.Error(w, "Workspace required", http.StatusBadRequest)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// RequireApex returns middleware that ensures the request is to the apex domain
// or is in single-workspace mode (where apex/subdomain distinction doesn't apply).
// Used for workspace management UI that should only be accessible at apex.
func RequireApex(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ws := FromRequest(r)
		if ws == nil || (!ws.IsApex && !ws.IsSingleWorkspace) {
			http.Error(w, "Not found", http.StatusNotFound)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// WorkspaceLookup defines the interface for looking up workspace info.
type WorkspaceLookup interface {
	GetByID(ctx context.Context, id primitive.ObjectID) (models.Workspace, error)
}

// RedirectNonSuperadminFromApex returns middleware that redirects logged-in
// non-superadmin users to their workspace domain. This prevents workspace users
// from accidentally accessing the apex domain via shared session cookies.
// Superadmins and visitors (not logged in) are allowed through.
//
// If the user has a WorkspaceID in their session, they are redirected to their
// workspace subdomain. Otherwise, they are redirected to the apex-denied page.
func RedirectNonSuperadminFromApex(store WorkspaceLookup, primaryDomain string, logger *zap.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ws := FromRequest(r)
			if ws == nil || !ws.IsApex {
				// Not on apex, allow through
				next.ServeHTTP(w, r)
				return
			}

			// Allow certain paths to prevent redirect loops and ensure assets load
			path := r.URL.Path
			if path == "/apex-denied" || path == "/logout" ||
				strings.HasPrefix(path, "/static/") ||
				strings.HasPrefix(path, "/assets/") {
				next.ServeHTTP(w, r)
				return
			}

			// On apex - check if logged in
			user, ok := auth.CurrentUser(r)
			if !ok {
				// Not logged in, allow through (visitor)
				next.ServeHTTP(w, r)
				return
			}

			// Logged in - check role
			if user.Role == "superadmin" {
				// Superadmin allowed at apex
				next.ServeHTTP(w, r)
				return
			}

			// Non-superadmin at apex - try to redirect to their workspace
			redirectURL := "/apex-denied" // fallback
			if user.WorkspaceID != "" {
				wsID, err := primitive.ObjectIDFromHex(user.WorkspaceID)
				if err == nil {
					ctx, cancel := context.WithTimeout(r.Context(), timeouts.Short())
					defer cancel()
					if userWS, err := store.GetByID(ctx, wsID); err == nil && userWS.Subdomain != "" {
						// Build redirect URL to user's workspace
						redirectURL = "https://" + userWS.Subdomain + "." + primaryDomain + r.URL.Path
						if r.URL.RawQuery != "" {
							redirectURL += "?" + r.URL.RawQuery
						}
						logger.Debug("redirecting user to their workspace",
							zap.String("user_id", user.ID),
							zap.String("workspace", userWS.Subdomain),
							zap.String("redirect", redirectURL))
					}
				}
			}

			// For HTMX requests, use HX-Redirect
			if r.Header.Get("HX-Request") == "true" {
				w.Header().Set("HX-Redirect", redirectURL)
				w.WriteHeader(http.StatusForbidden)
				return
			}

			http.Redirect(w, r, redirectURL, http.StatusSeeOther)
		})
	}
}

/*─────────────────────────────────────────────────────────────────────────────*
| Query Helpers                                                                |
*─────────────────────────────────────────────────────────────────────────────*/

// Filter adds workspace_id to a bson.M filter map for scoped queries.
// If no workspace context exists or it's the apex domain, the filter is unchanged.
// This allows queries to be automatically scoped to the current workspace.
//
// Usage:
//
//	filter := bson.M{"status": "active"}
//	workspace.Filter(r, filter)
//	// filter now has workspace_id added if in workspace context
func Filter(r *http.Request, filter map[string]interface{}) {
	ws := FromRequest(r)
	if ws == nil || ws.IsApex {
		return
	}
	filter["workspace_id"] = ws.ID
}

// FilterCtx adds workspace_id to a filter using context instead of request.
func FilterCtx(ctx context.Context, filter map[string]interface{}) {
	ws := FromContext(ctx)
	if ws == nil || ws.IsApex {
		return
	}
	filter["workspace_id"] = ws.ID
}

// IDPtrFromCtx returns a pointer to the workspace ID from the context.
// Returns nil if no workspace context is set or if on apex domain.
// Use this when setting workspace_id on new documents that use pointer types.
func IDPtrFromCtx(ctx context.Context) *primitive.ObjectID {
	ws := FromContext(ctx)
	if ws == nil || ws.IsApex {
		return nil
	}
	id := ws.ID
	return &id
}

// MustFilter adds workspace_id to a bson.M filter map and returns true if a workspace
// was found. Returns false if no workspace context exists, which callers can use
// to reject requests that require workspace scoping.
func MustFilter(r *http.Request, filter map[string]interface{}) bool {
	ws := FromRequest(r)
	if ws == nil || ws.IsApex {
		return false
	}
	filter["workspace_id"] = ws.ID
	return true
}

// SetOnDoc sets the workspace_id field on a document map for creating new records.
// This should be called when inserting new organizations, groups, resources, etc.
// Returns the workspace ID that was set (or NilObjectID if no workspace context).
func SetOnDoc(r *http.Request, doc map[string]interface{}) primitive.ObjectID {
	ws := FromRequest(r)
	if ws == nil || ws.IsApex {
		return primitive.NilObjectID
	}
	doc["workspace_id"] = ws.ID
	return ws.ID
}

/*─────────────────────────────────────────────────────────────────────────────*
| Test Helpers                                                                 |
*─────────────────────────────────────────────────────────────────────────────*/

// WithTestWorkspace returns a request with workspace context set for testing.
// This is exported for use in tests only.
func WithTestWorkspace(r *http.Request, id primitive.ObjectID, subdomain, name string) *http.Request {
	return withWorkspace(r, &Info{
		ID:        id,
		Subdomain: subdomain,
		Name:      name,
		Status:    "active",
		IsApex:    false,
	})
}

// WithTestApex returns a request with apex domain context set for testing.
func WithTestApex(r *http.Request) *http.Request {
	return withWorkspace(r, &Info{IsApex: true})
}

// WithTestWorkspaceCtx returns a context with workspace info set for testing.
func WithTestWorkspaceCtx(ctx context.Context, id primitive.ObjectID, subdomain, name string) context.Context {
	return context.WithValue(ctx, workspaceKey, &Info{
		ID:        id,
		Subdomain: subdomain,
		Name:      name,
		Status:    "active",
		IsApex:    false,
	})
}
