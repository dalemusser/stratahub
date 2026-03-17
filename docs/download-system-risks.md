# MHS Download System — Remaining Risk Areas

This document covers known risks to the MHS unit download system (Background Fetch + Service Worker + Cache API) that could cause download failures for students.

## Resolved Risks

### CloudFront CORS Cache Poisoning (Fixed 2026-03-01)

**Problem:** CloudFront cached CDN responses without differentiating CORS vs non-CORS requests. When a non-CORS request (e.g., a `<script>` tag load, direct URL visit) was cached first, subsequent CORS requests (Background Fetch, `fetch()` with `mode: 'cors'`) received the cached response without `Access-Control-Allow-Origin` headers, causing the download to fail.

**Root cause:** The CloudFront cache policy (`CachingOptimized`) did not include the `Origin` header in the cache key.

**Fix:** Created a custom cache policy (`CachingOptimized-CORS`) that includes `Origin` in the cache key, so CORS and non-CORS responses are cached separately. Applied to both `cdn.adroit.games` and `test.adroit.games` distributions.

**How it was triggered:** Playing a unit without downloading first loads `loader.js` via a `<script>` tag through the content fallback (302 redirect to CDN). Script tags don't send the `Origin` header, so CloudFront cached the S3 response without CORS headers. Any subsequent Background Fetch download of that unit then failed because it received the cached non-CORS response.

### iPad Safari Fullscreen Suppresses Letter Keys (Mitigated 2026-03-01)

**Problem:** iPad Safari in fullscreen mode completely suppresses all letter key DOM events (`keydown`, `keyup`, `keypress`, `input`) for keys A-Z. Non-letter keys (arrow keys, spacebar, tab) work normally. This is a Safari/WebKit limitation — the events never enter the DOM, so no JavaScript workaround exists.

**Impact:** WASD movement and M (map) keys don't work in fullscreen on iPad, making the game unplayable in that mode.

**Diagnosis:** A capture-phase `keydown` listener on `document` receives events for spacebar, arrows, and tab in fullscreen, but zero events for any letter key. A hidden `<input>` element also receives no `input` events for letters (only punctuation). This confirms Safari is consuming letter keystrokes before they reach the page.

**Mitigation:** The fullscreen button is hidden on iPad. The play page already fills the viewport, so fullscreen only hides Safari's toolbar. For a fully chrome-free experience, users can install the PWA (Add to Home Screen), which runs without Safari UI and does not have this keyboard limitation.

**Applies to:** `missionhydrosci_play.gohtml`.

### Service Worker Scope Conflict (Fixed 2026-03-01)

**Problem:** Both MHS Units (now removed) and Mission HydroSci registered service workers at `scope: '/'`. Only one SW can be active per scope, so one would replace the other, causing cache key mismatches and download failures.

**Fix:** Mission HydroSci SW uses `scope: '/missionhydrosci/'`. MHS Units has been removed.

---

## Active Risks

### 1. No Per-File Retry on Download Failure

**Impact:** High
**Likelihood:** Medium (depends on network quality)

Each unit has 15 files. If any single file fails during Background Fetch, the entire unit download is marked as failed. The student must retry the full download (100-200MB), not just the failed file. On unreliable school Wi-Fi networks, large units (especially Unit 2 at ~212MB) may fail repeatedly.

**Symptoms:** "Download failed. Please check your connection and try again." after partial progress.

**Mitigation options:**
- Implement per-file retry logic (retry failed files 2-3 times before marking the unit as failed)
- Implement resume capability that detects partially-cached units and only downloads missing files
- Show which specific file failed in the error message for debugging

### 2. Safari/iPad Has No Background Fetch API

**Impact:** High
**Likelihood:** Certain (affects all Safari users)

Safari does not support the Background Fetch API. The system falls back to sequential `fetch()` calls inside the service worker. This has several problems:

- **Memory pressure:** The ~199MB Unit 2 data file must be fetched and written to cache entirely through the SW. If the browser kills the SW due to memory pressure, the download fails silently.
- **No background execution:** If the student switches tabs or locks the iPad, the SW may be suspended, killing the in-progress download.
- **No progress notification:** Unlike Background Fetch (which shows a system-level download indicator), the fallback only shows progress while the page is open and active.

**Symptoms:** "Load failed" error on iPad. Downloads that seem to stall or fail when the student navigates away.

**Mitigation options:**
- Show a warning on iPad/Safari that the page must stay open during download
- Chunk large files or implement streaming cache writes to reduce memory pressure
- Consider a download approach that doesn't require the SW for Safari (e.g., direct fetch from the page context with manual cache writes)

### 3. Chromebook Storage Limits

**Impact:** High
**Likelihood:** Medium (depends on device and usage)

