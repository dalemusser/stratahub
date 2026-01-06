# Security Audit Logging Plan

This document outlines the plan for implementing the "Security" category of audit logging. The infrastructure exists (CategorySecurity constant in audit store) but no security events are currently implemented.

## Overview

Security audit events differ from authentication events in that they focus on **threat detection and response** rather than normal auth flow. These events help identify suspicious activity, track security incidents, and provide forensic data.

## Proposed Security Events

### Account Security Events

| Event Constant | Event Type | Description |
|----------------|------------|-------------|
| `EventAccountLocked` | `account_locked` | Account locked after too many failed login attempts |
| `EventAccountUnlocked` | `account_unlocked` | Account manually unlocked by admin |
| `EventSuspiciousLoginPattern` | `suspicious_login_pattern` | Multiple failed logins detected from same IP |
| `EventLoginFromNewLocation` | `login_from_new_location` | Login from unrecognized IP/location |
| `EventConcurrentSessionDetected` | `concurrent_session_detected` | Same account logged in from multiple locations |

### Access Control Events

| Event Constant | Event Type | Description |
|----------------|------------|-------------|
| `EventUnauthorizedAccessAttempt` | `unauthorized_access_attempt` | User attempted to access resource without permission |
| `EventPrivilegeEscalationAttempt` | `privilege_escalation_attempt` | User attempted admin action without admin role |
| `EventRoleChanged` | `role_changed_security` | User role was elevated (security-relevant) |

### Rate Limiting Events

| Event Constant | Event Type | Description |
|----------------|------------|-------------|
| `EventRateLimitExceeded` | `rate_limit_exceeded` | User/IP exceeded rate limit threshold |
| `EventIPBlocked` | `ip_blocked` | IP address was blocked |
| `EventIPUnblocked` | `ip_unblocked` | IP address was unblocked |

### Password Security Events

| Event Constant | Event Type | Description |
|----------------|------------|-------------|
| `EventPasswordResetRequested` | `password_reset_requested` | Password reset was requested |
| `EventPasswordResetCompleted` | `password_reset_completed` | Password reset was completed |
| `EventPasswordResetExpired` | `password_reset_expired` | Password reset link expired unused |

### Session Security Events

| Event Constant | Event Type | Description |
|----------------|------------|-------------|
| `EventSessionInvalidated` | `session_invalidated` | Session was forcibly terminated |
| `EventAllSessionsInvalidated` | `all_sessions_invalidated` | All user sessions were terminated |
| `EventSessionHijackingDetected` | `session_hijacking_detected` | Possible session hijacking detected |

### Data Security Events

| Event Constant | Event Type | Description |
|----------------|------------|-------------|
| `EventBulkDataExport` | `bulk_data_export` | Large data export performed |
| `EventSensitiveDataAccess` | `sensitive_data_access` | Access to sensitive/PII data |

## Implementation Requirements

### 1. Add Event Constants

**File:** `internal/app/store/audit/store.go`

```go
// Security event types
const (
    EventAccountLocked              = "account_locked"
    EventAccountUnlocked            = "account_unlocked"
    EventSuspiciousLoginPattern     = "suspicious_login_pattern"
    EventUnauthorizedAccessAttempt  = "unauthorized_access_attempt"
    EventRateLimitExceeded          = "rate_limit_exceeded"
    EventIPBlocked                  = "ip_blocked"
    EventPasswordResetRequested     = "password_reset_requested"
    EventPasswordResetCompleted     = "password_reset_completed"
    EventSessionInvalidated         = "session_invalidated"
    EventAllSessionsInvalidated     = "all_sessions_invalidated"
    EventBulkDataExport             = "bulk_data_export"
)
```

### 2. Add Logger Methods

**File:** `internal/app/system/auditlog/logger.go`

Each security event needs a corresponding logger method. Example:

```go
// AccountLocked logs when an account is locked due to failed attempts.
func (l *Logger) AccountLocked(ctx context.Context, r *http.Request, userID primitive.ObjectID, reason string, attemptCount int) {
    if l == nil || !l.shouldLog(l.cfg.Admin) { // or new l.cfg.Security
        return
    }

    event := audit.Event{
        Category:  audit.CategorySecurity,
        EventType: audit.EventAccountLocked,
        UserID:    &userID,
        IP:        extractIP(r),
        UserAgent: r.UserAgent(),
        Success:   true,
        Details: map[string]string{
            "reason":        reason,
            "attempt_count": strconv.Itoa(attemptCount),
        },
    }

    l.log(ctx, event, "account locked", zap.String("user_id", userID.Hex()))
}
```

