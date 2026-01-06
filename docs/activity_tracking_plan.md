# Activity Tracking Plan

This document describes the plan for implementing user activity tracking in StrataHub to support teacher visibility into student engagement and researcher data collection.

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
| `resource_return` | User returns from external resource | `resource_id`, `time_away_secs` |
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

### Implementation

Browser-side (JavaScript):
```javascript
// Send heartbeat every 60 seconds while page is open
setInterval(function() {
    fetch('/api/heartbeat', { method: 'POST', credentials: 'same-origin' });
}, 60000);
```

Server-side:
```go
// POST /api/heartbeat
func (h *Handler) Heartbeat(w http.ResponseWriter, r *http.Request) {
    // Update session.LastActiveAt = time.Now()
    // No new record created - just update existing session
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

### "My Class Right Now" View

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
- Shows most recent `resource_launch` event if in a resource
- Shows "Dashboard" or page name otherwise

### "Weekly Summary" View

Aggregated view of student activity:

```
┌──────────────────────────────────────────────────────────────────────┐
│ Week of Jan 13, 2025                                                 │
├───────────────┬──────────┬────────────┬───────────────┬──────────────┤
│ Student       │ Sessions │ Total Time │ Resource Time │ Outside Class│
├───────────────┼──────────┼────────────┼───────────────┼──────────────┤
│ Alice B.      │ 5        │ 2h 15m     │ 1h 45m        │ 3 sessions   │
│ Bob C.        │ 4        │ 1h 50m     │ 1h 20m        │ 0 sessions   │
│ Carol D.      │ 3        │ 1h 10m     │ 0h 55m        │ 1 session    │
└───────────────┴──────────┴────────────┴───────────────┴──────────────┘
```

**Metrics:**
- Sessions: Number of login sessions
- Total Time: Sum of session durations
- Resource Time: Time between `resource_launch` and `resource_return` events
- Outside Class: Sessions at unusual times (teacher interprets what this means)

### "Student Detail" View

Individual student timeline:

```
┌─────────────────────────────────────────────────────────────────┐
│ Alice B. - Activity History                                      │
├─────────────────────────────────────────────────────────────────┤
│ Jan 15, 2025                                                     │
│ ├─ 9:15 AM  Logged in                                           │
│ ├─ 9:16 AM  Launched "Math Quest"                               │
│ ├─ 9:48 AM  Returned from "Math Quest" (32 min)                 │
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

CSV or JSON export with:

**Sessions export:**
```csv
user_id,user_name,organization,group,login_at,logout_at,end_reason,duration_secs,ip
abc123,Alice B.,Lincoln Elementary,Period 3 Math,2025-01-15T09:15:00Z,2025-01-15T09:50:00Z,logout,2100,192.168.1.50
```

**Activity export:**
```csv
user_id,user_name,session_id,timestamp,event_type,resource_name,details
abc123,Alice B.,sess456,2025-01-15T09:16:00Z,resource_launch,Math Quest,{}
abc123,Alice B.,sess456,2025-01-15T09:48:00Z,resource_return,Math Quest,{"time_away_secs":1920}
```

### Aggregated Metrics

For research analysis:
- Average session duration per user
- Average time-on-resource per user
- Session frequency (daily, weekly)
- Active days per user
- Peak usage times

## Implementation Phases

### Phase 1: Session Tracking
1. Create `sessions` collection and store
2. Record session on login (create record)
3. Update session on logout (set LogoutAt, calculate duration)
4. Add session_id to user's session cookie

### Phase 2: Heartbeat System
1. Add `/api/heartbeat` endpoint
2. Add JavaScript heartbeat on all authenticated pages
3. Update `LastActiveAt` on each heartbeat
4. Background job to close stale sessions

### Phase 3: Activity Events
1. Create `activity_events` collection
2. Track resource launch events
3. Track resource return events (if detectable)
4. Track page views (optional, may be verbose)

### Phase 4: Teacher Dashboard
1. "Who's online" real-time view
2. Weekly summary view
3. Student detail view

### Phase 5: Researcher Tools
1. Data export (CSV, JSON)
2. Date range filtering
3. Aggregated reports

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
- No tracking of content within launched resources (handled separately)

## File Structure

```
internal/app/
├── store/
│   ├── sessions/
│   │   └── store.go       # Session CRUD
│   └── activity/
│       └── store.go       # Activity events
├── features/
│   └── activity/
│       ├── handler.go     # Heartbeat, dashboard endpoints
│       ├── routes.go
│       └── templates/
│           ├── dashboard.gohtml
│           └── student_detail.gohtml
```

## See Also

- [Audit Logging Plan](audit_logging_plan.md) - Security event logging
- [Multi-Tenancy and Roles](multi-tenancy-and-roles-architecture.md) - User roles and permissions
