# MHS-009 — Add "Reset all MHS data" (all versions) with member-auth gating

**Priority:** P0
**Status:** Implemented (2026-07-05)

## Summary

Add an in-app **"Reset all MHS data"** control that fully clears this device's
local MHS footprint — **all** unit caches across **all versions**, the app-shell
cache, and MHS localStorage keys — so testers switching builds/versions/accounts
can get a genuinely clean device without DevTools or resetting the Chromebook.
Gate it with the same workspace member-authorization model as Clear Downloads,
switch-version, and jump-to-unit.

## Motivation

- The version-management features (switch collection version, switch unit
  version, jump to unit) were added after the cache layer and produce orphaned
  caches (MHS-001). Even "Clear All Downloads" only clears the current version
  (MHS-002).
- Testers need a reliable, in-app "clean slate" between runs. Today the only
  reliable reset is clearing browser site data / resetting the Chromebook.

## Behavior

On confirm + authorization, delete:
- Every `missionhydrosci-*` Cache Storage cache (all `missionhydrosci-unit-*`
  versions **and** the `missionhydrosci-app-shell-*` cache).
- Abort any in-flight Background Fetches for MHS — enumerate via
  `reg.backgroundFetch.getIds()` (all `missionhydrosci-*` IDs, any version), not
  just current-manifest fetch IDs, for the same version-blindness reason as
  MHS-002. Abort **before** deleting caches: a `backgroundfetchsuccess` event
  firing after the purge would silently repopulate a unit cache.
- MHS localStorage keys: `mhs-manual-downloads`, `mhs-progress-queue`,
  `missionhydrosci-progress-changed`. Regenerate `mhs-device-id` only if a full
  device-identity reset is intended (default: keep it so telemetry stays
  continuous — decide during implementation).
  **Decision (2026-07-05): `mhs-device-id` is kept** so device-status telemetry
  stays continuous across resets.

Then reload `/missionhydrosci/units`.

**Does NOT touch server-side state:** progress (`mhs_user_progress`), collection
override, and game save/settings (`stratasave`) are untouched. This is a
**local device reset**, not an account reset. Game progress and preferences are
safe because they live server-side, not in local storage.

## Authorization

Same model as the other gated member actions. For a **member**, require
authorization per the workspace `MHSMemberAuth` setting
(`trust` | `keyword` | `staffauth`); for staff roles, no extra auth. Reuse the
existing client auth modal exactly as Clear All Downloads does:

- `templates/missionhydrosci_units.gohtml:1034-1045` (`mhsClearAllDownloads`
  pattern — gate on `isMember && memberAuthMode !== 'trust'`, else confirm).

Because the reset is a client-only cache operation there is no server endpoint to
enforce against; the auth modal (which validates keyword/staffauth against the
server) is the gate, consistent with Clear All Downloads. A technically capable
member could still clear browser storage via the OS/browser — that is outside our
control and the same as today.

## Affected code

- `internal/app/features/missionhydrosci/templates/missionhydrosci_units.gohtml`
  (new button in the Manage Downloads section + handler)
- `internal/app/resources/assets/js/mhs-delivery.js` (all-cache purge helper,
  shared with MHS-002)

## Proposed implementation

- Add `purgeAllMHSData()` to the delivery manager: enumerate `caches.keys()`,
  delete all `missionhydrosci-*`, abort MHS Background Fetches, clear the MHS
  localStorage keys.
- Add a "Reset all MHS data" button; wire through the auth modal like
  `mhsClearAllDownloads`, then call `purgeAllMHSData()` and reload.
- Share the cache-enumeration helper with MHS-001 (`pruneStaleCaches`) and
  MHS-002 (clear-all-versions).

## Risk / notes

- Destructive locally, but safe for player data (server-side). Confirm + auth
  gate mitigate accidental use.
- Keep copy clear: "Removes all downloaded Mission HydroSci files and local data
  from this device. Your progress and saved game are stored online and are not
  affected."
</content>
