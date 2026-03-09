# Mission HydroSci — Single Launch Point Design

## Overview

This document describes the design for a single-launch-point experience in Mission HydroSci within StrataHub. Instead of presenting users with 5 individual unit cards that they download and play independently, the user sees one Launch button that takes them to the correct unit based on their progress. Unit completion is detected, progress is stored, and the user is automatically transitioned to the next unit.

The implementation targets **Mission HydroSci** (`missionhydrosci`). Mission XydroSci (`missionhydroscix`) remains as a frozen snapshot of the original multi-card experience for comparison and experimentation.

The game operates in two modes: **Mode 1 (PWA)** where StrataHub manages everything, and **Mode 2 (URL)** where the game manages its own navigation. Both modes use URL parameters for identity.

---

## User Experience

All user roles (member, leader, coordinator, admin, superadmin) have the same experience. There is no role-specific variation in the launch UI or gameplay flow.

### First Visit

1. User opens Mission HydroSci in StrataHub (or launches the installed PWA).
2. The page shows a progress indicator (Unit 1 of 5) and begins downloading Unit 1 automatically.
3. Once Unit 1 is downloaded, a **Launch** button becomes active.
4. While the user waits (or plays Unit 1), Unit 2 downloads in the background.

### Playing

1. User taps **Launch**.
2. The play page loads with their current unit (e.g., Unit 3).
3. The game loads from cache (offline-capable) and uses save.adroit.games to restore the user's position within the unit.
4. The user plays. If they leave mid-unit (back button, close app), their in-unit progress is saved by the game's save service. Next launch returns them to the same unit.

### Completing a Unit

1. The game reaches end-of-unit and signals completion.
2. StrataHub records the completion (sets current unit to N+1).
3. Unity is fully torn down (Quit + AudioContext cleanup).
4. The play page navigates to the next unit: `/missionhydrosci/play/unit{N+1}?id=johndoe`.
5. The next unit loads fresh. The game's save service initializes the user at the start of the new unit.

### Completing the Game

1. User completes Unit 5 (the final unit).
2. StrataHub records the completion.
3. Instead of transitioning to a next unit, the page shows: **"You have completed Mission HydroSci."**
4. A button returns the user to the Mission HydroSci page.

---

## Two Operating Modes

The Unity WebGL game supports two operating modes. The mode is communicated to the game at startup.

### Mode 1: PWA (Managed by StrataHub)

- StrataHub tells the game it is running in a PWA via `SendMessage`.
- The game signals end-of-unit via a JavaScript callback (`window.mhsUnitComplete`).
- StrataHub handles unit transitions, progress tracking, identity, and downloads.
- The game does NOT navigate between units — StrataHub does.

### Mode 2: URL (Self-Managed)

