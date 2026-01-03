# Authentication Implementation Plan

This document captures the authentication architecture and planned features for StrataHub.

---

## Current State

### Auth Fields (Implemented)

The User model now includes these authentication-related fields:

| Field | Purpose |
|-------|---------|
| `login_id` | Primary identifier for login (email address, username, or SSO ID) |
| `login_id_ci` | Case/diacritic-insensitive version for matching |
| `auth_return_id` | ID returned by external auth providers (SSO sub/id) |
| `email` | Contact email (may differ from login_id for SSO users) |
| `auth_method` | How the user authenticates |

### Auth Methods

Defined in validators (`internal/app/system/validators/validators.go`):

| Method | Description | Status |
|--------|-------------|--------|
| `trust` | No password required, login by email only | Implemented |
| `password` | Email + password authentication | Planned |
| `email` | Magic link / email verification | Planned |
| `google` | Google OAuth/OIDC | Planned |
| `microsoft` | Microsoft OAuth/OIDC | Planned |
| `classlink` | ClassLink SSO | Planned |
| `clever` | Clever SSO | Planned |
| `internal` | Reserved for system/service accounts | Reserved |

### Current Login Flow (Trust)

1. User enters email at `/login`
2. System looks up user by `login_id_ci` (case-insensitive match)
3. If found and active, session is created
4. No password verification (trust mode)

---

## Planned: Password Authentication

### Overview

Add traditional email + password authentication with secure password handling.

### Database Changes

Add to User model:

```go
PasswordHash     string     `bson:"password_hash,omitempty"`
PasswordTempFlag bool       `bson:"password_temp,omitempty"`    // True if password is temporary
PasswordSetAt    *time.Time `bson:"password_set_at,omitempty"`  // When password was last set
```

### User Creation Flow

1. Admin creates user with `auth_method: "password"`
2. System generates temporary password (secure random string)
3. `password_temp` flag set to `true`
4. Temporary password displayed to admin (one time only)
5. Admin shares temporary password with user via secure channel

### Login Flow (Password)

1. User enters email + password at `/login`
2. System looks up user by `login_id_ci`
3. Verify `auth_method == "password"`
4. Compare password against `password_hash` using bcrypt
5. If `password_temp == true`:
   - Redirect to `/change-password` (forced)
   - User must set new password before continuing
6. Create session on success

### Password Change Flow

1. User accesses `/change-password` (forced or voluntary)
2. If forced (temp password): only show new password fields
3. If voluntary: require current password verification
4. Validate new password:
   - Minimum 8 characters
   - At least one letter and one number
   - Not same as current password
5. Hash new password with bcrypt (cost 12)
6. Update `password_hash`, clear `password_temp`, set `password_set_at`

### Password Reset (Admin-Initiated)

1. Admin clicks "Reset Password" on user edit page
2. System generates new temporary password
3. Sets `password_temp = true`
4. Displays new temporary password to admin
5. User must change on next login

### Security Considerations

- Use `bcrypt` with cost factor of 12
- Never store plaintext passwords
- Never log passwords (even hashed)
- Rate limit login attempts
- Lock account after N failed attempts (consider)
- Temporary passwords expire after N hours (consider)

---

## Planned: Email Verification

### Overview

Verify user email addresses and optionally use email for passwordless login (magic links).

### Database Changes

Add to User model:

```go
EmailVerified   bool       `bson:"email_verified,omitempty"`
EmailVerifiedAt *time.Time `bson:"email_verified_at,omitempty"`
```

New collection `email_tokens`:

```go
type EmailToken struct {
    ID        primitive.ObjectID `bson:"_id,omitempty"`
    UserID    primitive.ObjectID `bson:"user_id"`
    Token     string             `bson:"token"`      // Secure random token
    TokenHash string             `bson:"token_hash"` // For lookup (hashed)
    Purpose   string             `bson:"purpose"`    // "verify" or "magic_link"
    ExpiresAt time.Time          `bson:"expires_at"`
    UsedAt    *time.Time         `bson:"used_at,omitempty"`
    CreatedAt time.Time          `bson:"created_at"`
}
```

### Email Verification Flow

1. User created or email changed
2. System generates verification token (32-byte random, base64url encoded)
3. Store hashed token in `email_tokens` with expiry (24 hours)
4. Send email with verification link: `/verify-email?token=...`
5. User clicks link
6. System validates token (not expired, not used)
7. Mark `email_verified = true`, `email_verified_at = now`
8. Mark token as used

### Magic Link Login (auth_method: "email")

1. User enters email at `/login`
2. System verifies user exists with `auth_method: "email"`
3. Generate magic link token (shorter expiry: 15 minutes)
4. Send email with login link: `/login/magic?token=...`
5. User clicks link
6. System validates token
7. Create session, mark token as used

