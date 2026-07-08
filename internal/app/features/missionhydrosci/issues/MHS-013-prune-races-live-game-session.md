# MHS-013 — Stale-cache prune can delete a cache a live game session is reading

**Priority:** P2
**Status:** Implemented (2026-07-06)

**Implementation:** the play page writes a heartbeat to localStorage
(`mhs-active-play`, a map of `<unitId>-v<version>` → timestamp) on load and
every 60s, removing its entry on `pagehide`. `pruneStaleCaches` skips any
cache with a heartbeat fresher than 3 minutes, so a crashed tab can't block
pruning for long. "Reset All MHS Data" clears the heartbeat key along with
the other MHS localStorage keys (explicit resets ignore live sessions by
design — the reset deletes the game's cache anyway).

## Summary

`pruneStaleCaches()` (mhs-delivery.js, runs on every units/play-overlay
`init()`) deletes any unit cache whose unit+version is not in the current
manifest, guarding only against in-flight Background Fetches. It does not know
about caches currently serving a **live play session**. If a content deploy
bumps a unit's version while a student is mid-mission on the old version,
opening the units page in another tab (or the MHS-012 controllerchange
auto-reload firing there) prunes the old-version cache out from under the
running game.

## Why this is P2, not P0

Actual harm is narrow:

- Unity front-loads `.data`/`.framework`/`.wasm` at startup; post-load fetches
  are limited to lazy Addressables (e.g. Localization) and mid-session page
  reloads.
- A cache miss degrades to the Go handler's 302-to-CDN, which still works as
  long as the old build remains on S3 (builds are only deleted manually).
- A hard failure needs an unlucky stack: version bump mid-session + lazy fetch
  (or reload) + retired CDN build or offline play.

MHS-001 documents the cross-account prune churn on shared devices as a
deliberate trade-off, but never contemplated the same-device **mid-session**
case — hence this issue.

## Possible fixes (pick at implementation time)

- Have the play page register its active unit+version in localStorage (or via
  a SW message) with a heartbeat; `pruneStaleCaches` skips caches with a fresh
  heartbeat.
- Or defer pruning of a cache whose unit is the user's *current* unit until
  the next session.

## Affected code

- `internal/app/resources/assets/js/mhs-delivery.js` (`pruneStaleCaches`)
- `internal/app/features/missionhydrosci/templates/missionhydrosci_play.gohtml`
