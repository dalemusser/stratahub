# MHS-003 — `fallbackFetch` leaves a partial cache on error

**Priority:** P0
**Status:** Implemented (2026-07-05)

## Summary

When the sequential fallback download fails partway (a file errors, or a
`cache.put` hits the quota), the already-written files are left in the unit's
cache and the download is reported as `error`. The next status check sees a
`partial` cache. This both wastes space (contributing to MHS-001) and leaves the
unit in a confusing half-downloaded state.

## Evidence / symptoms

- Testers on flaky wifi and on guest profiles saw repeated download failures and
  "partial"/retry states. On the guest/Incognito path, `fallbackFetch` is the
  active code path (see MHS-004), so its failure handling matters most there.

## Root cause

`fallbackFetch`'s catch block broadcasts `error` but does **not** delete the
partially-populated cache:

- `static/sw-background-fetch.js:114-118`

```js
} catch (err) {
  console.error('Fallback fetch failed for ' + unitId + ':', err);
  broadcastStatus(unitId, 'error', { error: err.message });
  return false;   // partial cache entries remain
}
```

By contrast, the Background Fetch failure handler is fine — the browser discards
the fetch — but it also does not clear any stray cache
(`static/sw-background-fetch.js:183-191`).

## Affected code

- `internal/app/features/missionhydrosci/static/sw-background-fetch.js`

## Proposed fix

- On `fallbackFetch` failure, delete the unit cache before broadcasting `error`
  (or, better, retry the failing file a couple of times first — see MHS-006 —
  and only purge on final failure).
- Keeping successfully-fetched files across an intentional *retry* of the same
  unit is fine and already handled (the loop skips files already in cache,
  `sw-background-fetch.js:82-92`); the purge should apply to a *terminal* failure,
  not an in-progress retry.

Two hardening items in the same code area, worth doing together:

- **Concurrent fallback dedupe:** Background Fetch dedupes by `fetchId`, but the
  SW `fallbackDownload` message path has no guard — two tabs (or the units page
  plus the play page's completion overlay, which constructs a second independent
  `MHSDeliveryManager`, `templates/missionhydrosci_play.gohtml:382-387`) can run
  two sequential loops for the same unit at once. Mostly wasteful (later
  `cache.put` wins) but doubles bandwidth/quota churn on exactly the constrained
  devices that use this path. Fix: an in-SW "active fallback" set keyed by
  unit+version; ignore duplicate requests while one is running.
- **Integrity spot-check:** `checkUnitCacheStatus` / `_checkUnitCache` count a
  file as cached on any `cache.match` hit, without verifying size. Before
  declaring a unit `cached`, spot-check the largest file's cached size against
  the manifest size so eviction/partial artifacts can't produce a false
  "Ready to play."

## Implementation notes (2026-07-05, revised in code review)

- **The purge-on-terminal-failure idea above was implemented and then reversed
  in review.** With only a ~6s per-file retry window (3 attempts, 2s/4s
  backoff), any real outage exhausted retries and the purge threw away every
  fully-downloaded file — on flaky connections a large unit could never
  converge, defeating the batch's own reliability goal on exactly the
  devices that use this path. Final behavior: **partial caches are kept on
  failure** so a retry resumes via the skip-already-cached check; reclamation
  of abandoned partials is handled by MHS-001's prune (version changes) and
  the explicit Clear/Reset controls. The "confusing partial state" is
  addressed by the UI's Re-download affordance instead.
- The purge DOES apply on **cancellation** (Clear All / Reset / per-unit
  delete): fallback loops now carry an AbortController, canceled via a
  `cancelFallbacks` SW message before any cache purge, and delete their own
  cache on abort so a late write can't repopulate it or broadcast a false
  'cached'.
- Both hardening items shipped: the in-SW active-fallback dedupe set, and the
  largest-file size spot-check (with a stored-bytes fallback when the
  content-length header disagrees — transfer compression can leave a
  compressed content-length on a decoded body, and Content-Encoding is not
  readable cross-origin).
</content>
