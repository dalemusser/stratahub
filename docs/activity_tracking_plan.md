# Activity Tracking

This document describes the user activity tracking system in StrataHub that supports teacher visibility into student engagement and researcher data collection.

## Overview

Activity tracking captures user sessions and actions to help teachers monitor student engagement and provide researchers with usage data for the educational effectiveness study.

## Context

StrataHub is used in a Department of Education research project comparing educational video games to traditional instruction in K-12 schools. The project has:
- IRB approval
- Parental consent for student participation
- Agreements with schools and teachers
- Obligation to honor data deletion requests

Game-internal activity (levels, scores, etc.) is tracked by a separate logging server. StrataHub tracks platform-level activity only.

## Stakeholders

| Role | Access | Use Case |
|------|--------|----------|
| Teacher (Leader) | Members in their groups | Monitor student engagement during class, track participation |
| Researcher (Coordinator/Admin) | All users in organization | Verify usage, collect data for research, compare groups |

## Data Model

### `sessions` Collection

Tracks login/logout and session duration:

```go
type Session struct {
    ID             primitive.ObjectID  `bson:"_id"`
    UserID         primitive.ObjectID  `bson:"user_id"`
    OrganizationID primitive.ObjectID  `bson:"organization_id"`

    // Timing
    LoginAt        time.Time           `bson:"login_at"`
    LogoutAt       *time.Time          `bson:"logout_at"`       // nil if active or abandoned
    LastActiveAt   time.Time           `bson:"last_active_at"`  // updated by heartbeat

    // How did session end?
    EndReason      string              `bson:"end_reason"`      // "logout", "expired", "inactive", ""

    // Context
    IP             string              `bson:"ip"`
    UserAgent      string              `bson:"user_agent"`

    // Computed on session close
    DurationSecs   int64               `bson:"duration_secs"`
}
```

### `activity_events` Collection

Tracks specific user actions:

```go
type ActivityEvent struct {
    ID             primitive.ObjectID  `bson:"_id"`
    UserID         primitive.ObjectID  `bson:"user_id"`
    SessionID      primitive.ObjectID  `bson:"session_id"`
    OrganizationID primitive.ObjectID  `bson:"organization_id"`
    Timestamp      time.Time           `bson:"timestamp"`

    // What happened
    EventType      string              `bson:"event_type"`

    // Context (varies by event type)
    ResourceID     *primitive.ObjectID `bson:"resource_id,omitempty"`
    ResourceName   string              `bson:"resource_name,omitempty"`
    PagePath       string              `bson:"page_path,omitempty"`
    Details        map[string]any      `bson:"details,omitempty"`
}
```

### Event Types

| Event Type | When | Details |
|------------|------|---------|
| `resource_launch` | User clicks to launch a game/resource | `resource_id`, `resource_name` |
| `resource_view` | User views a resource detail page | `resource_id`, `resource_name` |
| `page_view` | User navigates to a page | `page_path` |

### Indexes

```go
// Active sessions query (for "who's online")
{Keys: bson.D{{Key: "logout_at", Value: 1}, {Key: "last_active_at", Value: -1}}}

// User session history
{Keys: bson.D{{Key: "user_id", Value: 1}, {Key: "login_at", Value: -1}}}

// Organization queries (for researcher exports)
{Keys: bson.D{{Key: "organization_id", Value: 1}, {Key: "login_at", Value: -1}}}

// Activity by session
{Keys: bson.D{{Key: "session_id", Value: 1}, {Key: "timestamp", Value: 1}}}

// Activity by user
{Keys: bson.D{{Key: "user_id", Value: 1}, {Key: "timestamp", Value: -1}}}
```

## Heartbeat System

### Purpose
- Track "last active" time for online status
- Detect browser close/tab close (session abandonment)
- Efficient: only stores latest timestamp, not every heartbeat
- Record page views when user navigates

### Implementation

Browser-side (JavaScript in layout template):
```javascript
// Send heartbeat every 60 seconds while page is open
setInterval(function() {
    fetch('/api/heartbeat', {
        method: 'POST',
        credentials: 'same-origin',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ page: window.location.pathname })
    });
}, 60000);
```

Server-side:
```go
// POST /api/heartbeat
func (h *Handler) ServeHeartbeat(w http.ResponseWriter, r *http.Request) {
    // Update session.LastActiveAt = time.Now()
    // Record page_view event if page changed since last heartbeat
}
```

### Online Status Logic

```go
// User is "online" if:
// 1. Session has no LogoutAt (not explicitly logged out)
// 2. LastActiveAt is within the last 2 minutes

func IsOnline(s Session) bool {
    if s.LogoutAt != nil {
        return false
    }
    return time.Since(s.LastActiveAt) < 2*time.Minute
}

// User is "idle" if:
// LastActiveAt is 2-10 minutes ago

// User is "offline" if:
// LogoutAt is set, or LastActiveAt > 10 minutes ago
```

### Session End Detection

| Scenario | Detection | EndReason |
|----------|-----------|-----------|
| User clicks logout | Immediate | `"logout"` |
| Browser closed | Heartbeats stop, detected on next query | `"inactive"` |
| Session cookie expires | Next request redirects to login | `"expired"` |

Background job (or lazy evaluation) closes stale sessions:
```go
// Sessions with no logout and LastActiveAt > 10 minutes ago
// → Set LogoutAt = LastActiveAt, EndReason = "inactive"
```

## Teacher Dashboard

