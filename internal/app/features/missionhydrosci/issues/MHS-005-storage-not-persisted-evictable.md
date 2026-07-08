# MHS-005 — Cache Storage is evictable — `persist()` never requested

**Priority:** P1
**Status:** Documented

## Summary

The app never calls `navigator.storage.persist()`, so MHS data lives in
**best-effort** (evictable) storage. Under storage pressure the browser can
silently evict cached unit files, flipping a unit from `cached` to `partial` and
triggering re-download loops — especially painful on devices already near quota.

## Evidence / symptoms

- Intermittent re-downloads and "partial" states that don't correspond to a user
  action.
- New/returning sessions finding previously-downloaded units no longer complete.

## Root cause

No call to `navigator.storage.persist()` anywhere (confirmed by grep). The only
storage API used is `navigator.storage.estimate()`:

- `assets/js/mhs-delivery.js:595-600` (`getStorageEstimate` — estimate only)
- `templates/missionhydrosci_units.gohtml:447-453, 1048-1089` (estimate for the
  storage bar / device report)

## Affected code

- `internal/app/resources/assets/js/mhs-delivery.js`
- `internal/app/features/missionhydrosci/templates/missionhydrosci_units.gohtml`

## Proposed fix

- Request persistence during `init()`:
  `if (navigator.storage && navigator.storage.persist) await navigator.storage.persist();`
- Surface the result (`navigator.storage.persisted()`) in the device-status
  telemetry (MHS-008) so we know which devices got persistent storage.

## Risk / notes

- Persistence is **granted or denied by the browser** based on engagement /
  PWA-install signals and is **denied in guest/Incognito**. So this helps real
  profiles (especially installed PWAs) but will not rescue guest profiles — pair
  with MHS-001/003/006 for those.
- No downside to requesting it; denial is a silent no-op.
</content>
