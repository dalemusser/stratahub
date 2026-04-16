# MHSBridge Integration Guide (2026-04-15)

> **What you need to do:**
>
> 1. **Get player identity from MHSBridge** — use `GetPlayerID()` and `GetPlayerName()` instead of calling `/api/user` or using a separate jslib for identity. **Remove the old jslib that fetched identity from `/api/user`.** Do not leave it in the project and do not use it in any way.
> 2. **Get service endpoint configs from MHSBridge** — use `GetLogSubmitConfig()`, `GetStateSaveConfig()`, `GetStateLoadConfig()`, `GetSettingsSaveConfig()`, and `GetSettingsLoadConfig()` for endpoint URLs and auth credentials. Each returns a full URL you POST to directly. Do not hardcode service URLs or auth strings. They are currently hardcoded in the game, which prevents us from using services at other URLs. Making this change makes it possible to deploy in another environment without rebuilding.
> 3. **Use `user_id` instead of `playerId`** — we are transitioning to `user_id` as the identity key in all log and save payloads.
> 4. **Navigate between units using MHSBridge** — use `GetUnitURL()`, `NavigateToUnit()`, and `CompleteUnit()` for all unit transitions instead of constructing URLs directly.
> 5. **Call `EndGame()` from the end-of-game screen** — when the player finishes the game and clicks Continue/Return, call `MHSBridge.Instance.EndGame()`. The game owns the end-of-game experience. StrataHub no longer shows its own end-of-game overlay.
> 6. **Replace the Unity-generated `index.html`** in each unit build folder with the provided replacement `index.html`. This replacement page sets up `window.__mhsBridgeConfig` before Unity starts. The Unity-generated `index.html` does not do this and must not be used. You can see the config chain in action at https://cdn.adroit.games/web/test-bridge-config.html — this test page demonstrates the same `/api/user` and `/api/game-config` fetches that the replacement `index.html` performs.
>
> **Why are we doing this?** The game currently has hardcoded domains, API keys, and URL patterns baked into the build. This makes it impossible to rotate credentials, change service endpoints, or host builds differently without rebuilding the game. We have a longer-term plan to support uploading new builds directly to StrataHub, serving all hosting contexts (PWA, URL-launched, developer testing) from a single set of game files in S3, managing build versions and channels (production/staging), and generating content manifests automatically. By making these MHSBridge changes now — where the game gets everything it needs from the host page config — **you won't need to make any further game-side changes** when that infrastructure rolls out. The game becomes portable and host-agnostic.
>
> For the full long-term plan, see [MHSBridge, Identity, and Game Hosting Plan](../mhsbridge_and_id_plan.md).

## What Changed

This is an updated MHSBridge that replaces the previous version. The key changes:

1. **Identity comes from the host page.** The host page sets `window.__mhsBridgeConfig` with the player's identity before Unity starts. No more separate `/api/user` calls needed.
2. **Service endpoint config comes from the host page.** Full endpoint URLs and auth credentials for stratalog and stratasave are no longer hardcoded — they come from the bridge config. Each URL is a complete endpoint (e.g., `https://save.adroit.games/api/state/save`) that you POST to directly.
3. **Navigation supports a unitMap.** The host page can provide a map of unit names to URLs. This enables managed navigation (locked units, opaque URLs) alongside the existing relative URL navigation.
4. **Backward compatible.** If `window.__mhsBridgeConfig` is absent (old host page), identity and service config are not available from the bridge. The game should fall back to its existing identity and service mechanisms.

## What Is NOT Changing

- **`CompleteUnit()` works the same way.** Same parameters, same behavior in PWA and URL modes. Call it for every unit, including the last one.
- **`OnPWAReady` is still called by the host page.** It now accepts identity JSON but also accepts empty string (backward compat).
- **The MHSBridge GameObject setup is identical.** Named "MHSBridge", in the first scene of every unit, `DontDestroyOnLoad`.
- **Editor behavior uses development defaults.** `GetPlayerID()` returns "mhs_developer", and service configs point to production endpoints so devs can test logging, saving, and settings from the Editor.
- **`EndGame()` is a no-op in the Editor.** It logs a message but takes no action, so development testing is unaffected.

