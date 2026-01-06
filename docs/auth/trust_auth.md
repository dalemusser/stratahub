# Trust Authentication

This document describes the trust authentication method, which provides immediate login without any password or verification step.

## Overview

Trust authentication is the simplest auth method. Users enter their login ID and are immediately authenticated without any additional verification. This method is selected per-user via the `auth_method` field set to `"trust"`.

**Use cases:**
- Development and testing environments
- Young students who cannot manage passwords
- Low-security scenarios where convenience is prioritized
- Environments with physical access control (e.g., supervised computer labs)

## Authentication Flow

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                          TRUST AUTHENTICATION FLOW                           │
└─────────────────────────────────────────────────────────────────────────────┘

User                    Browser                      Server
 │                         │                            │
 │  Enter login ID         │                            │
 │ ───────────────────────>│                            │
 │                         │  POST /login               │
 │                         │ ──────────────────────────>│
 │                         │                            │  Look up user
 │                         │                            │  Verify user exists
 │                         │                            │  Verify user not disabled
 │                         │                            │  Check auth_method = "trust"
 │                         │                            │  Create session immediately
 │                         │  Redirect to /dashboard    │
 │                         │ <──────────────────────────│
 │                         │                            │
```

This is a single-step authentication - no password page, no verification codes, no additional prompts.

## Routes

| Method | Path | Handler | Purpose |
|--------|------|---------|---------|
| GET | `/login` | ServeLogin | Display login form |
| POST | `/login` | HandleLoginPost | Process login, create session immediately |

## Implementation

Located in `internal/app/features/login/handler.go`:

```go
switch authMethod {
case "trust":
    // Trust auth: create session immediately
    h.createSessionAndRedirect(w, r, &u, ret)
// ... other auth methods
}
```

The entire trust authentication is handled in `HandleLoginPost()`:

1. Parse login ID from form
2. Look up user by `login_id_ci` (case-insensitive)
3. Verify user exists and is not disabled
4. Check `auth_method == "trust"`
5. Call `createSessionAndRedirect()` immediately
6. User is now logged in

## User Configuration

### Required Fields

When creating a user with trust auth:

| Field | Required | Description |
|-------|----------|-------------|
| `login_id` | Yes | Unique identifier for login |
| `auth_method` | Yes | Must be `"trust"` |
| `email` | No | Optional contact email |

### Admin Interface

In the Edit User form with Trust auth selected:
- **Login ID** field is shown (required)
- **Email** field is shown (optional)
- No password fields
- No additional auth configuration

## Session Management

Trust auth creates an authenticated session immediately:

```go
sess.Values["is_authenticated"] = true
sess.Values["user_id"]          = userID.Hex()
```

No pending state is used since there's no multi-step flow.

## Security Considerations

### When to Use Trust Auth

**Appropriate scenarios:**
- Development/testing environments
- Very young students (K-2) who cannot manage passwords
- Supervised environments with physical access control
- Demo accounts for product evaluation
- Situations where another system handles authentication (e.g., device login)

**Inappropriate scenarios:**
- Production systems with sensitive data
- Self-service user registration
- Remote access without supervision
- Systems containing PII or confidential information

### Risks

| Risk | Description |
|------|-------------|
| No verification | Anyone who knows a login ID can access the account |
| No audit trail | Cannot distinguish between legitimate and unauthorized access |
| Credential sharing | Login IDs may be easily shared or guessed |
| No password to compromise | But also no password to protect |

### Mitigations

If using trust auth in production:

1. **Physical access control** - Only allow from supervised locations
2. **Network restrictions** - Limit to specific IP ranges or VPN
3. **Non-obvious login IDs** - Don't use predictable patterns like `student1`, `student2`
4. **Regular monitoring** - Review access logs for anomalies
5. **Limited permissions** - Trust users should have minimal privileges

## Comparison with Other Methods

| Feature | Trust | Password | Email |
|---------|-------|----------|-------|
| Steps to login | 1 | 2 | 2-3 |
| Verification required | No | Yes (password) | Yes (code/link) |
| User manages credential | No | Yes | No |
| Suitable for young children | Yes | Maybe | No |
| Security level | Low | Medium | Medium |

## Database Schema

Users with trust auth have:

```go
type User struct {
    LoginID     *string `bson:"login_id"`      // Required for login
    LoginIDCI   *string `bson:"login_id_ci"`   // Case-folded for lookup
    AuthMethod  string  `bson:"auth_method"`   // "trust"
    Email       *string `bson:"email"`         // Optional
    // PasswordHash and PasswordTemp are not used
}
```

## Error Handling

| Scenario | Message |
|----------|---------|
| Login ID not found | "No account found with that login ID." |
| User disabled | "This account has been disabled." |
| Empty login ID | "Please enter a login ID or email." |

## Configuration

Trust auth has no additional configuration. It's enabled by setting a user's `auth_method` to `"trust"`.

### Converting Users to Trust Auth

To convert an existing user to trust auth:

1. Edit the user in the admin interface
2. Change Auth Method to "Trust"
3. Ensure Login ID is set
4. Save changes

The user can now log in with just their login ID.

## File Reference

| File | Purpose |
|------|---------|
| `internal/app/features/login/handler.go` | Login flow (lines 178-180) |
| `internal/app/features/login/routes.go` | Route definitions |
| `internal/app/features/login/templates/login.gohtml` | Login form |
| `internal/domain/models/authmethods.go` | Auth method definitions |

## See Also

- [Password Auth](password_auth.md) - Password authentication
- [Email Auth](email_auth.md) - Email verification authentication
- [Auth Plan](auth_plan.md) - Overall authentication architecture
- [Configuration](../configuration.md) - Full configuration reference
