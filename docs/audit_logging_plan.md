# Audit Logging Plan

This document describes the plan for implementing audit logging in StrataHub for security monitoring, incident response, and administrative oversight.

## Overview

Audit logging captures security-relevant events for review by system administrators and workspace admins. Events are stored in MongoDB for querying and will have a future admin UI.

## Stakeholders

| Role | Access | Use Case |
|------|--------|----------|
| System Admin | All events, all organizations | Security incidents, system health, technical support |
| Workspace Admin | Events within their organization | User management, access review, policy compliance |

## Data Model

### `audit_events` Collection

```go
type AuditEvent struct {
    ID             primitive.ObjectID  `bson:"_id"`
    Timestamp      time.Time           `bson:"timestamp"`
    OrganizationID *primitive.ObjectID `bson:"organization_id,omitempty"` // nil for system-level events

    // Event classification
    Category       string              `bson:"category"`     // "auth", "admin", "security"
    EventType      string              `bson:"event_type"`   // specific event name

    // Who
    UserID         *primitive.ObjectID `bson:"user_id,omitempty"`      // affected user
    ActorID        *primitive.ObjectID `bson:"actor_id,omitempty"`     // who performed action (for admin actions)

    // Context
    IP             string              `bson:"ip"`
    UserAgent      string              `bson:"user_agent,omitempty"`

    // Outcome
    Success        bool                `bson:"success"`
    FailureReason  string              `bson:"failure_reason,omitempty"`

    // Additional details (varies by event type)
    Details        map[string]string   `bson:"details,omitempty"`
}
```

### Indexes

```go
// Query by time range
{Keys: bson.D{{Key: "timestamp", Value: -1}}}

// Query by organization
{Keys: bson.D{{Key: "organization_id", Value: 1}, {Key: "timestamp", Value: -1}}}

// Query by user
{Keys: bson.D{{Key: "user_id", Value: 1}, {Key: "timestamp", Value: -1}}}

// Query by event type
{Keys: bson.D{{Key: "category", Value: 1}, {Key: "event_type", Value: 1}, {Key: "timestamp", Value: -1}}}

// Optional: TTL index for automatic cleanup (if desired)
// {Keys: bson.D{{Key: "timestamp", Value: 1}}, Options: options.Index().SetExpireAfterSeconds(90*24*60*60)} // 90 days
```

## Events to Capture

### Authentication Events (category: "auth")

| Event Type | Success | Failure | Details |
|------------|---------|---------|---------|
| `login_success` | User logged in | - | `auth_method`, `login_id` |
| `login_failed_user_not_found` | - | Login ID not found | `attempted_login_id` |
| `login_failed_wrong_password` | - | Incorrect password | `login_id` |
| `login_failed_user_disabled` | - | Account disabled | `login_id` |
| `login_failed_rate_limit` | - | Too many attempts | `login_id`, `limit_type` |
| `logout` | User logged out | - | `session_duration` |
| `password_changed` | Password updated | - | `was_temporary` |
| `verification_code_sent` | Email sent | - | `email` |
| `verification_code_resent` | Email resent | - | `email`, `resend_count` |
| `magic_link_used` | Login via link | - | `email` |

### Admin Events (category: "admin")

| Event Type | Details |
|------------|---------|
| `user_created` | `target_user_id`, `role`, `auth_method` |
| `user_updated` | `target_user_id`, `fields_changed` |
| `user_disabled` | `target_user_id`, `reason` |
| `user_enabled` | `target_user_id` |
| `user_deleted` | `target_user_id` |
| `group_created` | `group_id`, `group_name` |
| `group_updated` | `group_id`, `fields_changed` |
| `group_deleted` | `group_id` |
| `member_added_to_group` | `group_id`, `target_user_id` |
| `member_removed_from_group` | `group_id`, `target_user_id` |
| `organization_settings_changed` | `fields_changed` |

### Security Events (category: "security")

| Event Type | Details |
|------------|---------|
| `session_expired` | `user_id`, `session_age` |
| `suspicious_activity` | `reason`, `details` |

## Implementation

### Phase 1: Core Infrastructure

1. Create `audit_events` collection and indexes
2. Create `AuditStore` with methods:
   - `Log(ctx, event AuditEvent) error`
   - `Query(ctx, filter AuditFilter) ([]AuditEvent, error)`
3. Add audit logging to authentication handlers
4. Add structured zap logging alongside MongoDB storage

### Phase 2: Admin Actions

1. Add audit logging to user management (create, update, disable, delete)
2. Add audit logging to group management
3. Add audit logging to organization settings changes

### Phase 3: Admin UI

1. Event list view with filtering
2. User activity timeline
3. Export functionality (CSV, JSON)
4. Real-time event stream (optional)

## Logging Strategy

Events are logged to both:

1. **MongoDB** (`audit_events` collection) - For querying, UI display, long-term storage
2. **Structured logs** (zap/journald) - For real-time monitoring, log aggregation tools

Example zap logging alongside MongoDB:

```go
h.Log.Info("audit event",
    zap.String("audit", "true"),
    zap.String("category", "auth"),
    zap.String("event", "login_success"),
    zap.String("user_id", userID),
    zap.String("ip", ip),
)
```

The `zap.String("audit", "true")` tag allows filtering audit events from general logs:
```bash
journalctl -u stratahub | grep '"audit":"true"'
```

## Data Retention

- Default: Keep all events indefinitely
- Future: Admin UI option to purge old data or export and delete
- If TTL is desired, can add TTL index (e.g., 90 days, 1 year)

## Privacy Considerations

- IP addresses are logged (necessary for security)
- Passwords are never logged
- Login IDs for failed attempts are logged (to detect targeted attacks)
- Data subject to deletion requests per research agreements

## File Structure

```
internal/app/
├── store/
│   └── audit/
│       └── store.go       # AuditStore implementation
├── system/
│   └── audit/
│       └── logger.go      # Convenience functions for logging events
```

## See Also

- [Activity Tracking Plan](activity_tracking_plan.md) - Teacher-facing activity tracking
- [Systemd Configuration](systemd_info.md) - Log access via journald
