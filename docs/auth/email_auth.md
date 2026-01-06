# Email Authentication

This document describes the email verification authentication system, including the complete flow, cross-window communication, and implementation details.

## Overview

Email authentication allows users to log in by receiving a verification code or magic link via email. No password is required. This method is selected per-user via the `auth_method` field set to `"email"`.

**Two verification methods are provided:**
1. **6-digit code** - User manually enters the code from the email
2. **Magic link** - User clicks a link in the email to authenticate instantly

## Authentication Flow

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                           EMAIL AUTHENTICATION FLOW                          │
└─────────────────────────────────────────────────────────────────────────────┘

User                    Browser (Tab A)              Server                Email
 │                           │                          │                    │
 │  Enter login ID           │                          │                    │
 │ ─────────────────────────>│                          │                    │
 │                           │  POST /login             │                    │
 │                           │ ────────────────────────>│                    │
 │                           │                          │                    │
 │                           │                          │  Generate code     │
 │                           │                          │  Generate token    │
 │                           │                          │  Store in DB       │
 │                           │                          │                    │
 │                           │                          │  Send email ──────>│
 │                           │                          │                    │
 │                           │  Redirect to             │                    │
 │                           │  /login/verify-email     │                    │
 │                           │ <────────────────────────│                    │
 │                           │                          │                    │
 │  [Option A: Enter Code]   │                          │                    │
 │ ─────────────────────────>│                          │                    │
 │                           │  POST /login/verify-email│                    │
 │                           │ ────────────────────────>│                    │
 │                           │                          │  Verify code       │
 │                           │                          │  Create session    │
 │                           │  Redirect to /dashboard  │                    │
 │                           │ <────────────────────────│                    │
 │                           │                          │                    │
 │  [Option B: Click Link]   │                          │                    │
 │ ──────────────────────────────────────── Tab B opens │                    │
 │                           │             │            │                    │
 │                           │             │  GET /login/verify-email?token=…│
 │                           │             │ ──────────>│                    │
 │                           │             │            │  Verify token      │
 │                           │             │            │  Create session    │
 │                           │             │  Success   │                    │
 │                           │             │  page      │                    │
 │                           │             │ <──────────│                    │
 │                           │             │            │                    │
 │                           │  Broadcast  │            │                    │
 │                           │ <───────────│            │                    │
 │                           │  (login_success)        │                    │
 │                           │                          │                    │
 │                           │  Auto-redirect to        │                    │
 │                           │  /dashboard              │                    │
 │                           │                          │                    │
