# StrataLog and StrataSave Architecture

This document describes the architecture for integrating Unity WebGL games with StrataHub, StrataLog, and StrataSave services.

---

## Overview

The system consists of three independent services that work together:

| Service | Domain Example | Purpose | Auth Method |
|---------|---------------|---------|-------------|
| **StrataHub** | mhs.adroit.games | User authentication, game launching | Session cookies |
| **StrataLog** | log.adroit.games | Player activity logging | API key |
| **StrataSave** | save.adroit.games | Game save data persistence | API key |

**Key Design Principle:** User identity is authenticated by StrataHub and passed to games, which then include the identity in API requests to StrataLog/StrataSave. The log and save services trust the identity in the payload, authenticated by the API key.

---

## Architecture Diagram

```
                                    ┌─────────────────────────────┐
                                    │      Unity WebGL Game       │
                                    │   (cdn.adroit.games or      │
                                    │    test.adroit.games)       │
                                    └──────────────┬──────────────┘
                                                   │
                     ┌─────────────────────────────┼─────────────────────────────┐
                     │                             │                             │
                     ▼                             ▼                             ▼
        ┌────────────────────┐       ┌────────────────────┐       ┌────────────────────┐
        │     StrataHub      │       │     StrataLog      │       │    StrataSave      │
        │  mhs.adroit.games  │       │  log.adroit.games  │       │  save.adroit.games │
        ├────────────────────┤       ├────────────────────┤       ├────────────────────┤
        │                    │       │                    │       │                    │
        │ GET /api/user      │       │ POST /logs         │       │ POST /save         │
        │ (cookie auth)      │       │ (API key auth)     │       │ POST /load         │
        │                    │       │                    │       │ (API key auth)     │
        │ Returns:           │       │ Payload includes:  │       │                    │
        │ - isAuthenticated  │       │ - player_id        │       │ Payload includes:  │
        │ - name             │       │ - event data       │       │ - player_id        │
        │ - login_id         │       │                    │       │ - save data        │
        │                    │       │                    │       │                    │
        └────────────────────┘       └────────────────────┘       └────────────────────┘
                 │                            │                            │
                 │                            │                            │
                 ▼                            ▼                            ▼
        ┌────────────────────┐       ┌────────────────────┐       ┌────────────────────┐
        │ MongoDB (users,    │       │ MongoDB (logs)     │       │ MongoDB (saves)    │
        │ sessions, etc.)    │       │                    │       │                    │
        └────────────────────┘       └────────────────────┘       └────────────────────┘
```

---

## Production Flow

When a game is launched from StrataHub in production:

### Step 1: User Logs into StrataHub

```
Browser                                    StrataHub (mhs.adroit.games)
   │                                                │
   │  POST /login (credentials)                     │
   │ ─────────────────────────────────────────────► │
   │                                                │
   │  Set-Cookie: stratahub-session=...             │
   │  Domain=.adroit.games                          │
   │  SameSite=None; Secure; HttpOnly               │
   │ ◄───────────────────────────────────────────── │
   │                                                │
```

### Step 2: User Launches Game

StrataHub serves a page that loads the Unity WebGL game from the CDN:

```html
<!-- Page on mhs.adroit.games -->
<iframe src="https://cdn.adroit.games/games/my-game/index.html"></iframe>
<!-- or embedded directly -->
```

### Step 3: Game Fetches User Identity

The game calls StrataHub's `/api/user` endpoint to get the logged-in user's identity:

```javascript
// Game code running on cdn.adroit.games
fetch('https://mhs.adroit.games/api/user', {
    method: 'GET',
    credentials: 'include'  // Send cookies
})
.then(response => response.json())
.then(user => {
    if (user.isAuthenticated) {
        // Store for use in log/save calls
        this.playerId = user.login_id;
        this.playerName = user.name;
    }
});
```

