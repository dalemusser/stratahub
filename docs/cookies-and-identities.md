# Cookies, Sessions, and Identity in Multi-Workspace StrataHub

This document captures the findings from investigating why Unity WebGL games could retrieve user identity from `adroit.games/api/user` when launched from `dev.adroit.games` but NOT when launched from `mhs.adroit.games`. The investigation was conducted in February 2026.

## Architecture

StrataHub runs as a single Go process serving all subdomains:

- `adroit.games` — apex domain (superadmin workspace management)
- `dev.adroit.games` — the original workspace (was previously `mhs.adroit.games`, subdomain renamed)
- `mhs.adroit.games` — a newer workspace added after `dev`
- `cdn.adroit.games` — CloudFront CDN for static assets and game builds

Multi-workspace mode is enabled (`multi_workspace = true`, `primary_domain = "adroit.games"`).

## Session Cookie Configuration

### Cookie Domain

When `multi_workspace = true` and no explicit `session_domain` is set, the domain is auto-derived:

```go
// internal/app/bootstrap/config.go, lines 175-181
if appCfg.MultiWorkspace && appCfg.SessionDomain == "" && appCfg.PrimaryDomain != "" {
    appCfg.SessionDomain = "." + appCfg.PrimaryDomain
}
```

Result: `Domain=.adroit.games` — the cookie is sent to `adroit.games` and ALL subdomains.

### Cookie Attributes

| Attribute  | Value                 | Notes                                  |
|------------|-----------------------|----------------------------------------|
| Name       | `stratahub-session`   | Configured in config.toml              |
| Domain     | `.adroit.games`       | Auto-derived from primary_domain       |
| Path       | `/`                   | All paths                              |
| Secure     | `true`                | Production mode                        |
| HttpOnly   | `true`                | Not accessible to JavaScript           |
| SameSite   | `Lax`                 | Same-site requests + top-level nav     |
| MaxAge     | 24h (default)         | Configurable via session_max_age       |

**Confirmed via Chrome DevTools:** The cookie IS set with domain `adroit.games` and IS sent to the apex domain. The cookie itself is not the problem.

## Workspace Validation on Authentication

### Normal Flow

When `LoadSessionUser` middleware processes a request:

1. Decodes the session cookie (same signing key for all subdomains)
2. Fetches the user from MongoDB by `_id` (no workspace filter)
3. Runs the workspace checker:

```go
// internal/app/system/auth/auth.go, lines 296-310
if sm.workspaceChecker != nil {
    wsID, isApex := sm.workspaceChecker(r)
    if !isApex && !u.IsSuperAdmin {
        if u.WorkspaceID != wsID {
            u = nil // Treat as unauthenticated
        }
    }
}
```

- At the apex (`isApex=true`): workspace check is **skipped** — any user is authenticated
- At a workspace subdomain: user's `WorkspaceID` must match the subdomain's workspace ID

### The `/api/*` Special Case on Apex (THE ROOT CAUSE)

**File:** `internal/app/system/workspace/workspace.go`, lines 84-102

When a request hits the apex domain AND the path starts with `/api/`:

```go
if host == primaryDomain {
    if strings.HasPrefix(r.URL.Path, "/api/") {
        ws, err := store.GetFirst(ctx)
        if err == nil {
            r = withWorkspace(r, &Info{
                ID:        ws.ID,
                Subdomain: ws.Subdomain,
                Name:      ws.Name,
                Status:    ws.Status,
                IsApex:    false, // <-- NOT treated as apex!
            })
            next.ServeHTTP(w, r)
            return
        }
    }
    // Non-API requests: treated as apex (IsApex: true)
    r = withWorkspace(r, &Info{IsApex: true})
    ...
}
```

Instead of treating `/api/*` requests on the apex as apex requests (which would skip workspace validation), this code:

1. Calls `store.GetFirst()` to get the **first workspace** in the database
2. Sets `IsApex: false` — the request is treated as a **workspace request**
3. The workspace checker then compares the user's WorkspaceID against this first workspace's ID

**This was written as a legacy compatibility hack** when there was only one workspace, to support game clients that called `adroit.games/api/user` directly. The comment says: *"This supports legacy game clients that use apex domain for /api/* endpoints"*.

## Why Dev Works but MHS Doesn't

The first workspace in the database is the **original** workspace — the one created when multi-workspace was first set up. This workspace's subdomain was later renamed from `mhs` to `dev`, but it remains the first workspace.

| User logged into | User's WorkspaceID | GetFirst() returns | Match? | Result |
|---|---|---|---|---|
| `dev.adroit.games` | dev workspace ID | dev workspace ID | YES | Authenticated |
| `mhs.adroit.games` | mhs workspace ID | dev workspace ID | NO | **Unauthenticated** |

The `GetFirst()` call always returns the original (dev) workspace. Dev users match; mhs users don't.

## CORS Configuration

Production CORS settings (`config.toml`):

```toml
enable_cors = true
cors_allowed_origins = ["https://test.adroit.games", "https://cdn.adroit.games"]
cors_allow_credentials = true
```

Neither `mhs.adroit.games` nor `dev.adroit.games` is in the allowed origins. This means cross-origin XHR/fetch from workspace subdomains to the apex would also be blocked by CORS — but the `/api/*` workspace mismatch is the primary failure, occurring before CORS even matters.

## The Solution: Identity Bridge

Rather than fixing cross-origin identity retrieval (which requires both the `/api/*` special case fix AND CORS changes), the game launcher now uses a server-side identity bridge:

- **StrataHub launcher** (`mhs_play.gohtml`): Go handler injects user identity from the session into the page as JavaScript variables. XHR/fetch calls to `/api/user` are intercepted client-side and return the injected data. No cross-origin request needed.

- **CDN-hosted builds** (`mhs-index-template.html`): Identity is passed via URL query parameters (`?name=...&login_id=...`). Same client-side interception technique.

See `docs/game-get-login-id.md` for the recommended long-term approach.

## Fix Applied (February 2026)

The `/api/*` special case in `workspace.go` has been **removed**. Apex `/api/*` requests are now treated as true apex requests (`IsApex: true`), which skips workspace validation and authenticates any logged-in user from any workspace. This means `adroit.games/api/user` now works for users in all workspaces, not just the first one.

## Remaining Future Cleanup

1. **Update game jslib** to use relative URLs (`/api/user`) instead of absolute cross-origin URLs (`https://adroit.games/api/user`). This eliminates the need for CORS entirely.

2. **Add workspace subdomains to CORS** if cross-origin API access from workspace subdomains is still needed:
   ```toml
   cors_allowed_origins = ["https://test.adroit.games", "https://cdn.adroit.games", "https://*.adroit.games"]
   ```

## Key Files

| File | Role |
|------|------|
| `internal/app/system/workspace/workspace.go` | Workspace middleware, `/api/*` special case (lines 84-102) |
| `internal/app/system/auth/auth.go` | Session manager, workspace checker (lines 296-310) |
| `internal/app/bootstrap/config.go` | Session domain auto-derivation (lines 175-181) |
| `internal/app/store/users/fetcher.go` | User fetcher (queries by `_id`, no workspace filter) |
| `internal/app/features/userinfo/handler.go` | `/api/user` endpoint handler |
| `internal/app/features/mhsdelivery/templates/mhs_play.gohtml` | Identity bridge (XHR/fetch interception) |
| `host-test/mhs-index-template.html` | CDN identity bridge (query param based) |
