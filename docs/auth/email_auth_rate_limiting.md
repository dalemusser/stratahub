# Email Authentication Rate Limiting

This document describes the rate limiting implemented for email verification authentication to prevent abuse while remaining compatible with high-density environments like schools.

## Design Principle: Per-User, Not Per-IP

Rate limiting is applied **per-user** rather than per-IP address. This is intentional for school environments where hundreds of students may share the same IP address (behind NAT or a proxy). Per-IP rate limiting would unfairly block legitimate users in these scenarios.

## Rate Limits

### Verification Code Attempts

| Setting | Value |
|---------|-------|
| Maximum attempts | 5 |
| Scope | Per verification code |
| Reset | When a new code is requested |

**Behavior:**
- Each incorrect code entry increments the attempt counter
- After 5 failed attempts, further verification attempts are blocked
- User sees: *"Too many incorrect attempts. Please request a new verification code."*
- Requesting a new code (via resend) creates a fresh verification with the counter reset to 0

**Purpose:** Prevents brute-force attacks on the 6-digit verification code (1 million possible combinations).

### Code Resend Requests

| Setting | Value |
|---------|-------|
| Maximum resends | 3 |
| Time window | 10 minutes |
| Scope | Per user |

**Behavior:**
- Each resend request increments the resend counter within a 10-minute sliding window
- After 3 resends within the window, further resend requests are blocked
- User sees: *"Too many resend attempts. Please wait a few minutes before trying again."*
- The window resets after 10 minutes of no resend activity

**Purpose:** Prevents email spam/abuse by limiting how many verification emails can be triggered.

## Implementation Details

### Database Fields

The `email_verifications` collection stores rate limiting state:

```go
type Verification struct {
    // ... existing fields ...
    Attempts    int       `bson:"attempts"`     // Failed verification attempts
    ResendCount int       `bson:"resend_count"` // Resends within current window
    WindowStart time.Time `bson:"window_start"` // Start of rate limit window
}
```

### Constants

Defined in `internal/app/store/emailverify/store.go`:

```go
const (
    MaxVerifyAttempts = 5          // Max code verification attempts
    MaxResends        = 3          // Max resends within window
    ResendWindow      = 10 * time.Minute
)
```

### Error Types

```go
var (
    ErrTooManyAttempts = errors.New("too many verification attempts")
    ErrTooManyResends  = errors.New("too many resend requests")
)
```

## User Experience

### Normal Flow

1. User enters email at login
2. Verification email sent with 6-digit code and magic link
3. User enters code or clicks link
4. User is authenticated

### Rate Limited: Too Many Code Attempts

1. User enters wrong code 5 times
2. Error displayed: "Too many incorrect attempts. Please request a new verification code."
3. User clicks "Resend verification email"
4. New code is generated, attempt counter resets
5. User can try again with the new code

### Rate Limited: Too Many Resends

1. User requests resend 3 times within 10 minutes
2. Error displayed: "Too many resend attempts. Please wait a few minutes before trying again."
3. User must wait for the 10-minute window to expire
4. After waiting, resend becomes available again

## Security Considerations

- **6-digit codes**: With 5 attempts allowed, brute-force probability is 5/1,000,000 = 0.0005%
- **Magic link tokens**: 256-bit random tokens are not rate-limited (infeasible to brute-force)
- **Code expiry**: All codes expire after the configured time (default 10 minutes) regardless of attempt count
- **Single-use**: Both codes and magic links are deleted after successful verification

## Configuration

### Expiry Duration

The verification code/link expiry is configurable via:

- Config file: `email_verify_expiry = "15m"` (or `"1h"`, `"90s"`, etc.)
- Environment: `STRATAHUB_EMAIL_VERIFY_EXPIRY=15m`
- Command line: `--email_verify_expiry=15m`

Supported formats:
- Duration strings: `"10m"`, `"1h30m"`, `"90s"`, `"2h"`
- Numeric seconds: `600` (interpreted as 600 seconds)

Default is 10 minutes (`"10m"`).

### Rate Limit Values

Currently, rate limit values are constants in the code. To modify:

1. Edit `internal/app/store/emailverify/store.go`
2. Adjust `MaxVerifyAttempts`, `MaxResends`, or `ResendWindow`
3. Rebuild and deploy

Future enhancement: Make these values configurable via environment variables or site settings.

## See Also

- [Email Auth](email_auth.md) - Full email authentication documentation
- [Password Auth](password_auth.md) - Password authentication
- [Trust Auth](trust_auth.md) - Trust authentication
- [Auth Plan](auth_plan.md) - Overall authentication architecture