**Why this works:**
- Session cookie has `Domain=.adroit.games` (accessible to all subdomains)
- Session cookie has `SameSite=None` (sent in cross-origin requests)
- CORS on StrataHub allows `cdn.adroit.games` with credentials
- `/api/user` is a GET request (no CSRF token required)

### Step 4: Game Logs Activity

```javascript
// Game logs player activity
fetch('https://log.adroit.games/logs', {
    method: 'POST',
    headers: {
        'Authorization': 'Bearer ' + API_KEY,
        'Content-Type': 'application/json'
    },
    body: JSON.stringify({
        player_id: this.playerId,      // From /api/user
        player_name: this.playerName,  // From /api/user
        event_type: 'level_complete',
        level: 5,
        score: 1250,
        timestamp: new Date().toISOString()
    })
});
```

**Why this works:**
- API key in Authorization header (not cookies)
- CORS `*` allows any origin
- `AllowCredentials: false` (no cookies needed)
- No CSRF protection needed (API key auth)

### Step 5: Game Saves/Loads Data

```javascript
// Save game state
fetch('https://save.adroit.games/save', {
    method: 'POST',
    headers: {
        'Authorization': 'Bearer ' + API_KEY,
        'Content-Type': 'application/json'
    },
    body: JSON.stringify({
        player_id: this.playerId,
        game_id: 'my-game',
        slot: 1,
        data: {
            level: 5,
            inventory: [...],
            progress: {...}
        }
    })
});

// Load game state
fetch('https://save.adroit.games/load', {
    method: 'POST',
    headers: {
        'Authorization': 'Bearer ' + API_KEY,
        'Content-Type': 'application/json'
    },
    body: JSON.stringify({
        player_id: this.playerId,
        game_id: 'my-game',
        slot: 1
    })
});
```

---

## Development Flow

When developers run the game in the Unity Editor:

```
┌─────────────────────────────────────────────────────────────────────┐
│                        Developer's Machine                           │
├─────────────────────────────────────────────────────────────────────┤
│                                                                      │
│   Unity Editor                                                       │
│   ┌────────────────────────────────────────────────────────────┐    │
│   │                                                             │    │
│   │  // Hard-coded for development                              │    │
│   │  #if UNITY_EDITOR                                           │    │
│   │      playerId = "dev_test_user";                            │    │
│   │      playerName = "Developer";                              │    │
│   │  #else                                                      │    │
│   │      // Fetch from /api/user in production                  │    │
│   │  #endif                                                     │    │
│   │                                                             │    │
│   └────────────────────────────────────────────────────────────┘    │
│                              │                                       │
│                              │ No StrataHub interaction needed       │
│                              │                                       │
│                              ▼                                       │
│   ┌─────────────────────────────────────────────────────────────┐   │
│   │                                                              │   │
│   │  POST https://log.adroit.games/logs                          │   │
│   │  Authorization: Bearer API_KEY                               │   │
│   │  Body: { player_id: "dev_test_user", ... }                   │   │
│   │                                                              │   │
│   │  POST https://save.adroit.games/save                         │   │
│   │  Authorization: Bearer API_KEY                               │   │
│   │  Body: { player_id: "dev_test_user", ... }                   │   │
│   │                                                              │   │
│   └─────────────────────────────────────────────────────────────┘   │
│                                                                      │
└──────────────────────────────────────────────────────────────────────┘
                              │
                              │ CORS: * allows any origin
                              │ (including localhost, file://, etc.)
                              ▼
                    ┌───────────────────┐
                    │ log.adroit.games  │
                    │ save.adroit.games │
                    └───────────────────┘
```

