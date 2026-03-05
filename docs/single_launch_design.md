# Mission HydroSci — Single Launch Point Design

## Overview

This document describes the design for a single-launch-point experience in Mission HydroSci within StrataHub. Instead of presenting students with 5 individual unit cards that they download and play independently, the student sees one Launch button that takes them to the correct unit based on their progress. Unit completion is detected, progress is stored, and the student is automatically transitioned to the next unit.

The game operates in two modes: **Mode 1 (PWA)** where StrataHub manages everything, and **Mode 2 (URL)** where the game manages its own navigation. Both modes use URL parameters for identity.

---

## Student Experience

### First Visit (PWA)

1. Student opens Mission HydroSci in StrataHub (or launches the installed PWA).
2. The page shows a progress indicator (Unit 1 of 5) and begins downloading Unit 1 automatically.
3. Once Unit 1 is downloaded, a **Launch** button becomes active.
4. While the student waits (or plays Unit 1), Unit 2 downloads in the background.

### Playing

1. Student taps **Launch**.
2. The play page loads with their current unit (e.g., Unit 3).
3. The game loads from cache (offline-capable) and uses save.adroit.games to restore the student's position within the unit.
4. The student plays. If they leave mid-unit (back button, close app), their in-unit progress is saved by the game's save service. Next launch returns them to the same unit.

### Completing a Unit

1. The game reaches end-of-unit and signals completion.
2. StrataHub records the completion (sets current unit to N+1).
3. Unity is fully torn down (Quit + AudioContext cleanup).
4. The play page navigates to the next unit: `/missionhydrosci/play/unit{N+1}?id=johndoe`.
5. The next unit loads fresh. The game's save service initializes the student at the start of the new unit.

### Completing the Game

1. Student completes Unit 5 (the final unit).
2. StrataHub records the completion.
3. Instead of transitioning to a next unit, the page shows: **"You have completed Mission HydroSci."**
4. A button returns the student to the Mission HydroSci page.

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

The play page already intercepts Unity's attempts to navigate to external game URLs (adroit.games, cloudfront.net). Since we know which unit is loaded (StrataHub served the page), any external navigation attempt means the current unit is complete:

- Unit 1 loaded → external navigation intercepted → Unit 1 is complete, transition to Unit 2
- Unit 5 loaded → external navigation intercepted → Unit 5 is complete, show completion message

This works today with no game-side changes. It serves as the interim solution while the dev team implements the real API.

### Real Solution: jslib Callback

The game calls a JavaScript function when a unit is complete. This is explicit, debuggable, and follows the same pattern as Unity jslib plugins.

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
2. If an external navigation is intercepted and `mhsUnitComplete` was NOT called → treat it as completion (interim fallback).
3. If Unit 5 completes → show completion message instead of transitioning.

Once the dev team confirms the jslib callback is in the build, the navigation interception fallback for completion can be removed (the navigation blocker itself stays to prevent accidental external navigation).

---

## Data Model

### Collection: `mhs_user_progress`

Stored in the StrataHub MongoDB database, workspace-scoped.

```javascript
{
  _id: ObjectId,
  workspace_id: ObjectId,       // Workspace the student belongs to
  user_id: ObjectId,            // Student's user ID
  login_id: "johndoe",          // Student's login ID (denormalized for convenience)
  current_unit: "unit1",        // The unit to launch on next play
  completed_units: ["unit1", "unit2"],  // Units that have been completed
  created_at: ISODate,
  updated_at: ISODate
}
```

### Behavior

| Event | Action |
|---|---|
| Student has no record | Create record with `current_unit: "unit1"`, `completed_units: []` |
| Student completes Unit 3 | Set `current_unit: "unit4"`, append `"unit3"` to `completed_units` |
| Student completes Unit 5 (final) | Append `"unit5"` to `completed_units`, set `current_unit: "complete"` |
| Student launches game | Read `current_unit`, navigate to that unit |

### Indexes

```javascript
// Unique per user per workspace
{ workspace_id: 1, user_id: 1 }  // unique: true

// Lookup by login_id (used by the play page API)
{ workspace_id: 1, login_id: 1 }
```

### No Rewind

Progress only moves forward. There is no admin UI or API to set a student backward or forward. Game save data on save.adroit.games is tied to the unit the student has played into. Moving them to a different unit would cause a mismatch between their progress record and their save data.

---

## Download Strategy

### Automatic Download Pipeline

1. **Current unit** is always downloaded first. If not cached, download begins immediately when the page loads.
2. **Next unit** downloads in the background while the student plays the current unit. By the time they finish (hours of gameplay), the next unit is ready.
3. **Completed units** are automatically cleared after confirming the next unit is fully cached. At most 2 units are cached at any time (current + next), keeping storage under ~400 MB.
4. **Storage info** is displayed (existing storage bar) so the student can see available space.

### Teacher-Initiated Pre-Download

Teachers instruct students to open Mission HydroSci before the class session where they'll start playing. The page automatically begins downloading Unit 1 (and Unit 2 in the background). When class starts, units are cached and ready — no download delay.

### Download Flow

```
Page loads
  → Check current unit (from progress record)
  → Is current unit cached?
     → No: Start downloading current unit. Show progress bar. Launch button disabled.
     → Yes: Launch button enabled.
  → Is next unit cached?
     → No: Start downloading next unit in background (lower priority).
     → Yes: Nothing to do.
  → Are there completed units still cached (before current - 1)?
     → Yes: Auto-clear them to free storage.
```

