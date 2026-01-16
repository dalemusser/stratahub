# StrataLog Implementation Plan

This document outlines the plan for implementing StrataLog as a strata-based application.

---

## Overview

StrataLog is a game activity logging service that:
- Accepts log entries from games via API key authentication
- Supports both single and batch log entry submission
- Provides a developer UI for observing and analyzing logs
- Tracks statistics (entry counts, response times, etc.)

---

## Repository Setup

### Starting Point

1. Create `stratalog` repository on GitHub
2. Copy strata files as the starting point
3. Update module path from `github.com/dalemusser/strata` to `github.com/dalemusser/stratalog`
4. Update environment variable prefix from `STRATA_` to `STRATALOG_`

### Directory Structure

```
stratalog/
├── cmd/
│   └── stratalog/
│       └── main.go
├── internal/
│   └── app/
│       ├── bootstrap/
│       │   ├── config.go       # STRATALOG_* config
│       │   ├── appconfig.go
│       │   ├── deps.go
│       │   └── routes.go       # Route setup with API/UI separation
│       ├── features/
│       │   ├── api/            # Game API endpoints (API key auth)
│       │   │   ├── logs.go     # POST /api/logs (single + batch)
│       │   │   └── routes.go
│       │   ├── dashboard/      # Developer UI (session auth)
│       │   │   ├── handler.go
│       │   │   ├── routes.go
│       │   │   └── templates/
│       │   ├── login/          # Developer login (from strata)
│       │   ├── logout/
│       │   ├── health/
│       │   └── errors/
│       ├── store/
│       │   ├── logs/           # Log entry storage
│       │   ├── stats/          # Statistics storage
│       │   └── users/          # Developer accounts
│       ├── system/
│       │   ├── auth/           # Session management
│       │   └── viewdata/
│       └── resources/
│           └── templates/
│               └── layout.gohtml
├── static/
├── go.mod
└── go.sum
```

---

## Authentication Strategy

### Two Authentication Modes

| Route Group | Auth Method | Purpose |
|------------|-------------|---------|
| `/api/*` | API Key (Bearer token) | Game log submission |
| `/dashboard/*`, `/login`, etc. | Session cookies | Developer UI |

### Route Configuration

```go
// internal/app/bootstrap/routes.go

func BuildHandler(...) http.Handler {
    r := chi.NewRouter()

    // ─────────────────────────────────────────────────────────────
    // API Routes - API Key Auth, NO CSRF, CORS *
    // ─────────────────────────────────────────────────────────────
    r.Group(func(r chi.Router) {
        r.Use(corsMiddleware())  // AllowCredentials: false, Origins: *
        r.Use(apiKeyAuth(appCfg.APIKey))

        // No CSRF middleware - API key auth is not CSRF-vulnerable
        r.Mount("/api", apifeature.Routes(apiHandler))
    })

    // ─────────────────────────────────────────────────────────────
    // Web UI Routes - Session Auth, WITH CSRF
    // ─────────────────────────────────────────────────────────────
    r.Group(func(r chi.Router) {
        r.Use(sessionMgr.LoadSessionUser)
        r.Use(csrfMiddleware)

        // Public
        r.Mount("/login", loginfeature.Routes(...))
        r.Mount("/logout", logoutfeature.Routes(...))
        r.Mount("/health", healthfeature.Routes(...))

        // Protected - require developer role
        r.Group(func(r chi.Router) {
            r.Use(sessionMgr.RequireRole("developer", "admin"))
            r.Mount("/dashboard", dashboardfeature.Routes(...))
        })
    })

    return r
}
```

---

## API Endpoints

### POST /api/logs - Submit Log Entry (Single or Batch)

**Authentication:** API Key (Bearer token)

**Single Entry Request:**
```json
{
    "game": "my-game",
    "player_id": "user123",
    "event_type": "level_complete",
    "level": 5,
    "score": 1250
}
```

**Batch Request:**
```json
{
    "game": "my-game",
    "entries": [
        {
            "player_id": "user123",
            "event_type": "level_start",
            "level": 5,
            "timestamp": "2026-01-14T10:00:00Z"
        },
        {
            "player_id": "user123",
            "event_type": "item_collected",
            "item": "coin",
            "timestamp": "2026-01-14T10:00:05Z"
        },
        {
            "player_id": "user123",
            "event_type": "level_complete",
            "level": 5,
            "score": 1250,
            "timestamp": "2026-01-14T10:01:30Z"
        }
    ]
}
```

**Response (Single):**
```json
{
    "status": "success",
    "received_at": "2026-01-14T10:01:35Z",
    "count": 1
}
```

**Response (Batch):**
```json
{
    "status": "success",
    "received_at": "2026-01-14T10:01:35Z",
    "count": 3
}
```

**Implementation Notes:**
- Detect batch vs single by presence of `entries` array
- Add `dbtimestamp` (server time) to each entry
- Store in collection named by `game` field
- Track stats: increment entry count, record response time

