# MHSBridge Change Notice (2026-03-27)

## What Was Wrong

The service configuration provided by MHSBridge only included base domain URLs for log and save services (e.g., `https://save.adroit.games`). The game was expected to construct the full endpoint paths itself, but the integration guide never documented what those paths were. This caused 403 Forbidden errors because requests were hitting paths not recognized by the server.

Additionally, the configuration was missing endpoints for player settings (save/load). The game had no way to get settings service URLs from the bridge.

I apologize for these oversights in the original handoff.

## What Changed

The bridge now provides **five named service endpoints**, each with a **full URL** that you POST to directly — no path construction needed:

| Service Key | Endpoint | Purpose |
|-------------|----------|---------|
| `log_submit` | `https://log.adroit.games/api/log/submit` | Submit log entries |
| `state_save` | `https://save.adroit.games/api/state/save` | Save game state |
| `state_load` | `https://save.adroit.games/api/state/load` | Load game state |
| `settings_save` | `https://save.adroit.games/api/settings/save` | Save player settings |
| `settings_load` | `https://save.adroit.games/api/settings/load` | Load player settings |

The old `GetLogServiceConfig()` and `GetSaveServiceConfig()` methods have been replaced with:

- `GetLogSubmitConfig()`
- `GetStateSaveConfig()`
- `GetStateLoadConfig()`
- `GetSettingsSaveConfig()`
- `GetSettingsLoadConfig()`

Each returns a `ServiceConfig` with `url` (full endpoint) and `auth` (Authorization header value), or null if not configured.

## Updated Files

The following files in this folder have been updated and should replace your current versions:

- **`MHSBridge.cs`** — New service accessor methods, updated `BridgeServices` structure, updated editor defaults
- **`MHSBridge.jslib`** — No changes needed (it passes the config through as-is)
- **`index.html`** — Updated localhost development defaults with full endpoint URLs

Please refer to the updated **[MHSBridge-Integration-Guide.md](MHSBridge-Integration-Guide.md)** for complete usage examples and the new API reference.
