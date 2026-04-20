# Logging Game Crashes

## Problem

Unity WebGL games running as WASM sometimes crash on low-resource devices, typically Chromebooks with limited processors or system memory. These crashes are currently silent — no data is captured about what happened, which device was affected, which unit was being played, or which user experienced the crash. This makes it impossible to diagnose patterns or identify problematic hardware configurations.

## Solution

Capture crash information from the browser using JavaScript error handlers and send it to the existing stratalog service for analysis. No changes to the game are required — all crash handlers are browser-level JavaScript that runs on the host page, outside of Unity.

---

## How Browser-Level Crash Detection Works

When Unity WebGL (WASM) crashes, the failure surfaces as a JavaScript error in the browser. The game binary is already dead by the time our handler runs — the browser's WASM runtime translates the crash into a JS exception that we can catch.

### Detection Mechanisms

**1. `createUnityInstance().catch()`**
Fires when Unity fails to initialize — out of memory during WASM compilation, missing build files, corrupted data. Already exists in the host page but currently only shows an error message without reporting.

**2. `window.onerror`**
Standard browser API. Global handler for unhandled JavaScript errors. Catches WASM runtime crashes that surface as JS exceptions. Provides error message, source file, line number, column number, and error object with stack trace.

**3. `window.onunhandledrejection`**
Standard browser API. Catches unhandled promise rejections, including async WASM failures.

**4. Loader script `onerror`**
Fires when the Unity loader script (`unit1.loader.js`) fails to load — network error, 404, CORS issue. Already exists in the host page.

### Limitation

If the browser tab itself hard-crashes (the kernel kills the process due to system-wide out-of-memory), there is no JavaScript opportunity to catch anything. However, most Unity WebGL "crashes" are JavaScript-level aborts that ARE catchable by these handlers.

---

## What Information Is Captured

### Error Information
- **Message** — the error message (e.g., "out of memory", "RuntimeError: unreachable", "abort()")
- **Stack trace** — when available from the error object
- **Error type** — categorized as `load_error`, `runtime_error`, or `unhandled_rejection`
- **Phase** — when the crash occurred: `loading` (Unity initialization), `compiling` (WASM compilation), or `running` (during gameplay)

### Device Information
- **User agent** — identifies browser, OS, and device model (via `navigator.userAgent`)
- **Device memory** — approximate RAM in GB (via `navigator.deviceMemory`, Chrome only)
- **CPU cores** — processor core count (via `navigator.hardwareConcurrency`)
- **JS heap usage** — JavaScript heap size used and total (via `performance.memory`, Chrome only)
- **Screen resolution** — viewport dimensions
- **Online status** — whether the device has network connectivity

### Game Context
- **Player ID** — from `__mhsBridgeConfig.identity.user_id`
- **Unit ID** — which unit was being played (e.g., "unit3")
- **Unit version** — the build version (e.g., "2.2.4")
- **Workspace** — which workspace subdomain (e.g., "mhs")
- **PWA mode** — whether running as an installed PWA or in a browser tab
- **Timestamp** — when the crash occurred

---

## How Crash Data Is Sent

### Endpoint
Crash reports are sent to the existing stratalog `log_submit` endpoint — the same one the game uses for gameplay event logging. Crash reports use `eventType: "crash"` to distinguish them from gameplay events.

### Transport
Uses `navigator.sendBeacon()` for reliability. `sendBeacon` is a browser API specifically designed for sending data as a page is dying — it won't be cancelled by page unload or navigation, unlike `fetch`. If `sendBeacon` is unavailable (very old browsers), falls back to `fetch`.

### Payload Format

The crash report matches the structure of existing game log events in stratalog so they can be queried consistently:

```json
{
  "game": "mhs",
  "playerId": "student@example.com",
  "eventType": "crash",
  "version": "2.2.4",
  "data": {
    "type": "runtime_error",
    "message": "RuntimeError: unreachable",
    "stack": "at wasm-function[12345]:0x1a2b3c\nat ...",
    "phase": "running",
    "unitId": "unit3",
    "workspace": "mhs",
    "isPWA": true
  },
  "device": {
    "os": "Linux x86_64",
    "platform": "UnityWebGL",
    "processors": 2,
    "memory": 4096,
    "resolution": {
      "width": 1366,
      "height": 768
    },
    "userAgent": "Mozilla/5.0 (X11; CrOS x86_64 14541.0.0) AppleWebKit/537.36 ...",
    "online": true,
    "jsHeapUsedMB": 512,
    "jsHeapTotalMB": 1024
  }
}
```

Key fields that match the game's existing log format:
- `game`, `playerId`, `eventType`, `version` — top-level fields, same as gameplay events
- `data` — crash-specific details (replaces the game's action/event data)
- `device` — matches the structure from Unity's SystemInfo (os, platform, processors, memory, resolution)
- Additional browser-only fields: `userAgent`, `online`, `jsHeapUsedMB`, `jsHeapTotalMB`

---

## Implementation

### Where Crash Handlers Are Added

**StrataHub play template** (`missionhydrosci_play.gohtml`):
- Handles crashes for PWA mode and browser-launched games
- Has direct access to the log endpoint URL and auth from server-side template injection
- Game context (unit ID, version, user ID) is available from template variables

**Standalone index.html** (`docs/dev-handoff041526/index.html`):
- Handles crashes for URL-launched developer/tester builds
- Gets log endpoint from `/api/game-config` fetch (or uses localhost defaults)
- On localhost, crashes are logged to console only

### What Is Added

1. **`reportCrash(type, message, stack, phase)` function** — collects device info, builds the payload, sends via `sendBeacon` to the stratalog log_submit endpoint, and logs to console.

2. **`window.onerror` handler** — catches unhandled JS/WASM errors during gameplay and calls `reportCrash`.

3. **`window.onunhandledrejection` handler** — catches unhandled promise rejections and calls `reportCrash`.

4. **Updated `createUnityInstance().catch()`** — the existing error handler is enhanced to also call `reportCrash` in addition to showing the error UI.

5. **Updated loader script `onerror`** — enhanced to also call `reportCrash`.

### No Game Changes Required

All crash handlers run in the browser's JavaScript layer on the host page. The Unity game binary is not modified. The crash has already occurred (in the WASM runtime) before our JavaScript handler runs — we're observing the aftermath, not intercepting the crash inside Unity.

---

## Analyzing Crash Data

Crash reports appear in stratalog alongside gameplay events, filtered by `eventType: "crash"`. Useful queries:

- **Crashes by device model** — group by `device.user_agent` to identify problematic hardware
- **Crashes by unit** — group by `crash.unit_id` to find units that crash more often
- **Crashes by phase** — `loading` vs `running` tells you if it's a resource issue (loading) or a game bug (running)
- **Memory patterns** — correlate `device.device_memory_gb` and `device.js_heap_used_mb` with crash frequency
- **Crash rate by workspace** — compare mhs.adroit.games (production) vs dev.adroit.games (testing)

---

## Files Modified

| File | Change |
|------|--------|
| `internal/app/features/missionhydrosci/templates/missionhydrosci_play.gohtml` | Add crash handlers and reportCrash function |
| `docs/dev-handoff041526/index.html` | Add crash handlers for standalone mode |