```

## Key Components

### Routes

Defined in `internal/app/features/login/routes.go`:

| Method | Path | Handler | Purpose |
|--------|------|---------|---------|
| GET | `/login` | ServeLogin | Display login form |
| POST | `/login` | HandleLoginPost | Process login, start email flow |
| GET | `/login/verify-email` | ServeVerifyEmail | Display code entry form / handle magic link |
| POST | `/login/verify-email` | HandleVerifyEmailSubmit | Verify entered code |
| POST | `/login/resend-code` | HandleResendCode | Resend verification email |

### Handler Functions

Located in `internal/app/features/login/handler.go`:

#### `startEmailFlow()`
Initiates email verification:
1. Gets user's email (from `Email` field or `LoginID` if it's an email)
2. Creates verification record via `EmailVerify.Create()`
3. Builds magic link URL
4. Sends verification email via `Mailer.Send()`
5. Stores pending state in session
6. Redirects to `/login/verify-email`

#### `handleMagicLink()`
Processes magic link clicks:
1. Extracts token from query parameter
2. Verifies token via `EmailVerify.VerifyToken()`
3. Loads user from database
4. Creates authenticated session
5. Renders success page (broadcasts to other tabs)

#### `HandleVerifyEmailSubmit()`
Processes manually entered codes:
1. Gets pending user ID from session
2. Verifies code via `EmailVerify.VerifyCode()`
3. Creates authenticated session
4. Redirects to dashboard

### Email Verification Store

Located in `internal/app/store/emailverify/store.go`:

```go
type Verification struct {
    ID          primitive.ObjectID `bson:"_id,omitempty"`
    UserID      primitive.ObjectID `bson:"user_id"`
    Email       string             `bson:"email"`
    CodeHash    string             `bson:"code_hash"`  // bcrypt hash of 6-digit code
    Token       string             `bson:"token"`      // 64-char hex token for magic link
    ExpiresAt   time.Time          `bson:"expires_at"` // TTL field (10 minutes)
    CreatedAt   time.Time          `bson:"created_at"`
    Attempts    int                `bson:"attempts"`     // Failed verification attempts
    ResendCount int                `bson:"resend_count"` // Resends within window
    WindowStart time.Time          `bson:"window_start"` // Rate limit window start
}
```

**Key methods:**
- `Create(ctx, userID, email, isResend)` - Generate new code/token, store hashed
- `VerifyCode(ctx, userID, code)` - Verify code, delete on success
- `VerifyToken(ctx, token)` - Verify magic link token, delete on success

**Security features:**
- Codes are bcrypt hashed (cost 10) before storage
- Tokens are 32 bytes (256 bits) of random data
- Records auto-expire via MongoDB TTL index after 10 minutes
- Single-use: records deleted after successful verification

### Mailer

Located in `internal/app/system/mailer/`:

- `mailer.go` - SMTP email sending
- `templates.go` - Email template builder

```go
type VerificationEmailData struct {
    SiteName  string
    Code      string
    MagicLink string
    ExpiresIn string
}
```

The email contains both the 6-digit code and a clickable magic link button.

## Cross-Window Communication (BroadcastChannel)

When a user clicks the magic link, it opens in a new browser tab/window. To provide a seamless experience, we use the BroadcastChannel API to notify the original tab that authentication succeeded.

### How It Works

1. **Original tab** (`/login/verify-email`) sets up a listener
2. **Magic link tab** completes authentication and broadcasts a message
3. **Original tab** receives the message and redirects to dashboard
4. **Magic link tab** shows "You can close this window" message

### Implementation

#### Listener (verify_email.gohtml)

```html
<!-- Listen for magic link success from another tab -->
<script>
(function() {
  if (typeof BroadcastChannel === 'undefined') return;
  var channel = new BroadcastChannel('stratahub_auth');
  channel.onmessage = function(event) {
    if (event.data && event.data.type === 'login_success') {
      window.location.href = event.data.returnURL || '/dashboard';
    }
  };
})();
</script>
```

#### Broadcaster (magic_link_success.gohtml)

```html
<script>
(function() {
  var returnURL = "{{ .ReturnURL }}";
  var dest = returnURL || '/dashboard';

  // Broadcast to other tabs/windows that login succeeded
  if (typeof BroadcastChannel !== 'undefined') {
    var channel = new BroadcastChannel('stratahub_auth');
    channel.postMessage({ type: 'login_success', returnURL: dest });
  }
})();
</script>
```

### Channel Details

| Property | Value |
|----------|-------|
| Channel name | `stratahub_auth` |
| Message type | `login_success` |
| Payload | `{ type: 'login_success', returnURL: '/dashboard' }` |

### Browser Support

BroadcastChannel is supported in all modern browsers. The implementation gracefully degrades - if not supported, each tab operates independently (user manually navigates).

### User Experience

**With BroadcastChannel (modern browsers):**
- Original tab: Auto-redirects to dashboard
- Magic link tab: Shows "You're signed in! You can close this window."

**Without BroadcastChannel (older browsers):**
- Original tab: Stays on verify page (user can refresh or navigate)
- Magic link tab: Shows success message

## Session Management

### Pending State (during verification)

Stored in session while awaiting code/link verification:

```go
sess.Values["pending_user_id"]    = userID.Hex()
sess.Values["pending_login_id"]   = loginID
sess.Values["pending_email"]      = email
sess.Values["pending_return_url"] = returnURL
```

### Authenticated State (after verification)

```go
sess.Values["is_authenticated"] = true
sess.Values["user_id"]          = userID.Hex()
// pending_* values are cleared
```

## Templates

| File | Purpose |
|------|---------|
| `login/templates/verify_email.gohtml` | Code entry form with BroadcastChannel listener |
| `login/templates/magic_link_success.gohtml` | Success page shown after magic link auth |

## Configuration

### Environment Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `STRATAHUB_MAIL_SMTP_HOST` | localhost | SMTP server hostname |
| `STRATAHUB_MAIL_SMTP_PORT` | 1025 | SMTP server port |
| `STRATAHUB_MAIL_SMTP_USER` | (empty) | SMTP username |
| `STRATAHUB_MAIL_SMTP_PASS` | (empty) | SMTP password |
| `STRATAHUB_MAIL_FROM` | noreply@stratahub.com | From email address |
| `STRATAHUB_MAIL_FROM_NAME` | StrataHub | From display name |
| `STRATAHUB_BASE_URL` | http://localhost:3000 | Base URL for magic links |
| `STRATAHUB_EMAIL_VERIFY_EXPIRY` | 10m | Email verification code/link expiry (e.g., 10m, 1h, 90s) |

### Development Setup

Use [Mailpit](https://github.com/axllent/mailpit) for local email testing:

```bash
# Install
brew install mailpit