---

## Files to Update

### 1. `Assets/Plugins/WebGL/MHSBridge.jslib`

Replace the existing file with the new `MHSBridge.jslib`. New functions added:

| Function | Purpose |
|----------|---------|
| `MHSBridge_GetConfig` | Returns the full `window.__mhsBridgeConfig` as JSON |
| `MHSBridge_Free` | Frees memory (unchanged) |
| `MHSBridge_NotifyUnitComplete` | Tells host page a unit is done (unchanged) |
| `MHSBridge_EndGame` | Signals game ended — calls `window.mhsEndGame()` or closes tab (new) |
| `MHSBridge_NavigateToUnit` | Navigates to a URL (no longer carries URL params forward) |
| `MHSBridge_GetUnitURL` | Returns URL for a unit (from unitMap or relative fallback) |

### 2. `Assets/Scripts/MHSBridge.cs`

Replace the existing file with the new `MHSBridge.cs`. New public API:

| Method | Purpose |
|--------|---------|
| `GetPlayerID()` | Returns user_id from config or OnPWAReady |
| `GetPlayerName()` | Returns display name (new) |
| `GetLogSubmitConfig()` | Returns log submit endpoint URL + auth, or null (new) |
| `GetStateSaveConfig()` | Returns state save endpoint URL + auth, or null (new) |
| `GetStateLoadConfig()` | Returns state load endpoint URL + auth, or null (new) |
| `GetSettingsSaveConfig()` | Returns settings save endpoint URL + auth, or null (new) |
| `GetSettingsLoadConfig()` | Returns settings load endpoint URL + auth, or null (new) |
| `GetUnitURL(unitName, out isLocked)` | Returns URL for a unit (unitMap or relative); null if locked (new) |
| `CompleteUnit(unitId, nextUrl)` | Signals unit completion (unchanged) |
| `EndGame()` | Signals the game has ended — exits back to StrataHub or closes tab (new) |
| `NavigateToUnit(unitName)` | Navigates to unit via GetUnitURL (updated) |
| `IsPWA` | True if in PWA mode (unchanged) |
| `HasConfig` | True if `__mhsBridgeConfig` was found (new) |

### 3. GameObject Setup

**No change.** Same as before — empty GameObject named "MHSBridge" with the script attached, in the first scene of every unit and the loader.

---

## How to Use the New API

### Player Identity

Replace all `/api/user` calls with:

```csharp
string userId = MHSBridge.Instance.GetPlayerID();
string userName = MHSBridge.Instance.GetPlayerName();
```

`GetPlayerID()` returns the `user_id` value. For now, this is the login_id (e.g., "dale@example.com"). In a future phase, it will be the user's database ID. **Use `user_id` as the key name** when sending to stratalog and stratasave.

### Log Service

Instead of hardcoded log service URL and auth:

```csharp
// Old way (hardcoded):
// string logUrl = "https://log.adroit.games/api/log/submit";
// string logAuth = "Bearer hardcoded-key";

// New way:
var logConfig = MHSBridge.Instance.GetLogSubmitConfig();
if (logConfig != null)
{
    string logUrl = logConfig.url;    // e.g., "https://log.adroit.games/api/log/submit"
    string logAuth = logConfig.auth;  // e.g., "Bearer abc123..."
    // POST directly to logUrl — no path appending needed
}
else
{
    // Fall back to hardcoded values (old host page without config)
}
```

### State Save/Load

The game state (progress, inventory, etc.) is saved and loaded via separate endpoints:

```csharp
// Save game state:
var stateSave = MHSBridge.Instance.GetStateSaveConfig();
if (stateSave != null)
{
    // POST to stateSave.url (e.g., "https://save.adroit.games/api/state/save")
    // Authorization: stateSave.auth
}

// Load game state:
var stateLoad = MHSBridge.Instance.GetStateLoadConfig();
if (stateLoad != null)
{
    // POST to stateLoad.url (e.g., "https://save.adroit.games/api/state/load")
    // Authorization: stateLoad.auth
}
```

