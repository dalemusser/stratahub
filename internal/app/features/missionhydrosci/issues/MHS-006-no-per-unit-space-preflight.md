# MHS-006 — No per-unit space pre-flight before downloading

**Priority:** P1
**Status:** Documented

## Summary

The auto-download pipeline gates only on a global "storage ≥ 90%" heuristic. It
never checks whether the **specific unit's** `totalSize` actually fits in
`quota - usage`. So it will start a download that cannot complete, and it fails
mid-write with `QuotaExceededError` instead of surfacing a clear "not enough
space" state.

## Evidence / symptoms

- Downloads that begin then fail on constrained devices.
- Storage bar not yet at the 90% threshold, but an individual large unit still
  doesn't fit in the remaining headroom.

## Root cause

- Pipeline auto-downloads the next unit gated only on `lowStorage` (`pct >= 90`):
  - `templates/missionhydrosci_units.gohtml:716-722` (pipeline)
  - `templates/missionhydrosci_units.gohtml:1074-1087` (`lowStorage = pct >= 90`)
- `updateDownloadButtonVisibility()` *does* compute `available = quota - usage`
  and hides buttons that won't fit, but this governs only the **manual** buttons
  and is a UI-hide, not a pre-flight for the auto pipeline:
  - `templates/missionhydrosci_units.gohtml:668-698`
- `downloadUnit()` itself performs no space check before starting:
  - `assets/js/mhs-delivery.js:400-491`

## Affected code

- `internal/app/resources/assets/js/mhs-delivery.js`
- `internal/app/features/missionhydrosci/templates/missionhydrosci_units.gohtml`
- `internal/app/features/missionhydrosci/static/sw-background-fetch.js`

## Proposed fix

- Before starting a unit download, compare `unit.totalSize` against
  `quota - usage` (with a safety margin). If it doesn't fit:
  1. First attempt to reclaim space (prune stale caches per MHS-001; optionally
     evict non-essential cached units), then re-check.
  2. If it still doesn't fit, emit a distinct status (e.g. `insufficient_space`)
     with a clear message rather than a generic failure.
- In `fallbackFetch`, retry a failing file a couple of times with backoff before
  failing the unit (pairs with MHS-003/004).

## Risk / notes

- `estimate()` is approximate and quota is browser-managed; use a margin and
  treat the check as advisory, not exact.
- Distinguishing `insufficient_space` from `error` improves the UX and the
  telemetry (MHS-008).
</content>