### GET /api/logs - List Recent Logs (JSON)

**Authentication:** API Key (Bearer token)

**Query Parameters:**
- `game` (required): Game name/collection
- `player_id` (optional): Filter by player
- `limit` (optional): Number of entries (default: 20, max: 100)

**Response:**
```json
{
    "game": "my-game",
    "total_count": 15432,
    "returned": 20,
    "entries": [...]
}
```

---

## Developer UI

### Dashboard Features

| Page | Route | Description |
|------|-------|-------------|
| Overview | `/dashboard` | Stats summary, recent activity |
| Live View | `/dashboard/live` | Real-time log stream (WebSocket or polling) |
| Search | `/dashboard/search` | Search logs by game, player, event type, date |
| Player View | `/dashboard/player/{id}` | All logs for a specific player |
| Game View | `/dashboard/game/{name}` | All logs for a specific game |
| Download | `/dashboard/download` | Export logs as JSON/CSV |
| Stats | `/dashboard/stats` | Detailed statistics and charts |

### Dashboard Overview Page

```
┌─────────────────────────────────────────────────────────────────────────┐
│  StrataLog Dashboard                                     [User ▼] [Logout]│
├─────────────────────────────────────────────────────────────────────────┤
│                                                                          │
│  ┌──────────────┐  ┌──────────────┐  ┌──────────────┐  ┌──────────────┐ │
│  │ Total Logs   │  │ Today        │  │ Active Games │  │ Avg Response │ │
│  │   1,234,567  │  │    12,345    │  │      8       │  │    45ms      │ │
│  └──────────────┘  └──────────────┘  └──────────────┘  └──────────────┘ │
│                                                                          │
│  Recent Activity                                              [Live View]│
│  ┌─────────────────────────────────────────────────────────────────────┐│
│  │ Time       │ Game      │ Player    │ Event          │ Details      ││
│  ├─────────────────────────────────────────────────────────────────────┤│
│  │ 10:01:35   │ my-game   │ user123   │ level_complete │ level:5      ││
│  │ 10:01:30   │ my-game   │ user456   │ item_collected │ item:sword   ││
│  │ 10:01:28   │ other-gm  │ user789   │ game_start     │              ││
│  │ ...        │           │           │                │              ││
│  └─────────────────────────────────────────────────────────────────────┘│
│                                                                          │
│  Top Games (Last 24h)                   Top Players (Last 24h)           │
│  ┌────────────────────────┐            ┌────────────────────────┐       │
│  │ my-game      5,432     │            │ user123      1,234     │       │
│  │ other-game   3,210     │            │ user456        987     │       │
│  │ test-game      543     │            │ user789        654     │       │
│  └────────────────────────┘            └────────────────────────┘       │
│                                                                          │
└─────────────────────────────────────────────────────────────────────────┘
```

### Player View Page

```
┌─────────────────────────────────────────────────────────────────────────┐
│  Player: user123                                                         │
├─────────────────────────────────────────────────────────────────────────┤
│                                                                          │
│  Stats                                                                   │
│  ┌──────────────┐  ┌──────────────┐  ┌──────────────┐                   │
│  │ Total Logs   │  │ Games Played │  │ Last Active  │                   │
│  │     1,234    │  │      3       │  │  5 min ago   │                   │
│  └──────────────┘  └──────────────┘  └──────────────┘                   │
│                                                                          │
│  Filter: [All Games ▼] [All Events ▼] [Last 24h ▼]        [Search]      │
│                                                                          │
│  Activity Log                                                            │
│  ┌─────────────────────────────────────────────────────────────────────┐│
│  │ Time       │ Game      │ Event          │ Details                   ││
│  ├─────────────────────────────────────────────────────────────────────┤│
│  │ 10:01:35   │ my-game   │ level_complete │ {"level":5,"score":1250}  ││
│  │ 10:00:05   │ my-game   │ item_collected │ {"item":"coin"}           ││
│  │ 10:00:00   │ my-game   │ level_start    │ {"level":5}               ││
│  │ ...        │           │                │                           ││
│  └─────────────────────────────────────────────────────────────────────┘│
│                                                                          │
│  [◄ Prev]  Page 1 of 50  [Next ►]                        [Download CSV] │
│                                                                          │
└─────────────────────────────────────────────────────────────────────────┘
```

---

## Statistics Tracking

### Stats to Collect

| Metric | Description | Storage |
|--------|-------------|---------|
| `total_entries` | Total log entries received | Counter in stats collection |
| `entries_today` | Entries received today | Daily counter, reset at midnight |
| `entries_by_game` | Entries per game | Map: game -> count |
| `entries_by_player` | Entries per player | Map: player_id -> count |
| `avg_response_time` | Average API response time | Rolling average |
| `requests_per_minute` | API request rate | Time-series data |
| `batch_vs_single` | Ratio of batch to single requests | Counters |

