# Plan: Multi-Workspace Support

## Overview

Implement multi-workspace (multi-tenant) support where each workspace operates at a subdomain (e.g., `mhs.adroit.games`, `abc.adroit.games`). Workspaces are isolated from each other, with users potentially having separate accounts across multiple workspaces.

## Goals

1. Support multiple isolated workspaces, each at its own subdomain
2. Introduce SuperAdmin role for cross-workspace management
3. Centralized OAuth callback at apex domain with redirect to subdomain
4. Single workspace mode for simple deployments
5. Backward compatible migration path for existing deployments

---

## Current State

### What's Already in Place
- `Workspace` model with `subdomain`, `name`, `logo`, `status` fields
- All entities have `workspace_id` field (users, orgs, groups, resources, materials)
- Workspace store with CRUD operations
- `EnsureDefault()` for creating initial workspace
- Session cookie domain is configurable

### What's Missing
- Workspace extraction from subdomain
- SuperAdmin role
- Workspace-scoped queries throughout the app
- Multi-workspace OAuth flow
- Workspace management UI
- Compound unique indexes for workspace+user

---

## Configuration

**File:** `internal/app/bootstrap/appconfig.go`

```go
// Multi-workspace configuration
MultiWorkspace  bool   // true = subdomain-based workspaces, false = single workspace
PrimaryDomain   string // Apex domain (e.g., "adroit.games") for OAuth callbacks and workspace selector
```

**Config file examples:**

```toml
# Single workspace mode (simple deployment)
multi_workspace = false
primary_domain = "mystratahub.com"

# Multi-workspace mode
multi_workspace = true
primary_domain = "adroit.games"
```

### Behavior by Mode

| Setting | Single Workspace | Multi-Workspace |
|---------|-----------------|-----------------|
| Subdomains | Not used | Required per workspace |
| Apex domain | Serves the app | Workspace selector + admin |
| SuperAdmin | Not needed (admin suffices) | Required |
| OAuth callback | `primary_domain/auth/*/callback` | Same |
| Session cookie domain | `.primary_domain` | `.primary_domain` |

---

## SuperAdmin Role

### Role Definition

A new system user role: `superadmin`

| Role | Workspace Scope | Capabilities |
|------|-----------------|--------------|
| superadmin | None (cross-workspace) | Manage workspaces, access any workspace as admin |
| admin | Single workspace | Full control within workspace |
| analyst | Single workspace | Read-only reports |
| coordinator | Single workspace | Manage assigned organizations |
| leader | Single workspace + org | Manage single organization |
| member | Single workspace + org | Access resources |

### SuperAdmin Characteristics
- `workspace_id` is `nil` (not tied to any workspace)
- Can access workspace management at apex domain
- Can navigate to any workspace subdomain and operate as admin
- Actions are audit logged with superadmin identity

### Database Changes

**File:** `internal/domain/models/user.go`

```go
// WorkspaceID is nil for superadmins (cross-workspace access)
WorkspaceID *primitive.ObjectID `bson:"workspace_id,omitempty"`
```

**Validation:** When role is `superadmin`, `workspace_id` must be nil.

---

## Workspace Model

### States

| State | Code | User Access | Data | SuperAdmin Access |
|-------|------|-------------|------|-------------------|
| Active | `active` | Yes | Retained | Yes |
| Suspended | `suspended` | No | Retained | Yes |
| Archived | `archived` | No | Retained | Yes (read-only) |

**Note:** Suspended and Archived are functionally similar (users blocked, data retained) but serve as different labels for administrative tracking. Suspended implies temporary, Archived implies permanent freeze.

### Deletion
- Workspace deletion removes all associated data
- Requires explicit confirmation
- Soft delete (status change) recommended before hard delete

### Model Updates

**File:** `internal/domain/models/workspace.go`

Current model is sufficient. Status values: `active`, `suspended`, `archived`.

---

## Session and Authentication

### SessionUser Extension

**File:** `internal/app/system/auth/auth.go`

```go
type SessionUser struct {
    ID               string
    Name             string
    LoginID          string
    Role             string
    OrganizationID   string   // For leaders/members
    OrganizationName string
    OrganizationIDs  []string // For coordinators

    // Coordinator permissions
    CanManageMaterials bool
    CanManageResources bool

    // NEW: Multi-workspace support
    WorkspaceID   string   // Current workspace (empty for superadmin)
    WorkspaceIDs  []string // All workspaces user has access to
    IsSuperAdmin  bool     // Quick check for superadmin role
}
```

