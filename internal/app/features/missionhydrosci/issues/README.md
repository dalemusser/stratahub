# Mission HydroSci — Download & Cache Issues

**Area:** Content delivery (PWA download, Cache Storage, quota management)

This folder tracks the issues MHS-001…021: a cluster of problems around
downloading units and managing the files and data stored on the device, first
reported by QA testers and reproduced in part by the team. All of them are
shipped except the guest-profile verification of MHS-004. How the delivery works
today is described in the feature [README](../README.md).

Later field issues (play gate, download integrity, held tail, pieces, one game
window, crash notices) are recorded in `docs/mission-hydrosci/`
(`mhs-issues-102826.md`, `mhs-issues-remaining-plan-110926.md`,
`mhs-resolution-plan-111426.md`); the open backlog, including the follow-ups
below, is `docs/mission-hydrosci/mhs-remaining-work-plan.md`.

## The core mental model

Cache Storage, IndexedDB, and localStorage are **per-origin (per browser
profile), not per-StrataHub-login.** Every StrataHub account used on the same
browser profile shares one pool of MHS caches, and switching StrataHub accounts
clears none of it. A fresh account therefore inherits whatever earlier accounts
and builds left behind, which is why the cache layer reclaims every version that
the current manifest no longer uses.

Game saves and settings are **not** stored locally — they live in the online
`stratasave` service — and progress lives on the server. Clearing local
downloads or caches never loses player progress or preferences.

## Reported symptoms → root cause

| Reported symptom | Root cause issue |
|---|---|
| Download frozen at 0% (or an arbitrary %), browser download icon active, then jumps to Ready | [MHS-011](MHS-011-frozen-progress-false-stall-abort.md) + [MHS-007](MHS-007-confusing-download-progress.md) |
| Downloads failing on slow wifi mid-unit / at unit transition | [MHS-011](MHS-011-frozen-progress-false-stall-abort.md) + [MHS-003](MHS-003-fallback-fetch-leaves-partial-cache.md) / [MHS-004](MHS-004-guest-profile-background-fetch-disabled.md) |
| Desktop "Device Storage 100% full" | [MHS-001](MHS-001-stale-unit-caches-never-cleaned.md) |
| New account can't download Unit 1 | [MHS-001](MHS-001-stale-unit-caches-never-cleaned.md) + [MHS-005](MHS-005-storage-not-persisted-evictable.md) |
| Chromebook **guest profile** fails; real profile works | [MHS-004](MHS-004-guest-profile-background-fetch-disabled.md) + [MHS-001](MHS-001-stale-unit-caches-never-cleaned.md) |
| "Clear All Downloads" doesn't reclaim space | [MHS-002](MHS-002-clear-all-leaves-orphaned-versions.md) |
| No in-app way to fully reset a device between builds | [MHS-009](MHS-009-reset-all-mhs-data.md) |
| Current unit stuck at 0% after switching devices | [MHS-021](MHS-021-stale-paused-bg-fetch-cross-device.md) |

## Issues