# Run
mailpit
# or
brew services start mailpit
```

- **SMTP**: localhost:1025
- **Web UI**: http://localhost:8025

### Constants

In `internal/app/store/emailverify/store.go`:

```go
const (
    CodeLength        = 6              // 6-digit code
    TokenLength       = 32             // 32 bytes = 64 hex chars
    DefaultExpiry     = 10 * time.Minute // Fallback if config is 0 or negative
    BcryptCost        = 10
    MaxVerifyAttempts = 5              // Rate limit: code attempts
    MaxResends        = 3              // Rate limit: resends per window
    ResendWindow      = 10 * time.Minute
)
```

**Note:** The expiry duration is configurable via `email_verify_expiry_minutes` in the config. `DefaultExpiry` is only used as a fallback if the configured value is 0 or negative.

## Database Indexes

Created in `internal/app/system/indexes/indexes.go`:

```go
// TTL index for auto-cleanup of expired verifications
{Keys: bson.D{{Key: "expires_at", Value: 1}}, Options: options.Index().SetExpireAfterSeconds(0)}

// Fast lookup by magic link token
{Keys: bson.D{{Key: "token", Value: 1}}}

// Fast lookup and cleanup by user
{Keys: bson.D{{Key: "user_id", Value: 1}}}
```

## Error Handling

| Error | Cause | User Message |
|-------|-------|--------------|
| `ErrNotFound` | Code expired or doesn't exist | "Invalid or expired verification code." |
| `ErrInvalidCode` | Wrong code entered | "Invalid or expired verification code." |
| `ErrTooManyAttempts` | 5+ failed attempts | "Too many incorrect attempts. Please request a new verification code." |
| `ErrTooManyResends` | 3+ resends in 10 min | "Too many resend attempts. Please wait a few minutes before trying again." |

## Security Considerations

1. **Code entropy**: 6 digits = 1,000,000 possibilities, rate-limited to 5 attempts
2. **Token entropy**: 256 bits, computationally infeasible to brute-force
3. **Bcrypt hashing**: Codes stored as bcrypt hashes, not plaintext
4. **Single-use**: Verification records deleted after successful use
5. **TTL expiry**: Records auto-expire after 10 minutes
6. **Rate limiting**: Per-user limits prevent abuse without blocking shared IPs

## File Reference

| File | Purpose |
|------|---------|
| `internal/app/features/login/handler.go` | Main login flow handlers |
| `internal/app/features/login/routes.go` | Route definitions |
| `internal/app/features/login/templates/verify_email.gohtml` | Code entry UI |
| `internal/app/features/login/templates/magic_link_success.gohtml` | Magic link success UI |
| `internal/app/store/emailverify/store.go` | Verification record CRUD |
| `internal/app/system/mailer/mailer.go` | SMTP email sending |
| `internal/app/system/mailer/templates.go` | Email template builder |

## See Also

- [Email Auth Rate Limiting](email_auth_rate_limiting.md) - Detailed rate limiting documentation
- [Password Auth](password_auth.md) - Password authentication
- [Trust Auth](trust_auth.md) - Trust authentication
- [Auth Plan](auth_plan.md) - Overall authentication architecture
- [Configuration](../configuration.md) - Full configuration reference
