# MHS-016 — Fallback download lifecycle gaps (reload adoption, prune cancel, cancel/redownload race)

**Priority:** P0
**Status:** **Fixed 2026-07-06.** 16a: `getActiveFallbacks` SW message
(ports-reply pattern, 3s timeout for older workers); `_reconnectActiveDownloads`
adopts loops whose unit+version is in the manifest (matched by key set — no
string parsing) and starts the fallback watchdog. 16b: `pruneStaleCaches`
posts a purge-cancel for each doomed cache (name parsed back into
unit+version) before deleting it. Additionally, all SW broadcasts now carry
`detail.version` and `_handleStatusUpdate` drops statuses for a different
version of a manifest unit — so an old-version loop's terminal 'not_cached'
can no longer clear tracking for the new version's live download. 16c:
backoff re-checks `signal.aborted` after the sleep; dedupe restructured to a
`fallbackRuns` Map of `{promise, aborter}` — a request finding an aborted run
awaits its settle and then starts. Retry-style cancels (`purge=false`) keep
the partial cache and broadcast nothing.
**Area:** `internal/app/resources/assets/js/mhs-delivery.js`
(`_reconnectActiveDownloads`, `pruneStaleCaches`),
`internal/app/features/missionhydrosci/static/sw-background-fetch.js`
(`fallbackFetch`, `fetchAndCacheFileWithRetry`, `cancelActiveFallbacks`),
`static/sw.js` (message handler)

Three CONFIRMED defects in the SW sequential-fallback lifecycle machinery
added by the round-1 fixes and MHS-014.

## 16a — a live fallback loop is never adopted after page reload

`_reconnectActiveDownloads` only enumerates Background Fetch registrations;
nothing queries the SW's `activeFallbacks`. After a reload mid-download:

- no `_activeDownloads`/`_stallState` entry exists, so the MHS-014 watchdog
  never runs — if the SW then dies, the unit sticks at "Downloading…" with
  no 'stalled'/Retry (the exact failure MHS-014 targets, reintroduced by any
  reload);
- `checkAllCacheStatus` (init + every visibilitychange) is not skipped for
  the unit, so it fires 'partial' mid-download, flashing a Re-download
  button over the live progress;
- worst leg: on a BG-capable browser where the download had fallen back
  (`backgroundFetch.fetch` threw), `runPipeline` sees 'partial' and
  `downloadUnit` starts a **duplicate full Background Fetch** alongside the
  running SW loop (the `activeFallbacks` dedupe only guards the non-BG
  path).

**Fix:** add a `getActiveFallbacks` SW message answered from
`activeFallbacks` via the `event.ports[0]` reply pattern (already used by
`getVersion`); in `_reconnectActiveDownloads`, adopt each returned
unit+version that is in the manifest: set `_activeDownloads` and start
`_monitorFallbackDownload`.

## 16b — pruneStaleCaches neither cancels nor exempts in-flight fallback loops

Round 1 added `_cancelFallbackDownloads` before purging in
deleteUnit/deleteAllUnits/purgeAllMHSData — but the prune path was missed.
Scenario: fallback loop downloading unit1 v1.0.0; deploy bumps to v1.0.1;
units-page reload → prune deletes the v1.0.0 cache (no BG fetch, no play
heartbeat protects it); the loop keeps writing into the doomed cache handle
and finally broadcasts 'cached' (payload has unitId only, no version) →
every page marks unit1 "Ready to play" while the v1.0.1 cache is empty;
Play misses the cache offline.

**Fix:** before deleting a `missionhydrosci-unit-<id>-v<ver>` cache, post
`cancelFallbacks` for that unit+version (the cache name parses back into
both), or skip caches present in a `getActiveFallbacks` reply (16a's query
makes this free).

## 16c — cancel/redownload race: non-abort-aware backoff + dedupe swallow

`fetchAndCacheFileWithRetry`'s backoff (`setTimeout` 2–4s) ignores the abort
signal, and the abort path awaits `caches.delete` before the `finally`
releases the `activeFallbacks` key — a canceled loop holds the dedupe key
for seconds. A fresh `fallbackDownload` posted right behind the cancel
(deleteUnit → runPipeline; Clear All → reload — the SW outlives the reload)
hits the dedupe and is silently swallowed while the page has already set
`_activeDownloads` and a watchdog; the old loop's terminal `'not_cached'`
broadcast then clears that tracking. Net: unit shows "Not downloaded",
nothing downloading, and the pipeline doesn't re-trigger ('cached' is its
only re-entry) — the automatic re-download silently dies.

**Fix:** (1) make the backoff abort-aware (check `signal.aborted` after the
sleep and throw); (2) restructure the dedupe so a new request for a key
whose loop is aborted **waits for the old run's promise to settle and then
starts** (store `{promise, aborter}` per key) instead of returning
early — this also guarantees the old loop's 'not_cached' lands before the
new loop's first 'downloading'.

## Verification note

A fourth candidate here — a cancel landing between the final file's
`cache.put` and the 'cached' broadcast — was **REFUTED**: the purge paths'
awaits mean the late 'cached' is delivered before the purging page fires
'not_cached', so the initiating page converges correctly; hidden tabs are
corrected by the visibilitychange recheck. No fix needed.