### 3. Add Configuration Option

**File:** `internal/app/bootstrap/appconfig.go`

```go
type AppConfig struct {
    // ... existing fields
    AuditLogSecurity string `toml:"audit_log_security"` // "all", "db", "log", "off"
}
```

**File:** `internal/app/system/auditlog/logger.go`

```go
type Config struct {
    Auth     string
    Admin    string
    Security string // Add this
}
```

### 4. Update Audit Log UI

**File:** `internal/app/features/auditlog/types.go`

Re-add Security to categories:
```go
func allCategories() []categoryOption {
    return []categoryOption{
        {Value: audit.CategoryAuth, Label: "Authentication"},
        {Value: audit.CategoryAdmin, Label: "Administration"},
        {Value: audit.CategorySecurity, Label: "Security"},
    }
}
```

Add security events to `eventTypesForCategory()`:
```go
securityEvents := []string{
    audit.EventAccountLocked,
    audit.EventAccountUnlocked,
    // ... etc
}

case audit.CategorySecurity:
    return securityEvents
```

## Supporting Features Required

Some security events require supporting infrastructure that doesn't exist yet:

### Account Lockout System

**Required for:** `account_locked`, `account_unlocked`, `suspicious_login_pattern`

- Track failed login attempts per user/IP
- Lock account after N failures (configurable threshold)
- Auto-unlock after timeout or manual admin unlock
- Store: `login_attempts` collection or Redis

**Implementation:**
1. Create `internal/app/store/loginattempts/store.go`
2. Add `LockedUntil` field to User model
3. Check lock status in login handler
4. Increment/reset attempts on login success/failure

### IP Tracking/Blocking

**Required for:** `ip_blocked`, `ip_unblocked`, `rate_limit_exceeded`

- Track requests per IP
- Block IPs that exceed thresholds
- Admin UI to manage blocked IPs
- Store: `blocked_ips` collection or Redis

### Session Management

**Required for:** `session_invalidated`, `all_sessions_invalidated`

- Track active sessions per user
- Ability to invalidate specific sessions
- "Log out all devices" feature
- Store: Already have sessions, need invalidation tracking

### Login Location Tracking

**Required for:** `login_from_new_location`

- Store known login IPs per user
- Compare new logins against history
- Optional: GeoIP lookup for location

## Phased Implementation

### Phase 1: Basic Security Events (Low effort)
- Password reset events (already have reset flow)
- Unauthorized access attempts (add to authz checks)
- Bulk data export (add to CSV export handlers)

### Phase 2: Account Lockout (Medium effort)
- Login attempt tracking
- Account locking logic
- Admin unlock UI
- Related audit events

### Phase 3: Rate Limiting (Medium effort)
- Request rate tracking (middleware)
- IP blocking system
- Admin UI for blocked IPs
- Related audit events

### Phase 4: Advanced Detection (High effort)
- Session anomaly detection
- Login location tracking
- GeoIP integration
- Concurrent session detection

## Event Details Schema

Security events should include rich details for forensic analysis:

```go
Details: map[string]string{
    // Common fields
    "ip":              "192.168.1.1",
    "user_agent":      "Mozilla/5.0...",
    "request_path":    "/admin/users",
    "request_method":  "POST",

    // Event-specific fields
    "attempt_count":   "5",
    "threshold":       "3",
    "lock_duration":   "30m",
    "blocked_reason":  "excessive_requests",
    "previous_ip":     "10.0.0.1",
    "geo_country":     "US",
    "geo_city":        "New York",
}
```

## Testing Considerations

- Unit tests for each logger method
- Integration tests for lockout flow
- Load tests for rate limiting
- Security audit of the audit system itself (ensure logs can't be tampered with)

## Related Documents

- `audit_logging_plan.md` - Overall audit logging architecture
- `configuration.md` - App configuration reference
