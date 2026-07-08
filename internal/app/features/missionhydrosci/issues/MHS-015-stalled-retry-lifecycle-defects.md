# MHS-015 — Stalled/Retry lifecycle defects (visibility unstick, non-monotonic criterion, retry races)

**Priority:** P0 (defeats the MHS-011 fix in the most common user behavior)
**Status:** **Fixed 2026-07-06.** 15a: visibility handler resets only
`lastProgressAt`. 15b: `_reportDownloadProgress` is the single owner of
stall-state mutation with `downloaded > maxDownloaded` as the only progress
criterion; the poller keeps only the threshold check (via `_maybeFireStalled`).
15c: per-unit `_retryInFlight` guard in `retryDownload` + the units template
disables the Retry buttons on click; retry also cancels a wedged fallback loop
with `purge=false` so partials survive for resume. 15d: implemented with a
45s grace and TWO reconcile rounds before surfacing 'error' — a single round
could condemn a slow-but-healthy cache-copy on low-end hardware. Fold-ins
done: `_startStallMonitor` + `_maybeFireStalled` extraction, both reconnect
paths now report through `_reportDownloadProgress`, constructor comment fixed.
**Area:** `internal/app/resources/assets/js/mhs-delivery.js` — poller, stall
state, retryDownload; plus the visibilitychange handler in `init()`

Four related defects in the stall/Retry machinery added by MHS-011 and its
post-review fixes. All were CONFIRMED by verification.

## 15a — visibilitychange handler force-clears `stalled` (most severe)

The handler in `init()` resets every `_stallState` entry:
`lastDownloaded = -1; lastProgressAt = Date.now(); stalled = false`.

For a genuinely dead Background Fetch showing 'stalled' + Retry, switching
tabs and returning:
1. clears the sticky flag, so `_recheckActiveDownloads` (same visibility
   event) fires progress — the sticky guard in `_reportDownloadProgress` is
   skipped because `stalled` was just cleared;
2. the next poll tick sees unchanged bytes `!== -1` and treats it as
   movement, firing 'downloading'.

Result: Retry is replaced by a disabled "Downloading..." button and the
2.5-minute clock restarts — on **every tab switch**, indefinitely, for a
download that will never progress. Checking another tab while waiting is the
single most common user behavior.

**Fix:** the handler must reset only the stall *timer* (`lastProgressAt`),
never the `stalled` flag; un-sticking must remain exclusively the
`downloaded > maxDownloaded` test.

## 15b — poller progress criterion is not monotonic

`_pollDownloadOnce` uses `downloaded !== state.lastDownloaded` as "bytes
moved" and clears `state.stalled` itself before calling
`_reportDownloadProgress` — so the round-1 monotonic guard in that helper is
unreachable on the poller path, and a **regressed** byte counter (browser
internally retried a request) counts as progress and dismisses the Retry UI.

**Fix:** single owner for stall-state mutation. `_reportDownloadProgress`
should own lastDownloaded/lastProgressAt/stalled updates using
`downloaded > maxDownloaded` as the only progress criterion; the poller keeps
only the stall-threshold check.

## 15c — retryDownload double-click race

The Retry button is never disabled and `cacheStatus` stays 'stalled' until
the retry's awaits (`_waitForSW`, `get`, `abort`) complete, so a second click
also routes to `retryDownload`. Interleaving one: click 2's
`delete _activeDownloads[unitId]` lands after click 1's `downloadUnit` set
it, defeating the dedupe — two `downloadUnit` runs; the second
`backgroundFetch.fetch` rejects on the duplicate ID and falls through to the
SW fallback path, yielding a Background Fetch **and** a fallback loop
downloading the same unit concurrently (with `_monitorFallbackDownload`
clobbering the BG monitor's state). Interleaving two: click 2's `get()`
finds click 1's fresh healthy fetch and aborts it.

**Fix:** re-entrancy guard in `retryDownload` (per-unit in-flight flag set
before the first await, cleared in finally); optionally disable the button
on click in the template.

## 15d — poller trusts the success broadcast unconditionally (PLAUSIBLE)

`_pollDownloadOnce` early-returns forever on `fresh.result === 'success'`.
If the SW dies during `handleBackgroundFetchSuccess`'s cache-copy (~1GB of
records) or the handler throws before broadcasting, a visible tab shows
"Downloading X%" indefinitely (checkAllCacheStatus skips the unit;
downloadUnit dedupes). Chrome may re-dispatch backgroundfetchsuccess on SW
restart, which would self-heal — unconfirmed, hence plausible.

**Fix (cheap hardening):** on `result === 'success'`, record
`state.successSince`; after a grace period (~30–60s) still without the
'cached' broadcast, reconcile via `_checkUnitCache` (fire 'cached' if
complete, else 'error' with Retry, clearing tracking).

## Fold-in cleanup (do with this fix, not separately)

`_monitorDownload` and `_monitorFallbackDownload` duplicate the entire
monitor scaffold (clear-existing block, stall-state initializer, interval
with identical cleanup-on-inactive body) and the 'stalled' detail
construction. Several of the bugs above exist because this logic lives in
two diverging copies. Extract one `_startStallMonitor(unitId, extraState,
tickFn)` plus one stalled-fire helper. Also: convert the two remaining
inline 'downloading'-fire blocks (`_reconnectActiveDownloads` and
downloadUnit's reconnect branch) to `_reportDownloadProgress` so all
progress goes through the monotonic/sticky guard, and fix the stale
`_stallState` field comment on the constructor.