### UserFetcher Updates

**File:** `internal/app/store/users/fetcher.go`

The `FetchUser` method needs to:
1. Load user's `workspace_id`
2. For superadmins: query all workspaces they can access (all active workspaces)
3. For regular users: query all workspaces where they have a user record with same `login_id_ci` + `auth_method`

```go
func (f *Fetcher) FetchUser(ctx context.Context, userID string) *SessionUser {
    // ... existing user fetch ...

    // Determine workspace access
    if user.Role == "superadmin" {
        // Superadmins can access all active workspaces
        su.IsSuperAdmin = true
        su.WorkspaceIDs = f.fetchAllActiveWorkspaceIDs(ctx)
    } else {
        su.WorkspaceID = user.WorkspaceID.Hex()
        // Find other workspaces with same login_id_ci + auth_method
        su.WorkspaceIDs = f.fetchUserWorkspaceIDs(ctx, user.LoginIDCI, user.AuthMethod)
    }

    return su
}
```

### Session Cookie Configuration

For cross-subdomain session sharing:

```go
store.Options = &sessions.Options{
    Domain:   "." + primaryDomain, // Leading dot for subdomain access
    Path:     "/",
    Secure:   true,
    HttpOnly: true,
    SameSite: http.SameSiteNoneMode, // Required for cross-subdomain
}
```

---

## OAuth Flow (Google Example)

### Current Flow
1. User at `stratahub.com` clicks "Login with Google"
2. Redirect to Google with callback `stratahub.com/auth/google/callback`
3. Google redirects back
4. Session created, redirect to `/dashboard`

### Multi-Workspace Flow
1. User at `mhs.adroit.games` clicks "Login with Google"
2. Store workspace subdomain in OAuth state: `{state: "random", workspace: "mhs"}`
3. Redirect to Google with callback `adroit.games/auth/google/callback` (apex)
4. Google redirects to apex domain
5. Extract workspace from state
6. Look up user in that specific workspace
7. Create session (cookie domain `.adroit.games`)
8. Redirect to `https://mhs.adroit.games/dashboard`

### State Store Updates

**File:** `internal/app/store/oauthstate/store.go`

```go
type OAuthState struct {
    State     string
    ReturnURL string
    Workspace string    // NEW: subdomain of originating workspace
    ExpiresAt time.Time
}
```

### Auth Handler Updates

**File:** `internal/app/features/authgoogle/handler.go`

```go
// ServeLogin - initiate OAuth
func (h *Handler) ServeLogin(w http.ResponseWriter, r *http.Request) {
    // Extract workspace from request context (set by middleware)
    workspace := middleware.GetWorkspace(r)

    // Store workspace in state
    if err := h.StateStore.Save(ctx, state, returnURL, workspace, expiresAt); err != nil {
        // ...
    }

    // Redirect to Google (callback is always apex domain)
    // ...
}

// ServeCallback - handle OAuth return
func (h *Handler) ServeCallback(w http.ResponseWriter, r *http.Request) {
    // Validate state and get workspace
    returnURL, workspace, valid, err := h.StateStore.Validate(ctx, state)

    // Look up user in specific workspace
    user, err := h.findUserInWorkspace(ctx, r, googleUser, workspace)

    // Create session
    h.createSessionAndRedirect(w, r, user, workspace, returnURL)
}

// findUserInWorkspace - workspace-scoped user lookup
func (h *Handler) findUserInWorkspace(ctx context.Context, r *http.Request, googleUser *googleUserInfo, workspaceSubdomain string) (*models.User, error) {
    // Get workspace ID from subdomain
    ws, err := h.workspaceStore.GetBySubdomain(ctx, workspaceSubdomain)
    if err != nil {
        return nil, errWorkspaceNotFound
    }

    // Query with workspace filter
    err := userColl.FindOne(ctx, bson.M{
        "workspace_id":   ws.ID,
        "auth_return_id": googleUser.ID,
        "auth_method":    "google",
    }).Decode(&u)
    // ...
}

// createSessionAndRedirect - redirect to correct subdomain
func (h *Handler) createSessionAndRedirect(w http.ResponseWriter, r *http.Request, u *models.User, workspace, returnURL string) {
    // ... create session ...

    // Build redirect URL to correct subdomain
    dest := fmt.Sprintf("https://%s.%s%s", workspace, h.PrimaryDomain, safePath)
    http.Redirect(w, r, dest, http.StatusSeeOther)
}
```