### Settings Save/Load

Player settings (preferences, accessibility options, etc.) use separate endpoints:

```csharp
// Save player settings:
var settingsSave = MHSBridge.Instance.GetSettingsSaveConfig();
if (settingsSave != null)
{
    // POST to settingsSave.url (e.g., "https://save.adroit.games/api/settings/save")
    // Authorization: settingsSave.auth
}

// Load player settings:
var settingsLoad = MHSBridge.Instance.GetSettingsLoadConfig();
if (settingsLoad != null)
{
    // POST to settingsLoad.url (e.g., "https://save.adroit.games/api/settings/load")
    // Authorization: settingsLoad.auth
}
```

All five service configs return a `ServiceConfig` with `url` (full endpoint) and `auth` (Authorization header value). If null, fall back to hardcoded values.

### Sending Identity to Stratalog

We are transitioning from `playerId` to `user_id` as the identity key. Use `user_id` in all new code — stratalog accepts both during the transition:

```csharp
// The identity key should be "user_id" in the JSON payload
string userId = MHSBridge.Instance.GetPlayerID();

// In your log submission JSON:
// { "game": "mhs", "user_id": "dale@example.com", "eventType": "...", ... }
```

### Sending Identity to Stratasave

Stratasave already uses `user_id` — no change needed in the key name:

```csharp
string userId = MHSBridge.Instance.GetPlayerID();

// In your save submission JSON:
// { "user_id": "dale@example.com", "game": "mhs", "save_data": { ... } }
```

### Unit Completion

**No change** — same as before. Call `CompleteUnit` for every unit, including the last one:

```csharp
// End of Unit 1:
MHSBridge.Instance.CompleteUnit("unit1", "../unit2/index.html");

// End of Unit 5 (currently the last unit):
MHSBridge.Instance.CompleteUnit("unit5", "../unit6/index.html");
```

Note: Always provide the next unit URL even for the last unit. In PWA mode, the URL is ignored (StrataHub handles the transition). In URL mode, if the next unit doesn't exist yet, the navigation simply won't happen. When unit 6 is added, no code change is needed — `CompleteUnit` will transition to it automatically.

### End of Game

**New.** Call `EndGame()` from the game's end-of-game screen when the player clicks Continue/Return:

```csharp
// Player clicks Continue on the end-of-game congratulatory screen:
MHSBridge.Instance.EndGame();
```

**The flow for the final unit:**
1. Unit gameplay completes → call `CompleteUnit("unit5", "../unit6/index.html")`
2. Show the game's end-of-game screen (congratulations, achievements, etc.)
3. Player clicks Continue → call `EndGame()`

**What EndGame does:**
- **PWA mode (StrataHub):** Navigates back to the Mission HydroSci units page
- **Browser tab (not PWA):** Closes the tab, or shows "You can close this tab now" if the browser blocks it
- **Editor:** Logs a message (no-op)

**Important:** `CompleteUnit` and `EndGame` are separate calls with different purposes:
- `CompleteUnit` = "This unit's gameplay is done" (signals progress)
- `EndGame` = "The player is done and wants to leave" (exits the game)

### Loader Navigation

The loader navigates to units by name — `GetUnitURL` uses the unitMap if one is present, otherwise returns a relative URL:

```csharp
void Start()
{
    string userId = MHSBridge.Instance.GetPlayerID();
    string currentUnit = DetermineCurrentUnit(userId); // Your save data logic

    MHSBridge.Instance.NavigateToUnit(currentUnit);
}
```

Unit locking was introduced because students figured out the URL pattern (e.g., changing `unit1` to `unit5` in the address bar) and skipped ahead to future units. The unitMap prevents this — a unit is locked when its entry is explicitly `null`. The host page builds the unitMap from the student's progress: completed and current units get real URLs, future units get `null`. If there's no unitMap at all (e.g., developer builds), nothing is ever locked and `GetUnitURL` returns a relative URL.