### Real-Time Dashboard View

Real-time view of current class status:

```
┌─────────────────────────────────────────────────────────────────┐
│ Period 3 Math - 24/28 students online                           │
├───────────────┬────────────┬──────────────────┬─────────────────┤
│ Student       │ Status     │ Current Activity │ Time Today      │
├───────────────┼────────────┼──────────────────┼─────────────────┤
│ Alice B.      │ 🟢 Active  │ Math Quest       │ 32 min          │
│ Bob C.        │ 🟢 Active  │ Dashboard        │ 28 min          │
│ Carol D.      │ 🟡 Idle    │ --               │ 15 min          │
│ Dave E.       │ ⚪ Offline │ --               │ 0 min           │
└───────────────┴────────────┴──────────────────┴─────────────────┘
```

**Status indicators:**
- 🟢 Active: Heartbeat within 2 minutes
- 🟡 Idle: Heartbeat 2-10 minutes ago
- ⚪ Offline: No active session or heartbeat > 10 minutes

**Current Activity:**
- Shows most recent `resource_launch` event if user launched a resource
- Shows "Dashboard" or page name otherwise

### Weekly Summary View

Aggregated view of student activity:

```
┌──────────────────────────────────────────────────────────────────────┐
│ Week of Jan 13, 2025                                                 │
├───────────────┬──────────┬────────────┬──────────────────────────────┤
│ Student       │ Sessions │ Total Time │ Outside Class                │
├───────────────┼──────────┼────────────┼──────────────────────────────┤
│ Alice B.      │ 5        │ 2h 15m     │ 3 sessions                   │
│ Bob C.        │ 4        │ 1h 50m     │ 0 sessions                   │
│ Carol D.      │ 3        │ 1h 10m     │ 1 session                    │
└───────────────┴──────────┴────────────┴──────────────────────────────┘
```

**Metrics:**
- Sessions: Number of login sessions
- Total Time: Sum of session durations
- Outside Class: Sessions at unusual times (teacher interprets what this means)

### Member Detail View

Individual student timeline:

```
┌─────────────────────────────────────────────────────────────────┐
│ Alice B. - Activity History                                      │
├─────────────────────────────────────────────────────────────────┤
│ Jan 15, 2025                                                     │
│ ├─ 9:15 AM  Logged in                                           │
│ ├─ 9:16 AM  Launched "Math Quest"                               │
│ ├─ 9:50 AM  Logged out                                          │
│                                                                  │
│ Jan 14, 2025                                                     │
│ ├─ 7:30 PM  Logged in (home)                                    │
│ ├─ 7:32 PM  Launched "Math Quest"                               │
│ ├─ 8:05 PM  Session ended (inactive)                            │
└─────────────────────────────────────────────────────────────────┘
```

## Researcher Data Export

### Export Format

CSV or JSON export with date range and organization/group filtering.

**Sessions export:**
```csv
user_id,user_name,email,organization,group,login_at,logout_at,end_reason,duration_secs,ip
abc123,Alice B.,alice@example.com,Lincoln Elementary,Period 3 Math,2025-01-15T09:15:00Z,2025-01-15T09:50:00Z,logout,2100,192.168.1.50
```

**Activity export:**
```csv
user_id,user_name,session_id,timestamp,event_type,resource_name,details
abc123,Alice B.,sess456,2025-01-15T09:16:00Z,resource_launch,Math Quest,{}
abc123,Alice B.,sess456,2025-01-15T09:20:00Z,resource_view,Math Quest,{}
abc123,Alice B.,sess456,2025-01-15T09:45:00Z,page_view,,{"page_path":"/member/resources"}
```

### Aggregated Metrics

The Data Export page displays:
- Total Sessions
- Unique Users
- Total Time (sum of all session durations)
- Avg Session (average session duration)
- Peak Hour (hour with most logins)
- Most Active Day (weekday with most logins)

## File Structure

```
internal/app/
├── store/
│   ├── sessions/
│   │   └── store.go           # Session CRUD, heartbeat updates
│   └── activity/
│       └── store.go           # Activity events (launch, view, page_view)
├── features/
│   ├── activity/
│   │   ├── handler.go         # Dashboard endpoints
│   │   ├── dashboard.go       # Real-time dashboard logic
│   │   ├── summary.go         # Weekly summary logic
│   │   ├── detail.go          # Member detail view logic
│   │   ├── export.go          # Data export (CSV, JSON)
│   │   ├── routes.go
│   │   ├── types.go           # View models
│   │   └── templates/
│   │       ├── activity_dashboard.gohtml
│   │       ├── activity_summary.gohtml
│   │       ├── activity_member_detail.gohtml
│   │       └── activity_export.gohtml
│   └── heartbeat/
│       ├── handler.go         # POST /api/heartbeat
│       └── routes.go
```

## Data Retention

- **Default**: Keep all data indefinitely (needed for research)
- **Future**: Admin UI for data management
  - Export and archive old data
  - Delete data for specific users (for deletion requests)
  - Generate summary statistics before purging detail

## Privacy Notes

- Parental consent obtained for all student participants
- Data subject to deletion requests per research agreements
- IP addresses stored but not displayed to teachers (only researchers/admins)
- No tracking of content within launched resources (handled by separate game logging server)

## See Also

- [Audit Logging Plan](audit_logging_plan.md) - Security event logging
- [Multi-Tenancy and Roles](multi-tenancy-and-roles-architecture.md) - User roles and permissions
