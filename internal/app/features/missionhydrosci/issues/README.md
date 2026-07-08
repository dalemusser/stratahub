# Mission HydroSci — Download & Cache Issues

**Discovered:** 2026-07-05
**Area:** Content delivery (PWA download, Cache Storage, quota management)
**Status (as of 2026-07-07): in QA.** All tracked fixes are implemented and
the build is in the QA team's hands; issues will be filed here as feedback
arrives. Timeline: MHS-001, -002, -003, -007, -009, -010, -011, -012
implemented 2026-07-05 (plus post-review fixes same day); MHS-013, -014
implemented 2026-07-06; **MHS-015..020 (review round 2 findings) fixed
2026-07-06** — see each file's Status line for what was done and any
deviations. The 2026-07-06 device smoke test (MacBook + HP Chromebook)
passed overall but surfaced
[MHS-021](MHS-021-stale-paused-bg-fetch-cross-device.md) (fixed same day).
Also 2026-07-06: help "?" icons + explainer modal added beside Clear All
Downloads / Reset All MHS Data in Manage downloads — its close handler
initially collided with the PWA install modal's global `mhsCloseHelp` and
was renamed `mhsShowActionHelp`/`mhsCloseActionHelp` (lesson: the units
template defines many `mhs*` globals in one scope; grep before naming).
SW_VERSION is 1.0.7.

Still open: MHS-004, -005, -006, -008 (pre-existing batch, not started) and
the status→UI table refactor deferred from MHS-020. An iPad smoke-test pass
(the fallback-only platform) has not been done yet — the 2026-07-06 device
test covered macOS + Chromebook; keep it in mind if iPad-specific reports
arrive.