If the target unit is locked, `NavigateToUnit` logs a warning and does nothing. Your code can check explicitly:

```csharp
bool isLocked;
string url = MHSBridge.Instance.GetUnitURL("unit3", out isLocked);

if (isLocked)
{
    // Show "unit not available" message
}
else if (url != null)
{
    MHSBridge.Instance.NavigateToUnit("unit3");
}
```

---

## The Host Page Contract

The host page sets this JavaScript global before Unity starts:

```javascript
window.__mhsBridgeConfig = {
  identity: {
    user_id: "dale@example.com",
    name: "Dale Musser"
  },
  services: {
    log_submit:    { url: "https://log.adroit.games/api/log/submit",       auth: "Bearer abc123..." },
    state_save:    { url: "https://save.adroit.games/api/state/save",      auth: "Bearer xyz789..." },
    state_load:    { url: "https://save.adroit.games/api/state/load",      auth: "Bearer xyz789..." },
    settings_save: { url: "https://save.adroit.games/api/settings/save",   auth: "Bearer xyz789..." },
    settings_load: { url: "https://save.adroit.games/api/settings/load",   auth: "Bearer xyz789..." }
  },
  navigation: {
    unitMap: {                     // optional
      "unit1": "/play/t/abc123",  // available
      "unit2": "/play/t/def456",  // available
      "unit3": null,              // locked
      "unit4": null,              // locked
      "unit5": null               // locked
    }
  }
};
```

- **`identity`** — required. `user_id` and `name`.
- **`services`** — required. Five service entries, each with `url` (full endpoint) and `auth` (Authorization header value): `log_submit`, `state_save`, `state_load`, `settings_save`, `settings_load`.
- **`navigation.unitMap`** — optional. If present, `NavigateToUnit` uses it. If absent, relative navigation.

The game reads this at startup. It does NOT make any network calls for identity or configuration — the host page provides everything.

---

## How the Host Page Provides the Config

### Mission HydroSci (PWA mode)

StrataHub's Go template injects the config server-side from the authenticated session and server configuration. The service credentials never appear in the HTML source — they are rendered by the server at request time.

### Developer Builds (URL mode)

