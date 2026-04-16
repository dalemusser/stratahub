# MHSBridge Change Notice — April 15, 2026

## Backward Compatibility

StrataHub supports both old and new versions of MHSBridge simultaneously. Game builds using the previous MHSBridge (before this update) continue to work with the same behavior and end-of-game experience — when the final unit completes, the game shows its end-of-game screen and the player uses the back button in the upper left corner to return to StrataHub. Game builds using the new MHSBridge with `EndGame()` get the improved flow where the Continue button on the end-of-game screen takes the player back to StrataHub automatically. No rebuilds of existing game versions are required.

## Summary

Added `EndGame()` to MHSBridge for signaling when the player has finished the game and is ready to exit. This replaces StrataHub's end-of-game overlay — the game now owns the entire end-of-game experience.

## What Changed

### New: `MHSBridge.EndGame()`

A new method that should be called when the player clicks the Continue/Return button on the game's end-of-game screen (after the final unit).

**What it does:**
- **PWA mode (StrataHub):** Navigates back to the Mission HydroSci units page
- **Browser tab:** Attempts to close the tab. If the browser blocks it, shows "You can close this tab now"
- **Unity Editor:** Logs a message (no-op)

**C# usage:**
```csharp
// On the end-of-game screen, when the player clicks Continue:
MHSBridge.Instance.EndGame();
```

### Updated Files

Replace these files in your Unity project:
- `MHSBridge.cs` — Added `EndGame()` method and `MHSBridge_EndGame` DllImport
- `MHSBridge.jslib` — Added `MHSBridge_EndGame` JavaScript function

### No Changes Needed

- `index.html` — Unchanged from the previous handoff
- `CompleteUnit()` — Still works exactly the same. Continue calling it for every unit completion.

## How to Use

### End of Game Flow

The game's end-of-game screen (shown after the final unit) should:

1. Display the congratulatory message and any end-of-game content the designers want
2. Have a button (e.g., "Continue" or "Return to StrataHub")
3. When that button is clicked, call `MHSBridge.Instance.EndGame()`

That's it. MHSBridge handles the rest based on the hosting environment.

### Unit Completion Flow (unchanged)

Continue calling `CompleteUnit()` for every unit, including the last one:

```csharp
// After unit gameplay is done (before showing end-of-game screen for final unit):
MHSBridge.Instance.CompleteUnit("unit5", "../unit6/index.html");
```

StrataHub handles determining what happens next — if there's a next unit, it transitions to it. The `nextUnitRelativeUrl` parameter is only used in URL mode and is ignored in PWA mode.

### Important: CompleteUnit vs EndGame

- `CompleteUnit("unit5", ...)` = "Unit 5's gameplay is done" → StrataHub advances progress and transitions to the next unit
- `EndGame()` = "The player is done and wants to leave" → exits back to StrataHub or closes the tab

Call `CompleteUnit` first (when gameplay ends), then show the end-of-game screen, then call `EndGame` when the player clicks Continue.

### End-of-Game Screen Text

The game's end-of-game screen can say whatever the designers want. Suggested text for the button:
- "Return to Mission HydroSci" (if running in StrataHub)
- "Continue" (generic, works everywhere)

The game can check `MHSBridge.Instance.IsPWA` to customize the button text if desired, but it's not required.

## When Unit 6 Is Added

When unit 6 is ready, no changes are needed to MHSBridge or StrataHub. The game developers should:

1. **Unit 5 → Unit 6 transition:** Handle it the same way as unit 4 → unit 5. Call `CompleteUnit("unit5", "../unit6/index.html")` at the end of unit 5's gameplay. StrataHub will transition the player to unit 6 automatically.

2. **End of game:** Move the end-of-game screen to the end of unit 6 (or wherever the game ends). When the player clicks Continue on that screen, call `MHSBridge.Instance.EndGame()` — same as before.

The key principle: `CompleteUnit` is called at the end of every unit to transition to the next one. `EndGame` is called once, wherever the game ends, from the end-of-game screen. If the game grows to 7, 8, or more units, the same pattern applies — `CompleteUnit` for transitions, `EndGame` at the very end.

## Testing

1. **PWA mode:** After clicking Continue on the end-of-game screen, the player should be taken back to the Mission HydroSci units page in StrataHub with all units showing as completed
2. **URL mode:** After clicking Continue, the tab should close (or show "You can close this tab now" if the browser blocks it)
3. **Editor:** `EndGame()` logs "MHSBridge: EndGame() ignored in Editor" — no action needed

---

## Hosting Scenarios

Mission HydroSci runs in three different hosting environments. MHSBridge handles the differences so the game code doesn't have to.

### Scenario 1: PWA (launched from installed app icon)

The game is installed as a Progressive Web App on the student's device (e.g., a Chromebook). The student taps the Mission HydroSci icon and the app opens in its own window managed by StrataHub.

- **Identity:** Injected by StrataHub via `__mhsBridgeConfig` before Unity starts
- **Service endpoints:** Injected by StrataHub via `__mhsBridgeConfig`
- **Unit transitions:** StrataHub handles everything. `CompleteUnit` signals completion, StrataHub advances progress, downloads the next unit if needed, and navigates to it — all within the same app window.
- **Back button:** A back button in the upper left corner of the game page returns the player to the Mission HydroSci units page in StrataHub.
- **End of game:** `EndGame()` navigates back to the Mission HydroSci units page, which shows all units as completed.
- **MHSBridge.IsPWA:** `true`

### Scenario 2: Browser tab (launched from Mission HydroSci in a browser)

The game is played in a browser tab after the student logs into StrataHub and navigates to Mission HydroSci. The game loads in the same tab or a new tab.

- **Identity:** Injected by StrataHub via `__mhsBridgeConfig` (same as PWA)
- **Service endpoints:** Injected by StrataHub via `__mhsBridgeConfig` (same as PWA)
- **Unit transitions:** Same as PWA — StrataHub handles transitions within the tab.
- **Back button:** The student can use the browser's back button or close the tab. StrataHub is typically open in another tab.
- **End of game:** `EndGame()` navigates back to the Mission HydroSci units page.
- **MHSBridge.IsPWA:** `true` (StrataHub's service worker is registered, so `OnPWAReady` fires)

From MHSBridge's perspective, Scenarios 1 and 2 are identical. The only difference is how the student launched the app and how they return to StrataHub.

### Scenario 3: URL-launched (standalone, not from Mission HydroSci)

The game is launched by navigating directly to a URL that serves the unit's build files with the `index.html` loader. This is used by developers and testers.

- **Identity:** The `index.html` loader fetches identity from StrataHub's `/api/user` endpoint (with cookies). On localhost, development defaults are used (`mhs_developer`).
- **Service endpoints:** Fetched from StrataHub's `/api/game-config` endpoint. On localhost, hardcoded development endpoints are used.
- **Unit transitions:** The game manages its own navigation. `CompleteUnit` uses the `nextUnitRelativeUrl` parameter to navigate via relative URLs (e.g., `../unit2/index.html`). Each unit folder has its own `index.html`.
- **Back button:** There is no back button. The student/tester closes the browser tab when done.
- **End of game:** `EndGame()` attempts to close the tab. If the browser blocks it (because the tab wasn't opened by JavaScript), a message is shown: "You can close this tab now."
- **MHSBridge.IsPWA:** `false`