### Email Sending

- Abstract email sending behind interface
- Support multiple providers (SMTP, SendGrid, SES, etc.)
- Template-based emails
- Configuration via environment variables

---

## Planned: OAuth/SSO Integration

### Supported Providers

| Provider | Protocol | Use Case |
|----------|----------|----------|
| Google | OAuth 2.0 / OIDC | General |
| Microsoft | OAuth 2.0 / OIDC | Enterprise / Education |
| ClassLink | OAuth 2.0 | K-12 Education |
| Clever | OAuth 2.0 | K-12 Education |

### Database Usage

- `login_id`: Provider-specific unique ID (e.g., `google:123456789`)
- `auth_return_id`: The `sub` claim or provider's user ID
- `email`: Email from provider (may differ from login_id)
- `auth_method`: Provider name (e.g., `google`, `clever`)

### OAuth Flow

1. User clicks "Login with [Provider]" button
2. Redirect to provider's authorization endpoint
3. User authenticates with provider
4. Provider redirects back with authorization code
5. Exchange code for tokens
6. Fetch user info from provider
7. Look up user by `auth_return_id` + `auth_method`
8. If found: create session
9. If not found: either reject or auto-provision (configurable)

### Provider Configuration

Each provider needs:
- Client ID
- Client Secret
- Redirect URI
- Scopes (email, profile, etc.)
- Discovery URL (for OIDC)

Store in app configuration with environment variable overrides.

---

## Implementation Priority

1. **Password Authentication** - Most requested, enables standalone operation
2. **Email Verification** - Required for self-service password reset
3. **Magic Link Login** - Alternative to passwords for some users
4. **Google OAuth** - Widely used, good OIDC support
5. **Microsoft OAuth** - Enterprise/education customers
6. **ClassLink/Clever** - K-12 education market

---

## Routes Summary

### Current
- `GET /login` - Login form
- `POST /login` - Process login (trust mode)
- `GET /logout` - End session
- `GET /api/user` - User info for games (JSON)

### Planned
- `GET /login` - Updated login form (email + password fields, SSO buttons)
- `POST /login` - Process email/password login
- `GET /change-password` - Password change form
- `POST /change-password` - Process password change
- `GET /verify-email` - Email verification handler
- `GET /login/magic` - Magic link handler
- `GET /auth/google` - Initiate Google OAuth
- `GET /auth/google/callback` - Google OAuth callback
- `GET /auth/microsoft` - Initiate Microsoft OAuth
- `GET /auth/microsoft/callback` - Microsoft OAuth callback
- `GET /auth/classlink` - Initiate ClassLink OAuth
- `GET /auth/classlink/callback` - ClassLink OAuth callback
- `GET /auth/clever` - Initiate Clever OAuth
- `GET /auth/clever/callback` - Clever OAuth callback

---

## Configuration Keys

```
# Password settings
STRATAHUB_PASSWORD_MIN_LENGTH=8
STRATAHUB_PASSWORD_BCRYPT_COST=12
STRATAHUB_PASSWORD_TEMP_EXPIRY_HOURS=72

# Email settings
STRATAHUB_EMAIL_PROVIDER=smtp
STRATAHUB_EMAIL_FROM=noreply@stratahub.example.com
STRATAHUB_SMTP_HOST=...
STRATAHUB_SMTP_PORT=587
STRATAHUB_SMTP_USER=...
STRATAHUB_SMTP_PASS=...

# Token expiry
STRATAHUB_EMAIL_VERIFY_EXPIRY_HOURS=24
STRATAHUB_MAGIC_LINK_EXPIRY_MINUTES=15

# OAuth providers
STRATAHUB_OAUTH_GOOGLE_CLIENT_ID=...
STRATAHUB_OAUTH_GOOGLE_CLIENT_SECRET=...
STRATAHUB_OAUTH_MICROSOFT_CLIENT_ID=...
STRATAHUB_OAUTH_MICROSOFT_CLIENT_SECRET=...
STRATAHUB_OAUTH_CLASSLINK_CLIENT_ID=...
STRATAHUB_OAUTH_CLASSLINK_CLIENT_SECRET=...
STRATAHUB_OAUTH_CLEVER_CLIENT_ID=...
STRATAHUB_OAUTH_CLEVER_CLIENT_SECRET=...
```

---

## Notes

- Trust mode remains available for development and legacy support
- Auth method is per-user, allowing mixed authentication within an organization
- All passwords use bcrypt; never store or log plaintext
- Rate limiting should be implemented at the login endpoint
- Consider account lockout after failed attempts
- SSO auto-provisioning is organization-configurable