We provide a replacement `index.html` that you drop into your build folders (replacing Unity's generated `index.html`). This replacement page:

1. Fetches identity from StrataHub: `GET /api/user`
2. Fetches service config from StrataHub: `GET /api/game-config?game=mhs`
3. Assembles `window.__mhsBridgeConfig`
4. Starts Unity

You must be logged into StrataHub (e.g., dev.adroit.games) in your browser for this to work.

### Local Testing (localhost)

If you're running a build locally (localhost), the replacement `index.html` detects this and uses development defaults automatically:
- `GetPlayerID()` returns "mhs_developer"
- All service configs (`GetLogSubmitConfig()`, `GetStateSaveConfig()`, etc.) return production endpoint URLs and auth
- No StrataHub login required — you can test logging and saving immediately

---

## S3 Directory Structure

**No change.** Same sibling folder structure:

```
my-build/
  loader/
    index.html          <-- Replace with provided index.html
    Build/
    StreamingAssets/
  unit1/
    index.html          <-- Replace with provided index.html
    Build/
    StreamingAssets/
  unit2/
    ...
```

## Build Settings

The replacement `index.html` expects Unity WebGL build files with the `.unityweb` extension:

```
Build/
  BuildName.loader.js
  BuildName.data.unityweb
  BuildName.framework.js.unityweb
  BuildName.wasm.unityweb
```

We recommend using Brotli compression for the `.unityweb` files. If your builds produce different file extensions (e.g., `.gz` or no extension), you will need to edit the file extension references in `index.html` to match.

---

## Testing

### Quick Smoke Test

Add this temporarily to verify the bridge is working:

```csharp
void Start()
{
    Debug.Log("Player ID: " + MHSBridge.Instance.GetPlayerID());
    Debug.Log("Player Name: " + MHSBridge.Instance.GetPlayerName());
    Debug.Log("Is PWA: " + MHSBridge.Instance.IsPWA);
    Debug.Log("Has Config: " + MHSBridge.Instance.HasConfig);

    var log = MHSBridge.Instance.GetLogSubmitConfig();
    Debug.Log("Log Submit: " + (log != null ? log.url : "null (using fallback)"));

    var stateSave = MHSBridge.Instance.GetStateSaveConfig();
    Debug.Log("State Save: " + (stateSave != null ? stateSave.url : "null (using fallback)"));

    var stateLoad = MHSBridge.Instance.GetStateLoadConfig();
    Debug.Log("State Load: " + (stateLoad != null ? stateLoad.url : "null (using fallback)"));

    var settingsSave = MHSBridge.Instance.GetSettingsSaveConfig();
    Debug.Log("Settings Save: " + (settingsSave != null ? settingsSave.url : "null (using fallback)"));

    var settingsLoad = MHSBridge.Instance.GetSettingsLoadConfig();
    Debug.Log("Settings Load: " + (settingsLoad != null ? settingsLoad.url : "null (using fallback)"));
}
```

### Test Scenarios

1. **With replacement index.html + logged into StrataHub:**
   - `HasConfig` should be `true`
   - `GetPlayerID()` should return your login ID
   - All five service configs should return full endpoint URLs and auth

2. **With replacement index.html on localhost:**
   - `HasConfig` should be `true` (localhost defaults)
   - `GetPlayerID()` returns "mhs_developer"
   - All service configs return production endpoint URLs and auth

3. **With replacement index.html on remote host + NOT logged into StrataHub:**
   - `HasConfig` should be `false` (config fetch failed)
   - `GetPlayerID()` returns empty string
   - Service configs return null

4. **In StrataHub PWA:**
   - `HasConfig` should be `true`
   - `IsPWA` should be `true`
   - `GetPlayerID()` should return the logged-in user's ID
   - Service configs should return full endpoint URLs and auth

5. **EndGame in PWA mode:**
   - After completing the final unit and seeing the end-of-game screen, click Continue
   - Should navigate to the Mission HydroSci units page with all units showing as completed

6. **EndGame in URL mode (browser tab):**
   - After the end-of-game screen, click Continue
   - Tab should close, or show "You can close this tab now" if the browser blocks it

---

## Summary of Changes

| What | Where | Change |
|------|-------|--------|
| Replace jslib | `Assets/Plugins/WebGL/MHSBridge.jslib` | New file (adds GetConfig, GetUnitURL; updates GetPlayerID, NavigateToUnit) |
| Replace C# script | `Assets/Scripts/MHSBridge.cs` | New file (adds service config, unitMap, config loading) |
| Get player ID | Wherever `/api/user` is used | Replace with `MHSBridge.Instance.GetPlayerID()` |
| Identity key | Log/save JSON payloads | Use `user_id` instead of `playerId` for stratalog |
| Get log config | Wherever log URL/auth is hardcoded | Use `MHSBridge.Instance.GetLogSubmitConfig()` with hardcoded fallback |
| Get state save/load | Wherever save URL/auth is hardcoded | Use `GetStateSaveConfig()` and `GetStateLoadConfig()` with hardcoded fallback |
| Get settings save/load | Wherever settings URL/auth is hardcoded | Use `GetSettingsSaveConfig()` and `GetSettingsLoadConfig()` with hardcoded fallback |
| Replace index.html | Each build folder | Drop in the provided replacement `index.html` |
| Loader navigation | Loader scene startup | `GetUnitURL` and `NavigateToUnit` handle URL resolution automatically |
| Signal unit complete | End-of-unit code | No change — `CompleteUnit()` works the same |
| Signal end of game | End-of-game screen Continue button | Call `MHSBridge.Instance.EndGame()` (new) |
| Build directory layout | S3 upload structure | No change — sibling folders still work |