---

## Middleware

### Workspace Extraction Middleware

**New File:** `internal/app/middleware/workspace.go`

```go
package middleware

import (
    "context"
    "net/http"
    "strings"
)

type ctxKey string
const workspaceKey ctxKey = "workspace"

// WorkspaceInfo holds workspace context for the request
type WorkspaceInfo struct {
    ID        string // ObjectID hex
    Subdomain string
    Name      string
    IsApex    bool   // true if request is to apex domain (no subdomain)
}

// ExtractWorkspace middleware extracts workspace from subdomain
func ExtractWorkspace(primaryDomain string, workspaceStore WorkspaceStore, multiWorkspace bool) func(http.Handler) http.Handler {
    return func(next http.Handler) http.Handler {
        return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
            if !multiWorkspace {
                // Single workspace mode - use default workspace
                ws, _ := workspaceStore.GetFirst(r.Context())
                if ws != nil {
                    r = withWorkspace(r, &WorkspaceInfo{
                        ID:        ws.ID.Hex(),
                        Subdomain: ws.Subdomain,
                        Name:      ws.Name,
                        IsApex:    false,
                    })
                }
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
                r = withWorkspace(r, &WorkspaceInfo{IsApex: true})
                next.ServeHTTP(w, r)
                return
            }

            // Extract subdomain
            suffix := "." + primaryDomain
            if !strings.HasSuffix(host, suffix) {
                // Not our domain
                http.Error(w, "Invalid domain", http.StatusBadRequest)
                return
            }

            subdomain := strings.TrimSuffix(host, suffix)

            // Look up workspace
            ws, err := workspaceStore.GetBySubdomain(r.Context(), subdomain)
            if err != nil || ws == nil {
                http.NotFound(w, r)
                return
            }

            // Check workspace status
            if ws.Status != "active" {
                // Workspace suspended or archived
                http.Error(w, "Workspace unavailable", http.StatusForbidden)
                return
            }

            r = withWorkspace(r, &WorkspaceInfo{
                ID:        ws.ID.Hex(),
                Subdomain: ws.Subdomain,
                Name:      ws.Name,
                IsApex:    false,
            })

            next.ServeHTTP(w, r)
        })
    }
}

// GetWorkspace returns workspace info from context
func GetWorkspace(r *http.Request) *WorkspaceInfo {
    if ws, ok := r.Context().Value(workspaceKey).(*WorkspaceInfo); ok {
        return ws
    }
    return nil
}

func withWorkspace(r *http.Request, ws *WorkspaceInfo) *http.Request {
    return r.WithContext(context.WithValue(r.Context(), workspaceKey, ws))
}
```

### Workspace Access Middleware

```go
// RequireWorkspaceAccess ensures user can access the current workspace
func RequireWorkspaceAccess(next http.Handler) http.Handler {
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        user, ok := auth.CurrentUser(r)
        if !ok {
            // Not authenticated - let RequireSignedIn handle it
            next.ServeHTTP(w, r)
            return
        }

        ws := GetWorkspace(r)
        if ws == nil || ws.IsApex {
            // Apex domain or no workspace - allow (handled elsewhere)
            next.ServeHTTP(w, r)
            return
        }

        // SuperAdmin can access any workspace
        if user.IsSuperAdmin {
            next.ServeHTTP(w, r)
            return
        }

        // Check if user has access to this workspace
        for _, wsID := range user.WorkspaceIDs {
            if wsID == ws.ID {
                next.ServeHTTP(w, r)
                return
            }
        }

        // User doesn't have access to this workspace
        // Redirect to workspace selector or show error
        http.Redirect(w, r, "/select-workspace", http.StatusSeeOther)
    })
}
```

---

## Database Index Changes

### Users Collection

**Current index:**
```go
{Keys: bson.D{{Key: "login_id_ci", Value: 1}}, Options: options.Index().SetUnique(true).SetSparse(true)}
```

**New compound unique index:**
```go
{
    Keys: bson.D{
        {Key: "workspace_id", Value: 1},
        {Key: "login_id_ci", Value: 1},
        {Key: "auth_method", Value: 1},
    },
    Options: options.Index().SetUnique(true).SetSparse(true).SetName("workspace_login_auth_unique"),
}
```

This allows the same `login_id` to exist in multiple workspaces with potentially different auth methods.

### Other Collections

All collections with `workspace_id` should have compound indexes for efficient queries:

```go
// Organizations
{Keys: bson.D{{Key: "workspace_id", Value: 1}, {Key: "name_ci", Value: 1}}}

// Groups
{Keys: bson.D{{Key: "workspace_id", Value: 1}, {Key: "organization_id", Value: 1}}}

// Resources
{Keys: bson.D{{Key: "workspace_id", Value: 1}, {Key: "status", Value: 1}}}

// Materials
{Keys: bson.D{{Key: "workspace_id", Value: 1}, {Key: "status", Value: 1}}}
```

---

## Query Updates

All queries throughout the application need workspace filtering. Pattern:

```go
// Before
filter := bson.M{"status": "active"}

// After
ws := middleware.GetWorkspace(r)
filter := bson.M{
    "workspace_id": ws.ID, // ObjectID, not string
    "status":       "active",
}
```

### Files Requiring Query Updates

| Feature | File | Queries to Update |
|---------|------|-------------------|
| Organizations | `features/organizations/*.go` | List, Create, Update, Delete |
| Groups | `features/groups/*.go` | List, Create, Update, Delete |
| Leaders | `features/leaders/*.go` | List, Create, Update, Delete |
| Members | `features/members/*.go` | List, Create, Update, Delete |
| Resources | `features/resources/*.go` | List, Create, Update, Delete |
| Materials | `features/materials/*.go` | List, Create, Update, Delete |
| System Users | `features/systemusers/*.go` | List, Create, Update, Delete |
| Reports | `features/reports/*.go` | All report queries |
| Dashboard | `features/dashboard/*.go` | Stats queries |
| Activity | `features/activity/*.go` | Session queries |
| Login | `features/login/*.go` | User lookup |
| Auth Google | `features/authgoogle/*.go` | User lookup |

---

## Workspace Management UI

### Location
Accessible at apex domain (`adroit.games`) for superadmins.

### New Feature
**Directory:** `internal/app/features/workspaces/`

### Routes
```go
// Only accessible by superadmins at apex domain
r.Route("/workspaces", func(r chi.Router) {
    r.Use(middleware.RequireApexDomain)
    r.Use(sm.RequireSignedIn)
    r.Use(sm.RequireRole("superadmin"))

    r.Get("/", h.ServeList)           // List all workspaces
    r.Get("/new", h.ServeNew)         // New workspace form
    r.Post("/", h.HandleCreate)       // Create workspace
    r.Post("/{id}/status", h.HandleStatusChange)  // Suspend/activate/archive
    r.Post("/{id}/delete", h.HandleDelete)        // Delete workspace
    r.Get("/{id}/stats", h.ServeStats)            // View workspace statistics
})
```

### List View
- Table: Name, Subdomain, Status, Users, Orgs, Created
- Actions: Suspend, Activate, Archive, Delete
- Link to access workspace (redirects to subdomain)

### Create Form
- Name (required)
- Subdomain (required, validated for uniqueness and format)

### Status Change
- Confirmation modal
- Audit logged

### Delete
- Requires typing workspace name to confirm
- Shows warning about data deletion
- Audit logged

### Statistics
- User count (by role)
- Organization count
- Group count
- Resource count
- Material count
- Last activity timestamp

---

## Apex Domain Pages

### Workspace Selector (for users)
**Route:** `GET /` at apex domain

Shows list of workspaces the user can access (from `SessionUser.WorkspaceIDs`).

```go
func (h *Handler) ServeWorkspaceSelector(w http.ResponseWriter, r *http.Request) {
    user, ok := auth.CurrentUser(r)
    if !ok {
        // Not logged in - show login or public landing
        h.renderLanding(w, r)
        return
    }

    // Get workspace details for user's accessible workspaces
    workspaces := h.workspaceStore.GetByIDs(r.Context(), user.WorkspaceIDs)

    // If superadmin, also show workspace management link
    h.renderSelector(w, r, workspaces, user.IsSuperAdmin)
}
```

### Login at Apex Domain
When logging in at apex domain:
1. Authenticate user
2. If user has access to multiple workspaces, show selector
3. If user has access to only one workspace, redirect to it
4. If superadmin, show selector + management option

---

## Bootstrap / First-Time Setup

### Single Workspace Mode

**File:** `internal/app/bootstrap/bootstrap.go`

