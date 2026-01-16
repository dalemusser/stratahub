# StrataSave Implementation Plan

This document outlines the plan for implementing StrataSave as a strata-based application.

---

## Overview

StrataSave is a game save data persistence service that:
- Stores player settings (single document per player/game, upsert)
- Stores player progress (append-only history)
- Provides a developer UI for observing and managing save data
- Tracks statistics (save counts, response times, etc.)

---

## Repository Setup

### Starting Point

1. Create `stratasave` repository on GitHub
2. Copy strata files as the starting point
3. Update module path from `github.com/dalemusser/strata` to `github.com/dalemusser/stratasave`
4. Update environment variable prefix from `STRATA_` to `STRATASAVE_`

### Directory Structure

```
stratasave/
├── cmd/
│   └── stratasave/
│       └── main.go
├── internal/
│   └── app/
│       ├── bootstrap/
│       │   ├── config.go       # STRATASAVE_* config
│       │   ├── appconfig.go
│       │   ├── deps.go
│       │   └── routes.go       # Route setup with API/UI separation
│       ├── features/
│       │   ├── api/            # Game API endpoints (API key auth)
│       │   │   ├── settings.go # Player settings (upsert)
│       │   │   ├── progress.go # Player progress (append)
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
│       │   ├── settings/       # Player settings storage
│       │   ├── progress/       # Player progress storage
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
| `/api/*` | API Key (Bearer token) | Game save/load operations |
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

### Two Save Types

StrataSave supports two distinct types of saved data:

| Type | Use Case | Behavior | Collection |
|------|----------|----------|------------|
| **Settings** | Player preferences, config | Upsert (one doc per player/game) | `{game}_settings` |
| **Progress** | Game state, checkpoints | Append (history of saves) | `{game}_progress` |

---

### POST /api/settings/save - Save Player Settings

**Authentication:** API Key (Bearer token)

**Purpose:** Store player settings (key bindings, audio levels, preferences). One document per player per game, updated on each save.

**Request:**
```json
{
    "game": "my-game",
    "player_id": "user123",
    "settings": {
        "audio_volume": 0.8,
        "music_enabled": true,
        "key_bindings": {
            "jump": "space",
            "fire": "ctrl"
        },
        "language": "en",
        "difficulty": "normal"
    }
}
```

**Response:**
```json
{
    "status": "success",
    "player_id": "user123",
    "game": "my-game",
    "saved_at": "2026-01-14T10:01:35Z",
    "is_new": false
}
```

**Behavior:**
- Upserts document (creates if new, updates if exists)
- Only one settings document per player/game combination
- Preserves `created_at`, updates `updated_at`

---

### POST /api/settings/load - Load Player Settings

**Authentication:** API Key (Bearer token)

**Request:**
```json
{
    "game": "my-game",
    "player_id": "user123"
}
```

**Response (found):**
```json
{
    "found": true,
    "player_id": "user123",
    "game": "my-game",
    "settings": {
        "audio_volume": 0.8,
        "music_enabled": true,
        "key_bindings": {...},
        "language": "en",
        "difficulty": "normal"
    },
    "created_at": "2026-01-10T08:00:00Z",
    "updated_at": "2026-01-14T10:01:35Z"
}
```

**Response (not found):**
```json
{
    "found": false,
    "player_id": "user123",
    "game": "my-game",
    "settings": null
}
```

---

### POST /api/progress/save - Save Player Progress

**Authentication:** API Key (Bearer token)

**Purpose:** Store game progress (level state, inventory, checkpoints). Each save creates a new document, maintaining history.

**Request:**
```json
{
    "game": "my-game",
    "player_id": "user123",
    "slot": 1,
    "progress": {
        "level": 5,
        "checkpoint": "castle_entrance",
        "inventory": ["sword", "shield", "potion"],
        "health": 85,
        "gold": 1250,
        "play_time_seconds": 3600
    }
}
```

**Response:**
```json
{
    "status": "success",
    "id": "507f1f77bcf86cd799439011",
    "player_id": "user123",
    "game": "my-game",
    "slot": 1,
    "saved_at": "2026-01-14T10:01:35Z"
}
```

**Behavior:**
- Always creates a new document (append-only)
- Maintains full history of saves
- Slot number allows multiple save slots per player

---

### POST /api/progress/load - Load Player Progress

**Authentication:** API Key (Bearer token)

**Request:**
```json
{
    "game": "my-game",
    "player_id": "user123",
    "slot": 1,
    "limit": 1
}
```

**Response (single, limit=1):**
```json
{
    "found": true,
    "player_id": "user123",
    "game": "my-game",
    "slot": 1,
    "saves": [
        {
            "id": "507f1f77bcf86cd799439011",
            "saved_at": "2026-01-14T10:01:35Z",
            "progress": {
                "level": 5,
                "checkpoint": "castle_entrance",
                "inventory": ["sword", "shield", "potion"],
                "health": 85,
                "gold": 1250,
                "play_time_seconds": 3600
            }
        }
    ]
}
```

**Parameters:**
- `slot` (optional): Filter by save slot
- `limit` (optional): Number of saves to return (default: 1, most recent)

---

### POST /api/progress/list - List Save Slots

**Authentication:** API Key (Bearer token)

**Purpose:** Get summary of all save slots for a player.

**Request:**
```json
{
    "game": "my-game",
    "player_id": "user123"
}
```

**Response:**
```json
{
    "player_id": "user123",
    "game": "my-game",
    "slots": [
        {
            "slot": 1,
            "last_saved": "2026-01-14T10:01:35Z",
            "save_count": 15,
            "preview": {
                "level": 5,
                "play_time_seconds": 3600
            }
        },
        {
            "slot": 2,
            "last_saved": "2026-01-12T14:30:00Z",
            "save_count": 8,
            "preview": {
                "level": 3,
                "play_time_seconds": 1800
            }
        }
    ]
}
```

---

## Developer UI

### Dashboard Features

| Page | Route | Description |
|------|-------|-------------|
| Overview | `/dashboard` | Stats summary, recent activity |
| Players | `/dashboard/players` | List all players with save data |
| Player View | `/dashboard/player/{id}` | All saves for a specific player |
| Game View | `/dashboard/game/{name}` | All saves for a specific game |
| Settings Browser | `/dashboard/settings` | Browse player settings |
| Progress Browser | `/dashboard/progress` | Browse progress saves |
| Stats | `/dashboard/stats` | Detailed statistics |

### Dashboard Overview Page

```
┌─────────────────────────────────────────────────────────────────────────┐
│  StrataSave Dashboard                                    [User ▼] [Logout]│
├─────────────────────────────────────────────────────────────────────────┤
│                                                                          │
│  ┌──────────────┐  ┌──────────────┐  ┌──────────────┐  ┌──────────────┐ │
│  │ Total Saves  │  │ Today        │  │ Active Games │  │ Unique Users │ │
│  │   45,678     │  │    1,234     │  │      5       │  │    892       │ │
│  └──────────────┘  └──────────────┘  └──────────────┘  └──────────────┘ │
│                                                                          │
│  ┌──────────────────────────────┐  ┌──────────────────────────────────┐ │
│  │ Settings Saves: 12,345       │  │ Progress Saves: 33,333           │ │
│  └──────────────────────────────┘  └──────────────────────────────────┘ │
│                                                                          │
│  Recent Activity                                              [Refresh] │
│  ┌─────────────────────────────────────────────────────────────────────┐│
│  │ Time       │ Type     │ Game      │ Player    │ Slot │ Action      ││
│  ├─────────────────────────────────────────────────────────────────────┤│
│  │ 10:01:35   │ Progress │ my-game   │ user123   │ 1    │ save        ││
│  │ 10:01:30   │ Settings │ my-game   │ user456   │ -    │ save        ││
│  │ 10:01:28   │ Progress │ other-gm  │ user789   │ 2    │ load        ││
│  │ ...        │          │           │           │      │             ││
│  └─────────────────────────────────────────────────────────────────────┘│
│                                                                          │
│  Top Games (Last 24h)                   Most Active Players              │
│  ┌────────────────────────┐            ┌────────────────────────┐       │
│  │ my-game      2,432     │            │ user123      234       │       │
│  │ other-game   1,210     │            │ user456      187       │       │
│  │ test-game      343     │            │ user789      154       │       │
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
│  │ Total Saves  │  │ Games        │  │ Last Active  │                   │
│  │     234      │  │      3       │  │  5 min ago   │                   │
│  └──────────────┘  └──────────────┘  └──────────────┘                   │
│                                                                          │
│  ┌─────────────────────────────────────────────────────────────────────┐│
│  │ Settings                                                             ││
│  ├─────────────────────────────────────────────────────────────────────┤│
│  │ Game        │ Last Updated    │ Actions                             ││
│  ├─────────────────────────────────────────────────────────────────────┤│
│  │ my-game     │ 5 min ago       │ [View] [Delete]                     ││
│  │ other-game  │ 2 days ago      │ [View] [Delete]                     ││
│  └─────────────────────────────────────────────────────────────────────┘│
│                                                                          │
│  ┌─────────────────────────────────────────────────────────────────────┐│
│  │ Progress (my-game)                                                   ││
│  ├─────────────────────────────────────────────────────────────────────┤│
│  │ Slot │ Saves │ Last Save       │ Preview              │ Actions     ││
│  ├─────────────────────────────────────────────────────────────────────┤│
│  │ 1    │ 15    │ 5 min ago       │ Level 5, 1h playtime │ [View] [Del]││
│  │ 2    │ 8     │ 2 days ago      │ Level 3, 30m         │ [View] [Del]││
│  └─────────────────────────────────────────────────────────────────────┘│
│                                                                          │
└─────────────────────────────────────────────────────────────────────────┘
```

---

## Data Models

### Player Settings

```go
// Collection: {game}_settings
type PlayerSettings struct {
    ID        primitive.ObjectID     `bson:"_id,omitempty"`
    PlayerID  string                 `bson:"player_id"`
    Game      string                 `bson:"game"`
    Settings  map[string]interface{} `bson:"settings"`
    CreatedAt time.Time              `bson:"created_at"`
    UpdatedAt time.Time              `bson:"updated_at"`
}
```

**Indexes:**
- Unique compound index on `(player_id, game)` for upsert operations

### Player Progress

```go
// Collection: {game}_progress
type PlayerProgress struct {
    ID        primitive.ObjectID     `bson:"_id,omitempty"`
    PlayerID  string                 `bson:"player_id"`
    Game      string                 `bson:"game"`
    Slot      int                    `bson:"slot"`
    Progress  map[string]interface{} `bson:"progress"`
    SavedAt   time.Time              `bson:"saved_at"`
}
```

**Indexes:**
- Index on `(player_id, game, slot, saved_at)` for efficient queries

### Statistics

```go
// Collection: stats
type DailyStats struct {
    ID                primitive.ObjectID `bson:"_id,omitempty"`
    Date              time.Time          `bson:"date"`
    SettingsSaves     int64              `bson:"settings_saves"`
    SettingsLoads     int64              `bson:"settings_loads"`
    ProgressSaves     int64              `bson:"progress_saves"`
    ProgressLoads     int64              `bson:"progress_loads"`
    UniquePlayerIds   int64              `bson:"unique_players"`
    AvgResponseMs     float64            `bson:"avg_response_ms"`
    SavesByGame       map[string]int64   `bson:"saves_by_game"`
    SavesByPlayer     map[string]int64   `bson:"saves_by_player"`
}
```

---

## Configuration

### Environment Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `STRATASAVE_API_KEY` | (required) | API key for game authentication |
| `STRATASAVE_MONGO_URI` | `mongodb://localhost:27017` | MongoDB connection |
| `STRATASAVE_MONGO_DATABASE` | `stratasave` | Database name |
| `STRATASAVE_SESSION_KEY` | (required in prod) | Session signing key |
| `STRATASAVE_CSRF_KEY` | (required in prod) | CSRF token key |
| `STRATASAVE_MAX_SETTINGS_SIZE` | `65536` | Max settings JSON size (64KB) |
| `STRATASAVE_MAX_PROGRESS_SIZE` | `1048576` | Max progress JSON size (1MB) |
| `STRATASAVE_MAX_SLOTS` | `10` | Max save slots per player/game |

---

## Migration from strata_save

### What to Keep

- Basic save/load logic
- Game name validation
- MongoDB storage patterns
- Timeout handling

### What to Change

- Split into Settings vs Progress endpoints
- Add upsert for settings
- Add session-based auth for UI
- Add CSRF for UI forms
- Add statistics tracking
- Use strata's template system

### What to Add

- Settings endpoints (new)
- Progress history (save slots)
- Developer login/logout
- Dashboard pages
- Player/game browsing
- Statistics collection

---

## Differences: Settings vs Progress

| Aspect | Settings | Progress |
|--------|----------|----------|
| **Purpose** | Player preferences | Game state |
| **Storage** | One doc per player/game | Many docs per player/game |
| **Operation** | Upsert (replace) | Insert (append) |
| **History** | No (only latest) | Yes (full history) |
| **Typical Size** | Small (< 64KB) | Medium (< 1MB) |
| **Slots** | N/A | Multiple (1-10) |
| **Collection** | `{game}_settings` | `{game}_progress` |

---

## Implementation Phases

### Phase 1: Foundation
1. Create repository from strata
2. Update module paths and config prefix
3. Set up dual auth (API key + session)
4. Implement Settings API (save/load)
5. Implement Progress API (save/load/list)
6. Basic health check

### Phase 2: Developer UI
1. Login/logout (adapt from strata)
2. Dashboard overview page
3. Player listing and search
4. Player detail view (settings + progress)

### Phase 3: Statistics
1. Stats collection on each operation
2. Stats display on dashboard
3. Response time tracking

### Phase 4: Advanced Features
1. Game-specific views
2. Save data export
3. Bulk operations (admin only)
4. Data cleanup tools
5. Save slot management

---

## API Summary

| Endpoint | Method | Auth | Purpose |
|----------|--------|------|---------|
| `/api/settings/save` | POST | API Key | Save player settings (upsert) |
| `/api/settings/load` | POST | API Key | Load player settings |
| `/api/progress/save` | POST | API Key | Save player progress (append) |
| `/api/progress/load` | POST | API Key | Load player progress |
| `/api/progress/list` | POST | API Key | List save slots |
| `/api/health` | GET | None | Health check |

---

## Security Considerations

1. **API Key Protection**: Store securely in game builds
2. **Data Isolation**: Players can only access their own data
3. **Size Limits**: Prevent oversized payloads
4. **Rate Limiting**: Consider for API endpoints
5. **CSRF Protection**: Enabled for all UI POST endpoints
6. **Input Validation**: Validate game names, slot numbers

---

## Future Enhancements

- [ ] Cloud sync status (for game UI)
- [ ] Save data versioning/migration
- [ ] Automatic cleanup of old saves
- [ ] Save data compression
- [ ] Multiple API keys per game
- [ ] Webhooks for save events
- [ ] Save data backup/restore
