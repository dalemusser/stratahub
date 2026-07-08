# MHS-012 — App-shell assets stale on first load after a deploy

**Priority:** P1 (ship with the first batch — otherwise every existing device
runs the old page JS on its first post-deploy load)
**Status:** Implemented (2026-07-05)

## Summary

`mhs-delivery.js` and `tailwind.css` are hash-versioned (`?v=<content hash>`),
which correctly busts the browser HTTP cache — but on MHS pages the service
worker intercepted those URLs with a cache-first strategy that matched with
`ignoreSearch: true`. The `?v=` was ignored, so a device with an old SW in
control served the **old** cached JS even though the fresh HTML referenced the
new hash. The fresh copy only arrived via the SW update cycle (install-time
`cache.addAll`), which completes after the triggering page load — so new
page-side behavior appeared only on the **second** load after a deploy.

Users should never need to know to "load it twice." That kind of magical step
destroys trust in the software.

## Root cause

- `sw.js` fetch handler routed the two assets to `cacheFirst`, which used
  `cache.match(request, { ignoreSearch: true })` — the version query was
  deliberately discarded so *some* copy could be served offline.
- The install handler pre-cached the assets under their **unversioned** URLs,
  so exact-match lookups could never work either.

## Fix (two parts)

1. **Versioned exact-match caching (permanent).**
   - `sw.go` injects the current content hashes into the served worker as a
     real constant (`MHS_ASSET_VERSIONS`) instead of a trailing comment (still
     changes the SW bytes on any asset change, which is what triggers browser
     SW updates).
   - `sw-cache.js` pre-caches the assets under their exact `?v=<hash>` URLs;
     `APP_SHELL_CACHE` bumped to `-v6` so the old unversioned entries are
     dropped on activate.
   - `sw.js` serves them via `versionedAssetFirst`: exact match (including
     `?v=`) → cache hit; new hash after a deploy → cache miss → network fetch
     on the FIRST load, stale versions pruned, new version cached; offline
     with no exact match → any cached version as fallback.

2. **Transition shim (units page).** Devices still controlled by the
   pre-fix worker get one unavoidable stale load (the old worker handles the
   load that installs the new one). The units page now listens for
   `controllerchange` and reloads **once, automatically** when an updated
   worker takes control — guarded to pages that were already SW-controlled at
   load time (first-ever visits don't reload), one reload per controller
   change, and a 10-second sessionStorage backstop against reload loops.
   Downloads survive the reload (they run in the SW / Background Fetch;
   `_reconnectActiveDownloads` reattaches).

The play page intentionally has **no** auto-reload — reloading would kill a
running game. It picks up new page-side JS on its next natural navigation; the
SW-side logic applies immediately everywhere once the new worker activates,
and the page↔SW message protocol must stay backward-compatible for exactly
this reason (an old page can briefly drive a new worker).

## Affected code

- `internal/app/features/missionhydrosci/sw.go`
- `internal/app/features/missionhydrosci/static/sw-cache.js`
- `internal/app/features/missionhydrosci/static/sw.js`
- `internal/app/features/missionhydrosci/templates/missionhydrosci_units.gohtml`

## Risk / notes

- The `controllerchange` reload fires within a second or two of the new
  worker activating — the user sees at most a quick refresh, never an
  instruction.
- Once this worker is in control, all **future** deploys are lag-free with no
  reload at all: fresh HTML carries the new hash, the exact-match miss fetches
  the new asset immediately.
- Compatibility rule going forward: keep SW message actions and broadcast
  status values backward-compatible, since an old page can drive a new worker
  for one load during any update.