```go
func (b *Bootstrap) EnsureWorkspace(ctx context.Context) error {
    if !b.Config.MultiWorkspace {
        // Single workspace mode - ensure default exists
        ws, err := b.WorkspaceStore.GetFirst(ctx)
        if err == nil && ws != nil {
            return nil // Already exists
        }

        // Create default workspace
        return b.WorkspaceStore.Create(ctx, &models.Workspace{
            Name:      "Default",
            Subdomain: "", // Not used in single workspace mode
            Status:    "active",
        })
    }
    return nil
}
```

### Multi-Workspace Mode - First Run

When no workspaces exist:
1. Show setup wizard at apex domain
2. Create first workspace (name, subdomain)
3. Create first superadmin account
4. Redirect to workspace

```go
func (h *Handler) ServeSetupWizard(w http.ResponseWriter, r *http.Request) {
    // Check if any workspaces exist
    count, _ := h.workspaceStore.Count(r.Context(), bson.M{})
    if count > 0 {
        http.Redirect(w, r, "/", http.StatusSeeOther)
        return
    }

    // Show setup form
    h.renderSetupWizard(w, r)
}

func (h *Handler) HandleSetupWizard(w http.ResponseWriter, r *http.Request) {
    // Validate inputs
    workspaceName := r.FormValue("workspace_name")
    subdomain := r.FormValue("subdomain")
    adminEmail := r.FormValue("admin_email")
    adminPassword := r.FormValue("admin_password")

    // Create workspace
    ws := &models.Workspace{
        Name:      workspaceName,
        Subdomain: subdomain,
        Status:    "active",
    }
    if err := h.workspaceStore.Create(ctx, ws); err != nil {
        // Handle error
    }

    // Create superadmin
    admin := &models.User{
        WorkspaceID: nil, // Superadmin has no workspace
        FullName:    "Administrator",
        LoginID:     &adminEmail,
        AuthMethod:  "password",
        Role:        "superadmin",
        Status:      "active",
    }
    // Hash password, save user...

    // Redirect to workspace
    dest := fmt.Sprintf("https://%s.%s/", subdomain, h.PrimaryDomain)
    http.Redirect(w, r, dest, http.StatusSeeOther)
}
```

---

## Migration Path

For existing deployments upgrading to multi-workspace:

### Migration Steps

1. **Add configuration:**
   ```toml
   multi_workspace = true
   primary_domain = "adroit.games"
   ```

2. **Run migration script:**
   ```go
   func MigrateToMultiWorkspace(ctx context.Context, db *mongo.Database) error {
       // 1. Create workspace for existing data
       ws := &models.Workspace{
           Name:      "Default",
           Subdomain: "app", // Or prompt user
           Status:    "active",
       }
       workspaceStore.Create(ctx, ws)

       // 2. Update all users with workspace_id
       db.Collection("users").UpdateMany(ctx,
           bson.M{"workspace_id": nil, "role": bson.M{"$ne": "superadmin"}},
           bson.M{"$set": bson.M{"workspace_id": ws.ID}},
       )

       // 3. Update all organizations
       db.Collection("organizations").UpdateMany(ctx,
           bson.M{"workspace_id": nil},
           bson.M{"$set": bson.M{"workspace_id": ws.ID}},
       )

       // 4. Update groups, resources, materials, etc.
       // ...

       // 5. Prompt for superadmin selection
       return promptSuperAdminSelection(ctx, db, ws.ID)
   }

   func promptSuperAdminSelection(ctx context.Context, db *mongo.Database, wsID primitive.ObjectID) error {
       // Find all admins
       cursor, _ := db.Collection("users").Find(ctx, bson.M{
           "workspace_id": wsID,
           "role":         "admin",
       })

       var admins []models.User
       cursor.All(ctx, &admins)

       // Display selection UI or use first admin
       // Selected admin(s) get:
       // - role changed to "superadmin"
       // - workspace_id set to nil

       return nil
   }
   ```

3. **Update DNS:**
   - Point `*.adroit.games` to the server
   - Point `adroit.games` to the server

4. **Update OAuth providers:**
   - Add `adroit.games/auth/google/callback` as authorized redirect URI

---

## Files to Create

| File | Purpose |
|------|---------|
| `internal/app/middleware/workspace.go` | Workspace extraction middleware |
| `internal/app/features/workspaces/handler.go` | Workspace management handler |
| `internal/app/features/workspaces/routes.go` | Routes |
| `internal/app/features/workspaces/types.go` | View models |
| `internal/app/features/workspaces/templates/*.gohtml` | Templates |
| `internal/app/features/setup/handler.go` | First-time setup wizard |
| `internal/app/features/setup/routes.go` | Setup routes |
| `internal/app/features/setup/templates/*.gohtml` | Setup templates |
| `cmd/migrate/multiworkspace.go` | Migration script |

