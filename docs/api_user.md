# /api/user Endpoint

The `/api/user` endpoint provides user authentication status and identity information to external applications like Unity WebGL games.

---

## Endpoint

```
GET /api/user
```

### Response

```json
{
  "isAuthenticated": true,
  "name": "John Doe",
  "email": "jdoe",
  "login_id": "jdoe"
}
```

| Field | Type | Description |
|-------|------|-------------|
| `isAuthenticated` | boolean | Whether the user has a valid session |
| `name` | string | User's full name (empty if not authenticated) |
| `email` | string | User's login ID (for backwards compatibility with existing games) |
| `login_id` | string | User's login ID (preferred field for new integrations) |

**Note:** The `email` field contains the login ID, not an email address. This naming exists for backwards compatibility with existing game integrations. New integrations should use `login_id`.

---

## Cross-Domain Usage

This endpoint is designed to be called from external applications (like Unity WebGL games) hosted on different subdomains.

### Example Scenario

- User logs into: `mhs.adroit.games`
- Unity game hosted at: `cdn.adroit.games` or `test.adroit.games`
- Game calls: `mhs.adroit.games/api/user` or `adroit.games/api/user`
- Response includes the logged-in user's information

### JavaScript/Unity Fetch Example

```javascript
fetch('https://mhs.adroit.games/api/user', {
    method: 'GET',
    credentials: 'include'  // Required to send cookies
})
.then(response => response.json())
.then(data => {
    if (data.isAuthenticated) {
        console.log('User:', data.name, data.login_id);
    } else {
        console.log('Not logged in');
    }
});
```

---

## How Cross-Domain Authentication Works

### 1. Cookie Domain Configuration

In multi-workspace mode, the session cookie is configured with a domain that covers all subdomains:

```go
// internal/app/bootstrap/config.go
if appCfg.MultiWorkspace && appCfg.SessionDomain == "" && appCfg.PrimaryDomain != "" {
    appCfg.SessionDomain = "." + appCfg.PrimaryDomain
    // Results in: ".adroit.games"
}
```

**The leading dot is the key.** When a cookie is set with `Domain=.adroit.games`, the browser sends it to:
- `adroit.games`
- `mhs.adroit.games`
- `cdn.adroit.games`
- `test.adroit.games`
- Any other `*.adroit.games` subdomain

### 2. SameSite Cookie Attribute

The session cookie uses `SameSite=None` in production to allow cross-origin requests:

```go
// internal/app/system/auth/auth.go
if secure {
    opts.SameSite = http.SameSiteNoneMode  // Production
} else {
    opts.SameSite = http.SameSiteLaxMode   // Development
}
```

**`SameSite=None`** allows cookies to be sent in cross-origin requests, which is required for:
- Unity WebGL games calling the API from a different subdomain
- Any cross-subdomain API calls with credentials

**`SameSite=None` requires `Secure=true`** (HTTPS only) - browsers reject `SameSite=None` cookies without the Secure flag.

### 3. Complete Session Cookie Attributes

The session cookie has these attributes in production:

| Attribute | Value | Purpose |
|-----------|-------|---------|
| Name | `stratahub-session` | Identifies the session cookie |
| Domain | `.adroit.games` | Accessible to all subdomains |
| Path | `/` | Valid for all paths |
| Secure | `true` | HTTPS only |
| HttpOnly | `true` | No JavaScript access (security) |
| SameSite | `None` | Sent in cross-origin requests |

---

## Why CSRF Doesn't Block This Endpoint

### GET Requests Are Exempt from CSRF

```go
// internal/app/features/userinfo/routes.go
r.Get("/api/user", h.ServeUserInfo)  // GET method
```

**CSRF protection only applies to state-changing methods:**
- POST, PUT, PATCH, DELETE → require CSRF token
- GET, HEAD, OPTIONS → exempt (considered "safe")

This follows OWASP CSRF prevention guidelines because:
1. GET requests should be idempotent (no side effects)
2. They don't modify server state
3. Reading user information is safe

### The Complete Request Flow

```
Unity WebGL (cdn.adroit.games)
    │
    │  fetch('https://mhs.adroit.games/api/user', {
    │      method: 'GET',
    │      credentials: 'include'  ← Tells browser to send cookies
    │  })
    │
    ▼
Browser checks:
    ├── CORS: Is cdn.adroit.games allowed? ✓ (configured in CORS settings)
    ├── Cookie: Should I send stratahub-session?
    │   ├── Domain .adroit.games matches mhs.adroit.games ✓
    │   ├── SameSite=None allows cross-origin ✓
    │   └── Secure=true and request is HTTPS ✓
    │
    ▼
Request sent to mhs.adroit.games/api/user WITH session cookie
    │
    ▼
Server:
    ├── CSRF middleware: GET request? Skip validation ✓
    ├── Session middleware: Valid session cookie? ✓
    └── Returns: { "isAuthenticated": true, "name": "...", ... }
```

---

## CORS Configuration

For this endpoint to work from external domains, CORS must be configured to allow:

1. **Origin**: The domain hosting the game (e.g., `cdn.adroit.games`, `test.adroit.games`)
2. **Credentials**: Must be `true` to allow cookies
3. **Methods**: At minimum `GET`

CORS is configured via environment variables:
- `WAFFLE_ENABLE_CORS=true`
- `WAFFLE_CORS_ORIGINS=https://cdn.adroit.games,https://test.adroit.games`
- `WAFFLE_CORS_CREDENTIALS=true`

---

## Security Summary

| Mechanism | Setting | Purpose |
|-----------|---------|---------|
| **Cookie Domain** | `.adroit.games` | Makes cookie available to all subdomains |
| **SameSite** | `None` (production) | Allows cookie in cross-origin requests |
| **Secure** | `true` | Required for SameSite=None; HTTPS only |
| **HttpOnly** | `true` | Prevents JavaScript from reading the cookie |
| **CORS** | Whitelist specific origins | Permits cross-origin requests from allowed domains |
| **CSRF** | GET exempt | No token required for read-only endpoints |

The combination of `Domain=.adroit.games` + `SameSite=None` + `Secure=true` allows the session cookie to work across subdomains while still protecting against CSRF attacks on POST requests.

---

## Files Involved

| File | Purpose |
|------|---------|
| `internal/app/features/userinfo/handler.go` | Endpoint implementation |
| `internal/app/features/userinfo/routes.go` | Route registration |
| `internal/app/bootstrap/routes.go` | Mounts the userinfo routes |
| `internal/app/bootstrap/config.go` | Session domain auto-derivation |
| `internal/app/system/auth/auth.go` | Session cookie configuration |
