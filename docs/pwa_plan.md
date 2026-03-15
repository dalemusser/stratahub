# StrataHub PWA Architecture Plan

## Problem

StrataHub currently has multiple competing PWA implementations on the same origin:

- **mhsdelivery** (`/manifest.json`, `/sw.js`) — the original MHS Units feature
- **missionhydrosci** (`/missionhydrosci-manifest.json`, `/missionhydrosci-sw.js`) — Mission HydroSci

Each registers its own service worker and manifest. Chrome only supports one installed PWA per origin, so these conflict — installing from the wrong page picks up the wrong manifest, causing navigation overlay issues on Chromebooks and confusing the install experience.

Additionally, the PWA is branded around a single game (Mission HydroSci), but StrataHub is a platform that hosts multiple features and will host additional games in the future.

## End-State Architecture

**StrataHub is the PWA.** The manifest, service worker, and install flow are platform-level concerns. Features like Mission HydroSci are content that runs inside the platform PWA.

### Design Principles

1. **One origin, one PWA** — a single manifest and service worker for the entire site
2. **Workspace-aware branding** — the PWA name, icons, and colors come from workspace settings
3. **Feature-agnostic service worker** — the SW doesn't know about specific games; features declare their caching needs
4. **Existing patterns** — follow the established storage, settings, and login actions patterns in the codebase

## Architecture

### 1. Workspace PWA Icon (Settings Addition)

Add a PWA icon field to workspace settings, alongside the existing logo upload.

**Why separate from logo:** Logos are displayed in the sidebar menu and can be any aspect ratio. PWA icons must be square (192x192 and 512x512) and are used by the OS for app icons, splash screens, and taskbars.

#### Model Changes

**`internal/domain/models/sitesettings.go`** — add fields:

```go
PWAIconPath string `bson:"pwa_icon_path,omitempty"`
PWAIconName string `bson:"pwa_icon_name,omitempty"`
```

Add helper:

```go
func (s SiteSettings) HasPWAIcon() bool {
    return s.PWAIconPath != ""
}
```

#### Settings Handler Changes

**`internal/app/features/workspaces/settings.go`**:

- Accept `pwa_icon` file upload (same pattern as logo upload)
- Accept `remove_pwa_icon` checkbox
- Store at path: `pwa-icons/YYYY/MM/{uuid}{ext}` via `h.Storage.Put()`
- Validate: must be image/png, recommend square dimensions
- Pass `PWAIconURL` to template via `h.Storage.URL()`

#### Settings Template Changes

**`workspace_settings.gohtml`** — add PWA Icon section after the existing Logo section:

- File upload input for icon
- Preview of current icon if set
- Remove checkbox
- Help text: "Square PNG image used as the app icon when installed as a PWA. Recommended: 512x512 pixels."

#### Fallback

When no PWA icon is configured, the manifest serves a default icon. The current waterdrop icons at `/assets/mhs/icon-192.png` and `/assets/mhs/icon-512.png` can serve as the system default, or we can create a generic StrataHub default icon.

---

### 2. Platform Manifest

Replace the per-feature manifests with a single dynamic manifest served at `/manifest.json`.

#### New Package

**`internal/app/pwa/`** — platform-level PWA infrastructure.

**`internal/app/pwa/manifest.go`**:

```go
func (h *Handler) ServeManifest(w http.ResponseWriter, r *http.Request) {
    // 1. Get workspace from context
    // 2. Load site settings for workspace
    // 3. Build manifest dynamically:
    //    - name: site settings SiteName (or workspace Name as fallback)
    //    - short_name: workspace Name
    //    - start_url: "/missionhydrosci/units" (or "/" if no game feature is primary)
    //    - scope: "/"
    //    - display: "standalone"
    //    - background_color, theme_color: from workspace settings (or defaults)
    //    - icons: from PWAIconPath if set, else default icons
    // 4. Serve as application/manifest+json
}
```

**Icon serving:** PWA icons stored in S3/storage need to be served at predictable URLs. Two options:

- **Option A:** Serve them through the existing storage URL (e.g., S3 presigned or public). This works if storage URLs are same-origin or CORS-enabled.
- **Option B:** Serve through a StrataHub endpoint (e.g., `/pwa/icon-192.png`, `/pwa/icon-512.png`) that proxies from storage. This guarantees same-origin and avoids CORS issues.

**Recommended: Option B** — add icon-serving endpoints that read from storage and serve with appropriate cache headers. This also allows on-the-fly resizing if the uploaded icon isn't exactly the right dimensions (future enhancement).

#### `start_url` Design Decision

The `start_url` determines where the PWA opens when launched. Setting it to `"/"` (the welcome/landing page) would be a UX regression — students currently open the PWA and go directly to the game. With `"/"`, they'd land on a generic page and need to navigate to Mission HydroSci every session.

**Decision:** Default `start_url` to `"/missionhydrosci/units"` for workspaces that have Mission HydroSci as their primary feature. In the future, this could be a workspace setting (`PWAStartURL`), but for now hardcoding to the game units page matches the current behavior and user expectation.

When the installed PWA launches, the user should land where they need to be — ready to play.

#### Route Changes

**`bootstrap/routes.go`**:

- Remove: `r.Get("/manifest.json", mhsRootHandler.ServeManifest)` (mhsdelivery)
- Remove: `r.Get("/missionhydrosci-manifest.json", ...)` (missionhydrosci)
- Add: `r.Get("/manifest.json", pwaHandler.ServeManifest)`
- Add: `r.Get("/pwa/icon-192.png", pwaHandler.ServeIcon192)`
- Add: `r.Get("/pwa/icon-512.png", pwaHandler.ServeIcon512)`

#### Layout Template

**`layout.gohtml`** — the existing default manifest block already points to `/manifest.json`:

```gohtml
{{ block "manifest" . }}<link rel="manifest" href="/manifest.json">{{ end }}
```

Remove the `{{ define "manifest" }}` override from `missionhydrosci_units.gohtml`. All pages use the same manifest.

---

### 3. Platform Service Worker

Replace per-feature service workers with one platform-level service worker that features can plug into.

#### Service Worker Design

The SW needs to handle:

1. **App shell caching** — platform assets (CSS, JS) for offline fallback
2. **Feature content caching** — per-feature file caching (Unity builds, etc.)
3. **Background Fetch** — large file downloads that survive tab closure
4. **Page routing** — network-first for dynamic pages, cache-first for cached content
5. **Offline fallback** — generic offline page when network is unavailable
6. **Error categorization** — distinguish storage, network, CORS, and HTTP errors in broadcasts
7. **Cross-tab safety** — check for active play pages before deleting caches

**Critical design rule:** The SW must NEVER interfere with paths it doesn't recognize. If the request path doesn't match any registered feature prefix or app shell URL, the fetch handler must return without calling `event.respondWith()`, letting the browser handle the request normally. A bug here breaks the entire site.

#### Feature Registration

The SW discovers features through a **feature registry** — a simple configuration array embedded in the SW at serve time (Go template or JSON injection).

Each feature declares:

```javascript
{
  id: 'missionhydrosci',           // unique feature identifier
  contentPrefix: '/missionhydrosci/content/',  // URL prefix for cached content (prefix match)
  pageUrls: ['/missionhydrosci/units'],        // exact-match pages to cache for offline
  pagePrefixes: ['/missionhydrosci/play/'],    // prefix-match pages (e.g., /play/unit1, /play/unit2)
  manifestUrl: '/missionhydrosci/api/manifest', // content manifest endpoint
  unitCachePrefix: 'missionhydrosci-unit-',     // cache name prefix
  channelName: 'missionhydrosci-delivery',      // BroadcastChannel name
  fetchIdPrefix: 'missionhydrosci-'             // Background Fetch ID prefix
}
```

**URL matching semantics:** `pageUrls` uses exact match; `pagePrefixes` uses `startsWith()`. Both are routed network-first with offline fallback. `contentPrefix` is always prefix-matched and routed cache-first.

This is injected into the SW by the Go handler at serve time, similar to how the current `sw.go` concatenates files.

#### Service Worker Files

**`internal/app/pwa/static/sw.js`** — main fetch handler:

- Install: pre-cache app shell URLs AND `/offline` page. The app shell URL list is injected by the Go handler at serve time (same mechanism as the feature registry), derived from the asset manifest. Minimum set: `/offline`, the delivery manager JS (`/assets/js/content-delivery.js?v={hash}`). Feature page URLs from `pageUrls` can optionally be included for offline access.
- Activate: clean stale app shell caches (including old feature caches like `missionhydrosci-app-shell-v5` and `mhs-app-shell-v1`), then `self.clients.claim()`
- `self.skipWaiting()` in install — updates take effect immediately
- Fetch routing based on registered feature prefixes:
  - `{feature.contentPrefix}*` → cache-first across that feature's unit caches
  - `/assets/*` → cache-first app shell cache
  - Feature page URLs (matched via `feature.pageUrls`) → network-first with offline fallback
  - All other paths → **pass through to network, no interception**
- Offline fallback: when network-first fails and no cached page exists, serve the cached `/offline` page
- Message handler with these actions:

```javascript
// Actions the SW handles via postMessage:
'download'          — start Background Fetch (with icon URL from message payload)
'fallbackDownload'  — sequential fetch when BG Fetch stalls
'deleteUnit'        — delete a unit's cache (with cross-tab safety check)
'checkStatus'       — check cache status for a unit
'getVersion'        — return SW_VERSION
'cleanStaleVersions' — delete caches for old versions of a feature's units
```

**`internal/app/pwa/static/sw-cache.js`** — cache utilities:

- `APP_SHELL_CACHE` = `'stratahub-app-shell-v1'`
- Feature-aware `unitCacheName(featureId, unitId, version)`
- `cacheKeyFromPath(featureId, cdnUrl)` — converts CDN URLs to same-origin cache keys
- `checkUnitCacheStatus(featureId, unitId, version, expectedFiles)`
- `cleanStaleCaches(featureId, currentVersions)` — per-feature stale cache cleanup
- `findFeatureByPrefix(path)` — look up which feature owns a content path
- `findFeatureByFetchId(fetchId)` — look up which feature owns a Background Fetch

**`internal/app/pwa/static/sw-background-fetch.js`** — Background Fetch:

- Fetch ID format: `{featureId}-{unitId}-v{version}`
- Parse fetch ID to determine which feature owns it
- Route success/failure handlers to the correct feature's cache and channel
- Broadcast status on the correct feature's BroadcastChannel
- **Icon from message payload:** The `startBackgroundFetch()` function receives an `icon` URL from the client (passed in the download message). Falls back to a default icon if not provided:

```javascript
async function startBackgroundFetch(unitId, version, files, cdnBaseUrl, title, icon) {
  // ...
  var bgFetch = await self.registration.backgroundFetch.fetch(fetchId, requests, {
    title: title || 'Downloading ' + unitId,
    icons: [{ sizes: '192x192', src: icon || '/assets/mhs/icon-192.png', type: 'image/png' }],
    downloadTotal: 0
  });
  // ...
}
```

- **Error categorization in `fallbackFetch()`:** Distinguish error types, clean up partial cache, and include the category in the broadcast:

```javascript
} catch (err) {
  var errorType = 'unknown';
  var errorMessage = err.message || 'Download failed';

  if (err.name === 'QuotaExceededError' ||
      errorMessage.toLowerCase().indexOf('quota') !== -1) {
    errorType = 'storage';
    errorMessage = 'Not enough storage on this device. Try clearing old downloads or freeing space.';
  } else if (err instanceof TypeError && errorMessage.indexOf('Failed to fetch') !== -1) {
    errorType = 'network';
    errorMessage = 'Download failed — check your internet connection.';
  } else if (err.status && err.status >= 400) {
    errorType = 'http';
    errorMessage = 'Download failed — the server returned an error. Try again later.';
  }

  // Clean up partial cache to reclaim storage — a failed download
  // can leave behind hundreds of MB of unusable partial data
  await caches.delete(unitCacheName(featureId, unitId, version));

  broadcastStatus(featureId, unitId, 'error', { error: errorMessage, errorType: errorType });
  return false;
}
```

- **Cross-tab safety for deleteUnit:** Before deleting a cache, check if any client window is on a play page for that unit:

```javascript
// In SW message handler for 'deleteUnit':
async function safeDeleteUnit(featureId, unitId, version) {
  var allClients = await clients.matchAll({ type: 'window' });
  for (var i = 0; i < allClients.length; i++) {
    if (allClients[i].url.indexOf('/play/' + unitId) !== -1) {
      // Unit is being played in another window — skip deletion
      broadcastStatus(featureId, unitId, 'cached', {});
      return;
    }
  }
  await caches.delete(unitCacheName(featureId, unitId, version));
  broadcastStatus(featureId, unitId, 'not_cached', {});
}
```

#### Offline Fallback Page

**`internal/app/pwa/templates/offline.gohtml`** — a minimal page that works without network access:

```html
<!DOCTYPE html>
<html lang="en">
<head>
  <meta charset="UTF-8">
  <meta name="viewport" content="width=device-width, initial-scale=1.0">
  <title>Offline</title>
  <style>
    body { font-family: sans-serif; text-align: center; padding: 4em 1em; background: #1e3a5f; color: white; }
    button { margin-top: 1em; padding: 0.75em 2em; font-size: 1rem; border: 2px solid white;
             background: transparent; color: white; border-radius: 0.5em; cursor: pointer; }
    button:hover { background: rgba(255,255,255,0.1); }
  </style>
</head>
<body>
  <h1>You are offline</h1>
  <p>Check your internet connection and try again.</p>
  <button onclick="location.reload()">Try Again</button>
</body>
</html>
```

This page is pre-cached during SW install. The SW serves it as a last-resort fallback when a network-first page fetch fails and no cached version exists. It uses a "Try Again" button (which reloads the page the user was trying to reach) instead of linking to another page — linking to `/` would just show the offline page again since `/` also requires network.