## Files to Modify

| File | Changes |
|------|---------|
| `internal/app/bootstrap/appconfig.go` | Add MultiWorkspace, PrimaryDomain |
| `internal/app/bootstrap/config.go` | Load new config values |
| `internal/app/bootstrap/routes.go` | Add workspace middleware, new routes |
| `internal/app/system/auth/auth.go` | Extend SessionUser |
| `internal/app/store/users/fetcher.go` | Fetch workspace access |
| `internal/app/store/users/indexes.go` | Update unique index |
| `internal/app/store/oauthstate/store.go` | Add workspace field |
| `internal/app/features/authgoogle/handler.go` | Workspace-scoped OAuth |
| `internal/app/features/login/handler.go` | Workspace-scoped login |
| `internal/app/features/organizations/*.go` | Add workspace filtering |
| `internal/app/features/groups/*.go` | Add workspace filtering |
| `internal/app/features/leaders/*.go` | Add workspace filtering |
| `internal/app/features/members/*.go` | Add workspace filtering |
| `internal/app/features/resources/*.go` | Add workspace filtering |
| `internal/app/features/materials/*.go` | Add workspace filtering |
| `internal/app/features/systemusers/*.go` | Add workspace filtering, superadmin support |
| `internal/app/features/reports/*.go` | Add workspace filtering |
| `internal/app/features/dashboard/*.go` | Add workspace filtering |
| `internal/app/features/activity/*.go` | Add workspace filtering |
| `internal/app/features/settings/*.go` | Workspace-scoped settings |
| `internal/domain/models/user.go` | Validate superadmin has no workspace |
| `internal/app/system/viewdata/viewdata.go` | Add workspace to BaseVM |
| Navigation templates | Add workspace indicator, selector link |

---

## Implementation Phases

### Phase 1: Foundation
1. Configuration changes
2. Workspace middleware
3. SessionUser extension
4. UserFetcher updates
5. Index updates

### Phase 2: Authentication
1. OAuth state with workspace
2. Workspace-scoped user lookup
3. Cross-subdomain session cookies
4. Login flow updates

### Phase 3: Query Updates
1. Organizations
2. Groups
3. Leaders/Members
4. Resources/Materials
5. Reports/Dashboard
6. Activity tracking

### Phase 4: SuperAdmin
1. SuperAdmin role support
2. Workspace management UI
3. Workspace selector (apex domain)

### Phase 5: Setup & Migration
1. First-time setup wizard
2. Migration script
3. Documentation

### Phase 6: Testing
1. Single workspace mode
2. Multi-workspace mode
3. OAuth flows
4. SuperAdmin access
5. Cross-workspace sessions

---

## Security Considerations

1. **Workspace isolation:** All queries MUST include workspace_id filter
2. **SuperAdmin audit:** All superadmin actions logged with identity
3. **Session validation:** Verify workspace access on each request
4. **OAuth state:** Validate workspace in state matches request origin
5. **Subdomain validation:** Sanitize subdomain input (alphanumeric + hyphen only)

---

## Testing Checklist

- [ ] Single workspace mode works without subdomains
- [ ] Multi-workspace extracts subdomain correctly
- [ ] Invalid subdomain returns 404
- [ ] Suspended workspace returns 403
- [ ] SuperAdmin can access any workspace
- [ ] Regular user cannot access other workspaces
- [ ] OAuth callback redirects to correct subdomain
- [ ] Session cookie works across subdomains
- [ ] User with accounts in multiple workspaces sees selector
- [ ] Workspace management CRUD works
- [ ] First-time setup creates workspace + superadmin
- [ ] Migration script populates workspace_id correctly
- [ ] All queries filter by workspace_id
- [ ] Audit logs include workspace context

---

## Configuration Example (Production)

```toml
# Multi-workspace production configuration
multi_workspace = true
primary_domain = "adroit.games"

# Session cookie will use domain ".adroit.games"
session_domain = ".adroit.games"

# OAuth callbacks go to apex domain
base_url = "https://adroit.games"

# TLS with wildcard certificate
use_https = true
use_lets_encrypt = true
lets_encrypt_challenge = "dns-01"
domains = ["adroit.games", "*.adroit.games"]
route53_hosted_zone_id = "Z..."
```