| ID | Title | Status |
|---|---|---|
| [MHS-001](MHS-001-stale-unit-caches-never-cleaned.md) | Stale unit caches (old versions) are never cleaned up | Implemented |
| [MHS-002](MHS-002-clear-all-leaves-orphaned-versions.md) | "Clear All Downloads" leaves orphaned old-version caches | Implemented |
| [MHS-003](MHS-003-fallback-fetch-leaves-partial-cache.md) | `fallbackFetch` leaves a partial cache on error | Implemented (partials kept for resume; purged only on cancel) |
| [MHS-004](MHS-004-guest-profile-background-fetch-disabled.md) | Guest/Incognito disables Background Fetch → fragile sequential path | **Open:** the direct path now retries and resumes; a guest-profile verification run remains (backlog RDY-6) |
| [MHS-005](MHS-005-storage-not-persisted-evictable.md) | Cache Storage is evictable — `persist()` never requested | Implemented; device verification pending |
| [MHS-006](MHS-006-no-per-unit-space-preflight.md) | No per-unit space pre-flight before downloading | Implemented; device verification pending |
| [MHS-007](MHS-007-confusing-download-progress.md) | Confusing 0%→100% progress | Implemented |
| [MHS-008](MHS-008-diagnostics-logging-gap.md) | Diagnostics gap — can't see failure reasons server-side | Superseded by the step log and its stored records |
| [MHS-009](MHS-009-reset-all-mhs-data.md) | Add "Reset all MHS data" (all versions) with member-auth gating | Implemented |
| [MHS-010](MHS-010-collection-override-auth-not-enforced.md) | Collection-override (switch version) member auth not enforced server-side | Implemented |
| [MHS-011](MHS-011-frozen-progress-false-stall-abort.md) | Frozen progress display + false-stall abort of healthy downloads | Implemented |
| [MHS-012](MHS-012-app-shell-assets-stale-first-load.md) | App-shell assets stale on first load after a deploy | Implemented |
| [MHS-013](MHS-013-prune-races-live-game-session.md) | Stale-cache prune can delete a cache a live game session is reading | Implemented |
| [MHS-014](MHS-014-fallback-download-no-watchdog.md) | Fallback downloads have no watchdog | Implemented |
| [MHS-015](MHS-015-stalled-retry-lifecycle-defects.md) | Stalled/Retry lifecycle defects | Fixed |
| [MHS-016](MHS-016-fallback-download-lifecycle-gaps.md) | Fallback lifecycle gaps — reload adoption, prune vs cancel, cancel/redownload race | Fixed |
| [MHS-017](MHS-017-play-overlay-dead-ends.md) | Play overlay dead ends | Fixed |
| [MHS-018](MHS-018-heartbeat-hidden-tab-suspension.md) | Play-session heartbeat dies under hidden-tab timer suspension | Fixed |
| [MHS-019](MHS-019-size-verification-repair-in-place.md) | Size-verification fallback re-reads huge files every check | Fixed |
| [MHS-020](MHS-020-minor-cleanups-round2.md) | Minor fixes and cleanups from review round 2 | Fixed, except the status→UI table refactor (backlog ARCH-1) |
| [MHS-021](MHS-021-stale-paused-bg-fetch-cross-device.md) | Stale/paused BG fetch strands current unit at 0% (cross-device switch) | Fixed |

## Smoke-test checklist

Run on dev after a change to delivery, the service worker or the MHS pages,
with a member account, on a Chromebook (Background Fetch) and, when the change
touches the direct path, on Safari/iPad or a guest profile. Open "Status
details" on each page to read the step log.

**Units page**
1. Fresh profile: the current unit downloads, then the next; the bar moves and
   the step log shows the method, download and verify rows.
2. Reload and switch tabs mid-download: the download is adopted, not started
   twice, and finishes.
3. Delete one file from the unit's cache (DevTools → Application → Cache
   Storage): the unit shows partial and only that file downloads again.
4. A large file (32 MB or more) arrives as `.pieces/NNN` requests and the unit
   launches.

**Play page**
5. Launch: the game opens in its game window; Play again from the units page
   switches that window instead of opening a second one.
6. Offline after download: the unit launches from the cache.
7. Block the log or save host (DevTools request blocking): the game does not
   start and the loading screen names the host; unblock it and the game starts
   by itself within about 30 s; the units page warns before the next Launch.
8. Finish a unit: the completion overlay downloads the next unit and moves on.

**Manage page**
9. In a `staffauth` workspace: the gate shows; after staff authorize, the
   countdown runs, a gated action restarts it, and Lock now returns to the gate.
10. Download / Clear a unit, Clear All Downloads (the current unit downloads
    again) and Reset all MHS data: progress and saves are unchanged.
11. Change version and Use default: the units page shows the new version and
    the old version's caches are pruned.

**Service worker**
12. After a deploy that bumps `SW_VERSION`: the step log reports the new worker
    on the first load and the pages' assets are current without a second reload.