All 5 units total ~700MB. Many school Chromebooks have 16-32GB total storage, shared between the OS, user files, Android apps, and browser storage. If a student's device is low on space, the download will fail with a generic error — there's no pre-flight storage check or helpful message.

**Symptoms:** Download fails with a generic error. No indication that storage is the problem. Student may retry repeatedly.

**Mitigation options:**
- Before starting a download, check `navigator.storage.estimate()` and compare available quota against the unit's `totalSize`. Show a clear message if there isn't enough space.
- Suggest clearing other units to free space (e.g., "Not enough storage. Try clearing Unit 1 to free 104MB.")
- Show current storage usage more prominently on the units page

### 4. School IT Policies Clearing Browser Data

**Impact:** High
**Likelihood:** Varies by district

Some school districts configure Chromebooks to wipe browser data (cookies, cache, service worker registrations) on logout or restart. This means students lose all downloaded units and must re-download them each session. On a slow school network with 30 students downloading simultaneously, this could consume significant bandwidth and class time.

**Symptoms:** All units show "Not downloaded" every time the student logs in, despite having downloaded them previously.

**Mitigation options:**
- Document this as a requirement for IT administrators: MHS requires persistent browser storage
- Request persistent storage via `navigator.storage.persist()` to signal to the browser that this data should not be evicted
- Consider whether units could be pre-loaded on managed Chromebooks via IT policy (e.g., a Chrome extension that pre-populates the cache)
- Track download frequency server-side to identify students who are re-downloading excessively (indicating data is being cleared)

### 5. Service Worker Registration Failure

**Impact:** High
**Likelihood:** Low

If the service worker fails to register (browser restrictions, IT policy blocking SW, private/incognito browsing, browser bugs), the entire download and caching system is unavailable. Units cannot be downloaded for offline use, and the play page falls back to streaming from CDN on every launch.

**Symptoms:** "Service worker not available" in the status area. Download buttons may not appear or may fail immediately.

**Mitigation options:**
- Detect SW registration failure and show a clear message explaining the requirement
- Provide a direct-play fallback that streams from CDN without requiring SW/cache (works but requires network for every play session)

### 6. Manifest API Failure

**Impact:** Medium
**Likelihood:** Low

If `/missionhydrosci/api/manifest` is unreachable when the units page loads (server restart, network issue, authentication expiry), the delivery manager cannot load the unit list. The page shows "Checking..." indefinitely for all units with no error state or retry.

**Symptoms:** All unit cards stuck on "Checking..." forever. No download or play buttons appear.

**Mitigation options:**
- Add a timeout and error state for manifest loading (e.g., "Failed to load unit list. Tap to retry.")
- Cache the manifest in `localStorage` as a fallback so previously-seen units can still show their cached status
- Add a retry button or automatic retry with backoff

### 7. CDN or S3 Outage

**Impact:** High
**Likelihood:** Low

If CloudFront or S3 is unavailable, no new downloads can complete. Units already cached locally will continue to work (the SW serves them from cache), but any unit not yet downloaded will fail.

**Symptoms:** Download progress may stall or fail. Play works for cached units but not uncached ones.

**Mitigation options:**
- This is largely outside our control, but ensuring units are downloaded before class (not during) reduces the impact
- The existing offline-first architecture (SW cache-first for content) already handles this well for previously-downloaded units
- Consider a status endpoint that checks CDN health and warns teachers if downloads are currently unavailable

### 8. Play-Without-Download Streams from CDN Every Time

**Impact:** Medium
**Likelihood:** Medium

If a student clicks Play on a unit that hasn't been downloaded, the play page loads directly from CDN via the 302 content fallback. This works but downloads 100-200MB every session with no local caching benefit. On a shared school network, multiple students doing this simultaneously could saturate bandwidth.

**Symptoms:** Slow load times on the play page. No error, but poor performance. Students may not realize they should download first.

**Mitigation options:**
- Disable the Play button until the unit is downloaded (current behavior — Play only appears after download)
- If Play is ever made available without download, show a warning about expected load time
- Consider caching content fetched via the play page so subsequent plays don't re-download

---

## Priority Recommendations

1. **Storage pre-check with clear messaging** (Risk #3) — Highest practical impact for Chromebook deployments. Quick to implement.
2. **Manifest loading error/retry state** (Risk #6) — "Checking..." forever is confusing. Quick to implement.
3. **Per-file retry logic** (Risk #1) — Reduces download failures on unreliable networks. Medium effort.
4. **`navigator.storage.persist()` request** (Risk #4) — One line of code, reduces risk of browser evicting cached units.
5. **Safari download warning** (Risk #2) — Helps iPad users understand limitations. Quick to implement.