**Why this works:**
- CORS `*` allows requests from any origin (localhost, file://, etc.)
- No cookies involved, so `AllowCredentials: false` is fine
- Developer hard-codes identity, bypassing need for StrataHub
- Same API key authentication as production

---

## Authentication Comparison

### StrataHub: Cookie-Based Session Auth

```
┌─────────────────────────────────────────────────────────────────┐
│ Request to mhs.adroit.games/api/user                            │
├─────────────────────────────────────────────────────────────────┤
│ GET /api/user HTTP/1.1                                          │
│ Host: mhs.adroit.games                                          │
│ Cookie: stratahub-session=eyJ...                                │
│         ▲                                                       │
│         │                                                       │
│         └── Browser automatically sends this cookie             │
│             (Domain=.adroit.games matches)                      │
└─────────────────────────────────────────────────────────────────┘
```

**Characteristics:**
- Browser automatically sends cookies
- Requires `SameSite=None` for cross-origin
- Requires `Domain=.adroit.games` for subdomain access
- Vulnerable to CSRF (hence CSRF protection)
- CORS must whitelist specific origins with `credentials: true`

### StrataLog/StrataSave: API Key Auth

```
┌─────────────────────────────────────────────────────────────────┐
│ Request to log.adroit.games/logs                                │
├─────────────────────────────────────────────────────────────────┤
│ POST /logs HTTP/1.1                                             │
│ Host: log.adroit.games                                          │
│ Authorization: Bearer sk_live_abc123...                         │
│                ▲                                                │
│                │                                                │
│                └── Game code explicitly adds this header        │
│                    (browser never auto-sends it)                │
└─────────────────────────────────────────────────────────────────┘
```

**Characteristics:**
- Game code must explicitly add Authorization header
- Browser never auto-sends Authorization headers
- NOT vulnerable to CSRF (attacker can't add headers)
- CORS `*` is safe with `AllowCredentials: false`
- Works from any origin (CDN, localhost, file://)

---

## Security Model

### Trust Hierarchy

```
┌─────────────────────────────────────────────────────────────────┐
│                        TRUST CHAIN                               │
├─────────────────────────────────────────────────────────────────┤
│                                                                  │
│  StrataHub                                                       │
│  ─────────                                                       │
│  "I authenticate users via login credentials"                   │
│  "I issue session cookies to prove identity"                    │
│  "GET /api/user returns the authenticated user's info"          │
│       │                                                          │
│       │ User identity                                            │
│       ▼                                                          │
│  Game (with API Key)                                             │
│  ───────────────────                                             │
│  "I got user identity from StrataHub"                           │
│  "I include this identity in my API requests"                   │
│  "I have the API key that proves I'm a legitimate game"         │
│       │                                                          │
│       │ API key + player identity in payload                     │
│       ▼                                                          │
│  StrataLog / StrataSave                                          │
│  ──────────────────────                                          │
│  "I trust requests with a valid API key"                        │
│  "I store data associated with the player_id in the payload"   │
│  "I don't verify player identity - that's StrataHub's job"     │
│                                                                  │
└─────────────────────────────────────────────────────────────────┘
```

### What Each Service Trusts

| Service | Trusts | Does NOT Trust |
|---------|--------|----------------|
| **StrataHub** | User credentials (password, OAuth) | Nothing else |
| **Game** | StrataHub's /api/user response | Raw user input |
| **StrataLog** | Valid API key = legitimate game | Player identity (accepts from payload) |
| **StrataSave** | Valid API key = legitimate game | Player identity (accepts from payload) |

### Security Boundaries

| Attack Vector | Mitigation |
|--------------|------------|
| Stolen session cookie | HttpOnly flag prevents JS access; Secure flag requires HTTPS |
| CSRF on StrataHub | CSRF tokens required for POST requests |
| CSRF on StrataLog/Save | N/A - API key auth not vulnerable to CSRF |
| Forged API requests | API key required; key should be kept secret in game builds |
| Player impersonation | API key holder (game) is responsible for correct identity |
| Origin spoofing | CORS is advisory; API key is the real protection |

---

## CORS Configuration

### StrataHub

```go
// Specific origins, credentials allowed
cors.Options{
    AllowedOrigins:   []string{
        "https://cdn.adroit.games",
        "https://test.adroit.games",
    },
    AllowCredentials: true,  // Allow cookies
    AllowedMethods:   []string{"GET", "POST", "OPTIONS"},
}
```

### StrataLog / StrataSave

```go
// Any origin, no credentials
cors.Options{
    AllowedOrigins:   []string{"*"},
    AllowCredentials: false,  // No cookies
    AllowedMethods:   []string{"GET", "POST", "OPTIONS"},
    AllowedHeaders:   []string{"Authorization", "Content-Type"},
}
```

**Why CORS `*` is safe for StrataLog/StrataSave:**
1. No cookies are sent (`AllowCredentials: false`)
2. Authentication is via API key in header
3. Browsers don't auto-send Authorization headers
4. An attacker's page cannot forge the Authorization header

---

## Data Flow Summary

### Production: User Plays Game

```
1. User logs into mhs.adroit.games
   └── Gets session cookie (Domain=.adroit.games)

2. User clicks "Play Game"
   └── Game loads from cdn.adroit.games

3. Game calls GET mhs.adroit.games/api/user
   ├── Cookie sent automatically (same parent domain)
   └── Gets: { isAuthenticated: true, login_id: "jsmith", name: "John" }

4. Game plays, logs events to log.adroit.games
   ├── Authorization: Bearer API_KEY
   └── Body: { player_id: "jsmith", event_type: "score", ... }

5. Game saves progress to save.adroit.games
   ├── Authorization: Bearer API_KEY
   └── Body: { player_id: "jsmith", game_id: "game1", data: {...} }
```

### Development: Developer Tests Game

```
1. Developer runs game in Unity Editor
   └── No browser, no cookies, no StrataHub

2. Game uses hard-coded identity
   └── player_id = "dev_user", player_name = "Developer"

3. Game logs events to log.adroit.games
   ├── Authorization: Bearer API_KEY (same as production)
   └── Body: { player_id: "dev_user", ... }

4. Game saves progress to save.adroit.games
   ├── Authorization: Bearer API_KEY
   └── Body: { player_id: "dev_user", ... }
```

---

## API Endpoints Summary

### StrataHub

| Endpoint | Method | Auth | Purpose |
|----------|--------|------|---------|
| `/api/user` | GET | Session cookie | Get current user's identity |

### StrataLog

| Endpoint | Method | Auth | Purpose |
|----------|--------|------|---------|
| `POST /logs` | POST | API key | Log player activity |
| `GET /logs` | GET | API key | List recent logs (JSON) |
| `GET /logs/view` | GET | None | View logs (HTML page) |
| `GET /logs/download` | GET | None | Download logs (CSV) |
| `GET /health` | GET | None | Health check |

### StrataSave

| Endpoint | Method | Auth | Purpose |
|----------|--------|------|---------|
| `POST /save` | POST | API key | Save game data |
| `POST /load` | POST | API key | Load game data |
| `GET /health` | GET | None | Health check |

---

## Implementation Notes

### When Building on Strata

Since strata includes CSRF protection by default, when implementing stratalog and stratasave:

1. **Mount API routes WITHOUT CSRF middleware**
   ```go
   // API routes - API key auth, no CSRF
   r.Group(func(r chi.Router) {
       r.Use(corsMiddleware())  // AllowCredentials: false
       r.Use(APIKeyAuth(cfg.APIKey))
       r.Post("/logs", h.LogsHandler)
       // No CSRF middleware here
   })
   ```

2. **Keep CORS `*` with `AllowCredentials: false`**
   - This allows development from Unity Editor
   - API key provides the security, not CORS

3. **Include player identity in request payload**
   - Don't try to read from cookies
   - Trust the identity provided by API key holder

### Environment Variables

| Service | Variable | Purpose |
|---------|----------|---------|
| StrataLog | `STRATALOG_API_KEY` | API key for authentication |
| StrataSave | `STRATASAVE_API_KEY` | API key for authentication |

---

## Related Documentation

- [/api/user Endpoint](api_user.md) - Details on the user identity endpoint
- [CSRF Implementation](csrf_implementation.md) - CSRF protection in StrataHub