**Served by:** `internal/app/pwa/offline.go` — a simple handler that serves the template. Does not require authentication (it must work when the network is down).

**Route:** `r.Get("/offline", pwaHandler.ServeOffline)` — mounted at root level, no auth required.

#### SW Handler

**`internal/app/pwa/sw.go`**:

```go
func (h *Handler) ServeServiceWorker(w http.ResponseWriter, r *http.Request) {
    // 1. Build feature registry from registered features
    // 2. Inject as: const FEATURES = [...];
    // 3. Concatenate: registry + sw-cache.js + sw-background-fetch.js + sw.js
    // 4. Append content hashes for change detection:
    //    fmt.Fprintf(w, "\n// sw-hash: %s\n", swContentHash)
    //    fmt.Fprintf(w, "\n// delivery-js-hash: %s\n", appresources.ContentDeliveryVersion())
    // 5. Serve with Content-Type: application/javascript, Cache-Control: no-cache
}
```

#### Route Changes

**`bootstrap/routes.go`** — all PWA routes must be at the root level, **outside** the auth middleware group (same location as the current `/sw.js` and `/manifest.json` routes). SW registration, manifest fetching, and offline fallback must work without authentication:

- Remove: `r.Get("/sw.js", mhsRootHandler.ServeServiceWorker)` (mhsdelivery)
- Remove: `r.Get("/missionhydrosci-sw.js", ...)` (missionhydrosci)
- Add: `r.Get("/sw.js", pwaHandler.ServeServiceWorker)`
- Add: `r.Get("/offline", pwaHandler.ServeOffline)`

#### Feature Content Routes Stay

Each feature keeps its own content routes:

- `/missionhydrosci/content/*` — CDN fallback (already exists)
- `/missionhydrosci/api/manifest` — content manifest (already exists)
- Future features add their own at their own prefix

---

### 4. Version Hashing and Cache Busting

All served PWA files must include version hashing so browsers detect changes and update.

#### Service Worker (`/sw.js`)

The SW is served with `Cache-Control: no-cache`, which tells the browser to revalidate on every check. Chrome checks for SW updates every 24 hours and on every navigation. The current approach of appending a content hash comment (e.g., `// app-shell-hash: {hash}`) at the end of the concatenated file is effective — any byte-level change triggers Chrome's SW update flow (install → activate → claim).

The platform SW handler must continue this pattern:

```go
// Append content hashes so browser detects SW changes
fmt.Fprintf(w, "\n// sw-hash: %s\n", swContentHash)
fmt.Fprintf(w, "\n// delivery-js-hash: %s\n", appresources.ContentDeliveryVersion())
```

When any SW source file or the delivery JS changes, the hash changes, triggering an update.

#### Manifest (`/manifest.json`)

The manifest is dynamic (workspace-aware), so its content changes when workspace settings change. Serve with `Cache-Control: no-cache, must-revalidate` — Chrome re-fetches the manifest on its own schedule (every 24-48 hours and on every PWA launch). No query-string hash needed on the `<link>` tag. The layout default `<link rel="manifest" href="/manifest.json">` is sufficient.

#### Delivery Manager JS

Continue using the existing `appresources.ContentHash()` pattern for the JS file, as is done today with `MHSDeliveryVersion()`. The versioned URL in templates (e.g., `/assets/js/content-delivery.js?v={hash}`) ensures browsers fetch the updated file.

#### App Shell Cache Versioning

The app shell cache name includes a version number (`stratahub-app-shell-v1`). When app shell URLs change (e.g., the delivery JS URL changes), bump this version. The SW activate handler cleans up old app shell caches automatically.

---

### 5. Client-Side Delivery Manager

**`mhs-delivery.js`** is already parameterized and nearly feature-agnostic. Changes needed:

#### Rename