### Manual Override

A collapsible "Storage" section at the bottom of the page shows what's downloaded and lets the student manually clear units if needed. Not prominent, but accessible for troubleshooting.

---

## Unit Transition Flow (Mode 1 — PWA)

```
Student playing Unit 3
  → Game signals completion: window.mhsUnitComplete('unit3')
  → Page JS receives signal
  → POST /missionhydrosci/api/progress/complete { unit: "unit3" }
     → Server validates: unit3 is the student's current unit
     → Server updates: current_unit = "unit4", completed_units += "unit3"
     → Server responds: { next_unit: "unit4", is_final: false }
  → Page runs cleanupUnity() — Quit + close AudioContexts
  → Is Unit 4 cached?
     → Yes: Navigate to /missionhydrosci/play/unit4?id=johndoe
     → No: Show "Downloading next unit..." with progress, then navigate
  → New play page loads Unit 4 fresh

If Unit 5 (final):
  → Server responds: { next_unit: null, is_final: true }
  → Page shows: "You have completed Mission HydroSci."
  → Button: "Back to Mission HydroSci" → /missionhydrosci/units
```

---

## API Endpoints

### GET /missionhydrosci/api/progress

Returns the student's current progress. Used by the units page to determine which unit to show and what to download.

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

**Validation:**
- The unit in the request must match the student's `current_unit`. If a student tries to complete a unit they're not on (stale page, duplicate request), the server rejects it.
- If the unit is already in `completed_units`, the server returns the current state without modifying it (idempotent for retries).

### GET /missionhydrosci/api/manifest

Already exists. Returns the content manifest with unit file lists, sizes, and CDN base URL. No changes needed.

---

## Page Design: Single Launch Point

The Mission HydroSci units page transforms from individual unit cards to a single launch experience.

### Layout

```
┌─────────────────────────────────────────┐
│  Mission HydroSci                       │
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
└─────────────────────────────────────────┘
```

### Elements

- **Unit title**: Shows the current unit's title prominently.
- **Progress dots**: Filled (●) for completed, ring with dot (◉) for current, empty (○) for future. Gives the student a sense of where they are.
- **Status**: "Ready to play", "Downloading Unit 3... 62%", "You have completed Mission HydroSci."
- **Launch button**: Single green button. Disabled while downloading. Hidden after game completion.
- **Next unit progress**: Small text showing background download of the next unit.
- **Storage section**: Collapsible, shows the existing storage bar and per-unit cache status.

### Completed State

```
┌─────────────────────────────────────────┐
│  Mission HydroSci                       │
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

If the student launched with `?id=johndoe&group=Group1&org=MyOrg`, every unit they navigate to receives the same parameters.

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

**No game changes required.** Uses existing navigation interception as interim completion signal.

- Create `mhs_user_progress` MongoDB collection, model, and store
- Create `GET /missionhydrosci/api/progress` endpoint
- Create `POST /missionhydrosci/api/progress/complete` endpoint
- Redesign units page: single Launch button, progress dots, current unit display
- Add `?id=login_id` to play page URL
- Wire completion signal (navigation interception) to progress API
- Wire unit transition (teardown → navigate to next unit)
- Show completion message after Unit 5

### Phase 2: Smart Download Management

**No game changes required.**

- Auto-download current unit on page load
- Background-download next unit while playing
- Auto-clear completed units (keep current + next only)
- Download progress UI for current and next unit
- Handle low-storage gracefully

### Phase 3: Real Completion Signal (jslib)

**Requires game build with MHSBridge.** Dev team implements the spec from the [Game Developer Spec](#game-developer-spec) section.

- Add `window.mhsUnitComplete` handler to play page
- Add `SendMessage('MHSBridge', 'OnPWAReady', '')` call after Unity loads
- Keep navigation interception as fallback during transition
- Remove navigation interception fallback once confirmed working

### Phase 4: Toggleable Name Parameter

- Add `MHSIncludeNameInURL` config flag
- Add `name` parameter to play page URL and resource launch URL when enabled
- Test with survey/assessment apps

---

## Files to Create or Modify

### New Files

| File | Purpose |
|---|---|
| `internal/domain/models/mhs_user_progress.go` | Progress model struct |
| `internal/app/store/mhsuserprogress/store.go` | MongoDB store (CRUD) |
| `internal/app/features/missionhydrosci/progress.go` | API handlers for progress endpoints |

### Modified Files

| File | Change |
|---|---|
| `internal/app/features/missionhydrosci/routes.go` | Add progress API routes |
| `internal/app/features/missionhydrosci/units.go` | Pass progress data to template, add `id` to play URL |
| `internal/app/features/missionhydrosci/templates/missionhydrosci_units.gohtml` | Single launch UI, progress dots, auto-download logic |
| `internal/app/features/missionhydrosci/templates/missionhydrosci_play.gohtml` | Add `mhsUnitComplete` handler, `SendMessage` call, transition logic |
| `internal/app/system/indexes/indexes.go` | Add `mhs_user_progress` indexes |
| `internal/app/bootstrap/routes.go` | Wire progress store dependency |
| `internal/app/bootstrap/appconfig.go` | Add `MHSIncludeNameInURL` config flag |

### Files for Dev Team (Not in StrataHub)

| File | Purpose |
|---|---|
| `Assets/Plugins/WebGL/MHSBridge.jslib` | JavaScript bridge functions |
| `Assets/Scripts/MHSBridge.cs` | C# bridge script |
