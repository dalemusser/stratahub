# MHSBridge Integration Guide

## What This Is

MHSBridge is a small integration layer (one jslib file + one C# script) that connects the Mission HydroSci Unity WebGL game to its hosting environment. It provides three capabilities:

1. **Player identity** — the game gets the player's login ID from the URL (`?id=johndoe`) instead of calling `/api/user`
2. **Unit completion signaling** — the game tells the host page when a unit is complete
3. **Two operating modes** — the same build works in the StrataHub PWA (managed transitions) and as a standalone URL (self-managed navigation)

No changes to the Unity build pipeline are required. The jslib file is a standard Unity WebGL plugin.

### Why Identity Is in the URL

Putting the player's login ID in the URL (`?id=johndoe`) means identity works everywhere without depending on any particular hosting environment:

- **Standalone/URL mode** — the game reads `?id=` from the URL. No server-side session, no cookies, no `/api/user` call needed.
- **PWA mode** — StrataHub constructs the URL with `?id=` from the authenticated session. Same mechanism, same code path in the game.
- **StrataHub resource link** — when the game is listed as a resource in StrataHub, the `?id=` parameter is automatically appended to the URL when a student launches it. No manual configuration needed.
- **Testing** — anyone can test with identity by appending `?id=tester123` to the URL. No StrataHub account, no login, no server required. Open `unit1/index.html?id=tester123` on localhost or any web server and the game has a player ID.

One identity mechanism, one code path, works in every context.

---

## Files to Add

### 1. `Assets/Plugins/WebGL/MHSBridge.jslib`

Copy the provided `MHSBridge.jslib` file into your project at this exact path. Unity automatically includes `.jslib` files from `Plugins/WebGL/` in WebGL builds.

This file contains four JavaScript functions that the C# script calls via `[DllImport("__Internal")]`:

| Function | Purpose |
|----------|---------|
| `MHSBridge_NotifyUnitComplete` | Tells the host page a unit is done |
| `MHSBridge_GetPlayerID` | Reads the `?id=` parameter from the URL. Returns unmanaged UTF-8 pointer; freed via `MHSBridge_Free` (handled internally by `MHSBridge.GetPlayerID()`) |
| `MHSBridge_Free` | Frees memory allocated by `GetPlayerID` |
| `MHSBridge_NavigateToUnit` | Navigates to a relative URL, preserving `?id=` params |

### 2. `Assets/Scripts/MHSBridge.cs`

Copy the provided `MHSBridge.cs` file into your project. This is the C# API that your game code calls. It wraps the jslib functions and handles mode detection.

### 3. GameObject Setup

1. In the **first scene that loads** (the loader scene, or Unit 1's scene if there's no loader), create an empty GameObject.
2. Name it exactly: **`MHSBridge`** (case-sensitive — the host page calls `SendMessage('MHSBridge', ...)`)
3. Attach the `MHSBridge` script to it.
4. The script calls `DontDestroyOnLoad(gameObject)` so it persists across scene changes.

---

## How It Works

### Player Identity

The player's login ID is passed in the URL: `?id=johndoe`

To get it in your code:

```csharp
string playerId = MHSBridge.Instance.GetPlayerID();
// Returns "johndoe" from ?id=johndoe
// Returns "editor-test-user" when running in the Unity Editor
// Returns "" if the ?id parameter is missing
```

Use this for stratalog calls and save state. This replaces the current approach of calling `/api/user` — the login ID is already in the URL, so no network request is needed.

### Unit Completion

When the player finishes a unit (the point where the game currently navigates to the next unit), call `CompleteUnit`:

```csharp
MHSBridge.Instance.CompleteUnit("unit3", "../unit4/index.html");
```

**Parameters:**
- `currentUnitId` — the unit that was just completed: `"unit1"`, `"unit2"`, `"unit3"`, `"unit4"`, or `"unit5"`
- `nextUnitRelativeUrl` — relative URL to the next unit's `index.html` (only used in URL mode, ignored in PWA mode). In URL mode, passing empty string is a no-op — used for Unit 5 where there is no next unit.

**What happens depends on the mode:**

| Mode | Behavior |
|------|----------|
| PWA mode | `CompleteUnit` calls `window.mhsUnitComplete("unit3")` on the host page. The host page records progress, tears down Unity, and navigates to the next unit. The game does nothing else — no loading screen, no navigation. |
| URL mode | `CompleteUnit` navigates the browser to `../unit4/index.html?id=johndoe` (the URL parameters are carried forward automatically). |

### For the Final Unit (Unit 5)

```csharp
MHSBridge.Instance.CompleteUnit("unit5", "");
```

Pass an empty string for the next URL. In PWA mode, the host page shows a completion message. In URL mode, nothing happens (there's nowhere to navigate).

---

## The Two Modes

The game operates in one of two modes. The mode is determined automatically — you don't set it.

### PWA Mode (Managed by StrataHub)

- The game is loaded inside the StrataHub PWA play page.
- After Unity finishes loading, the host page calls: `SendMessage('MHSBridge', 'OnPWAReady', '')`
- After this call, `MHSBridge.Instance.IsPWA` returns `true`.
- When a unit is complete, call `CompleteUnit()`. The host page handles everything after that.
- **Do NOT navigate between units.** The host page does it.
- **Do NOT show a "loading next unit" screen.** The host page handles the transition.

### URL Mode (Self-Managed)

- The game is loaded directly from a URL (e.g., `https://test.adroit.games/builds/.../unit1/index.html`).
- `SendMessage` is never called, so `IsPWA` stays `false`.
- When a unit is complete, `CompleteUnit()` navigates the browser to the next unit using the relative URL you provide.
- The game manages its own unit-to-unit flow.

### Checking Mode (Optional)

If you need to change behavior based on mode:

```csharp
if (MHSBridge.Instance.IsPWA)
{
    // PWA mode — host page handles transitions
    // Don't show "loading next unit" UI
}
else
{
    // URL mode — game handles its own navigation
    // Show your own loading/transition screen if desired
}
```

Most game code doesn't need to check this. `CompleteUnit()` already does the right thing in both modes.

---

## S3 Directory Structure

For URL mode to work with relative URLs, the build output must follow this directory structure. The example below uses `20260218-10634-WebResizeTest` as the build name:

```
20260218-10634-WebResizeTest/
  loader/
    index.html          <-- Entry point (this is the URL in StrataHub resources)
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

The parent path above the build name folder doesn't matter — it could be `builds/20260218-10634-WebResizeTest/`, `releases/20260218-10634-WebResizeTest/`, or just `20260218-10634-WebResizeTest/` at the root. The relative URLs work regardless of where the build folder lives on the server.

### Key requirements:

1. **Inside the build folder, `loader/`, `unit1/`, `unit2/`, etc. must be sibling folders.** This is what makes the relative URLs (`../unit2/index.html`) work — each folder navigates up to the shared parent and back down into a sibling.
2. **The loader is a sibling of the unit folders** — it's `loader/`, not a parent of the unit folders.
3. **Each folder contains** `index.html`, `Build/`, and `StreamingAssets/` (standard Unity WebGL build output).

### Why this structure matters

All unit-to-unit navigation uses relative URLs:

```
From loader/index.html  → ../unit1/index.html
From unit1/index.html   → ../unit2/index.html
From unit4/index.html   → ../unit5/index.html
```

Because URLs are relative, the same build works on any host — `cdn.adroit.games`, `test.adroit.games`, `localhost`, or anywhere else — without changing any code.

### What the loader does

The loader is only used in URL mode (not in PWA mode — StrataHub launches the correct unit directly). It is the entry point URL that StrataHub resource links and direct URLs point to.

The loader's job is to determine which unit the student should be playing and navigate there. It runs as a Unity WebGL build with the MHSBridge GameObject, so it has access to `MHSBridge.Instance`.

**Loader flow:**

1. The loader scene starts. MHSBridge initializes (no `OnPWAReady` call arrives — this is URL mode).
2. The loader gets the player's ID via `MHSBridge.Instance.GetPlayerID()`.
3. The loader queries the save service (save.adroit.games) using that player ID to determine what unit the student is currently on.
4. The loader navigates to the correct unit via `MHSBridge.Instance.NavigateToUnit()`, which constructs the relative URL and preserves the `?id=` parameter automatically.

```csharp
// In the loader scene's startup script:

void Start()
{
    string playerId = MHSBridge.Instance.GetPlayerID();

    // Query save.adroit.games to determine the student's current unit.
    // This depends on your save data format — the key question is:
    // "What unit should this student play next?"
    //
    // If no save data exists (new student), default to unit1.
    string currentUnit = DetermineCurrentUnit(playerId); // Your implementation

    // Navigate to the correct unit.
    // This navigates to ../unit3/index.html?id=johndoe (URL params preserved automatically).
    MHSBridge.Instance.NavigateToUnit(currentUnit); // e.g., "unit1" or "unit3"
}
```

If there is no save data for this player, `DetermineCurrentUnit` should return `"unit1"`.

The loader doesn't run the game — it checks save data and redirects. The student sees it only briefly (or not at all if the redirect is fast).

---

## Integration Points — Where to Call MHSBridge

### 1. Replace `/api/user` calls

Wherever the game currently calls `/api/user` to get the player's identity for stratalog or save state, replace it with:

```csharp
string playerId = MHSBridge.Instance.GetPlayerID();
```

This is synchronous and instant — no network call needed. The ID is read directly from the page URL.

### 2. Implement the loader (URL mode)

The loader scene is the entry point for URL mode. It needs to:
1. Get the player ID: `MHSBridge.Instance.GetPlayerID()`
2. Check save data (save.adroit.games) to determine the player's current unit
3. Navigate: `MHSBridge.Instance.NavigateToUnit("unit3")`

If there's no save data, navigate to `"unit1"`. The loader is not used in PWA mode — StrataHub launches the correct unit directly.

### 3. Replace end-of-unit navigation

Wherever the game currently navigates to the next unit (setting `window.location`, calling `Application.OpenURL`, etc.), replace it with:

```csharp
MHSBridge.Instance.CompleteUnit("unit3", "../unit4/index.html");
```

The unit IDs must be exactly: `"unit1"`, `"unit2"`, `"unit3"`, `"unit4"`, `"unit5"`.

Here are all five completion calls:

```csharp
// End of Unit 1:
MHSBridge.Instance.CompleteUnit("unit1", "../unit2/index.html");

// End of Unit 2:
MHSBridge.Instance.CompleteUnit("unit2", "../unit3/index.html");

// End of Unit 3:
MHSBridge.Instance.CompleteUnit("unit3", "../unit4/index.html");

// End of Unit 4:
MHSBridge.Instance.CompleteUnit("unit4", "../unit5/index.html");

// End of Unit 5 (final):
MHSBridge.Instance.CompleteUnit("unit5", "");
```

---

## Testing

### In the Unity Editor

`GetPlayerID()` returns `"editor-test-user"`. `CompleteUnit()` logs a message to the console but does not navigate (guarded by `#if UNITY_WEBGL && !UNITY_EDITOR`). Normal Unity Editor workflow is unaffected.

### WebGL Build — URL Mode (No StrataHub)

1. Build the WebGL project and upload to any web server (or use a local server).
2. Open `unit1/index.html?id=testplayer` in a browser.
3. Play through to the end of the unit.
4. When `CompleteUnit("unit1", "../unit2/index.html")` is called, the browser should navigate to `../unit2/index.html?id=testplayer`.
5. Verify the `?id=testplayer` parameter carried through.

### WebGL Build — PWA Mode (With StrataHub)

1. Deploy the build content to the CDN.
2. Open Mission HydroSci in StrataHub and launch a unit.
3. After Unity loads, check the browser console for: `MHSBridge: PWA mode activated`
4. Play through to the end of the unit.
5. The host page should handle the transition (you'll see StrataHub navigate to the next unit).

### Quick Smoke Test

To verify MHSBridge is set up correctly without playing through an entire unit:

```csharp
// Add this temporarily to any MonoBehaviour.Start():
Debug.Log("Player ID: " + MHSBridge.Instance.GetPlayerID());
Debug.Log("Is PWA: " + MHSBridge.Instance.IsPWA);
```

Open the browser console and confirm the player ID matches the `?id=` URL parameter, and `IsPWA` is `true` in StrataHub or `false` when loaded directly.

---

## Summary of Changes

| What | Where | Change |
|------|-------|--------|
| Add jslib | `Assets/Plugins/WebGL/MHSBridge.jslib` | New file (provided) |
| Add C# script | `Assets/Scripts/MHSBridge.cs` | New file (provided) |
| Create GameObject | First loaded scene | Empty GameObject named "MHSBridge" with the script attached |
| Get player ID | Wherever `/api/user` is called | Replace with `MHSBridge.Instance.GetPlayerID()` |
| Signal unit complete | Wherever end-of-unit navigation happens | Replace with `MHSBridge.Instance.CompleteUnit(unitId, nextUrl)` |
| Loader navigation | Loader scene startup | Query save data, call `MHSBridge.Instance.NavigateToUnit(unitName)` |
| Build directory layout | S3 upload structure | Sibling folders: `loader/`, `unit1/`, `unit2/`, `unit3/`, `unit4/`, `unit5/` |

Nothing else in the game needs to change. The existing save system (save.adroit.games), stratalog, gameplay, rendering, audio — all unchanged.