Rename from `mhs-delivery.js` to `content-delivery.js` (or keep the name and just update the class). The class `MHSDeliveryManager` becomes `ContentDeliveryManager` (or stays — it's internal).

#### Configuration

Each feature page instantiates the manager with its own config:

```javascript
const delivery = new ContentDeliveryManager({
  swUrl: '/sw.js',
  swScope: '/',
  manifestUrl: '/missionhydrosci/api/manifest',
  contentPrefix: '/missionhydrosci/content/',
  channelName: 'missionhydrosci-delivery',
  unitCachePrefix: 'missionhydrosci-unit-',
  fetchIdPrefix: 'missionhydrosci-'
});
```

A future game would use:

```javascript
const delivery = new ContentDeliveryManager({
  swUrl: '/sw.js',
  swScope: '/',
  manifestUrl: '/newgame/api/manifest',
  contentPrefix: '/newgame/content/',
  channelName: 'newgame-delivery',
  unitCachePrefix: 'newgame-unit-',
  fetchIdPrefix: 'newgame-'
});
```

Both share the same service worker (`/sw.js`) and the same `ContentDeliveryManager` class.

#### SW Registration

The manager already registers the SW. Since all features use `/sw.js` with scope `/`, the SW is registered once — subsequent calls to `navigator.serviceWorker.register()` with the same URL are no-ops.

#### Pre-Download Storage Check

`downloadUnit()` must check available storage before starting a download. If insufficient space, broadcast a specific storage error instead of starting a doomed download:

```javascript
ContentDeliveryManager.prototype.downloadUnit = async function(unitId) {
  // Prevent duplicate download of same unit
  if (this._activeDownloads[unitId]) return;

  // Global download lock — one unit at a time
  if (this._downloadInProgress && this._downloadInProgress !== unitId) {
    this._enqueueDownload(unitId);
    return;
  }

  // Network check
  if (!navigator.onLine) {
    this._broadcastLocalStatus(unitId, 'error', {
      error: 'You are offline. Downloads will start when you reconnect.',
      errorType: 'network'
    });
    this._pendingDownload = unitId;
    return;
  }

  // Storage check
  var unit = this.getUnit(unitId);
  if (unit) {
    var estimate = await this.getStorageEstimate();
    if (estimate) {
      var available = estimate.quota - estimate.usage;
      if (unit.totalSize > available) {
        this._broadcastLocalStatus(unitId, 'error', {
          error: 'Not enough storage. Need ' + this._formatMB(unit.totalSize) +
                 ' but only ' + this._formatMB(available) + ' available.' +
                 ' Delete a downloaded unit to free up space.',
          errorType: 'storage',
          needed: unit.totalSize,
          available: available
        });
        return;
      }
    }
  }

  this._downloadInProgress = unitId;
  this._activeDownloads[unitId] = true;

  // ... send download message to SW (existing logic)
};
```

#### Global Download Lock and Queue

Only one unit downloads at a time. Additional download requests are queued and processed sequentially when the current download completes or fails:

```javascript
// Queue a download request and notify UI
ContentDeliveryManager.prototype._enqueueDownload = function(unitId) {
  if (this._downloadQueue.indexOf(unitId) === -1) {
    this._downloadQueue.push(unitId);
    // Broadcast 'queued' so UI can show a "Queued" indicator instead of
    // leaving the user wondering if their click did anything
    this._broadcastLocalStatus(unitId, 'queued', {});
  }
};

// Process next queued download (called when current download completes or errors)
ContentDeliveryManager.prototype._processQueue = function() {
  this._downloadInProgress = null;
  if (this._downloadQueue.length > 0) {
    var next = this._downloadQueue.shift();
    this.downloadUnit(next);
  }
};
```

In the status handler, when a download completes (`'cached'`) or fails (`'error'`):
1. `delete this._activeDownloads[unitId]` — clear the per-unit flag so retries are possible
2. Call `this._processQueue()` — clear the global lock and start the next queued download

On error, the download button for that unit should re-enable (the `_activeDownloads` flag is cleared, so clicking "Download" again will call `downloadUnit()` which will attempt a fresh download). The UI should show the error message alongside a re-enabled download button — no separate "Retry" button needed.

#### Stale Version Cache Cleanup

When the manifest is loaded (during `init()` and `refreshManifest()`), clean up caches from old versions. This reclaims storage that would otherwise be wasted:

```javascript
ContentDeliveryManager.prototype._cleanStaleVersionCaches = async function() {
  if (!this.manifest || !this.manifest.units) return;

  // Build map of current versions
  var currentVersions = {};
  for (var i = 0; i < this.manifest.units.length; i++) {
    var u = this.manifest.units[i];
    currentVersions[u.id] = u.version;
  }

  // Find and delete stale caches
  var allCaches = await caches.keys();
  var prefix = this._unitCachePrefix;
  for (var i = 0; i < allCaches.length; i++) {
    var name = allCaches[i];
    if (!name.startsWith(prefix)) continue;

    // Parse "missionhydrosci-unit-unit1-v2.2.1"
    var suffix = name.substring(prefix.length); // "unit1-v2.2.1"
    var vIdx = suffix.lastIndexOf('-v');
    if (vIdx === -1) continue;

    var unitId = suffix.substring(0, vIdx);
    var cachedVersion = suffix.substring(vIdx + 2);
    var currentVersion = currentVersions[unitId];

    if (currentVersion && cachedVersion !== currentVersion) {
      await caches.delete(name);
    }
  }
};
```

Call `this._cleanStaleVersionCaches()` at the end of `init()` after the manifest is loaded.

#### Network-Aware Behavior

Check `navigator.onLine` before downloads (shown above in `downloadUnit()`). Also listen for the `online` event to auto-retry a pending download:

```javascript
// In init():
var self = this;
window.addEventListener('online', function() {
  if (self._pendingDownload) {
    var unitId = self._pendingDownload;
    self._pendingDownload = null;
    // Broadcast a transitional status so UI clears the offline error
    // and shows downloading state before downloadUnit() does its checks
    self._broadcastLocalStatus(unitId, 'downloading', { percent: 0 });
    self.downloadUnit(unitId);
  }
});
```

#### Error Reporting to Server

The delivery manager should expose a callback for error events so the feature template can report them to the server. The status handler already receives errors — add a hook:

```javascript
// In the status handler, when status === 'error':
if (this._onError) {
  this._onError(unitId, detail.error, detail.errorType);
}
```

The MHS units template registers this callback to POST error data to `/missionhydrosci/api/device-status`:

```javascript
manager.onError(function(unitId, message, errorType) {
  reportDeviceStatus(cacheStatus, {
    error_unit: unitId,
    error_message: message,
    error_type: errorType,
    error_time: new Date().toISOString()
  });
});
```

---

### 6. Install Flow

#### Event Capture at Layout Level

The `beforeinstallprompt` event fires once per page load. If not captured, it's lost. Since the platform manifest is served on every page, this event can fire on any page — not just Mission HydroSci. It must be captured globally.

**Add to `layout.gohtml`** in the `<script>` block:

```javascript
// Capture PWA install prompt globally — features check this to show install UI.
// Dispatches a custom 'pwa-install-available' event so feature page JS that
// already ran can react (beforeinstallprompt fires asynchronously and may
// arrive after feature scripts have checked window.__pwaInstallPrompt).
window.__pwaInstallPrompt = null;
window.addEventListener('beforeinstallprompt', function(e) {
  e.preventDefault();
  window.__pwaInstallPrompt = e;
  window.dispatchEvent(new Event('pwa-install-available'));
});
window.addEventListener('appinstalled', function() {
  window.__pwaInstallPrompt = null;
});
```

#### Feature-Opt-In Install Banners

Features that benefit from PWA installation include their own install banner UI. The banner checks `window.__pwaInstallPrompt` and shows/hides accordingly. No per-feature manifest override needed.

**Mission HydroSci's install banner** (`missionhydrosci_units.gohtml`):
- Update the banner text: change "Install Mission HydroSci for the best experience" to "Install this app for the best experience" (the installed app will use the workspace name and icon from settings — the banner should not promise a different name/icon than what the OS will show)
- Remove the `{{ define "manifest" }}` block override
- Remove the `beforeinstallprompt` event listener from the template JS
- Check `window.__pwaInstallPrompt` on page load AND listen for the layout's custom event (the `beforeinstallprompt` event fires asynchronously and may arrive after page JS has already run):

```javascript
// Show install banner — handles both "event already fired" and "event fires later"
function maybeShowInstallBanner() {
  if (window.__pwaInstallPrompt && !sessionStorage.getItem('missionhydrosci-install-dismissed')) {
    document.getElementById('install-banner').classList.remove('hidden');
  }
}

// Check immediately (in case event already fired before this script ran)
maybeShowInstallBanner();

// Also listen for late-arriving event (layout dispatches this after capturing)
window.addEventListener('pwa-install-available', maybeShowInstallBanner);

function mhsInstall() {
  if (window.__pwaInstallPrompt) {
    window.__pwaInstallPrompt.prompt();
    window.__pwaInstallPrompt.userChoice.then(function(result) {
      window.__pwaInstallPrompt = null;
      document.getElementById('install-banner').classList.add('hidden');
    });
  }
}
```

- The iOS install banner (manual "Add to Home Screen" instructions) stays as-is — iOS doesn't support `beforeinstallprompt`

**Other features** don't show an install banner. The app is already installable from any page via Chrome's address bar install icon — the banner is just a convenience for the game feature where PWA install is most valuable.

---

### 7. Download Strategy

#### Current Bugs (Early Download Never Works)

The login action has **three** bugs that prevent early download from ever working:

1. **Wrong scope:** Registers the SW with `scope: '/missionhydrosci/'`, but the user lands on `/` (welcome page) after login — outside that scope. So `navigator.serviceWorker.controller` is `null`.
2. **Wrong message format:** Sends `{type: 'DOWNLOAD_UNIT', ...}` but the SW expects `{action: 'download', ...}`. The SW's message handler checks `data.action` and ignores messages with `data.type`.
3. **Wrong manifest access:** Accesses `manifest[progress.current_unit]` (treating manifest as a map) but the manifest API returns `{units: [...], cdnBaseUrl: "..."}` (units is an array, not a map).

Downloads only start when the user visits `/missionhydrosci/units` (the template JS and delivery manager handle it there via a completely different code path).

#### Fix

With the platform SW at `scope: '/'`, the SW controls every page on the origin. The login action's `navigator.serviceWorker.controller` will no longer be `null` after registration, so early download will actually work.

#### When Downloads Start

| Trigger | Who | What happens |
|---|---|---|
| **Login** | Members with MHS assigned (`missionhydrosci` in enabled apps) | Login action registers SW, fetches progress, starts downloading current unit |
| **Login** | Non-members (admin, coordinator, leader) | Same — login action always runs for non-member roles |
| **Visit MHS units page** | Any role with access | Template JS checks cache status, starts downloading current unit if not cached, auto-downloads next unit if current is cached and storage permits |
| **Manual download** | Any role | User clicks download button for a specific unit |

#### Login Action Changes

**`bootstrap/routes.go`** — update the `missionhydrosci-early-download` login action:

1. Register `/sw.js` with `scope: '/'` instead of `/missionhydrosci-sw.js` with `scope: '/missionhydrosci/'`
2. Use `navigator.serviceWorker.ready` then `reg.active.postMessage()` (not `navigator.serviceWorker.controller` which can be null on first registration)
3. Check storage before sending the download message
4. Include the workspace icon URL in the download message for Background Fetch notifications

```javascript
navigator.serviceWorker.register('/sw.js', { scope: '/' })
  .then(function() { return navigator.serviceWorker.ready; })
  .then(function(reg) {
    return fetch('/missionhydrosci/api/progress')
      .then(function(resp) { return resp.json(); })
      .then(function(progress) {
        if (!progress.current_unit || progress.current_unit === 'complete') return;

        return fetch('/missionhydrosci/api/manifest')
          .then(function(mr) { return mr.json(); })
          .then(function(manifest) {
            var unit = manifest.units
              ? manifest.units.find(function(u) { return u.id === progress.current_unit; })
              : null;
            if (!unit || !reg.active) return;

            var sendDownload = function() {
              reg.active.postMessage({
                action: 'download',
                unitId: unit.id,
                version: unit.version,
                files: unit.files,
                cdnBaseUrl: manifest.cdnBaseUrl,
                title: unit.title,
                icon: '/pwa/icon-192.png'
              });
            };

            // Check storage before downloading
            if (navigator.storage && navigator.storage.estimate) {
              return navigator.storage.estimate().then(function(est) {
                var available = est.quota - est.usage;
                if (unit.totalSize > available) return; // Not enough space — skip
                sendDownload();
              });
            } else {
              // No storage API — try anyway
              sendDownload();
            }
          });
      });
  })
  .catch(function(e) { console.warn('MHS early download:', e); });
```

#### Auto-Download Pipeline (on MHS units page)

The pipeline in `missionhydrosci_units.gohtml` manages automatic downloading. Updated behavior:

1. On page load, check cache status for all units
2. Clean stale version caches (via delivery manager `_cleanStaleVersionCaches()`)
3. If current unit is not cached → check storage → start downloading (via `downloadUnit()` which now includes storage and online checks)
4. When current unit finishes → if storage usage < 90% of quota → auto-download next unit
5. Auto-cleanup: remove cached units not in {current, next, manually-selected}, respecting cross-tab safety (SW checks for active play pages before deleting)

The storage check for the current unit is now handled inside `downloadUnit()` (see Section 5), so the pipeline doesn't need separate logic — it just calls `downloadUnit()` and the manager handles the rest.

#### What About Users Who Don't Have MHS?

The `ShouldRun` check on the login action already handles this:

- **Members:** only runs if `missionhydrosci` is in their `EnabledApps` (assigned via Groups > Manage > Apps)
- **Non-members:** always runs (admin, coordinator, leader always have MHS in their menu)

If a workspace doesn't use Mission HydroSci at all, no members will have it in their enabled apps and non-members won't have it in their menu — the login action still runs for non-members but the `/missionhydrosci/api/progress` fetch will return no current unit, so no download starts.

#### Two Download Paths

There are two ways a download starts, and both must be maintained:

1. **Client-initiated (delivery manager):** The units page JS calls `reg.backgroundFetch.fetch()` directly from the client. This is the primary path — it handles reconnecting to existing fetches, progress tracking, stall detection, and fallback to SW sequential fetch. Used when the user is on the units page.

2. **SW-initiated (login action):** The login action sends `{action: 'download'}` via `reg.active.postMessage()`. The SW receives this and calls its own `startBackgroundFetch()`. This is the early-download path — it starts a download before the user reaches the units page. The delivery manager on the units page will reconnect to this in-progress fetch when the user navigates there.

Both paths ultimately use the same Background Fetch API and the same cache naming. The SW's `backgroundfetchsuccess` handler stores files in the cache and broadcasts completion status regardless of which path initiated the fetch.

#### Future: Per-Feature Login Actions

When a second game is added, it would register its own login action following the same pattern. Each feature's login action independently checks whether the user should get early downloads and talks to its own progress/manifest API. The platform SW handles the download mechanics via the feature registry.

---

### 8. mhsdelivery Feature Changes

The mhsdelivery feature (`/mhs/*`) continues to exist as a route for students currently using it. Changes:

- **Remove PWA infrastructure:** Delete `manifest.go`, `sw.go`, and `static/` directory from mhsdelivery
- **Remove from bootstrap/routes.go:** The root-level routes for `/sw.js` and `/manifest.json` that point to mhsdelivery
- **Keep routes:** `/mhs/units`, `/mhs/play/{unit}`, `/mhs/api/manifest` remain functional
- **Templates:** Remove the `beforeinstallprompt` install banner from `mhs_units.gohtml` (the platform install flow replaces it, or mhsdelivery simply doesn't offer PWA install anymore)

Students accessing `/mhs/units` in the browser continue to work. They just can't install a separate PWA for it (which is the desired behavior).

---

### 9. missionhydrosci Feature Changes

- **Remove:** `manifest.go`, `sw.go`, `static/sw*.js` files — PWA infrastructure moves to `internal/app/pwa/`
- **Remove:** `{{ define "manifest" }}` block override in `missionhydrosci_units.gohtml`
- **Keep:** All routes, templates, game logic, progress tracking, device status
- **Keep:** Content manifest (`mhs_content_manifest.json`, `api_manifest.go`)
- **Keep:** Content fallback (`content.go`)
- **Update:** Templates to use platform SW and delivery manager config
- **Update:** Install banner references (if any) to use platform manifest

#### Game Launch Cache Pre-Check (play page)

The play page (`missionhydrosci_play.gohtml`) must verify the unit is cached before attempting to load the Unity player. Without this, a user who reaches the play page via deep link, bookmark, or after cache eviction will see a broken Unity loader or a generic error.

**Add to the play page JS, before `createUnityInstance()`:**

The existing loading overlay ("Loading Unit Title...") should be visible by default. The cache pre-check runs during this loading state — no layout shift or blank screen.

```javascript
// Check if unit files are cached before loading Unity.
// The loading overlay is already visible at this point.
// Use caches.has() — NOT caches.open() — to avoid creating an empty cache.
var cacheName = 'missionhydrosci-unit-{{ .UnitID }}-v{{ .UnitVersion }}';
caches.has(cacheName).then(function(exists) {
  if (!exists) {
    if (!navigator.onLine) {
      // Offline + not cached — can't play, show specific guidance
      document.getElementById('unity-container').classList.add('hidden');
      var msg = document.getElementById('not-downloaded-msg');
      msg.classList.remove('hidden');
      msg.querySelector('.msg-text').textContent =
        'This unit is not available offline. Connect to the internet and download it from the units page.';
      return;
    }
    // Online + not cached — let it load from CDN (SW falls through to backend).
    // This is slower but works. The user can still play.
    loadUnityPlayer();
    return;
  }
  // Cache exists — but verify it has files (could be partial/empty)
  caches.open(cacheName).then(function(cache) {
    return cache.keys();
  }).then(function(keys) {
    if (keys.length === 0 && !navigator.onLine) {
      // Empty cache + offline — can't play
      document.getElementById('unity-container').classList.add('hidden');
      var msg = document.getElementById('not-downloaded-msg');
      msg.classList.remove('hidden');
      msg.querySelector('.msg-text').textContent =
        'This unit is not available offline. Connect to the internet and download it from the units page.';
      return;
    }
    // Cached (or online with empty cache — CDN fallback will work)
    loadUnityPlayer();
  });
});
```

**Add HTML to the play template:**

```html
<div id="not-downloaded-msg" class="hidden flex flex-col items-center justify-center h-full gap-4 p-8 text-center">
  <p class="msg-text text-lg text-gray-700"></p>
  <a href="/missionhydrosci/units" class="btn btn-primary">Go to Units</a>
</div>
```

**Key UX decision:** Only block play when **offline and not cached**. When online, always allow play — the CDN fallback works (just slower). This avoids breaking the flow for users who reach the play page via a link, bookmark, or after cache eviction. The current "Failed to load game" error only appears when offline and not cached; this improves it with a clear message and a link to the units page.

#### Device Status Error Reporting

Update `device_status.go` to accept error data in status reports:

```go
// Add to the device status request struct:
ErrorUnit    string `json:"error_unit,omitempty"`
ErrorMessage string `json:"error_message,omitempty"`
ErrorTime    string `json:"error_time,omitempty"`
```

The units page JS reports errors when downloads fail:

```javascript
// In the status broadcast handler:
if (status === 'error') {
  reportDeviceStatus(cacheStatus, {
    error_unit: unitId,
    error_message: detail.error || 'unknown',
    error_time: new Date().toISOString()
  });
}
```

This enables the MHS Dashboard to surface which students are having download problems and why — critical for teachers managing a classroom of Chromebooks.

---

## New File Structure

```
internal/app/pwa/
  handler.go          — Handler struct (DB, Storage, Logger, CDNBaseURL, features config)
  manifest.go         — GET /manifest.json (workspace-aware dynamic manifest)
  sw.go               — GET /sw.js (concatenates SW files with feature registry)
  icons.go            — GET /pwa/icon-192.png, /pwa/icon-512.png (proxy from storage)
  offline.go          — GET /offline (generic offline fallback page)
  features.go         — Feature registry type and registration
  static/
    sw.js             — Main SW fetch handler (feature-aware routing)
    sw-cache.js       — Cache utilities (feature-aware naming)
    sw-background-fetch.js — Background Fetch (feature-aware routing)
  templates/
    offline.gohtml    — Generic offline fallback page
```

## Files Removed

```
internal/app/features/mhsdelivery/
  manifest.go         — replaced by pwa/manifest.go
  sw.go               — replaced by pwa/sw.go
  static/
    sw.js             — replaced by pwa/static/sw.js
    sw-cache.js       — replaced by pwa/static/sw-cache.js
    sw-background-fetch.js — replaced by pwa/static/sw-background-fetch.js

internal/app/features/missionhydrosci/
  manifest.go         — replaced by pwa/manifest.go
  sw.go               — replaced by pwa/sw.go
  static/
    sw.js             — replaced by pwa/static/sw.js
    sw-cache.js       — replaced by pwa/static/sw-cache.js
    sw-background-fetch.js — replaced by pwa/static/sw-background-fetch.js

internal/app/features/missionhydroscix/  — already removed
```

## Files Modified

```
internal/domain/models/sitesettings.go          — add PWAIconPath, PWAIconName
internal/app/features/workspaces/settings.go    — add PWA icon upload handling
internal/app/features/workspaces/templates/workspace_settings.gohtml — add PWA icon UI
internal/app/bootstrap/routes.go                — swap feature PWA routes for platform routes,
                                                  update login action (SW URL, scope, postMessage,
                                                  storage check)
internal/app/features/missionhydrosci/templates/missionhydrosci_units.gohtml
    — remove manifest block override, update install banner to use global
      beforeinstallprompt event, add storage checks to auto-download pipeline,
      add cross-tab safety to auto-cleanup
internal/app/features/missionhydrosci/templates/missionhydrosci_play.gohtml
    — add cache status pre-check before loading Unity player
internal/app/features/mhsdelivery/templates/mhs_units.gohtml
    — remove install banner
internal/app/resources/assets/js/mhs-delivery.js
    — rename to content-delivery.js (optional), add pre-download storage check,
      add global download lock, add stale version cache cleanup,
      add navigator.onLine check, add error categorization,
      add error reporting to device status
internal/app/resources/templates/layout.gohtml
    — add global beforeinstallprompt capture in layout JS,
      add old SW unregistration cleanup (for already-logged-in users
      who won't trigger the login action)
internal/app/features/missionhydrosci/device_status.go
    — accept error data in device status reports
```

## Files Also Removed (Cleanup)

```
internal/app/features/mhsdelivery/templates/mhs_offline.gohtml
    — replaced by pwa/templates/offline.gohtml (generic offline page)
internal/app/features/missionhydrosci/templates/missionhydrosci_offline.gohtml
    — replaced by pwa/templates/offline.gohtml (generic offline page)
```

Note: These offline templates exist in the codebase but are **not currently routed** — they're only referenced by the feature SWs' inline offline HTML. The platform SW replaces this with a pre-cached `/offline` page served by the pwa handler.

## Implementation Order

### Phase 1: Platform PWA Package

1. Create `internal/app/pwa/` package
2. Implement feature registry type
3. Port service worker files from missionhydrosci, generalizing hardcoded prefixes to use the feature registry
4. Implement `sw.go` — serve concatenated SW with injected feature registry
5. Implement `manifest.go` — workspace-aware dynamic manifest (use existing icons as default)
6. Implement `icons.go` — serve PWA icons from storage (with default fallback)

### Phase 2: Workspace Icon Settings

7. Add `PWAIconPath`/`PWAIconName` to SiteSettings model
8. Add upload handling to workspace settings handler
9. Add UI to workspace settings template
10. Wire icon serving in `pwa/icons.go` to read from storage

### Phase 3: Wire Up Routes

11. Add platform PWA routes to `bootstrap/routes.go`
12. Remove mhsdelivery PWA routes (manifest, SW)
13. Remove missionhydrosci PWA routes (manifest, SW)
14. Register missionhydrosci as a feature in the platform SW registry
15. Update login action to register `/sw.js` instead of `/missionhydrosci-sw.js`
16. Add old SW unregistration logic to login action / layout JS

### Phase 4: Template Cleanup

17. Remove `{{ define "manifest" }}` override from `missionhydrosci_units.gohtml`
18. Update `missionhydrosci_units.gohtml` install banner (if needed — it may work as-is since `beforeinstallprompt` fires from the platform manifest)
19. Remove install banner from `mhs_units.gohtml`
20. Remove `manifest.go`, `sw.go`, and `static/` from mhsdelivery feature
21. Remove `manifest.go`, `sw.go`, and `static/` from missionhydrosci feature
22. Remove SW registration from `missionhydrosci_play.gohtml` (currently registers old `/missionhydrosci-sw.js`)

### Phase 5: Download Robustness (Delivery Manager + SW)

23. Add pre-download storage check in `downloadUnit()` — reject with specific error if insufficient space
24. Add `QuotaExceededError` detection in fallback fetch — show storage-specific error message
25. Categorize download errors (storage, network, HTTP, CORS) with distinct user messages
26. Add partial cache cleanup on download failure — delete the cache to reclaim storage
27. Add global download lock — one unit at a time, queue additional requests
28. Add stale version cache cleanup — delete old version caches when manifest loads
29. Add auto-cleanup cross-tab safety — check for active play pages before deleting caches
30. Add `navigator.onLine` check — prevent downloads when offline, auto-retry on reconnect
31. Add storage check to login action early download
32. Add storage check to auto-download pipeline for current unit (not just next unit)

### Phase 6: Game Launch Safety

33. Add cache status pre-check on play page — use `caches.has()`, only block play when **offline + not cached** (online users can still play via CDN fallback), show clear guidance when blocked
34. Ensure loading overlay is visible during cache check (no blank screen or layout shift)

### Phase 7: Device Status Improvements

35. Report download errors to server (unit, error type, error message, timestamp)
36. Include error data in MHS Dashboard for teacher visibility

### Phase 8: Verification

37. Test PWA install from welcome page — should install as workspace name with workspace icon
38. Test PWA install from Mission HydroSci — same manifest, same app
39. Test unit download and caching in PWA mode
40. Test unit download and caching in browser mode
41. Test on Chromebook — no more navigation overlay issue
42. Test `/mhs/units` still works in browser (no PWA install offered)
43. Test offline play
44. Test login action early download triggers on login
45. Test storage-full scenario — verify specific error message, no partial download left behind
46. Test download with lid-close/resume — verify stall detection recovers
47. Test version upgrade — verify old caches are cleaned, new version downloads
48. Test simultaneous download attempts — verify queuing works
49. Test playing in one tab while units page is in another — verify auto-cleanup doesn't delete active unit
50. Test offline game launch without cache — verify clear "not available offline" message with link to units page
51. Test online game launch without cache — verify game loads from CDN (no blocking)
52. Test guest mode Chromebook — login, download, play, sign out, sign back in
53. Test old SW unregistration — install old PWA, deploy new code, verify old registration is removed
54. Test failed download cleanup — interrupt a download, verify partial cache is deleted
55. Test download queue — click download on two units, verify first downloads and second shows "Queued"
56. Test install banner timing — verify banner appears on MHS units page (even if beforeinstallprompt fires late)
57. Test PWA opens to MHS units page (not welcome page)
58. Test offline page — verify "Try Again" button reloads, no dead-end links

## Review Findings and Corrections

The following issues were found during critical review and must be addressed during implementation.

### Manifest Cache Busting (Corrected)

The original plan called for `?v={hash}` on the manifest `<link>` tag, with the hash computed from workspace data. This is impractical because the hash would need to vary per workspace per request.

**Corrected approach:** Serve `/manifest.json` with `Cache-Control: no-cache, must-revalidate` (same strategy as the SW). Chrome re-fetches the manifest on its own schedule (typically every 24-48 hours and on every PWA launch). No query-string hash needed. Remove the `{{ block "manifest" }}` override approach entirely — the layout default `<link rel="manifest" href="/manifest.json">` is sufficient.

### Workspace Context Is Available on Root Routes

The workspace middleware runs globally (bootstrap/routes.go line 158), so the `/manifest.json` handler **can** use `workspace.FromRequest(r)` and `workspace.IDFromRequest(r)` to load workspace-specific settings. This is confirmed.

**Edge case — apex domain:** On `stratahub.adroit.games` (apex), `workspace.IDFromRequest()` returns `NilObjectID`. The manifest handler must return a generic fallback manifest (default name, default icon) for apex requests. Apex is only used by superadmins for workspace management, so this is low priority but must not error.

### Offline Fallback Must Be Feature-Agnostic

The current SW hardcodes `/missionhydrosci/units` in the inline offline HTML. A platform SW cannot know which feature the user was accessing.

**Fix:** Pre-cache a generic offline page (e.g., `/offline`) during SW install. This page shows "You are offline" with a link to `/` (the home page). Each feature can also pre-cache its own pages (MHS already caches `/missionhydrosci/units`) and the SW checks feature caches first before falling back to the generic page.

### Background Fetch Notification Icon

Integrated into Section 3 (Platform Service Worker > sw-background-fetch.js) — icon URL passed from client in the download message payload, with default fallback.

### Login Action: Use `reg.active.postMessage()` Not `navigator.serviceWorker.controller`

Integrated into Section 7 (Download Strategy > Login Action Changes) — uses `reg.active.postMessage()` with storage check and icon URL.

### Platform SW Must Not Interfere with Non-Feature Pages

Integrated into Section 3 (Platform Service Worker) — explicit design rule: unrecognized paths pass through to the network with no interception.

### `beforeinstallprompt` Must Be Captured at Layout Level

Integrated into Section 6 (Install Flow) — event captured in `layout.gohtml`, features check `window.__pwaInstallPrompt`.

---

## Chromebook-Specific Considerations

### Power/Sleep During Download

Closing a Chromebook lid suspends the device. Background Fetch is OS-level and should survive sleep, but if the device reconnects to a different network or the connection drops, the fetch may stall.

The existing stall detection (15-second checks, 30-second timeout, fallback to sequential fetch) handles this. This behavior must carry forward to the platform SW unchanged.

### Shared Chromebooks

School Chromebooks are often shared between students.

- **Guest mode:** All storage (Cache API, localStorage, SW registration) is wiped when the user signs out of ChromeOS. Every login starts fresh — early download on login is critical for these users.
- **Managed profiles:** Each ChromeOS user profile has its own browser storage. Downloads persist between StrataHub sessions. Early download on login still helps (downloads begin before the student navigates to MHS).

No code changes needed, but this reinforces the importance of the login-action early download working correctly.

### Storage Quota

Chromebook storage varies by device (typically 500MB–2GB available for web storage). See the "Storage and Download Robustness" section below for specific issues and fixes.

### Multiple Workspaces = Multiple Origins

Each workspace subdomain (e.g., `mhs.adroit.games`, `demo.adroit.games`) is a separate origin. Each gets its own PWA install, service worker, and cache storage. No cross-workspace interference is possible. This is correct by design and needs no changes.

### BroadcastChannel Across Browser Tab + PWA Window

If a user has both a browser tab and the installed PWA open on `/missionhydrosci/units`, both windows receive download status broadcasts. This could cause duplicate progress UI updates.

This is an existing issue, not introduced by the platform SW change. It's low severity (the UI updates are idempotent — showing the same progress in both windows is harmless). If it becomes a problem in the future, each window could filter broadcasts by including a `clientId` in messages.

---

## Storage and Download Robustness

The current download system has several gaps that can cause silent failures, wasted bandwidth, or confusing experiences — especially on storage-constrained Chromebooks. These are addressed in the architecture sections above. This section documents the current problems and maps each to its fix.

| Problem | Current Behavior | Fix Location |
|---|---|---|
| No space check before current-unit auto-download | Auto-download pipeline starts download regardless of space | Section 5: `downloadUnit()` pre-check, Section 7: auto-download pipeline |
| No space check in login action | Login action sends download with zero storage check | Section 7: login action code |
| Partial cache left after failed download | QuotaExceededError or network failure leaves partial cache wasting storage | Section 3: `fallbackFetch()` error handler deletes partial cache |
| Generic "Download failed" error message | All errors show same message regardless of cause | Section 3: error categorization (storage, network, HTTP, CORS) |
| Multiple simultaneous downloads | Users can start parallel downloads competing for bandwidth/storage | Section 5: global download lock and sequential queue |
| Auto-cleanup deletes actively-played unit | Cleanup can delete a unit while it's being played in another tab | Section 3: `safeDeleteUnit()` cross-tab check |
| Stale version caches waste storage | Old version caches (e.g., v2.2.1) persist after manifest update | Section 5: `_cleanStaleVersionCaches()` |
| No `navigator.onLine` check | Downloads start while offline and fail silently | Section 5: `downloadUnit()` online check, auto-retry on reconnect |
| Game launch without cached files | Play page loads Unity which falls through to CDN or fails offline | Section 9: cache pre-check — block only when offline+not cached; allow CDN fallback when online |
| Download errors not reported to server | Only success events tracked; teachers can't see who is stuck | Section 5: error callback, Section 9: device status error reporting |

### Error Message Reference

| Error type | Message to user |
|---|---|
| `QuotaExceededError` | "Not enough storage on this device. Try clearing old downloads or freeing space." |
| Network error (`TypeError: Failed to fetch`) | "Download failed — check your internet connection." |
| HTTP error (4xx/5xx) | "Download failed — the server returned an error. Try again later." |
| CORS error | "Download failed — configuration error. Contact your teacher." |
| Background Fetch failure | "Download was interrupted. Tap Retry to continue." |
| Insufficient space (pre-check) | "Not enough storage. Need X MB but only Y MB available. Delete a downloaded unit to free up space." |
| Offline | "You are offline. Downloads will start when you reconnect." |
| Queued (another download active) | UI shows "Queued" indicator on the unit card |

### Summary of Robustness Fixes

| Issue | Priority | Fix |
|---|---|---|
| No space check before current-unit download | High | Check storage in `downloadUnit()` and pipeline |
| No space check in login action | High | Check storage before sending download message |
| Generic error messages | High | Categorize errors (storage, network, CORS) |
| Stale version caches not cleaned | High | Clean old version caches on manifest load |
| Auto-cleanup can delete playing unit | High | Check for active play pages before deleting |
| Multiple simultaneous downloads | Medium | Global download lock, sequential queue |
| No `QuotaExceededError` handling | Medium | Detect and show specific storage message |
| No `navigator.onLine` check | Medium | Check before download, auto-retry on reconnect |
| Game launch without cache | Medium | Pre-check cache status on play page |
| Error events not reported to server | Medium | Include errors in device status reports |

---

## Migration Notes

### Migration Deploy Order

To minimize confusion when the installed PWA's name and icon change, consider this deploy order:
1. Deploy workspace icon settings (Phase 2) and have admins set recognizable PWA icons **before** switching to the platform manifest
2. Deploy the platform manifest and SW changes — users' installed apps update to the workspace name/icon they've already been configured with

If the switch happens before icons are configured, students will see a generic default icon and the workspace name (instead of "Mission HydroSci") — recognizable but potentially confusing.

### Existing PWA Installations

Users who have the old mhsdelivery or missionhydrosci PWA installed:

- Their installed app will pick up the new `/manifest.json` on next launch (Chrome checks manifests periodically)
- The name and icon will update to the workspace branding
- The new `/sw.js` will be registered by the login action or by visiting any feature page
- **Old SW registrations must be explicitly unregistered.** Chrome uses the most specific matching scope — the old `/missionhydrosci-sw.js` registration at `scope: '/missionhydrosci/'` would take precedence over the new `/sw.js` at `scope: '/'` for all pages under `/missionhydrosci/*`. A 404 on update checks does NOT unregister the old SW; Chrome keeps it active. Fix: unregister old registrations during the new SW's activate event (see below)
- Old app shell caches (`missionhydrosci-app-shell-v5`, `mhs-app-shell-v1`) should be cleaned up by the new SW's activate handler to reclaim storage

#### Old SW Unregistration

Add to the platform SW's `activate` handler:

```javascript
// Clean up old feature-specific SW registrations
self.addEventListener('activate', function(event) {
  event.waitUntil(
    self.registration.unregister !== undefined
      ? Promise.resolve() // We ARE the active registration — can't unregister ourselves
      : Promise.resolve()
  );
  // The real cleanup happens client-side (see below)
});
```

The SW can't unregister other registrations — that must happen from the page. Add cleanup to the login action or layout JS:

```javascript
// After registering the new platform SW, unregister any old feature SWs
if ('serviceWorker' in navigator) {
  navigator.serviceWorker.getRegistrations().then(function(registrations) {
    registrations.forEach(function(reg) {
      // Keep the platform SW, unregister everything else
      if (reg.active && reg.active.scriptURL && !reg.active.scriptURL.endsWith('/sw.js')) {
        reg.unregister();
      }
    });
  });
}
```

This runs once per page load. After the old registrations are gone, this is a no-op.

### Cache Naming

To avoid conflicts with old caches:

- New app shell cache: `stratahub-app-shell-v1` (distinct from `missionhydrosci-app-shell-v5` and `mhs-app-shell-v1`)
- Feature unit caches can keep the same prefix (`missionhydrosci-unit-{unitId}-v{version}`) for continuity — this means existing cached units don't need to be re-downloaded

**Decision:** Keep `missionhydrosci-unit-` as the cache prefix for Mission HydroSci units. The new platform SW reads these caches using the feature registry configuration. Users with already-cached units don't need to re-download.

### localStorage Keys

Existing localStorage keys (`mhs-device-id`, `mhs-manual-downloads`, `mhs-progress-queue`, install dismissal flags) continue to work — they're read by the feature templates, not the SW infrastructure.
