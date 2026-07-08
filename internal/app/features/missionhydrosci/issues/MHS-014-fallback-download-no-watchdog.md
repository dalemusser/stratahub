# MHS-014 — Fallback downloads have no watchdog: a dead SW loop strands the UI at "Downloading"

**Priority:** P2
**Status:** Implemented (2026-07-06)

**Implementation:** `_monitorFallbackDownload` in mhs-delivery.js watches the
broadcast stream instead of a registration: every 'downloading' broadcast
refreshes the stall timer (fed via `_fireStatus`); silence for the stall
threshold while visible fires `'stalled'` with the same Retry affordance as
the Background Fetch path. Retry re-posts `fallbackDownload` — a dead SW is
revived by the postMessage, its module state (dedupe set) is fresh, and the
loop resumes from the last completed file. `_recheckActiveDownloads` no
longer bails when `backgroundFetch` is absent: fallback downloads reconcile
from the cache on visibilitychange (fires 'cached' if completed while
hidden; otherwise leaves tracking for the watchdog).

**Residual limitation (accepted):** if the SW loop is alive but hung on a
single fetch (network black hole — no bytes, no error), Retry is deduped by
the still-registered active loop and effectively waits for the browser's own
fetch timeout (minutes); when that fires, the loop's error path broadcasts
and a retry then resumes normally. The stalled UI is accurate the whole time.

## Summary

On platforms without Background Fetch (Safari/iPad — a primary classroom
target — and some guest profiles), downloads run entirely in the SW's
sequential `fallbackFetch` loop, and every terminal status reaches the page
only via BroadcastChannel. If the SW is terminated mid-download (iOS routinely
kills SWs whose work outlives the `waitUntil` budget), neither `cached` nor
`error` is ever broadcast:

- `_activeDownloads[unitId]` stays set on the page,
- `checkAllCacheStatus` skips units with active downloads,
- `_recheckActiveDownloads` early-returns when `reg.backgroundFetch` is
  absent (so the visibilitychange reconciler never runs on these platforms),
- the MHS-011 stall poller also bails on the same `backgroundFetch` check.

Result: the unit shows "Downloading N%" with a disabled button until a full
page reload. This is **wholly pre-existing** (the old stall detector only
watched Background Fetch objects, never the fallback path), but the MHS-011
rework gave the Background Fetch path a stall/Retry story that the fallback
path still lacks.

## Proposed fix

Give the fallback path the same watchdog shape MHS-011 gave Background Fetch:

- Track `lastBroadcastAt` per active fallback download on the page; if no
  'downloading' broadcast arrives for ~2–3 minutes (broadcasts now arrive at
  least every second during real progress), fire `'stalled'` locally so the
  user gets the same Retry affordance.
- On Retry for a fallback download, re-post `fallbackDownload` — the SW loop's
  skip-already-cached check resumes from the last completed file (partial
  caches are kept on failure as of the MHS-003 revision).
- Make `_recheckActiveDownloads` reconcile via `_checkUnitCache` when
  `reg.backgroundFetch` is unavailable instead of returning early.

## Affected code

- `internal/app/resources/assets/js/mhs-delivery.js`
  (`_recheckActiveDownloads`, download tracking, stall poller)
- `internal/app/features/missionhydrosci/static/sw-background-fetch.js`
  (`fallbackFetch` broadcast cadence is already sufficient)