- The game is NOT notified that it's in a PWA.
- The game manages its own unit-to-unit navigation using relative URLs.
- A loader checks save data to determine the current unit and navigates to it.
- Identity comes from URL parameters passed through every navigation.
- Progress is tracked only through stratalog (the game's own logging).

### Mode Detection

The game determines its mode by whether `SendMessage` is called after initialization:

| Step | Mode 1 (PWA) | Mode 2 (URL) |
|---|---|---|
| Game loads | `createUnityInstance()` resolves | Game loads from index.html |
| PWA notification | `unityInstance.SendMessage('MHSBridge', 'OnPWAReady', '')` is called | Nothing is called |
| Game behavior | Calls `window.mhsUnitComplete(unitId)` on end-of-unit | Navigates to `../unit{N+1}/index.html` with URL params |
| Identity source | URL parameter `?id=login_id` | URL parameter `?id=login_id` |

---

## Identity

Both modes use URL parameters for identity. This provides a single mechanism that works everywhere: in the PWA, from a StrataHub resource link, and for standalone testing.

### URL Parameters

| Parameter | Always included | Source | Purpose |
|---|---|---|---|
| `id` | Yes | User's login_id | Logging and save state — the only field the game uses |
| `group` | Yes (resource launch) | Group name | Used by survey/assessment apps |
| `org` | Yes (resource launch) | Organization name | Used by survey/assessment apps |
| `name` | Toggleable | User's full name | Future use — disabled if it causes issues with survey apps |

### How Identity Flows

**Mode 1 (PWA):**
- The Launch button URL includes `?id=login_id`: `/missionhydrosci/play/unit3?id=johndoe`
- StrataHub constructs this URL server-side using the authenticated session.
- On unit transition, StrataHub navigates to the next unit with the same parameter.
- The game reads `id` from `window.location.search` on startup.
- The identity bridge (XHR/fetch intercept) remains as a backward-compatible fallback.

**Mode 2 (URL):**
- StrataHub's resource launch injects `?id=johndoe&group=Group1&org=MyOrg` into the URL.
- Every navigation (loader→unit, unit→unit) carries the query string forward:
  ```javascript
  window.location.href = '../unit2/index.html' + window.location.search;
  ```
- For testing without StrataHub, append `?id=tester123` to any URL manually.

### Toggleable Name Parameter

The `name` parameter inclusion is controlled by a config flag in StrataHub's app config:

```go
MHSIncludeNameInURL bool // Whether to include the user's full name in play/resource URLs
```

When enabled, resource launch URLs and PWA play URLs include `&name=John+Doe`. When disabled, the parameter is omitted. This allows quick disabling if survey/assessment apps have issues with unexpected URL parameters.

---

## Unit Completion Signal

### Interim Solution: Navigation Interception

The play page already intercepts Unity's attempts to navigate to external game URLs (adroit.games, cloudfront.net). Since we know which unit is loaded (StrataHub served the page), any external navigation attempt means the current unit is complete. The transition target is always current unit number + 1:

- Unit 1 loaded → external navigation intercepted → Unit 1 is complete, transition to Unit 2
- Unit 5 loaded → external navigation intercepted → Unit 5 is complete, show completion message

This works today with no game-side changes. It serves as the interim solution while the dev team implements the real API. The intercepted URL does not tell us which unit to transition to, but we don't need it — we know the current unit and the next unit is always N+1.

### Real Solution: jslib Callback

The game calls a JavaScript function when a unit is complete. This is explicit, debuggable, and follows the same pattern as Unity jslib plugins. Unlike the navigation interception, the jslib callback provides the explicit unit ID, which serves as confirmation.

**JavaScript side (defined on the play page):**

```javascript
window.mhsUnitComplete = function(unitId) {
  // unitId: string, e.g., "unit3"
  // POST completion to StrataHub API, then transition to next unit
};
```

**Unity C# side (jslib plugin — implemented by dev team):**

See the [Game Developer Spec](#game-developer-spec) section for exact implementation instructions.

### Handling Both Signals

During the transition period, the play page handles both:

1. If `window.mhsUnitComplete(unitId)` is called → use it (real signal).
2. If an external navigation is intercepted and `mhsUnitComplete` was NOT called → treat it as completion (interim fallback), transition to current unit + 1.
3. If Unit 5 completes → show completion message instead of transitioning.

Once the dev team confirms the jslib callback is in the build, the navigation interception fallback for completion can be removed (the navigation blocker itself stays to prevent accidental external navigation).

---

## Data Model

### Collection: `mhs_user_progress`

Stored in the StrataHub MongoDB database, workspace-scoped. Tracks which unit the user should play next and which units they have completed. This data is universal — the same record applies regardless of which device the user is on.

```javascript
{
  _id: ObjectId,
  workspace_id: ObjectId,       // Workspace the user belongs to
  user_id: ObjectId,            // User's ID
  login_id: "johndoe",          // User's login ID (denormalized for convenience)
  current_unit: "unit1",        // The unit to launch on next play
  completed_units: ["unit1", "unit2"],  // Units that have been completed
  created_at: ISODate,
  updated_at: ISODate
}
```

### Behavior

| Event | Action |
|---|---|
| User has no record | Create record with `current_unit: "unit1"`, `completed_units: []` |
| User completes Unit 3 | Set `current_unit: "unit4"`, append `"unit3"` to `completed_units` |
| User completes Unit 5 (final) | Append `"unit5"` to `completed_units`, set `current_unit: "complete"` |
| User launches game | Read `current_unit`, navigate to that unit |

### Indexes

```javascript
// Unique per user per workspace
{ workspace_id: 1, user_id: 1 }  // unique: true

// Lookup by login_id (used by the play page API)
{ workspace_id: 1, login_id: 1 }
```

### No Rewind (Normal Operation)

During normal operation, progress only moves forward. There is no UI to set a user backward to a specific unit. Game save data on save.adroit.games is tied to the unit the user has played into, so moving them to an arbitrary unit would cause a mismatch between their progress record and their save data.

However, a **full reset** (back to Unit 1 with all save data cleared) is supported as an admin action. See [Student Reset](#student-reset).

---

### Collection: `mhs_device_status`

Tracks per-device readiness: which units are cached, whether the PWA is installed, and the device type. This is separate from progress because a user may play on multiple devices (e.g., school Chromebook + home Mac), and download state is device-specific.

```javascript
{
  _id: ObjectId,
  workspace_id: ObjectId,       // Workspace the user belongs to
  user_id: ObjectId,            // User's ID
  login_id: "johndoe",          // Denormalized for dashboard queries
  device_id: "a1b2c3d4",       // Random ID generated on first visit, stored in localStorage
  device_type: "Chromebook",    // Derived from user agent: "Chromebook", "iPad", "macOS", "Windows", "Other"
  pwa_installed: false,         // Whether the PWA is installed on this device
  sw_registered: true,          // Whether the service worker is active
  unit_status: {                // Per-unit cache status on this device
    "unit1": "cached",          // "cached", "downloading", "not_cached"
    "unit2": "downloading",
    "unit3": "not_cached",
    "unit4": "not_cached",
    "unit5": "not_cached"
  },
  storage_quota: 2147483648,    // Origin storage quota in bytes (from navigator.storage.estimate())
  storage_usage: 356515840,     // Origin storage usage in bytes (from navigator.storage.estimate())
  storage_baseline_usage: 12582912, // Usage at first report, before any units were cached (set once)
  last_seen: ISODate,           // Last time this device reported status
  created_at: ISODate,
  updated_at: ISODate
}
```

### Device Identification

A device ID represents a **browser profile's cache instance**, not a physical device. The same Chromebook with two Chrome profiles produces two device IDs with two independent caches. This is correct behavior — each profile has its own service worker cache that needs to be tracked independently.

A random device ID is generated on first visit and stored in `localStorage` as `mhs-device-id`. This ID is included in all status reports to the server. If the user clears localStorage (or browser data), the device ID is lost. On next visit, a new device ID is generated and a new device record is created. The old record becomes stale and ages out via the `last_seen` timestamp — there is no way to link the old record to the new one, and no need to. The teacher dashboard may briefly show two entries for the same student (one stale, one fresh), which is accurate: the old cache instance is gone.

### Device Type Detection

Derived from user agent on the client:

| Detection | Device Type |
|---|---|
| `navigator.userAgent` contains "CrOS" | "Chromebook" |
| iPad (touch + Mac/iPad UA) | "iPad" |
| `navigator.userAgent` contains "Macintosh" (not iPad) | "macOS" |
| `navigator.userAgent` contains "Windows" | "Windows" |
| Everything else | "Other" |

### Storage Tracking

Each device report includes storage information from `navigator.storage.estimate()`:

- **`storage_quota`**: Total storage available to this origin (bytes). Updated on every report.
- **`storage_usage`**: Current storage used by this origin (bytes). Updated on every report.
- **`storage_baseline_usage`**: Storage usage at the time of the first report, before any game units were cached. Set once on record creation, never updated. This captures the "before" state.

The **approximate game cache size** can be derived: `storage_usage - storage_baseline_usage`. This is approximate because other origin storage (e.g., the StrataHub app shell cache) may change between reports, but it's a reasonable indicator of how much space the game units are consuming.

**Important caveat:** `navigator.storage.estimate()` reports the quota for the *browser origin*, not the whole device. On Chromebooks this is typically a large fraction of available disk. On desktop browsers it's often capped at a percentage of total disk. The values are still useful for answering "is there room for more units?" — just not equivalent to total device free space.

### Stale Device Cleanup

Device records that haven't reported in (e.g., 30 days) can be considered stale. The dashboard should deprioritize or hide stale devices. A background cleanup job is not needed initially — the `last_seen` field is sufficient for filtering.

### Indexes

```javascript
// Unique per user per device per workspace
{ workspace_id: 1, user_id: 1, device_id: 1 }  // unique: true

// Dashboard queries: all devices for all users in a workspace
{ workspace_id: 1, last_seen: -1 }
```

---

## Download Strategy

### Service Worker and PWA Installation Are Independent

Two separate concepts:

- **Service worker registration** happens when JS calls `navigator.serviceWorker.register()`. This enables the Cache API for offline storage. It can be triggered from *any* StrataHub page — the SW file is at root level (`/missionhydrosci-sw.js`).
- **PWA installation** (Add to Home Screen / Chrome install) makes the app appear as a standalone app. This is optional and independent of caching.

Downloads can begin as soon as the service worker is registered. PWA installation is not required.

### Early Download Trigger

When the game is assigned to a user, downloads should begin as early as possible — ideally before the user ever visits the Mission HydroSci page.

**How game assignment works:**
- For **members**: the game is assigned via Group > Manage > Apps. StrataHub knows at login which apps are assigned.
- For **non-member roles** (leader, coordinator, admin, superadmin): the game is always considered assigned since it's always in their menu and there is no assignment mechanism for these roles.

**Early download trigger point:** The early download is triggered via the **Login Actions Registry** (see `stratahub/docs/loginactions_design.md`). Mission HydroSci registers a client-side login action that runs after successful authentication. This is architecturally clean — login doesn't know about Mission HydroSci, it just runs registered actions.

**Early download flow:**

```
User logs into StrataHub
  → Login Actions Registry runs registered client-side actions
  → Mission HydroSci login action checks:
     → Is the game assigned to this user? (server provides this in action data)
     → If yes:
        → JS registers the missionhydrosci service worker
        → JS fetches /missionhydrosci/api/progress to get current unit
        → JS sends download message to SW for current unit
        → Unit downloads silently in background
        → JS reports device status to server
```

The Mission HydroSci page also verifies and triggers downloads when visited, as a safety net (e.g., if the login action failed or the user was already logged in when the game was assigned).

This means that when a teacher assigns the game and tells students to log in, Unit 1 starts downloading immediately — even if the student is just looking at their dashboard. By the time the teacher says "open Mission HydroSci," the unit is already cached.

### Automatic Download Pipeline

1. **Current unit** is always downloaded first. If not cached, download begins immediately (either from the login action or when the Mission HydroSci page loads).
2. **Next unit** downloads in the background while the user plays the current unit. By the time they finish (hours of gameplay), the next unit is ready.
3. **Completed units** are automatically cleared after confirming the next unit is fully cached. At most 2 units are cached at any time (current + next), keeping storage under ~400 MB.
4. **Storage info** is displayed (existing storage bar) so the user can see available space.

### Download Flow (Mission HydroSci Page)

```
Page loads
  → Fetch progress from /missionhydrosci/api/progress
  → Check current unit from progress record
  → Is current unit cached?
     → No: Start downloading current unit. Show progress bar. Launch button disabled.
     → Yes: Launch button enabled.
  → Is next unit cached?
     → No: Start downloading next unit in background (lower priority).
     → Yes: Nothing to do.
  → Are there completed units still cached (before current - 1)?
     → Yes: Auto-clear them to free storage.
  → Report device status to server
```

### Manual Override

A collapsible "Storage" section at the bottom of the page shows what's downloaded. The user can manually download future unplayed units that haven't been downloaded yet. This is for situations where the user wants to pre-download several units ahead of time (e.g., before going offline for a trip). Completed units cannot be re-downloaded — they are cleared automatically.

The manual override does NOT show individual unit cards for all 5 units. It shows a compact list of future units with download buttons:

```
▸ Storage
  [████████░░░░░░░░] 340 MB / 2.1 GB

  Unit 3 (current)    ● Downloaded
  Unit 4              ◐ Downloading... 45%
  Unit 5              ○ Not downloaded  [Download]
```

### Status Reporting

The client periodically reports device status to the server. This happens:

1. On page load (Mission HydroSci page or via login action)
2. After a download completes or fails
3. After PWA install/uninstall

The report is a `POST /missionhydrosci/api/device-status` with the device ID, device type, PWA installed flag, SW registered flag, and per-unit cache status.

---

## Multi-Device Considerations

A user may play on multiple devices (school Chromebook, home Mac, home Windows PC, iPad). This creates an important distinction:

### Server-Side (Universal)

**Game progress** (`mhs_user_progress`) is universal. Regardless of which device the user plays on:
- The current unit is the same everywhere
- Completed units are the same everywhere
- The game's save service (save.adroit.games) handles in-unit position

When a user completes Unit 2 on their Chromebook, their home Mac will show Unit 3 as current on next visit.

### Client-Side (Per-Device)

**Download status** (`mhs_device_status`) is per-device (per-cache-instance). Each browser profile has its own service worker cache:
- The Chromebook may have Units 3 and 4 cached
- The home Mac may have only Unit 3 cached (or nothing)
- Each device manages its own downloads independently

When the user opens Mission HydroSci on any device:
1. Fetch progress → learn current unit is Unit 3
2. Check local cache → is Unit 3 cached on *this* device?
3. If not, download it. If yes, enable Launch.

The login action trigger helps here: if the user logs into their home Mac, the download starts automatically even before they navigate to Mission HydroSci.

---

## Teacher Dashboard Integration

The MHS Dashboard (`/mhsdashboard`) shows teachers the readiness of their students' devices.

### What Teachers See

For each student in their group(s):

| Student | Device | PWA | Unit 1 | Unit 2 | Unit 3 | Unit 4 | Unit 5 | Storage | Progress |
|---------|--------|-----|--------|--------|--------|--------|--------|---------|----------|
| John D. | Chromebook | Yes | ● | ● | ● | ◐ | ○ | 340 MB / 2.0 GB (17%) | Unit 3 |
| John D. | macOS | No | ● | ○ | ○ | ○ | ○ | 195 MB / 4.1 GB (5%) | Unit 3 |
| Jane S. | iPad | Yes | ● | ● | ○ | ○ | ○ | 3.8 GB / 4.1 GB (93%) | Unit 2 |
| Bob T. | — | — | — | — | — | — | — | — | Not started |

Legend: ● cached, ◐ downloading, ○ not cached
Storage shows origin usage / quota. High usage (e.g., Jane S. at 93%) indicates the device may not have room for additional units.

Students who have never logged in or never triggered the early download show as "Not started" with no device info.

### Data Sources

- **Progress** column: from `mhs_user_progress` (server-side, universal)
- **Device/PWA/Unit columns**: from `mhs_device_status` (client-reported, per-device)
- **Student list**: from group membership (existing feature)

### Non-Member Roles on Dashboard

Leaders, coordinators, and admins who use Mission HydroSci will have their own progress and device records. They could see their own readiness in a personal dashboard section, useful for testing. This is lower priority and not part of the initial implementation.

---

## Offline Progress Queue

When the user completes a unit while offline (or the server is unreachable), the `POST /missionhydrosci/api/progress/complete` call will fail. The client must handle this gracefully.

### Design

1. When `mhsUnitComplete` fires (or navigation interception detects completion), attempt the POST immediately.
2. If the POST fails (network error, server error), store the completion event in `localStorage`:
   ```javascript
   // Key: mhs-progress-queue
   // Value: JSON array of pending completions
   [{ "unit": "unit3", "timestamp": "2026-03-05T14:30:00Z" }]
   ```
3. On the next page load (or when connectivity returns), check the queue and replay pending completions.
4. The server's idempotency guarantee (completing an already-completed unit is a no-op) means replaying is safe even if the original POST actually succeeded but the client didn't receive the response.

### Unit Transition While Offline

Even if the progress POST fails, the client can still transition to the next unit locally:
1. The client knows the current unit (StrataHub served the page for that unit).
2. The next unit is always current unit number + 1.
3. If the next unit is cached, navigate to it immediately.
4. Store the completion in the queue for later sync.
5. On next online page load, the queue replays and the server catches up.

The risk is minimal: the only scenario where this causes a problem is if the user completes a unit offline on Device A, then immediately switches to Device B before Device A syncs. Device B would show the old current unit. This resolves itself once Device A comes online and syncs.

---

## Completion Endpoint Idempotency

The `POST /missionhydrosci/api/progress/complete` endpoint is designed to be safe for retries and stale requests:

| Request | Server State | Action |
|---|---|---|
| Complete unit3 | current_unit = "unit3" | Advance to unit4, return `{ next_unit: "unit4", is_final: false }` |
| Complete unit3 (retry) | current_unit = "unit4" (already advanced) | No-op, return current state `{ next_unit: "unit4", is_final: false }` |
| Complete unit2 (stale) | current_unit = "unit4" | No-op, return current state `{ next_unit: "unit4", is_final: false }` |
| Complete unit5 | current_unit = "unit5" | Advance to complete, return `{ next_unit: null, is_final: true }` |

Any request for a unit that is already in `completed_units` or is before the current unit returns success with the current state. This makes the endpoint safe for the offline progress queue, stale pages, and duplicate requests.

---

## Unit Transition Flow (Mode 1 — PWA)

```
User playing Unit 3
  → Completion signal fires:
     → Either: window.mhsUnitComplete('unit3') (jslib callback)
     → Or: external navigation intercepted (interim — implies current unit + 1)
  → Page JS receives signal
  → POST /missionhydrosci/api/progress/complete { unit: "unit3" }
     → Success:
        → Server advances: current_unit = "unit4", completed_units += "unit3"
        → Server responds: { next_unit: "unit4", is_final: false }
     → Failure (offline/error):
        → Store { unit: "unit3" } in localStorage progress queue
        → Derive next unit: current unit number + 1
  → Page runs cleanupUnity() — Quit + close AudioContexts
  → Is Unit 4 cached?
     → Yes: Navigate to /missionhydrosci/play/unit4?id=johndoe
     → No: Show "Downloading next unit..." with progress, then navigate
  → New play page loads Unit 4 fresh
  → Report updated device status to server

If Unit 5 (final):
  → Server responds: { next_unit: null, is_final: true }
  → Page shows: "You have completed Mission HydroSci."
  → Button: "Back to Mission HydroSci" → /missionhydrosci/units
```

---

## API Endpoints

### GET /missionhydrosci/api/progress

Returns the user's current progress. Used by the units page to determine which unit to show and what to download.

**Response:**
```json
{
  "current_unit": "unit3",
  "completed_units": ["unit1", "unit2"],
  "is_complete": false
}
```

If no progress record exists, returns `current_unit: "unit1"` with empty `completed_units`.

### POST /missionhydrosci/api/progress/complete

Records a unit completion and returns the next unit. Called by the play page when the game signals end-of-unit.

**Request:**
```json
{
  "unit": "unit3"
}
```

**Response:**
```json
{
  "next_unit": "unit4",
  "is_final": false
}
```

**Idempotency:** If the unit is already completed or is before the current unit, the server returns success with the current state (no modification). This makes retries and stale requests safe. See [Completion Endpoint Idempotency](#completion-endpoint-idempotency) for the full behavior matrix.

### POST /missionhydrosci/api/device-status

Reports the current device's readiness state. Called by the client on page load and after download events.

**Request:**
```json
{
  "device_id": "a1b2c3d4",
  "device_type": "Chromebook",
  "pwa_installed": false,
  "sw_registered": true,
  "unit_status": {
    "unit1": "cached",
    "unit2": "cached",
    "unit3": "downloading",
    "unit4": "not_cached",
    "unit5": "not_cached"
  },
  "storage_quota": 2147483648,
  "storage_usage": 356515840
}
```

**Response:** `204 No Content`

This is an upsert — creates the device record if it doesn't exist, updates it if it does. The server sets `last_seen` to the current time on every report. On the first report (record creation), the server also sets `storage_baseline_usage` to the reported `storage_usage` value to capture the pre-download state.

### GET /missionhydrosci/api/manifest

Already exists. Returns the content manifest with unit file lists, sizes, and CDN base URL. No changes needed.

---

## Page Design: Single Launch Point

The Mission HydroSci units page transforms from individual unit cards to a single launch experience.

### Layout

```
┌─────────────────────────────────────────┐
│  Mission HydroSci                    ?  │
│                                         │
│  ┌─────────────────────────────────┐    │
│  │  Unit 3: [Unit Title]           │    │
│  │                                 │    │
│  │  ● Unit 1  ● Unit 2  ◉ Unit 3  │    │
│  │  ○ Unit 4  ○ Unit 5            │    │
│  │                                 │    │
│  │  Ready to play                  │    │
│  │                                 │    │
│  │  ┌───────────────────────┐     │    │
│  │  │       Launch          │     │    │
│  │  └───────────────────────┘     │    │
│  │                                 │    │
│  │  Next unit downloading... 45%   │    │
│  └─────────────────────────────────┘    │
│                                         │
│  ▸ Storage                              │
│    [████████░░░░░░░░] 340 MB / 2.1 GB   │
│    Unit 3 (current)  ● Downloaded       │
│    Unit 4            ◐ 45%              │
│    Unit 5            ○ [Download]       │
└─────────────────────────────────────────┘
```

### Elements

- **Title + help icon**: "Mission HydroSci" with `?` circle for platform-specific install instructions.
- **Unit title**: Shows the current unit's title prominently.
- **Progress dots**: Filled (●) for completed, ring with dot (◉) for current, empty (○) for future. Gives the user a sense of where they are.
- **Status**: "Ready to play", "Downloading Unit 3... 62%", "You have completed Mission HydroSci."
- **Launch button**: Single green button. Disabled while downloading. Hidden after game completion.
- **Next unit progress**: Small text showing background download of the next unit.
- **Storage section**: Collapsible, shows storage bar and per-unit cache status for future unplayed units. Users can manually download future units. Completed units are not shown (auto-cleared).

### Completed State

```
┌─────────────────────────────────────────┐
│  Mission HydroSci                    ?  │
│                                         │
│  ┌─────────────────────────────────┐    │
│  │                                 │    │
│  │  ● Unit 1  ● Unit 2  ● Unit 3  │    │
│  │  ● Unit 4  ● Unit 5            │    │
│  │                                 │    │
│  │  You have completed             │    │
│  │  Mission HydroSci.             │    │
│  │                                 │    │
│  └─────────────────────────────────┘    │
└─────────────────────────────────────────┘
```

---

## Game Developer Spec

This section contains exact instructions for the Unity development team. It describes what to implement, how to name things, and includes complete code.

### 1. Create the jslib Plugin

Create a file `Assets/Plugins/WebGL/MHSBridge.jslib` with the following content:

```javascript
mergeInto(LibraryManager.library, {

  // Called by C# to notify the PWA that a unit has been completed.
  // unitIdPtr: pointer to a C string containing the unit ID (e.g., "unit3")
  MHSBridge_NotifyUnitComplete: function(unitIdPtr) {
    var unitId = UTF8ToString(unitIdPtr);
    if (typeof window.mhsUnitComplete === 'function') {
      window.mhsUnitComplete(unitId);
    }
  },

  // Called by C# to get the player's login ID from the URL parameters.
  // Returns a pointer to a _malloc'd UTF-8 C string (null-terminated).
  // Caller MUST free the returned pointer via MHSBridge_Free().
  MHSBridge_GetPlayerID: function() {
    var params = new URLSearchParams(window.location.search);
    var id = params.get('id') || '';
    var bufferSize = lengthBytesUTF8(id) + 1;
    var buffer = _malloc(bufferSize);
    stringToUTF8(id, buffer, bufferSize);
    return buffer;
  },

  // Called by C# to free any _malloc'd pointer returned by this plugin.
  MHSBridge_Free: function(ptr) {
    if (ptr) {
      _free(ptr);
    }
  },

  // Called by C# to navigate to a unit using a relative URL.
  // Resolves the relative URL against the current page and carries all
  // URL parameters (?id=, ?group=, etc.) forward to the destination.
  MHSBridge_NavigateToUnit: function(nextUnitPtr) {
    var nextUnit = UTF8ToString(nextUnitPtr);
    // Resolve relative URL against current page, then merge current params
    var url = new URL(nextUnit, window.location.href);
    var currentParams = new URLSearchParams(window.location.search);
    currentParams.forEach(function(value, key) {
      url.searchParams.set(key, value);
    });
    window.location.href = url.href;
  }

});
```

### 2. Create the C# Bridge Script

Create a file `Assets/Scripts/MHSBridge.cs`:

```csharp
using System;
using System.Runtime.InteropServices;
using UnityEngine;

/// <summary>
/// Bridge between the Unity game and the StrataHub PWA host page.
/// Attach this script to a GameObject named "MHSBridge" in the first scene that loads.
/// </summary>
public class MHSBridge : MonoBehaviour
{
    // --- JS function imports (from MHSBridge.jslib) ---

    [DllImport("__Internal")]
    private static extern void MHSBridge_NotifyUnitComplete(string unitId);

    [DllImport("__Internal")]
    private static extern IntPtr MHSBridge_GetPlayerID();

    [DllImport("__Internal")]
    private static extern void MHSBridge_Free(IntPtr ptr);

    [DllImport("__Internal")]
    private static extern void MHSBridge_NavigateToUnit(string url);

    // --- State ---

    private bool _isPWA = false;

    /// <summary>True if the game is running inside the StrataHub PWA.</summary>
    public bool IsPWA => _isPWA;

    // --- Singleton ---

    public static MHSBridge Instance { get; private set; }

    private void Awake()
    {
        if (Instance != null && Instance != this) { Destroy(gameObject); return; }
        Instance = this;
        DontDestroyOnLoad(gameObject);
    }

    // --- Called by the PWA host page via SendMessage ---

    /// <summary>
    /// Called by the PWA host page after Unity finishes loading.
    /// Signals that the game is running inside the managed PWA environment.
    /// The game should NOT navigate between units when in PWA mode.
    /// </summary>
    public void OnPWAReady(string unused)
    {
        _isPWA = true;
        Debug.Log("MHSBridge: PWA mode activated");
    }

    // --- Called by game code ---

    /// <summary>
    /// Call this when the current unit is complete.
    /// In PWA mode: notifies the host page, which handles the transition.
    /// In URL mode: navigates to the next unit using a relative URL.
    ///              Passing empty or null for nextUnitRelativeUrl is a no-op
    ///              (used for Unit 5, the final unit).
    /// </summary>
    /// <param name="currentUnitId">The unit that was just completed, e.g., "unit3"</param>
    /// <param name="nextUnitRelativeUrl">
    /// Relative URL to the next unit, e.g., "../unit4/index.html".
    /// Ignored in PWA mode. Pass null or empty for the final unit in URL mode.
    /// </param>
    public void CompleteUnit(string currentUnitId, string nextUnitRelativeUrl)
    {
#if UNITY_WEBGL && !UNITY_EDITOR
        if (_isPWA)
        {
            // PWA mode: notify the host page. It handles the transition.
            MHSBridge_NotifyUnitComplete(currentUnitId);
        }
        else
        {
            // URL mode: navigate to the next unit directly.
            if (!string.IsNullOrEmpty(nextUnitRelativeUrl))
            {
                MHSBridge_NavigateToUnit(nextUnitRelativeUrl);
            }
        }
#else
        Debug.Log($"MHSBridge: CompleteUnit(\"{currentUnitId}\") ignored in Editor");
#endif
    }

    /// <summary>
    /// Returns the player's login ID from the URL parameter ?id=value.
    /// Works in both PWA and URL modes. No network call needed.
    /// Use this instead of calling /api/user.
    /// </summary>
    public string GetPlayerID()
    {
#if UNITY_WEBGL && !UNITY_EDITOR
        IntPtr ptr = IntPtr.Zero;
        try
        {
            ptr = MHSBridge_GetPlayerID();
            if (ptr == IntPtr.Zero)
                return string.Empty;
            return PtrToStringUTF8(ptr);
        }
        finally
        {
            if (ptr != IntPtr.Zero)
                MHSBridge_Free(ptr);
        }
#else
        return "editor-test-user";
#endif
    }

    /// <summary>
    /// Navigates to a unit by name (e.g., "unit1", "unit3").
    /// Used by the loader to send the student to their current unit.
    /// URL parameters (?id=...) are preserved automatically.
    /// No-ops in PWA mode — StrataHub handles navigation directly.
    /// </summary>
    public void NavigateToUnit(string unitName)
    {
#if UNITY_WEBGL && !UNITY_EDITOR
        if (_isPWA)
        {
            Debug.LogWarning("MHSBridge: NavigateToUnit ignored in PWA mode");
            return;
        }
        if (string.IsNullOrEmpty(unitName))
        {
            Debug.LogError("MHSBridge: NavigateToUnit called with null or empty unitName");
            return;
        }
        MHSBridge_NavigateToUnit("../" + unitName + "/index.html");
#endif
    }

    // --- Helpers ---

    private static string PtrToStringUTF8(IntPtr ptr)
    {
        if (ptr == IntPtr.Zero) return string.Empty;
        int len = 0;
        while (Marshal.ReadByte(ptr, len) != 0) len++;
        if (len == 0) return string.Empty;
        byte[] bytes = new byte[len];
        Marshal.Copy(ptr, bytes, 0, len);
        return System.Text.Encoding.UTF8.GetString(bytes);
    }
}
```

### 3. GameObject Setup

1. Create an empty GameObject in the first scene that loads (the loader scene or Unit 1 scene).
2. Name it exactly: **`MHSBridge`**
3. Attach the `MHSBridge.cs` script to it.
4. The script uses `DontDestroyOnLoad` so it persists across scene changes.

### 4. Usage in Game Code

**Getting the player identity:**
```csharp
string playerId = MHSBridge.Instance.GetPlayerID();
// Use playerId for logging and save state instead of calling /api/user
```

**Signaling unit completion:**
```csharp
// At the point where the unit ends (where you currently navigate to the next unit):
MHSBridge.Instance.CompleteUnit("unit3", "../unit4/index.html");
// In PWA mode: this notifies the host page. Do NOT navigate.
// In URL mode: this navigates to the next unit with identity params preserved.
```

**Navigating to a unit (loader — URL mode only):**
```csharp
string playerId = MHSBridge.Instance.GetPlayerID();
string currentUnit = DetermineCurrentUnit(playerId); // e.g., "unit1" or "unit3"
MHSBridge.Instance.NavigateToUnit(currentUnit);       // navigates to ../unit3/index.html?id=johndoe
```

**Checking mode (if needed):**
```csharp
if (MHSBridge.Instance.IsPWA)
{
    // Running in PWA — don't show "next unit" loading screen, the host page handles it
}
```

### 5. What the PWA Host Page Does

After Unity finishes loading, the PWA play page calls:

```javascript
unityInstance.SendMessage('MHSBridge', 'OnPWAReady', '');
```

This sets `_isPWA = true` in the game. From that point, the game knows to call `MHSBridge_NotifyUnitComplete` instead of navigating.

The play page defines:

```javascript
window.mhsUnitComplete = function(unitId) {
  // POST to /missionhydrosci/api/progress/complete
  // On success: teardown Unity, navigate to next unit (or show completion)
  // On failure: store in localStorage queue, transition locally
};
```

---

## Relative URL Guide for Mode 2

### S3 Directory Structure

All units for a build version live under a common parent folder:

```
test.adroit.games/
  builds/
    test030426-webtest01/
      loader/
        index.html          ← Entry point (resource URL points here)
        Build/
        StreamingAssets/
      unit1/
        index.html
        Build/
        StreamingAssets/
      unit2/
        index.html
        Build/
        StreamingAssets/
      unit3/
        index.html
        Build/
        StreamingAssets/
      unit4/
        index.html
        Build/
        StreamingAssets/
      unit5/
        index.html
        Build/
        StreamingAssets/
```

### Relative URL Navigation

From the loader at `test030426-webtest01/loader/index.html`:
```
../unit1/index.html  → test030426-webtest01/unit1/index.html
../unit3/index.html  → test030426-webtest01/unit3/index.html
```

From Unit 1 at `test030426-webtest01/unit1/index.html`:
```
../unit2/index.html  → test030426-webtest01/unit2/index.html
```

From Unit 4 at `test030426-webtest01/unit4/index.html`:
```
../unit5/index.html  → test030426-webtest01/unit5/index.html
```

### Identity Preservation

Every navigation carries the query string:

```javascript
// In MHSBridge.jslib — MHSBridge_NavigateToUnit already does this:
window.location.href = '../unit2/index.html' + window.location.search;
```

If the user launched with `?id=johndoe&group=Group1&org=MyOrg`, every unit they navigate to receives the same parameters.

### Key Rules for the Dev Team

1. **All units must be sibling folders** under a common parent directory.
2. **Each unit folder contains** `index.html`, `Build/`, and `StreamingAssets/`.
3. **The loader is also a sibling folder** (not a parent of the unit folders).
4. **Use `MHSBridge.Instance.CompleteUnit()` for end-of-unit transitions and `MHSBridge.Instance.NavigateToUnit()` for the loader** instead of hardcoded fully-qualified URLs. These methods handle identity preservation automatically.
5. **The same build works on any host** — `cdn.adroit.games`, `test.adroit.games`, or `localhost` — because all URLs are relative.

---

## Resource URL Configuration

For Mode 2 access through StrataHub resource links, the resource URL points to the **loader**:

```
https://test.adroit.games/builds/test030426-webtest01/loader/index.html
```

StrataHub's resource launch automatically appends identity parameters:

```
https://test.adroit.games/builds/test030426-webtest01/loader/index.html?id=johndoe&group=Group1&org=MyOrg
```

The loader reads save data to determine the current unit, then navigates:

```javascript
// Loader determines student is on Unit 3:
window.location.href = '../unit3/index.html' + window.location.search;
// Result: https://test.adroit.games/builds/test030426-webtest01/unit3/index.html?id=johndoe&group=Group1&org=MyOrg
```

---

## Implementation Phases

### Phase 1: Progress Storage and Single Launch UI

**No game changes required.** Uses existing navigation interception as interim completion signal. Transition target is always current unit + 1.

- Create `mhs_user_progress` MongoDB collection, model, and store
- Create `GET /missionhydrosci/api/progress` endpoint
- Create `POST /missionhydrosci/api/progress/complete` endpoint (with idempotency)
- Redesign units page: single Launch button, progress dots, current unit display
- Add `?id=login_id` to play page URL
- Wire completion signal (navigation interception) to progress API
- Wire unit transition (teardown → navigate to next unit)
- Show completion message after Unit 5
- Add offline progress queue (localStorage) with replay on reconnect

### Phase 2: Smart Download Management

**No game changes required.**

- Auto-download current unit on page load
- Background-download next unit while playing
- Auto-clear completed units (keep current + next only)
- Download progress UI for current and next unit
- Manual override: download future unplayed units
- Handle low-storage gracefully

### Phase 3: Early Download and Device Tracking

**No game changes required.** Depends on the Login Actions Registry (see `stratahub/docs/loginactions_design.md`).

- Create `mhs_device_status` MongoDB collection, model, and store
- Create `POST /missionhydrosci/api/device-status` endpoint
- Generate device ID (localStorage) and detect device type
- Implement Login Actions Registry (`internal/app/loginactions/`)
- Register Mission HydroSci login action for early download
- Also verify and trigger downloads on Mission HydroSci page load (safety net)
- Report device status after downloads and on page loads
- Detect and report PWA installation status
- Add device readiness columns to MHS Dashboard

### Phase 4: Real Completion Signal (jslib)

**Requires game build with MHSBridge.** Dev team implements the spec from the [Game Developer Spec](#game-developer-spec) section.

- Add `window.mhsUnitComplete` handler to play page
- Add `SendMessage('MHSBridge', 'OnPWAReady', '')` call after Unity loads
- Keep navigation interception as fallback during transition
- Remove navigation interception fallback once confirmed working

### Phase 5: Toggleable Name Parameter

- Add `MHSIncludeNameInURL` config flag
- Add `name` parameter to play page URL and resource launch URL when enabled
- Test with survey/assessment apps

### Phase 6: Student Reset

- Add reset API and UI on teacher dashboard (see [Student Reset](#student-reset))
- Integrate with stratasave database for save data deletion
- Test full reset flow end-to-end

---

## Student Reset

A "nuclear option" for when a student's game state is corrupted or they need to start completely over. Available to admins, coordinators, and leaders.

### What It Does

A full reset for one student clears **all** game state across all devices and services:

| Data | Action | Location |
|---|---|---|
| `mhs_user_progress` | Reset `current_unit` to `"unit1"`, clear `completed_units` | StrataHub MongoDB |
| `mhs_device_status` | Delete all device records for this user | StrataHub MongoDB |
| Save data | Delete this student's Mission HydroSci save records | stratasave database |
| Device caches | Cannot be cleared remotely — handled on next visit (see below) | Browser Cache API |

### Device Cache After Reset

We cannot remotely clear a browser's Cache API. After a reset:

1. The student's `mhs_device_status` records are deleted.
2. On next visit to any StrataHub page (or via login action), the client:
   - Fetches progress → sees `current_unit: "unit1"`
   - Generates a new device status report
   - The auto-download pipeline downloads Unit 1 (likely already cached from before)
   - The auto-clear pipeline removes cached units beyond current + next
3. The old cached units don't cause gameplay problems — save data has been deleted, so the game starts fresh regardless of which unit files are in cache.

### Where It Lives

On the teacher dashboard, next to each student row. A "Reset" button (or icon) that opens a confirmation dialog:

> **Reset John D.?**
>
> This will reset their Mission HydroSci progress to Unit 1 and delete all saved game data. This cannot be undone.
>
> [Cancel] [Reset]

### API

`POST /missionhydrosci/api/admin/reset-student`

**Request:**
```json
{
  "user_id": "60f7b2c3..."
}
```

**Response:**
```json
{
  "progress_reset": true,
  "devices_deleted": 2,
  "saves_deleted": 5
}
```

**Authorization:** Requires admin, coordinator, or leader role. Leaders can only reset students in their own groups.

### Design Considerations

- **Not a partial rewind.** This is all-or-nothing back to Unit 1. There is no "move to Unit 3" because save data is per-unit and selectively clearing it is fragile.
- **Single student.** No bulk reset initially. If needed later, it can be added as a batch operation using the same underlying API.
- **Audit logged.** The reset action is recorded in the audit log with who performed it, which student was reset, and what was deleted.
- **stratasave integration.** Save records are keyed by game identifier `"mhs"` and the student's login ID. The stratasave service (`/Users/dale/Documents/catchupstratahub/stratasave/`) manages these records. During implementation, review the stratasave data model to determine the exact deletion query — the combination of game `"mhs"` + login ID should identify all records to clear.

---

## Files to Create or Modify

### New Files

| File | Purpose |
|---|---|
| `internal/app/loginactions/registry.go` | Login Actions Registry (see loginactions_design.md) |
| `internal/domain/models/mhs_user_progress.go` | Progress model struct |
| `internal/app/store/mhsuserprogress/store.go` | MongoDB store (CRUD for progress) |
| `internal/domain/models/mhs_device_status.go` | Device status model struct |
| `internal/app/store/mhsdevicestatus/store.go` | MongoDB store (upsert for device status) |
| `internal/app/features/missionhydrosci/progress.go` | API handlers for progress endpoints |
| `internal/app/features/missionhydrosci/device_status.go` | API handler for device status endpoint |
| `internal/app/features/missionhydrosci/admin_reset.go` | API handler for student reset (Phase 6) |

### Modified Files

| File | Change |
|---|---|
| `internal/app/features/missionhydrosci/routes.go` | Add progress and device-status API routes |
| `internal/app/features/missionhydrosci/units.go` | Pass progress data to template, add `id` to play URL |
| `internal/app/features/missionhydrosci/handler.go` | Add progress store and device status store dependencies |
| `internal/app/features/missionhydrosci/templates/missionhydrosci_units.gohtml` | Single launch UI, progress dots, auto-download logic, manual override |
| `internal/app/features/missionhydrosci/templates/missionhydrosci_play.gohtml` | Add `mhsUnitComplete` handler, `SendMessage` call, transition logic, offline queue |
| `internal/app/features/mhsdashboard/` | Add device readiness columns (Phase 3) |
| `internal/app/system/indexes/indexes.go` | Add `mhs_user_progress` and `mhs_device_status` indexes |
| `internal/app/bootstrap/routes.go` | Wire login actions registry, progress store, device status store |
| `internal/app/bootstrap/appconfig.go` | Add `MHSIncludeNameInURL` config flag |
| `internal/app/features/login/handler.go` | Call login actions registry after session creation |

### Files for Dev Team (Not in StrataHub)

| File | Purpose |
|---|---|
| `Assets/Plugins/WebGL/MHSBridge.jslib` | JavaScript bridge functions |
| `Assets/Scripts/MHSBridge.cs` | C# bridge script |