Smoke-test checklist (for re-verification after future changes): download a
unit; tab-switch away and back during a stalled download (Retry must
persist); reload mid-fallback-download (progress must resume, no duplicate
download); Retry a stalled download (must resume, not restart from zero);
unit-complete overlay with the next unit not yet downloaded; Clear All
Downloads; Reset all MHS data; member collection-switch under `staffauth`;
one-time auto-reload on a device still on the previous service worker;
stale-fetch recovery (MHS-021's verification note).

A compatibility note for future SW changes: broadcast `detail` now carries
`version` on unit statuses, and pages ignore version-tagged statuses that
don't match their manifest's version for that unit. Untagged broadcasts
(older workers) still pass — keep it that way.

This folder tracks a cluster of related problems around downloading units and
managing the files/data stored on the device. They were surfaced by QA testers
(reports on Discord, 6/30/26) and reproduced in part by the team.

## The core mental-model correction

Cache Storage, IndexedDB, and localStorage are **per-origin (per browser
profile), not per-StrataHub-login.** Every StrataHub account used on the same
browser profile shares one pool of MHS caches. Switching StrataHub accounts
clears none of it.

The buildup testers observed across accounts/runs is real, but the mechanism is
**not** "each login uses its own slice." It is that the shared per-origin cache
**never clears old build versions**, so a fresh account inherits a nearly-full
cache from every prior run and can no longer download.

## Background — why this regressed

The download and cache management code was written **before** the "Manage
Versions" capabilities were added to Mission HydroSci — i.e. before users could
switch between collection (game) versions, switch unit versions, and jump to
specific units within the selected version. The cache layer therefore assumes a
single active set of unit versions and has no concept of "a version I used to
have but no longer do." Every problem in this folder traces back to that gap:
the newer version-switching abilities create stale caches the older cache
management was never designed to reclaim.

## Member-authorization model (relevant to several fixes)

Certain member actions require authorization by a higher-privileged user
(superadmin / admin / coordinator / leader). **Which** authorization is required
is a **workspace setting** (`MHSMemberAuth`: `trust` | `keyword` | `staffauth`),
so the "dev" workspace can make testing frictionless (`trust`) while the "mhs"
workspace requires a teacher/coordinator/admin to authorize.

Actions that are gated for members:

| Action | Client gate (modal) | Server-enforced |
|---|---|---|
| Jump to a unit (`set-unit`) | yes | **yes** (`progress.go`) |
| Switch collection version (`collection-override`) | yes | **yes** since [MHS-010](MHS-010-collection-override-auth-not-enforced.md) (`requireMemberAuth`) |
| Clear All Downloads | yes | n/a (client-only cache op) |
| Reset all MHS data (new) | yes (to build) | n/a (client-only cache op) |

Note: game **save data and settings/prefs are NOT stored locally** — they are
saved to the online `stratasave` service. Clearing local downloads/caches does
**not** lose player progress or preferences.

## How units are delivered (context)

- Each unit's files are cached in a Cache Storage cache keyed by unit **and
  version**: `missionhydrosci-unit-<unitId>-v<version>` (e.g.
  `missionhydrosci-unit-unit1-v2.2.3`).
- Two download paths:
  1. **Background Fetch API** (Chrome primary path) — resilient, OS-level.
  2. **SW sequential `fallbackFetch`** — used when Background Fetch is
     unavailable or fails; fetches files one at a time.
- `navigator.storage.estimate()` (`{usage, quota}`) drives the storage bar and
  low-storage gating. It is **origin-wide** — it counts all storage for the
  origin (Cache Storage + IndexedDB + localStorage), not just the MHS unit
  caches.

## Reported symptoms → root cause

| Reported symptom | Root cause issue |
|---|---|
| Download frozen at 0% (or an arbitrary %), browser download icon active, then jumps to Ready | [MHS-011](MHS-011-frozen-progress-false-stall-abort.md) + [MHS-007](MHS-007-confusing-download-progress.md) |
| Downloads failing on slow wifi mid-unit / at unit transition | [MHS-011](MHS-011-frozen-progress-false-stall-abort.md) (false-stall abort → fallback) + [MHS-003](MHS-003-fallback-fetch-leaves-partial-cache.md)/[MHS-004](MHS-004-guest-profile-background-fetch-disabled.md) |
| Desktop "Device Storage 100% full" | [MHS-001](MHS-001-stale-unit-caches-never-cleaned.md) |
| New account can't download Unit 1 | [MHS-001](MHS-001-stale-unit-caches-never-cleaned.md) + [MHS-005](MHS-005-storage-not-persisted-evictable.md) |
| Chromebook **guest profile** fails; real profile works | [MHS-004](MHS-004-guest-profile-background-fetch-disabled.md) + [MHS-001](MHS-001-stale-unit-caches-never-cleaned.md) |
| "Failed to download, redirecting…" used to let me proceed, now doesn't | download fails ([MHS-001](MHS-001-stale-unit-caches-never-cleaned.md)/[MHS-004](MHS-004-guest-profile-background-fetch-disabled.md)); redirect masks it |
| "Clear All Downloads" doesn't reclaim space | [MHS-002](MHS-002-clear-all-leaves-orphaned-versions.md) |
| No in-app way to fully reset a device between builds | [MHS-009](MHS-009-reset-all-mhs-data.md) |

## Issues

| ID | Priority | Title |
|---|---|---|
| [MHS-001](MHS-001-stale-unit-caches-never-cleaned.md) | P0 | Stale unit caches (old versions) are never cleaned up |
| [MHS-002](MHS-002-clear-all-leaves-orphaned-versions.md) | P0 | "Clear All Downloads" leaves orphaned old-version caches |
| [MHS-003](MHS-003-fallback-fetch-leaves-partial-cache.md) | P0 | `fallbackFetch` leaves a partial cache on error |
| [MHS-004](MHS-004-guest-profile-background-fetch-disabled.md) | P1 | Guest/Incognito disables Background Fetch → fragile sequential path |
| [MHS-005](MHS-005-storage-not-persisted-evictable.md) | P1 | Cache Storage is evictable — `persist()` never requested |
| [MHS-006](MHS-006-no-per-unit-space-preflight.md) | P1 | No per-unit space pre-flight before downloading |
| [MHS-007](MHS-007-confusing-download-progress.md) | P2 | Confusing 0%→100% progress; `downloadTotal: 0` + per-file granularity |
| [MHS-008](MHS-008-diagnostics-logging-gap.md) | P3 | Diagnostics gap — can't see cache inventory / failure reasons server-side |
| [MHS-009](MHS-009-reset-all-mhs-data.md) | P0 | Add "Reset all MHS data" (all versions) with member-auth gating |
| [MHS-010](MHS-010-collection-override-auth-not-enforced.md) | P1 | Collection-override (switch version) member auth not enforced server-side |
| [MHS-011](MHS-011-frozen-progress-false-stall-abort.md) | P1 | Frozen progress display + false-stall abort of healthy downloads |
| [MHS-012](MHS-012-app-shell-assets-stale-first-load.md) | P1 | App-shell assets stale on first load after a deploy (SW ignored `?v=` hash) |
| [MHS-013](MHS-013-prune-races-live-game-session.md) | P2 | Stale-cache prune can delete a cache a live game session is reading |
| [MHS-014](MHS-014-fallback-download-no-watchdog.md) | P2 | Fallback downloads have no watchdog — dead SW loop strands UI at "Downloading" |
| [MHS-015](MHS-015-stalled-retry-lifecycle-defects.md) | P0 | Stalled/Retry lifecycle defects — visibility unstick, non-monotonic criterion, retry races |
| [MHS-016](MHS-016-fallback-download-lifecycle-gaps.md) | P0 | Fallback lifecycle gaps — no reload adoption, prune misses cancel, cancel/redownload race |
| [MHS-017](MHS-017-play-overlay-dead-ends.md) | P1 | Play overlay dead ends — 'not_cached' unhandled, 'stalled' has no exit |
| [MHS-018](MHS-018-heartbeat-hidden-tab-suspension.md) | P1 | Play-session heartbeat dies under hidden-tab timer suspension |
| [MHS-019](MHS-019-size-verification-repair-in-place.md) | P2 | Size-verification blob fallback re-reads huge files every check — repair entry in place |
| [MHS-020](MHS-020-minor-cleanups-round2.md) | P3 | Minor fixes and cleanups from review round 2 |
| [MHS-021](MHS-021-stale-paused-bg-fetch-cross-device.md) | P1 | Stale/paused BG fetch strands current unit at 0% (cross-device switch) |

## Recommended sequencing

Start with **P0** (MHS-001, -002, -003, -009): stops unbounded cache
accumulation, which is the engine behind most of the storage-full /
download-fail reports, and gives testers a real in-app reset. Low-risk. Then
**P1** (MHS-004, -005, -006, -010, -011) to make constrained devices (guest
profiles, small quota, flaky wifi) succeed and to close the version-switch auth
gap. **P2/P3** are UX and observability.

Note: MHS-011's false-stall abort is a *reliability* bug dressed as a UX bug —
it converts healthy downloads on slow networks into failures. Strongly consider
shipping it (with MHS-007, same code) in the first batch.

MHS-002 and MHS-009 share a cache-enumeration/purge helper and should be built
together.
</content>
</invoke>
