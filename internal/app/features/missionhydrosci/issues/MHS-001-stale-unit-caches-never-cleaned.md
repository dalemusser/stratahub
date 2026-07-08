# MHS-001 — Stale unit caches (old versions) are never cleaned up

**Priority:** P0
**Status:** Implemented (2026-07-05)
**Primary driver of:** "Device Storage 100% full", "new account can't download",
guest-profile failures, and the cross-account/cross-build accumulation.

## Summary

Unit files are cached per unit **and version**
(`missionhydrosci-unit-<unitId>-v<version>`). When a new build ships with a
bumped version, the old version's cache becomes an orphan. Nothing ever deletes
these orphans, so storage grows without bound across builds until the origin
quota is exhausted and all further downloads fail.

## Evidence / symptoms

- Desktop reported "Device Storage showing 100% full."
- A brand-new StrataHub account "failed to download Unit 1" on a machine that
  had been used for prior test runs — the fresh account inherits the full
  shared per-origin cache.
- QA workflow of "separate StrataHub account per test run" + frequent build
  switching maximizes orphan accumulation, which correlated with more problems.

## Root cause

`cleanStaleCaches(manifest)` exists specifically to delete unit caches whose
`(unitId, version)` is not in the current manifest — **but it is never called
anywhere in the codebase.**

- Defined: `static/sw-cache.js:33`
- Never invoked (confirmed by grep across `internal/`).

The SW `activate` handler only cleans stale **app-shell** caches, not unit
caches:

- `static/sw.js:19-34` — filters on `missionhydrosci-app-shell-` only.

The client-side cleanup that *does* run, `autoCleanup()`, only iterates the
**current** manifest's units at their **current** version, so it can never
enumerate — let alone delete — an old-version cache:

- `assets/js/mhs-delivery.js:575-589` (`autoCleanup`)
- `assets/js/mhs-delivery.js:496-519` (`deleteUnit` builds the cache name from
  the current version)

## Affected code

- `internal/app/features/missionhydrosci/static/sw-cache.js`
- `internal/app/features/missionhydrosci/static/sw.js`
- `internal/app/resources/assets/js/mhs-delivery.js`

## Proposed fix

Enumerate all caches and delete any `missionhydrosci-unit-*` whose unit+version
is not in the current manifest. Do it **client-side** in `mhs-delivery.js` (after
`refreshManifest()`) so it does not depend on the SW lifecycle or on the SW
holding a manifest:

- Add a `pruneStaleCaches()` method: build the set of valid cache names from
  `this.manifest.units`, then `caches.keys()` and delete every
  `missionhydrosci-unit-` cache not in that set.
- Call it from `init()` right after `refreshManifest()` and
  `_reconnectActiveDownloads()`.
- Optionally also call `cleanStaleCaches` from the SW after a manifest fetch, but
  the client-side prune is sufficient and simpler.

## Risk / notes

- Low risk: only deletes caches for versions not referenced by the active
  manifest. Current + referenced units are untouched.
- Must run **after** reconnecting to active downloads so an in-flight download of
  a valid current-version unit is not disturbed. Additionally, skip any cache
  whose unit+version has an active Background Fetch (enumerate via
  `reg.backgroundFetch.getIds()`), so one user's prune can't kill another's
  in-flight download.
- **Shared-device / per-user-manifest subtlety (design decision):** the manifest
  is resolved **per user** (override → group pin → workspace default) but the
  cache is **per device/profile**. Pruning to "my manifest" means two accounts on
  the same profile using different collections will delete each other's unit
  caches at each login, forcing re-downloads. This is *acceptable and even
  desirable*: students in a class share one collection (no churn in practice),
  QA testers switching accounts/builds *want* the old data gone, and under quota
  pressure evicting the other collection is the correct trade. Documenting the
  choice here so it is deliberate, not accidental.
- **Prune triggers:** run in `init()` after `refreshManifest()` +
  `_reconnectActiveDownloads()`. A successful collection switch reloads the page,
  so init-time pruning also covers switches — do not implement this as a
  one-time migration.
- Related: MHS-002 (manual "clear all" has the same version-blindness).
</content>