### Stats Collection

```go
// internal/app/store/stats/stats.go

type Stats struct {
    ID              primitive.ObjectID `bson:"_id,omitempty"`
    Date            time.Time          `bson:"date"`           // Day (truncated)
    TotalEntries    int64              `bson:"total_entries"`
    TotalRequests   int64              `bson:"total_requests"`
    BatchRequests   int64              `bson:"batch_requests"`
    SingleRequests  int64              `bson:"single_requests"`
    AvgResponseMs   float64            `bson:"avg_response_ms"`
    EntriesByGame   map[string]int64   `bson:"entries_by_game"`
    EntriesByPlayer map[string]int64   `bson:"entries_by_player"`
}

// Increment stats after each request
func (s *Store) RecordRequest(ctx context.Context, game, playerID string,
                               entryCount int, isBatch bool, responseMs float64)
```

---

## Data Model

### Log Entry (Flexible Schema)

```go
// Stored in collection named by "game" field
type LogEntry struct {
    ID          primitive.ObjectID     `bson:"_id,omitempty"`
    Game        string                 `bson:"game"`
    PlayerID    string                 `bson:"player_id"`
    EventType   string                 `bson:"event_type,omitempty"`
    DBTimestamp time.Time              `bson:"dbtimestamp"`     // Server time
    Timestamp   time.Time              `bson:"timestamp,omitempty"` // Client time (optional)
    Data        map[string]interface{} `bson:",inline"`         // All other fields
}
```

### Developer User

```go
// Reuse strata's user model with role = "developer" or "admin"
type User struct {
    ID       primitive.ObjectID `bson:"_id,omitempty"`
    LoginID  string             `bson:"login_id"`
    FullName string             `bson:"full_name"`
    Role     string             `bson:"role"`  // "developer" or "admin"
    // ... other fields from strata
}
```

---

## Configuration

### Environment Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `STRATALOG_API_KEY` | (required) | API key for game authentication |
| `STRATALOG_MONGO_URI` | `mongodb://localhost:27017` | MongoDB connection |
| `STRATALOG_MONGO_DATABASE` | `stratalog` | Database name |
| `STRATALOG_SESSION_KEY` | (required in prod) | Session signing key |
| `STRATALOG_CSRF_KEY` | (required in prod) | CSRF token key |
| `STRATALOG_MAX_BATCH_SIZE` | `100` | Max entries per batch request |
| `STRATALOG_MAX_BODY_SIZE` | `1048576` | Max request body (1MB) |

---

## Migration from strata_log

### What to Keep

- Basic log insertion logic
- Game name validation
- Collection-per-game pattern
- Timeout handling
- Body size limiting

### What to Change

- Add batch support
- Add session-based auth for UI
- Add CSRF for UI forms
- Add statistics tracking
- Replace simple HTML views with proper dashboard
- Use strata's template system
- Use strata's viewdata pattern

### What to Add

- Developer login/logout
- Dashboard pages
- Player search/view
- Game search/view
- Statistics collection and display
- Download/export functionality

---

## Implementation Phases

### Phase 1: Foundation
1. Create repository from strata
2. Update module paths and config prefix
3. Set up dual auth (API key + session)
4. Implement basic API endpoints (single + batch)
5. Basic health check

### Phase 2: Developer UI
1. Login/logout (adapt from strata)
2. Dashboard overview page
3. Basic log viewing
4. Player search

### Phase 3: Statistics
1. Stats collection on each request
2. Stats display on dashboard
3. Response time tracking

### Phase 4: Advanced Features
1. Live view (polling or WebSocket)
2. Advanced search/filtering
3. CSV/JSON export
4. Game-specific views
5. Date range queries

---

## Testing Strategy

### API Tests
- Single entry submission
- Batch entry submission
- Invalid JSON handling
- Missing required fields
- API key validation
- Rate limiting (if implemented)

### UI Tests
- Login/logout flow
- Dashboard rendering
- Search functionality
- Pagination
- Export functionality

### Integration Tests
- End-to-end log submission and viewing
- Stats accuracy
- Cross-origin requests (CORS)

---

## Security Considerations

1. **API Key Protection**: Store securely, rotate periodically
2. **Rate Limiting**: Consider adding for API endpoints
3. **Input Validation**: Validate game names, limit field sizes
4. **CSRF Protection**: Enabled for all UI POST endpoints
5. **Session Security**: HttpOnly, Secure, SameSite cookies
6. **SQL/NoSQL Injection**: Use parameterized queries (MongoDB driver handles this)

---

## Future Enhancements

- [ ] WebSocket for real-time log streaming
- [ ] Alerting on specific events
- [ ] Log retention policies (auto-delete old logs)
- [ ] Multiple API keys per game
- [ ] Rate limiting per API key
- [ ] Grafana/Prometheus integration for metrics
