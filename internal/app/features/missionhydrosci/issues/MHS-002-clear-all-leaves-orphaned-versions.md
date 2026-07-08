# MHS-002 — "Clear All Downloads" leaves orphaned old-version caches

**Priority:** P0
**Status:** Implemented (2026-07-05)

## Summary

The "Clear All Downloads" action (and the play page's cleanup) only clears caches
for units in the **current** manifest at their **current** version. Orphaned
caches from older builds are left untouched, so the one manual escape hatch a
tester has does **not** actually reclaim the space that MHS-001 accumulates.

## Evidence / symptoms

- Testers were told to clear browser cache via DevTools as a workaround — the
  in-app "Clear All Downloads" button was not enough, which is consistent with it
  leaving orphaned versions behind.
- **Confirmed:** the suspicion that "Clear All Downloads may only be clearing the
  current version" is correct — see root cause below.

## Root cause

`deleteAllUnits()` iterates `this.manifest.units` and calls `deleteUnit(unitId)`,
which constructs the cache name from the **current** version only:

- `assets/js/mhs-delivery.js:564-569` (`deleteAllUnits` — loops current manifest)
- `assets/js/mhs-delivery.js:496-519` (`deleteUnit` — `unitCachePrefix + id + '-v' + unit.version`)

So any `missionhydrosci-unit-unit1-v<oldVersion>` cache is invisible to this path.
The units-page handler that calls it also only clears the manual-download
localStorage key:

- `templates/missionhydrosci_units.gohtml:1024-1032` (`doClearAllDownloads`)

## Affected code

- `internal/app/resources/assets/js/mhs-delivery.js`
- `internal/app/features/missionhydrosci/templates/missionhydrosci_units.gohtml`

## Proposed fix

Fix "Clear All Downloads" so it clears **all versions**: add an all-cache purge
that enumerates `caches.keys()` and deletes **every** `missionhydrosci-unit-*`
cache regardless of version (not just current-manifest units). Share the
cache-enumeration helper with MHS-001's `pruneStaleCaches`.

The broader **"Reset all MHS data on this device"** control (which also clears
app-shell cache and MHS localStorage keys) is tracked separately in
[MHS-009](MHS-009-reset-all-mhs-data.md); build the two together.

Preserve the existing **member-authorization gate**: for members, "Clear All
Downloads" already routes through the auth modal when the workspace
`MHSMemberAuth` setting is not `trust`
(`templates/missionhydrosci_units.gohtml:1034-1045`). Keep that behavior.

## Risk / notes

- Overlaps with MHS-001's `pruneStaleCaches`; both can share a cache-enumeration
  helper. MHS-001 prunes *stale* versions automatically; MHS-002 makes the manual
  "clear all" actually clear all versions.
- **No IndexedDB step needed for game state.** Game save data and settings/prefs
  are stored in the online `stratasave` service, **not** in Unity's IndexedDB, so
  clearing local caches does not affect player progress or preferences.
</content>
