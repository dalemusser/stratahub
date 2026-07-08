# Mission HydroSci — Feature Overview

Mission HydroSci (MHS) delivers a Unity WebGL science-adventure game to students
as an **installable Progressive Web App (PWA)** served through StrataHub. It is
the delivery vehicle for a research impact study, so it has to run reliably on
the devices schools actually use — low-end Chromebooks and iPads, frequently on
slow or flaky Wi-Fi — and it has to keep working **offline** once a unit is
downloaded.

Almost every hard problem in this feature comes from one fact: the game is a
large, versioned, offline-capable binary payload delivered through the browser's
storage and background-download APIs, and those APIs behave very differently
across platforms, profiles, and network conditions. This document explains what
the feature does, how it works, and the cross-platform / cross-use-case
challenges that the issues in [`issues/`](issues/) exist to solve.

---

## Contents

1. [Purpose](#purpose)
2. [Key concepts](#key-concepts)
3. [How it operates](#how-it-operates)
   - [Server side (Go)](#server-side-go)
   - [Client side (PWA)](#client-side-pwa)
   - [The lifecycle of a download](#the-lifecycle-of-a-download)
4. [Authorization model](#authorization-model)
5. [The cross-platform / cross-use-case challenges](#the-cross-platform--cross-use-case-challenges)
6. [The issues folder](#the-issues-folder)
7. [Where state lives (and what is safe to delete)](#where-state-lives-and-what-is-safe-to-delete)
8. [Building, deploying, and testing](#building-deploying-and-testing)

---

## Purpose

- **Deliver the game offline-first.** Students may be on shared devices, behind
  slow school networks, or intermittently offline. A unit's files are downloaded
  once and served from the browser's Cache Storage thereafter, so gameplay does
  not depend on a live connection.
- **Support a research impact study.** Which build a given student plays must be
  controllable and reproducible (see *collections/versions* below), and device
  conditions are reported back for diagnostics. Game telemetry and save data go
  to external services (`stratalog`, `stratasave`), not to local storage.
- **Work as an installed app.** The PWA installs to the home screen / shelf and
  runs standalone, which is what makes background download and reliable offline
  launch possible on both Chromebooks and iPads.

## Key concepts

| Term | Meaning |
|------|---------|
| **Unit** | One chapter of the game (`unit1`…`unit5`), a Unity WebGL build (loader + `.data` + `.framework` + `.wasm` + assets). |
| **Version** | A unit's build version (e.g. `1.0.0`). Unit files are cached per **unit + version**. |
| **Build** (`mhs_builds`) | The source of truth for a unit-version's file list and sizes. The manifest is assembled from build records. |
| **Collection** (`mhs_collections`) | A named set of `unit → version` pairs — effectively "a version of the whole game." Switching collections is how you switch game versions. |
| **Manifest** | The JSON a client fetches (`/missionhydrosci/api/manifest`) describing the units, files, sizes, and CDN base for **that user right now**. |
| **CDN** | Where the actual build files live (`MHSCDNBaseURL`, e.g. `https://cdn.adroit.games/mhs`). The origin only ever redirects to it or serves it from cache. |
| **Cache Storage** | The browser's per-origin cache. Unit files live in caches named `missionhydrosci-unit-<unitId>-v<version>`; the app shell lives in `missionhydrosci-app-shell-v<n>`. |

**The load-bearing mental model:** Cache Storage, IndexedDB, and localStorage are
**per-origin (per browser profile), not per-StrataHub-login.** Every StrataHub
account used in the same browser profile shares one pool of MHS caches. Switching
StrataHub accounts clears none of it. Much of the issue history traces back to
this — and to the fact that the version-management features (switch collection,
switch unit version, jump to unit) were added **after** the original cache layer,
which had no concept of "a version I used to have but no longer do."

## How it operates

### Server side (Go)

All files are in this directory. The handler (`handler.go`) is a dependency
container wired up in `bootstrap/routes.go`.

| File | Responsibility |
|------|----------------|
| `routes.go` | Chi routes under `/missionhydrosci` (units, play, manifest, progress, auth, device-status, collections). |
| `units.go` | `ServeUnits` — the launcher page: resolves the manifest, loads progress, computes current/next unit and per-unit status. |
| `play.go` | `ServePlay` — the game launcher for one unit, with a **progress gate** (members may only open their current or completed units; staff may open any). `RedirectToPlay` handles game-initiated URL-mode unit transitions. |
| `api_manifest.go` | Manifest + collection resolution (`resolveCollection`, `collectionToManifest`), the collection-override endpoints, and `requireMemberAuth`. |
| `progress.go` | Per-user progress (`GetOrCreate`, complete-unit, set-to-unit) with server-side member-auth enforcement on jumps. |
| `staffauth.go` | Staff-authorization flow (login-id → password/email-code/trust → single-use token) used to authorize gated member actions. |
| `content.go` | `ContentFallback` — redirects `/missionhydrosci/content/*` to the CDN when no service worker intercepts it. |
| `sw.go` | Serves `/sw.js` — concatenates `static/sw-cache.js` + `static/sw-background-fetch.js` + `static/sw.js` and injects the asset-version constants. Served from **root scope** (`Service-Worker-Allowed: /`). |
| `manifest.go` | Serves the PWA `manifest.json`. |
| `device_status.go` | Receives device/storage/telemetry reports (`mhs_device_status`). |
| `appcheck.go` | `RequireApp` middleware — members only reach MHS if it is in their `EnabledApps`; staff always pass. |

**Manifest resolution is per user, most-specific wins** (`resolveCollection`):

1. Per-user override (`mhs_user_progress.collection_override_id`)
2. Per-group pin (`group_app_settings.mhs_collection_id`)
3. Workspace active collection (`site_settings.mhs_active_collection_id`)
4. None

This is what lets a QA tester pin a specific build to their own account while
the class default is unchanged — and it is also the source of the shared-device
tension described below, because the manifest is per-user but the cache is
per-device.

### Client side (PWA)

- **`internal/app/resources/assets/js/mhs-delivery.js`** — the
  `MHSDeliveryManager`. Registers the service worker, fetches the manifest,
  tracks per-unit cache status, orchestrates downloads, monitors progress and
  stalls, prunes stale caches, and exposes Clear/Reset. This is the brain of the
  client and the single most heavily-iterated file in the feature.
- **`templates/missionhydrosci_units.gohtml`** — the launcher UI: progress dots,
  the current-unit download/launch card, the "Manage downloads" and "Manage
  version" panels, the member-auth modal, and the auto-download pipeline.
- **`templates/missionhydrosci_play.gohtml`** — the Unity host page: loads the
  build, bridges identity/logging/save services into the game, and runs the
  end-of-unit completion overlay that downloads and transitions to the next unit.
- **`static/sw*.js`** — the service worker (see `sw.go` for how they are
  combined). Handles install/activate, content interception, the two download
  paths, and cache-status queries.

### The lifecycle of a download

1. **Login.** For staff (non-members), a post-login action registers the SW and
   pre-fetches the current unit (`bootstrap/routes.go` `missionhydrosci-early-download`).
   Members land on the units page, which does its own download.
2. **Units page.** `MHSDeliveryManager.init()` registers the SW, fetches the
   manifest, reconnects to any in-flight downloads, **prunes stale caches**, and
   checks per-unit cache status. An auto-download pipeline downloads the current
   unit, then pre-fetches the next.
3. **Download.** Two paths:
   - **Background Fetch API** (Chrome/Chromebook primary) — OS-level, resilient,
     survives navigation; progress is polled from a fresh registration.
   - **SW sequential `fallbackFetch`** (Safari/iPad, guest profiles, or when
     Background Fetch fails to start) — fetches files one at a time inside the
     service worker, streaming byte progress.
   Files are cached under `missionhydrosci-unit-<id>-v<version>`.
4. **Serve.** On launch, the SW intercepts `/missionhydrosci/content/*` and
   serves from the matching unit cache; a miss falls through to a CDN redirect.
5. **Play → complete → next.** The play page bridges identity and services into
   Unity, reports completion to the server, and its overlay downloads the next
   unit before transitioning.

## Authorization model

Certain member actions require authorization by a higher-privileged user. **Which**
authorization is a workspace setting, `MHSMemberAuth`:

- `trust` — no extra check (used by the **dev** workspace so testing is frictionless).
- `keyword` — a shared keyword, validated server-side.
- `staffauth` — a teacher/coordinator/admin authorizes via login-id + password or
  emailed code, yielding a single-use token.

| Action | Client gate (modal) | Server-enforced |
|--------|---------------------|-----------------|
| Jump to a unit (`set-unit`) | yes | **yes** (`progress.go`) |
| Switch collection version (`collection-override`, incl. "Use default") | yes | **yes** (`api_manifest.go`, `requireMemberAuth`) |
| Clear All Downloads | yes | n/a (client-only cache op) |
| Reset all MHS data | yes | n/a (client-only cache op) |

Cache operations are client-only because there is no server state to enforce
against; the modal (which validates keyword/token against the server) is the
gate, and a technically capable member could always clear browser storage via the
OS — the same as today, and acceptable because **no player progress lives locally**
(see below).

## The cross-platform / cross-use-case challenges

This is the heart of why the feature is hard. The same code has to behave
correctly across a matrix of platforms, network conditions, and usage patterns
that each stress a different browser API.

### By platform

- **Chrome / Chromebook (real profile).** The happy path: Background Fetch works,
  survives navigation, and reports byte-level progress. The main risks here are
  **storage exhaustion** from stale caches (the cache never reclaimed old versions
  — [MHS-001](issues/MHS-001-stale-unit-caches-never-cleaned.md)) and **progress
  display** artifacts where a page-held registration object freezes while the
  download is actually healthy, which the old code misread as a stall and aborted
  ([MHS-011](issues/MHS-011-frozen-progress-false-stall-abort.md)).
- **Chromebook guest / incognito profile.** Background Fetch is **disabled**, so
  every download falls to the fragile sequential `fallbackFetch` path
  ([MHS-004](issues/MHS-004-guest-profile-background-fetch-disabled.md)). This is
  why guest profiles failed while real profiles worked, and why the fallback
  path's robustness matters so much.
- **Safari / iPad.** No Background Fetch API at all — the fallback path is the
  *only* path. On top of that, iOS aggressively **terminates service workers**
  whose work outlives the event budget (a large unit on slow Wi-Fi routinely
  exceeds it), **suspends timers in hidden tabs**, and has its own HTTP/bfcache
  quirks. These platform behaviors are the root of the fallback-lifecycle and
  heartbeat issues ([MHS-014](issues/MHS-014-fallback-download-no-watchdog.md),
  [MHS-016](issues/MHS-016-fallback-download-lifecycle-gaps.md),
  [MHS-018](issues/MHS-018-heartbeat-hidden-tab-suspension.md)).

### By network condition

- **Slow / flaky Wi-Fi.** Two failure modes: a healthy-but-slow download that
  *looks* stalled (must not be aborted —
  [MHS-011](issues/MHS-011-frozen-progress-false-stall-abort.md)), and a genuine
  mid-download drop (must be able to **resume** rather than restart from zero —
  [MHS-003](issues/MHS-003-fallback-fetch-leaves-partial-cache.md)). The tension
  between reclaiming wasted space and preserving resumable partials is exactly
  what MHS-003 had to get right (and re-get-right in review).
- **Offline.** Once cached, units must launch with no network. This is why a
  cache deleted out from under a running game
  ([MHS-013](issues/MHS-013-prune-races-live-game-session.md)) or a false "Ready
  to play" over an empty cache
  ([MHS-002](issues/MHS-002-clear-all-leaves-orphaned-versions.md),
  [MHS-016](issues/MHS-016-fallback-download-lifecycle-gaps.md)) is so damaging:
  the failure only surfaces when the student is offline.

### By storage condition

- **Origin quota is finite and shared.** `navigator.storage.estimate()` is
  origin-wide, and Cache Storage is **evictable** unless persisted
  ([MHS-005](issues/MHS-005-storage-not-persisted-evictable.md)). Old-version
  caches that were never reclaimed
  ([MHS-001](issues/MHS-001-stale-unit-caches-never-cleaned.md)) fill the quota
  until *new* downloads fail — the engine behind most "storage full" / "new
  account can't download" reports. Downloading a unit that won't fit should be
  pre-flighted ([MHS-006](issues/MHS-006-no-per-unit-space-preflight.md)).

### By use case

- **QA testers.** They deliberately create a fresh StrataHub account per test run
  and switch builds frequently — which *maximizes* orphaned-cache accumulation on
  a shared per-origin pool, because a new account inherits a nearly-full cache
  from every prior run. They need a reliable in-app "clean slate"
  ([MHS-009](issues/MHS-009-reset-all-mhs-data.md)) and correct all-version
  clearing ([MHS-002](issues/MHS-002-clear-all-leaves-orphaned-versions.md)).
- **Students in a class.** A class shares one collection, so there is little
  version churn in practice; the design optimizes for this common case while
  tolerating the QA case.
- **Shared devices, per-user manifests.** The manifest is resolved **per user**
  but the cache is **per device**. Pruning to "my manifest" means two accounts on
  one profile using different collections evict each other's unit caches at login.
  This is a deliberate trade-off (documented in
  [MHS-001](issues/MHS-001-stale-unit-caches-never-cleaned.md)): students in a
  class share a collection (no churn), QA testers *want* the old data gone, and
  under quota pressure evicting the other collection is correct.
- **Mid-session version bumps.** A content deploy can bump a unit's version while
  a student is mid-game on the old one. The prune must not delete the cache the
  running game is reading ([MHS-013](issues/MHS-013-prune-races-live-game-session.md),
  [MHS-018](issues/MHS-018-heartbeat-hidden-tab-suspension.md)).

### By deploy lifecycle

- **Service worker updates.** A new deploy must reach controlled devices without
  asking users to "load twice." The SW now precaches/serves the versioned app
  assets under exact `?v=<hash>` URLs so a new hash is fresh on the *first* load,
  with a one-time auto-reload only for devices still on the pre-fix worker
  ([MHS-012](issues/MHS-012-app-shell-assets-stale-first-load.md)). A standing
  compatibility rule follows: **keep SW message actions and broadcast status
  values backward-compatible**, because an old page can briefly drive a new
  worker (and vice versa) during any update.

## The issues folder

[`issues/`](issues/) is the working tracker. Its own
[README](issues/README.md) has the symptom → root-cause map and the current
status of every issue. The issues arrived in waves, and understanding the waves
explains why the code is shaped the way it is:

1. **Original discovery (MHS-001…011, 2026-07-05).** QA surfaced a cluster of
   download/cache/storage failures. Root causes: stale caches never reclaimed,
   version-blind clearing, partial-cache leaks, guest-profile fragility,
   evictable storage, no pre-flight, confusing progress, false-stall aborts, and
   an unenforced version-switch auth gate. The P0 batch (001/002/003/009) plus
   007/010/011 were implemented.
2. **First pre-deploy review (2026-07-05).** A multi-agent review of that
   implementation found 10 defects; all were fixed the same day except two that
   were filed as new issues. Notable reversal: MHS-003's "purge partial cache on
   failure" was **undone** in favor of keeping partials for resume, because the
   purge undermined the very flaky-network goal it was meant to serve — purge now
   happens only on explicit cancellation.
3. **MHS-013 / MHS-014 (2026-07-06).** The two deferred findings: a stale-cache
   prune racing a live game session (fixed with a play-session heartbeat), and
   the fallback download path having no stall watchdog (fixed by watching the
   broadcast stream).
4. **Second pre-deploy review (2026-07-06).** A second review found that the
   round-1 fixes and MHS-013/014 had introduced their own defects — all in the
   new code. These are documented as **MHS-015…020**: the stalled/Retry
   lifecycle wasn't airtight, the fallback lifecycle had reload/prune/cancel
   gaps, the play overlay had dead-ends on newly-reachable statuses, the
   heartbeat died under iOS hidden-tab timer suspension, and a
   size-verification path could re-read huge files.
5. **Round-2 fixes (2026-07-06).** MHS-015…020 were implemented (see each
   issue file's Status line). Notable structural changes: one shared
   stall-monitor scaffold replaces the two diverging copies; the SW's
   fallback dedupe holds `{promise, aborter}` so a request behind a canceled
   loop waits for it to settle instead of being swallowed; SW broadcasts now
   carry `detail.version` and pages ignore statuses for a version they are
   not on; the play-heartbeat protocol lives in `mhs-delivery.js`
   (`startPlayHeartbeat`, 30-minute TTL, visibility re-arm); `SW_VERSION` is
   1.0.7.

6. **Device smoke test + follow-ups (2026-07-06).** The MacBook + HP
   Chromebook test passed overall but surfaced **MHS-021** (a stale *paused*
   Background Fetch from a prior session stranding the current unit at 0%),
   fixed same day: reconnect now matches fetches by unit **and version**
   (aborting mismatches), and the auto-download pipeline self-heals on
   `'not_cached'`/`'partial'` for the current/next unit. Help "?" icons with
   an explainer modal were added beside the Clear All / Reset buttons.

Still open (pre-existing, not yet started): MHS-004, -005, -006, -008; plus
the status→UI table refactor deferred from MHS-020.

> **Current status (2026-07-07):** in QA. All tracked fixes are implemented,
> statically verified, and the build is in the QA team's hands; new issues
> will be filed in [`issues/`](issues/) as feedback arrives. Note the iPad
> (fallback-only path) has not had a dedicated smoke pass yet.

## Where state lives (and what is safe to delete)

This matters because it is *why* the Clear/Reset controls are safe:

| State | Location | Cleared by local reset? |
|-------|----------|-------------------------|
| Player progress (current/completed units) | `mhs_user_progress` (server) | **No** — server-side |
| Game saves and settings | `stratasave` service (online) | **No** — online |
| Collection override (chosen version) | `mhs_user_progress` (server) | **No** — server-side |
| Downloaded unit files | Cache Storage (per origin) | Yes |
| App shell (page, CSS, delivery JS) | Cache Storage (per origin) | Yes (Reset only) |
| Manual-download list, progress queue, play heartbeat, device id | localStorage | Yes (device id kept for telemetry continuity) |

So "Clear All Downloads" and "Reset all MHS data" are **local device operations**.
They never lose progress or saves, which is the message the UI copy makes
explicit.

## Building, deploying, and testing

- **Build order:** `make css-prod` **then** `go build` — Tailwind CSS is embedded
  in the binary, so a new template class must be compiled before the Go build.
- **Service worker version:** `static/sw.js` carries `SW_VERSION`; the served
  worker also embeds the asset content hashes, so any asset change makes the SW
  bytes differ and browsers install the update.
- **Verification for JS/SW/template changes** (this feature has no Go unit
  tests): `node --check` each JS file; concatenate the three SW files with a
  stub `MHS_ASSET_VERSIONS` const and `node --check` the result (that is what
  `/sw.js` actually serves); and parse both `.gohtml` templates with Go's
  `html/template` (stub the custom template funcs).
- **Real-device smoke test** (the coverage gap the automated checks cannot fill):
  download a unit, Clear All Downloads, Reset all MHS data, a member
  collection-switch under `staffauth`, and the one-time auto-reload on a device
  that had the previous service worker — ideally on both a slow-network Chromebook
  and an iPad, since those exercise the two different download paths.
