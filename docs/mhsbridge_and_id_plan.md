# MHSBridge, Identity, and Game Hosting Plan

**Date:** 2026-03-19
**Status:** Planned (discussion complete, implementation not started)

---

## Table of Contents

1. [Background and Motivation](#1-background-and-motivation)
2. [The Host Page Contract](#2-the-host-page-contract)
3. [Identity System Changes](#3-identity-system-changes)
4. [Service Credential Delivery](#4-service-credential-delivery)
5. [Unit Navigation and URL Hacking Prevention](#5-unit-navigation-and-url-hacking-prevention)
6. [MHSBridge Changes](#6-mhsbridge-changes)
7. [Stratalog and Stratasave Changes](#7-stratalog-and-stratasave-changes)
8. [Three Hosting Contexts](#8-three-hosting-contexts)
9. [Game Build Hosting — Future Architecture](#9-game-build-hosting--future-architecture)
10. [Multi-Version and Build Management](#10-multi-version-and-build-management)
11. [Implementation Priority and Phasing](#11-implementation-priority-and-phasing)
12. [What Is NOT Changing](#12-what-is-not-changing)
13. [Open Questions](#13-open-questions)

---

## 1. Background and Motivation

### Problems with the current architecture

**Identity is domain-locked.** MHSBridge hardcodes `adroit.games/api/user` for player identity in URL mode. This means the game can only work with the adroit.games StrataHub instance. A different StrataHub deployment at `xyz.games` would require a game rebuild.

**Session cookie ambiguity.** StrataHub uses a single session cookie scoped to `.adroit.games` (the apex domain). In multi-workspace mode (mhs.adroit.games, dev.adroit.games), a user can only be logged into one workspace at a time — the second login overwrites the first. If a game calls `adroit.games/api/user`, it gets whichever session is current, which may not match the workspace the game was launched from.

**Service endpoints are hardcoded in the game binary.** The URLs and auth credentials for stratalog (`log.adroit.games`) and stratasave (`save.adroit.games`) are compiled into the Unity WebGL build. This prevents using different logging/saving services or rotating credentials without rebuilding the game.

**Duplicate game builds.** Developers maintain separate S3 copies of game builds for URL-based testing, separate from the builds used by Mission HydroSci PWA. The PWA uses a manifest embedded in the Go binary, requiring a StrataHub rebuild to update.

**Students can URL-hack unit access.** The play URL pattern `/missionhydrosci/play/unit1` is predictable. Students have changed `unit1` to `unit3` or `unit5` to skip ahead.

### Design principle

Everything the game needs should come from the host page, not from the game binary. The game reads a well-known JavaScript global at startup and uses whatever it finds. This makes game builds portable across StrataHub instances, hosting environments, and service configurations — and means the game can be updated once and then left alone while infrastructure changes happen around it.

---

## 2. The Host Page Contract

The host page (whether a Go template, a static HTML file, or a developer's local page) sets a single JavaScript global before Unity starts:

```javascript
window.__mhsBridgeConfig = {
  identity: {
    user_id: "dale@example.com",    // login_id value for now; ObjectID in Phase 2
    name: "Dale Musser"
  },
  services: {
    log: {
      url: "https://log.adroit.games",
      auth: "Bearer abc123..."
    },
    save: {
      url: "https://save.adroit.games",
      auth: "Bearer xyz789..."
    }
  },
  navigation: {
    unitMap: {                          // optional; omit for relative navigation
      "unit1": "/missionhydrosci/play/t/abc123",   // available (opaque URL)
      "unit2": "/missionhydrosci/play/t/def456",   // available
      "unit3": null,                                // locked
      "unit4": null,                                // locked
      "unit5": null                                 // locked
    }
  }
};
```

### Contract rules

- **MHSBridge reads `window.__mhsBridgeConfig` on startup.**
  - If present: use it (new builds).
  - If absent: fall back to legacy behavior (old builds — hardcoded service endpoints, relative navigation, existing identity mechanisms).
- **`identity`** provides user_id and name. The game uses `user_id` for all logging and saving.
- **`services`** provides URLs and full authorization headers for log and save services. The game uses these instead of hardcoded values.
- **`navigation.unitMap`** (optional) maps logical unit names to URLs. If a unit's value is a URL string, it's available. If `null`, it's locked. If the unitMap is absent entirely, the loader/game falls back to relative URL navigation (`../unit2/`). The loader game decides whether to use the unitMap or relative navigation based on what's available.

### Why this shape

- **No hardcoded domains anywhere in the game.** Every external dependency comes from the config.
- **The same config shape works in all hosting contexts** — only how it gets populated differs.
- **Backward compatible.** Old builds that don't look for `__mhsBridgeConfig` keep working. New builds that find it absent fall back to legacy.
- **Service credentials never appear in HTML source** in production (StrataHub injects them server-side from config). For developer builds, credentials are fetched via an authenticated API call before Unity starts.

---

## 3. Identity System Changes

### The `user_id` transition

Identity is transitioning from `login_id` (human-readable login string like "dale@example.com") to `user_id` (MongoDB ObjectID) as the canonical identifier. This is a multi-phase process.

**Phase 1 (current):** The key is named `user_id` everywhere, but the value it carries is the `login_id`. This establishes the pipeline — all systems use the `user_id` key — while the actual value remains the familiar login string.

**Phase 2 (future):** The value switches to the MongoDB ObjectID. This requires:
- Updating existing log and save records in the database
- Updating stratalog and stratasave to work with ObjectIDs
- Updating StrataHub to pass `user.ID.Hex()` instead of `user.LoginID`

### `/api/user` endpoint (already updated)

The StrataHub `/api/user` endpoint now returns `user_id` alongside the legacy fields:

```json
{
  "isAuthenticated": true,
  "name": "Dale Musser",
  "user_id": "dale@example.com",
  "login_id": "dale@example.com",
  "email": "dale@example.com"
}
```

- `user_id` — canonical field; carries login_id now, will carry ObjectID in Phase 2
- `login_id` — the human-readable login string (permanent)
- `email` — legacy alias for login_id (backward compat with older game builds)

**File:** `stratahub/internal/app/features/userinfo/handler.go`

### How identity reaches the game

| Context | How identity is provided |
|---------|-------------------------|
| Mission HydroSci PWA | Go template injects `__mhsBridgeConfig.identity` server-side from the authenticated session |
| StrataHub-served URL page (future) | Same — Go template injection |
| Replacement index.html (developer hosted builds) | JavaScript fetches `/api/user` from the StrataHub instance, extracts `user_id` and `name`, populates the config, then starts Unity |
| Developer local build | Same as replacement index.html — JS fetch to the StrataHub instance they're logged into |

In no case does MHSBridge itself make a network call for identity. The host page handles it.

---

## 4. Service Credential Delivery

### The problem

Stratalog and stratasave authenticate via Bearer token API keys. These keys are currently hardcoded in the game binary. Putting them in HTML source would expose them to anyone who views page source.

### The solution: `/api/game-config` endpoint (new)

A new StrataHub endpoint that returns service URLs and credentials for a specific game:

```
GET /api/game-config?game=mhs
Authorization: (session cookie — user must be logged in)

Response:
{
  "game": "mhs",
  "services": {
    "log": {
      "url": "https://log.adroit.games",
      "auth": "Bearer <stratalog-api-key>"
    },
    "save": {
      "url": "https://save.adroit.games",
      "auth": "Bearer <stratasave-api-key>"
    }
  }
}
```

The `game` query parameter is required. Different games can have different service endpoints, auth credentials, and configurations. Returns 400 if `game` is missing or unrecognized.

**Key properties:**
- Requires an authenticated session (same as `/api/user`)
- Returns the service credentials from StrataHub's server-side configuration — they never appear in HTML source
- Different games can point to different log/save instances with different credentials
- Different workspaces could also have different configurations per game
- The API keys are the same ones currently hardcoded in the game — this just moves where they're stored

### How service config reaches the game

| Context | How services are provided |
|---------|--------------------------|
| Mission HydroSci PWA | Go template calls the config internally at render time and injects `__mhsBridgeConfig.services` server-side. No client-side request. |
| StrataHub-served URL page (future) | Same — server-side injection |
| Replacement index.html (hosted builds) | JavaScript fetches `/api/game-config` from the StrataHub instance, populates the config, then starts Unity |
| Developer local build | Same as replacement index.html |

### What goes in StrataHub's config

New config keys in `config.toml`, organized per game:

```toml
[game_services.mhs]
log_url = "https://log.adroit.games"
log_auth = "Bearer <stratalog-api-key>"
save_url = "https://save.adroit.games"
save_auth = "Bearer <stratasave-api-key>"

# Additional games can be added:
# [game_services.anothergame]
# log_url = "https://log.otherdomain.com"
# log_auth = "Bearer <other-api-key>"
# save_url = "https://save.otherdomain.com"
# save_auth = "Bearer <other-api-key>"
```

---

## 5. Unit Navigation and URL Hacking Prevention

### Current problem

Students can change `/missionhydrosci/play/unit1` to `/missionhydrosci/play/unit5` in the URL bar to skip units.

### Solution: two layers

**Layer 1 — Server-side gate (belt):** The `/missionhydrosci/play/{unit}` handler checks the student's progress (`mhsuserprogress`) before serving the page. If the student hasn't completed the prerequisite units, return a "not yet available" page instead of the game. This is the real enforcement — it works regardless of how the student arrives at the URL.

**Layer 2 — Opaque navigation URLs in the unitMap (suspenders):** Instead of predictable URLs like `/missionhydrosci/play/unit3`, the unitMap contains opaque token-based URLs like `/missionhydrosci/play/t/abc8f2e1`. Students can't guess other unit URLs by pattern. The tokens are generated per-session or per-page-load.

### Navigation modes

MHSBridge exposes navigation capabilities. The loader game decides which to use.

**unitMap navigation:** MHSBridge provides a method like `GetUnitURL(unitName)` that looks up the unit in `__mhsBridgeConfig.navigation.unitMap`:
- Returns the URL string if the unit is available
- Returns null if the unit is locked (value is `null` in the map)
- Returns null if the unitMap doesn't exist or the unit isn't in it

**Relative navigation (fallback):** If no unitMap is available (or the loader chooses not to use it), the loader navigates using relative URLs (`../unit2/`). This is the current behavior and works with the developer folder structure.

**The loader game's logic:**
1. Get config from MHSBridge
2. If unitMap exists and has an entry for the target unit → use that URL
3. If no unitMap → use relative navigation
4. If unitMap entry is null → unit is locked, handle accordingly

### How the unitMap is populated

| Context | unitMap contents |
|---------|-----------------|
| Mission HydroSci PWA | StrataHub generates at render time from student's progress. Completed + current unit get real URLs; future units get `null`. URLs are opaque tokens. |
| StrataHub-served URL page (future) | Same as PWA |
| Replacement index.html | Optional. Could be omitted (loader uses relative nav) or could contain static relative paths. No locking — developers can access any unit. |
| Developer local build | Typically omitted — relative navigation is fine for testing |

---

## 6. MHSBridge Changes

### New MHSBridge behavior (summary)

On startup:
1. Check for `window.__mhsBridgeConfig`
2. If present → store identity, services, and navigation config
3. If absent → legacy mode (existing identity mechanisms, hardcoded services, relative navigation)

### MHSBridge.cs — Updated API

```
GetPlayerID()
  New: Returns user_id from stored config
  Legacy fallback: Returns empty string (game uses its existing identity mechanism)

GetServiceConfig()  [NEW]
  Returns log/save URLs and auth from stored config
  Legacy fallback: Returns null (game uses hardcoded values)

GetUnitURL(unitName)  [NEW]
  If unitMap exists and has entry → return URL string or null (locked)
  If no unitMap → return null (caller should use relative navigation)

CompleteUnit(currentUnitId, nextUnitRelativeUrl)
  PWA mode: Notifies host page via mhsUnitComplete callback (unchanged)
  URL mode: Navigates to next unit (unchanged)

NavigateToUnit(unitName)  [UPDATED]
  Uses unitMap if available, falls back to relative navigation
  Clean URL navigation — no params carried forward

OnPWAReady(configJson)  [UPDATED]
  Parameter changes from unused empty string to JSON string
  Parses and stores identity from the JSON: { user_id, name }
  Sets _isPWA = true (same as before)
  In Mission HydroSci, this receives the identity portion of the config
```

### MHSBridge.jslib — Updated functions

```
MHSBridge_GetPlayerID()
  Reads from window.__mhsBridgeConfig.identity.user_id
  Returns empty string if config absent

MHSBridge_GetConfig()  [NEW]
  Returns JSON.stringify(window.__mhsBridgeConfig) or empty string

MHSBridge_NavigateToUnit(url)
  Navigates to the given URL (resolves relative URLs against current page)
```

### Backward compatibility

Old game builds (before this update):
- Don't look for `__mhsBridgeConfig` — they keep using their existing identity mechanism and hardcoded services
- `OnPWAReady('')` still works — empty string is handled gracefully
- XHR/fetch intercept in the current play template still works for old builds

New game builds (after this update):
- Read `__mhsBridgeConfig` for everything
- If config is absent (e.g., running on an old host page), fall back to legacy behavior
- This means new builds work with both old and new host pages during transition

### The clean cut

Once all deployed builds use the new MHSBridge:
- Remove the XHR/fetch intercept from the play template
- Remove legacy identity fallback code from MHSBridge

---

## 7. Stratalog and Stratasave Changes

### Current identity key names

| Service | JSON key | JSON tag | MongoDB field |
|---------|----------|----------|---------------|
| Stratalog | `playerId` | `json:"playerId"` | `playerId` |
| Stratasave | `user_id` | `json:"user_id"` | `user_id` |

### What needs to change

**Stratasave:** Already uses `user_id` — no change needed.

**Stratalog:** Currently uses `playerId`. Needs to also accept `user_id` as an alias. During transition, the handler should:
1. Check for `user_id` in the incoming JSON
2. If present, use it as the player identifier
3. If absent, fall back to `playerId` (backward compat with old game builds)
4. Store as `playerId` in MongoDB for now (existing queries, grader, and dashboard all use `playerId`)

This means old builds sending `playerId` keep working. New builds sending `user_id` also work. The stored field name in MongoDB stays `playerId` until Phase 2 when we do the full data migration.

### Auth mechanism (no change needed)

Both services use Bearer token auth (`Authorization: Bearer <api-key>`). The mechanism stays the same — what changes is where the game gets the token from (config instead of hardcoded).

---

## 8. Three Hosting Contexts

### Context 1: Mission HydroSci PWA (StrataHub-managed)

**How it works today:**
- User navigates to `/missionhydrosci/play/unit1`
- Go template serves the play page with XHR/fetch intercept for identity
- `?id=login_id` is NOT in the URL — identity comes from the intercept
- Service worker caches game files; falls back to CDN via `/missionhydrosci/content/*`
- `OnPWAReady('')` signals PWA mode
- Unit completion triggers `mhsUnitComplete` callback

**How it will work (near-term):**
- Go template injects `window.__mhsBridgeConfig` with identity and services (server-side, from session and config)
- XHR/fetch intercept is kept temporarily for backward compat with old builds
- `OnPWAReady(configJson)` passes `{ user_id, name }` as JSON string
- Navigation unitMap injected with opaque URLs and null for locked units
- Server-side progress gate prevents URL hacking
- Service credentials come from StrataHub config, injected server-side (never in page source for inspection)

### Context 2: Replacement index.html (developer-hosted builds)

**How it works today:**
- Developers upload builds to S3 with folder structure: `<build-id>/loader/`, `<build-id>/unit1/`, etc.
- Unity-generated `index.html` is used as-is
- Identity comes from the game's existing jslib that calls `/api/user` on the StrataHub instance
- Log/save service URLs and auth are hardcoded in the game binary
- Unit navigation uses relative URLs (`../unit2/`)

**How it will work (near-term):**
- We provide replacement `index.html` files that developers drop into their builds (replacing Unity's generated `index.html`)
- The replacement index.html contains:
  - JavaScript that fetches `/api/user` from the StrataHub instance to get identity
  - JavaScript that fetches `/api/game-config` from the StrataHub instance to get service credentials
  - Assembles `window.__mhsBridgeConfig` from the fetch results
  - Starts Unity after the config is populated
- Log/save URLs and auth strings could also be statically defined in the replacement index.html as a fallback (for offline/disconnected testing)
- No unitMap — relative navigation is used (developers don't need unit locking)
- This serves as a surrogate for what StrataHub will provide in the future when it serves game pages directly

**Developer workflow:**
1. Do a Unity build
2. Replace the generated `index.html` files with the provided replacement files
3. Upload to S3 or run on local web server
4. Log into StrataHub (dev.adroit.games or mhs.adroit.games)
5. Navigate to the game URL — identity and service config are fetched automatically

### Context 3: Developer local build (localhost / file://)

**How it works:** Same as Context 2 but running on `localhost` or from the file system. The replacement `index.html` fetches from whichever StrataHub instance the developer is logged into. Since developers are typically logged into only one workspace, the session cookie ambiguity issue doesn't apply.

### Future Context: StrataHub-served URL pages

**Not being built now**, but the architecture supports it. When ready:
- StrataHub serves a Go template page for URL-launched games (e.g., `/games/<build-id>/unit1/`)
- The template injects `__mhsBridgeConfig` server-side (same as Mission HydroSci PWA)
- Game assets load from CDN
- Same build, same MHSBridge — no game changes needed
- The replacement index.html becomes unnecessary
- Identity is workspace-scoped (no cookie ambiguity)
- unitMap with opaque URLs prevents URL hacking
- This eliminates the `/api/user` dependency from the game entirely

---

## 9. Game Build Hosting — Future Architecture

### Current state (problems)

- Mission HydroSci uses builds at `cdn.adroit.games/mhs/unitX/vX.X.X/`
- Developers use separate builds at different S3 paths
- The manifest is embedded in the Go binary (`mhs_content_manifest.json`) — updating it requires a StrataHub rebuild
- Dale manually uploads builds to S3, generates manifests locally, and rebuilds StrataHub
- There is no unified build storage — PWA and URL-launched games use different copies

### Future state (goals)

**One copy of each build serves all contexts.** A single set of game files in S3 is used by both Mission HydroSci PWA and URL-launched games. The wrapper page (Go template or replacement index.html) provides the CDN base URL via config.

**Manifest is managed by StrataHub, not embedded.** When a build is registered or uploaded, StrataHub generates and stores the manifest. No rebuild needed to update game content.

**Build channels for versioning:**
- `production` — what members/teachers see
- `staging` / `testing` — what the team validates before promoting
- Each channel points to a specific build ID; promoting is updating a pointer, not copying files

**StrataHub-managed uploads (optional future feature):**
- Admin page to upload a build zip
- StrataHub extracts to S3, generates manifest, registers the build
- Eliminates manual S3 uploads

### Not being built now

This section documents the future direction so decisions made now don't conflict with it. The near-term changes (MHSBridge contract, `/api/game-config`, replacement index.html) are all compatible with this future architecture.

---

## 10. Multi-Version and Build Management

### The problem

- Different units may be at different versions (unit1 v2.2.2, unit2 v2.3.0)
- Need access to more than one version at a time (production vs. testing)
- Developer folder structure (`<build-id>/unit1/`, `<build-id>/unit2/`) uses relative URLs between sibling folders
- Mission HydroSci PWA uses versioned paths (`unit1/v2.2.2/Build/`)

### How the unitMap helps

The unitMap abstracts away the physical location of each unit. The loader/game asks for "unit2" and gets a URL — it doesn't know or care whether that URL points to:
- `cdn.adroit.games/mhs/unit2/v2.3.0/`
- `cdn.adroit.games/mhs/builds/abc123/unit2/`
- `../unit2/` (relative, developer builds)

This means:
- Different units can be at different versions (different CDN paths)
- Production and staging can coexist (different unitMaps pointing to different build IDs)
- The game code is identical in all cases

### Developer builds

Developers continue using the sibling folder structure:
```
my-build/
  loader/
  unit1/
  unit2/
  ...
```

With no unitMap in the config, relative navigation works as before. When the loader navigates to `../unit1/`, it gets the build sitting right there.

---

## 11. Implementation Priority and Phasing

### Priority: Game changes first

Game builds are difficult to update and having multiple incompatible versions creates complexity. The goal is to update MHSBridge once to support the full contract, then leave it alone. Everything else (StrataHub templates, endpoints, hosting changes) can be updated independently afterward.

### Near-term implementation order

| Step | What | Where | Depends on |
|------|------|-------|-----------|
| 1 | Define and lock the `__mhsBridgeConfig` contract | This document | — |
| 2 | Update MHSBridge (.cs and .jslib) | Game (Unity) | Step 1 |
| 3 | Create `/api/game-config` endpoint | StrataHub | Step 1 |
| 4 | Update stratalog to accept `user_id` key | Stratalog | Step 1 |
| 5 | Update Mission HydroSci play template | StrataHub | Steps 2, 3 |
| 6 | Create replacement index.html files | StrataHub docs/dev-handoff | Steps 2, 3 |
| 7 | Fix unit-skipping (server-side progress gate) | StrataHub | Independent |

Steps 3 and 4 can happen in parallel.
Steps 5 and 6 depend on 2 and 3 being complete.
Step 7 is independent and can happen at any time.

### What can be deferred

- StrataHub-served URL pages (future hosting)
- Build upload UI in StrataHub
- Manifest management (moving out of Go binary)
- Build channels (production/staging)
- Phase 2 of user_id (switching to ObjectID)
- Single-session enforcement (one workspace at a time)
- Removing legacy fallback code from MHSBridge

---

## 12. What Is NOT Changing

- **Save data format and storage** — stratasave continues to work as-is
- **Log data format and storage** — stratalog continues to work as-is (with added `user_id` alias)
- **Game build pipeline** — Unity build process is unchanged
- **Service worker and offline caching** — Mission HydroSci PWA caching is unchanged
- **Developer build folder structure** — sibling folders with relative URLs still work
- **MHS Dashboard** — unaffected
- **MHS Grader** — unaffected (reads from logdata collection using `playerId`)
- **Database schemas** — no migrations needed for near-term changes

---

## 13. Open Questions

### Resolved

- **Where does identity come from?** → Host page injects it via `__mhsBridgeConfig`
- **How do service credentials reach the game securely?** → `/api/game-config` endpoint, fetched via authenticated session
- **How do we prevent unit skipping?** → Server-side progress gate + opaque URLs in unitMap
- **How do developers test locally?** → Replacement index.html fetches from StrataHub instance
- **What identity key name do we use?** → `user_id` everywhere, carrying `login_id` value for Phase 1
- **Does stratasave need changes?** → No, already uses `user_id`
- **Does stratalog need changes?** → Yes, add `user_id` as accepted alias for `playerId`

### Still open (not blocking near-term work)

- **Exact opaque URL scheme for unitMap** — hash-based tokens? Signed URLs? Session-scoped? Time-limited? To be designed when implementing the server-side progress gate.
- **How does `OnPWAReady` interact with `__mhsBridgeConfig`?** — In PWA mode, the config is in the page AND passed via `OnPWAReady`. Need to decide if `OnPWAReady` carries the full config or just identity, and whether `__mhsBridgeConfig` is the single source of truth or if `OnPWAReady` can override it.
- **Should `/api/game-config` be workspace-scoped?** — Currently both log and save point to the same service regardless of workspace. If different workspaces ever need different services, the endpoint should be workspace-aware.
- **What happens when the config fetch fails?** — In the replacement index.html, if `/api/user` or `/api/game-config` fails (user not logged in, network error), should it show an error page or start Unity without config (triggering legacy fallback)?
- **Stratalog field name long-term** — Currently stores as `playerId` in MongoDB. In Phase 2, should the MongoDB field name change to `user_id`? This affects the grader, dashboard, and all queries.

---

## Appendix A: Current MHSBridge Files

Reference copies of the current MHSBridge implementation are in `stratahub/docs/dev-handoff/`:
- `MHSBridge.jslib` — JavaScript plugin (4 functions)
- `MHSBridge.cs` — C# bridge script (singleton, mode detection, navigation)
- `MHSBridge-Integration-Guide.md` — Developer documentation

## Appendix B: Current Service Identity Keys

| Service | Endpoint | JSON key for identity | Auth method |
|---------|----------|----------------------|-------------|
| Stratalog | `POST /api/log/submit` | `playerId` | `Authorization: Bearer <api-key>` |
| Stratasave | `POST /api/state/save` | `user_id` | `Authorization: Bearer <api-key>` |
| Stratasave | `POST /api/state/load` | `user_id` | `Authorization: Bearer <api-key>` |
| Stratasave | `POST /api/settings/save` | `user_id` | `Authorization: Bearer <api-key>` |
| Stratasave | `POST /api/settings/load` | `user_id` | `Authorization: Bearer <api-key>` |
| StrataHub | `GET /api/user` | `user_id`, `login_id`, `email` | Session cookie |
| StrataHub | `GET /api/game-config?game=mhs` (new) | — | Session cookie |

## Appendix C: Config Flow Diagram

```
                    StrataHub Config (config.toml)
                    ┌──────────────────────────────┐
                    │ game_log_url                  │
                    │ game_log_auth                 │
                    │ game_save_url                 │
                    │ game_save_auth                │
                    └──────────┬───────────────────┘
                               │
              ┌────────────────┼────────────────────┐
              │                │                     │
              ▼                ▼                     ▼
     Mission HydroSci    /api/game-config    StrataHub-served
     Play Template       endpoint            URL page (future)
     (server-side        (authenticated       (server-side
      injection)          session)              injection)
              │                │                     │
              ▼                ▼                     ▼
         ┌─────────────────────────────────────────────┐
         │        window.__mhsBridgeConfig              │
         │  { identity, services, navigation }          │
         └──────────────────┬──────────────────────────┘
                            │
                            ▼
                    ┌───────────────┐
                    │   MHSBridge   │
                    │   (in game)   │
                    └───┬───────┬───┘
                        │       │
                ┌───────┘       └────────┐
                ▼                        ▼
         ┌─────────────┐        ┌──────────────┐
         │  Stratalog   │        │  Stratasave   │
         │  (logging)   │        │  (saves)      │
         └─────────────┘        └──────────────┘
```
