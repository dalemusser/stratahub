# MHS-011 — Frozen progress display + false-stall abort of healthy downloads

**Priority:** P1 (ship with the first batch — same code area as MHS-007, and the
false-stall abort converts working downloads into failures)
**Status:** Implemented (2026-07-05)

## Summary

The page's download percentage can freeze (at 0% or at an arbitrary snapshot
value) while the underlying Background Fetch is downloading normally. The unit
then "jumps to Ready" when it finishes. Worse, the stall detector watches the
same frozen signal and after ~30–45s **aborts the healthy download** and
restarts it from zero on the fragile sequential fallback path.

## Observed symptom (Dale, direct experience)

> "Loading of a unit sitting at 0% download (or sometimes some other percentage
> value) without the percentage changing but the download is occurring
> [browser download icon active] and eventually the download completes and the
> unit status jumps to ready."

## Anatomy — progress and completion travel on different channels

- **Progress:** a `progress` event listener on a specific
  `BackgroundFetchRegistration` **object** held by the page, computing percent
  from `bgFetch.downloaded` (`assets/js/mhs-delivery.js:256-272`).
- **Completion:** the SW's `backgroundfetchsuccess` handler caches files and
  broadcasts `'cached'` over BroadcastChannel
  (`static/sw-background-fetch.js:125-178`).

When the page-held registration object stops receiving updates but the download
is healthy, the percent freezes while Chrome's own download UI stays active —
then the SW broadcast lands and the UI jumps straight to Ready. The download was
never stuck; only the page's view of it was.

## Root causes

1. **`bgFetch.downloaded` staleness on page-held objects.** The code already
   documents one variant: "bgFetch.downloaded is stale after being hidden —
   Chrome doesn't reliably update bgFetch.downloaded on background pages"
   (`assets/js/mhs-delivery.js:83-87`). The same class of staleness is not
   limited to hidden tabs.
2. **Reconnected registrations — explains "frozen at some other percentage."**
   When the download was started by a *different page* (play-page overlay starts
   the next unit; then the user lands on the units page — or any reload),
   `_reconnectActiveDownloads` re-obtains the registration via
   `backgroundFetch.get()` and attaches listeners to that **new object**
   (`assets/js/mhs-delivery.js:279-325`). It fires one snapshot percent at
   reconnect time; if live `progress` events don't reach re-obtained objects
   (a known-flaky area of the Background Fetch API), the number never moves
   again.
3. **Stall detector keyed to the same unreliable signal.** `_monitorDownload`
   checks `bgFetch.downloaded` on the page-held object every 15s; two unchanged
   checks (~30–45s) → `bgFetch.abort()` + restart via SW sequential
   `fallbackDownload` (`assets/js/mhs-delivery.js:194-250`). Consequences:
   - A healthy download with a frozen progress object is killed.
   - Partially-downloaded bytes are discarded (Background Fetch responses are
     only cached on success), and the restart uses the less reliable sequential
     path (MHS-003/004) — on flaky wifi this plausibly converts a would-have-
     succeeded download into "download failed." Likely contributor to the
     reported university-wifi failures at the unit1→unit2 transition.

## Affected code

- `internal/app/resources/assets/js/mhs-delivery.js`
  (`_attachBGFetchListeners`, `_monitorDownload`, `_reconnectActiveDownloads`,
  `_recheckActiveDownloads`)

## Proposed fix

Stop trusting the event stream / long-lived object entirely:

- **Poll-based progress:** while a unit is downloading, poll
  `reg.backgroundFetch.get(fetchId)` every few seconds. Each `get()` returns a
  fresh registration object whose `downloaded` reflects current state. Drive the
  UI percent from the freshest value (events, when they do fire, can update it
  too — take the max).
- **Poll-based stall detection:** compare successive *fresh-`get()`* `downloaded`
  values, not a page-held object's. Raise the threshold substantially (e.g. no
  progress across ~2–3 minutes of polling, tab visible) and check
  `result`/`failureReason` on the fresh object before concluding anything.
  Prefer surfacing "download appears stalled — Retry?" to the user over silently
  aborting and restarting on the weaker path.
- Pair with `downloadTotal: totalSize` (MHS-007) so the browser's native
  indicator is determinate too.

## Risk / notes

- Polling `get()` every 2–5s is cheap (in-browser IPC, no network).
- This self-healed in practice only because completion uses the SW broadcast
  channel — keep that path untouched.
- Relation to MHS-007: MHS-007 covers `downloadTotal: 0` and the fallback path's
  per-file progress granularity; MHS-011 covers the frozen event stream and the
  false-stall abort. Fix together — same file, same functions.
</content>
