# Mission HydroSci — Feature Overview

Mission HydroSci (MHS) delivers a Unity WebGL science-adventure game to students
as an **installable Progressive Web App (PWA)** served through StrataHub. It is
the delivery vehicle for a research impact study, so it has to run reliably on
the devices schools actually use — low-end Chromebooks and iPads, frequently on
slow or filtered school networks — it has to keep working **offline** once a
unit is downloaded, and every session a student plays has to be recorded.

Almost every hard problem in this feature comes from one fact: the game is a
large, versioned, offline-capable binary payload delivered through the browser's
storage and background-download APIs, and those APIs (and the school networks in
front of them) behave very differently across platforms, profiles, and network
conditions. This document explains what the feature does, how it works, and the
challenges the code is shaped around.

---

## Contents

1. [Purpose](#purpose)
2. [Key concepts](#key-concepts)
3. [How it operates](#how-it-operates)
   - [Server side (Go)](#server-side-go)
   - [Client side (PWA)](#client-side-pwa)
   - [The lifecycle of a download](#the-lifecycle-of-a-download)
   - [The play page](#the-play-page)
   - [Records and diagnostics](#records-and-diagnostics)
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
  controllable and reproducible (see *collections* below). Game telemetry and
  save data go to external services (`stratalog`, `stratasave`). Play that
  cannot reach those services is not part of the study, so the game does not
  start when a school blocks them (the [play gate](#the-play-page)).
- **Work as an installed app.** The PWA installs to the home screen / shelf and
  runs standalone, which is what makes background download and reliable offline
  launch possible on both Chromebooks and iPads.
- **Make field problems readable.** Every download and launch writes a step log;
  member pages store it on the server at each outcome, so a school's problem can
  be read without asking the student for anything.

## Key concepts

| Term | Meaning |
|------|---------|
| **Unit** | One chapter of the game (`unit1`…`unit5`), a Unity WebGL build (loader + `.data` + `.framework` + `.wasm` + assets). |
| **Version** | A unit's build version (e.g. `1.0.0`). Unit files are cached per **unit + version**. |
| **Build** (`mhs_builds`) | The source of truth for a unit-version's file list and sizes, and for each large file's piece count. The manifest is assembled from build records. Builds are uploaded and managed in the `mhsbuilds` feature. |
| **Pieces** | Every build file of 32 MB or more is also stored on the content server as 8 MB pieces at `<path>.pieces/NNN` (`models.MHSPieceBytes`, `MHSPieceMinFileBytes`, `MHSPiecePath`; written by `features/mhsbuilds/pieces.go` at upload and by a job for the builds of every collection in use). |
| **Collection** (`mhs_collections`) | A named set of `unit → version` pairs — effectively "a version of the whole game" — plus an optional end-of-game ceremony version. Switching collections is how you switch game versions. |
| **Manifest** | The JSON a client fetches (`/missionhydrosci/api/manifest`) describing the units, files, sizes, pieces, the ceremony, client timing values (`tuning`) and the game-service probes for **that user right now**. |
| **Content server** | Where the build files live (`MHSCDNBaseURL`, e.g. `https://<content-host>/mhs`). The origin only ever redirects to it or serves its files from cache. |
| **Cache Storage** | The browser's per-origin cache. Unit files live in caches named `missionhydrosci-unit-<unitId>-v<version>`; the app shell lives in `missionhydrosci-app-shell-v<n>`. |

**The load-bearing mental model:** Cache Storage, IndexedDB, and localStorage are
**per-origin (per browser profile), not per-StrataHub-login.** Every StrataHub
account used in the same browser profile shares one pool of MHS caches, and
switching StrataHub accounts clears none of it. The cache layer therefore has to
reclaim versions that no account on the device uses any more, and must never
delete a cache a running game is reading.

## How it operates

### Server side (Go)

All files are in this directory. The handler (`handler.go`) is a dependency
container wired up in `bootstrap/routes.go`, which also mounts the root-level
`/sw.js`, `/manifest.json` and `/missionhydrosci/content/*` routes and the
public device-test routes.

| File | Responsibility |
|------|----------------|
| `routes.go` | Chi routes under `/missionhydrosci`: units, manage, play, signed-in, ceremony, manifest, progress, save-data delete, device status, download errors, step logs and heartbeats, staff auth, collections. All but `/api/services-health` require a signed-in user with the MHS app enabled and run under a request timeout. |
| `units.go` | `ServeUnits` — the launcher page: resolves the manifest, loads progress, computes current/next unit and per-unit status. Status-only: per-unit actions live on the manage page. |
| `manage.go` | `ServeManage` — troubleshooting and testing tools (per-unit Download / Clear / Set current, end of game and its undo, Clear All Downloads, Reset all MHS data, game version, delete online saved game). For members in a `keyword`/`staffauth` workspace the page is gated server-side by a staff unlock; `HandleManageUnlock`, `HandleManageLock` and `ServeManageStatus` drive the gate and its countdown. |
| `play.go` | `ServePlay` — the Unity host page for one unit, with a **progress gate** (members may only open their current or completed units; staff may open any). `RedirectToPlay` handles game-initiated URL-mode unit transitions. |
| `api_manifest.go` | Collection resolution (`resolveCollection`), manifest assembly (`collectionToManifest`, `buildToManifestUnit`, pieces), the collection-override endpoints, and member auth (`checkMemberAuth`, `requireMemberAuth`, `unlockKey`). |
| `progress.go` | Per-user progress: get, complete-unit, set-to-unit (including the staff jump to the end of the game), and the game's end-game mark. Set-to-unit is member-auth enforced. |
| `savedata.go` | Deletes a user's online saved game state or settings in `stratasave` (game `mhs`), member-auth enforced, using the server-only delete key. |
| `staffauth.go` | Staff-authorization flow (login id → password / email code / trust → single-use token) used to authorize gated member actions. |
| `auththrottle.go` | Per-member backoff on failed member-auth attempts (see [Authorization model](#authorization-model)). |
| `ceremony.go` | The end-of-game ceremony host page, the student's EA checkpoint scores (read from the grader's database), and the viewed marks. Plan: `docs/mission-hydrosci/mhs-end-ceremony-plan.md`. |
| `services_health.go` | `/api/services-health` (public): the server's own view of the log and save services, probed at most once a minute. The play gate uses it as evidence that a service is up. |
| `logs_health.go` | Logging health: on every tenth heartbeat of a launch, checks whether the log service has received that launch's gameplay entries and returns the state in the heartbeat reply. |
| `steplog_member.go` | Stores a member page's step log as a `member` record at each outcome, and that record's heartbeats (see [Records and diagnostics](#records-and-diagnostics)). |
| `device_status.go` | Receives device/storage reports (`mhs_device_status`) and download failures (one bounded `"mhs download error"` log line each). |
| `devicetest.go` | The public **Unit 2 Device Test** (`/missionhydrosci/devicetest`, no session): landing form, run page, one-unit manifest, play in device-test mode, the run's telemetry endpoints, and the questionnaire; records in `mhs_device_tests`. Guides: `docs/mhs-device-test/`. |
| `signedin.go` | The page a student lands on after signing in again from a running game: it only points back to the game's tab. |
| `content.go` | `ContentFallback` — redirects `/missionhydrosci/content/*` to the content server when no service worker answers. |
| `sw.go` | Serves `/sw.js` — concatenates `static/sw-cache.js` + `static/sw-background-fetch.js` + `static/sw.js` and injects the asset-version constants. Served from **root scope** (`Service-Worker-Allowed: /`). |
| `manifest.go` | Serves the PWA `manifest.json`. |
| `appcheck.go` | `RequireApp` middleware — members only reach MHS if it is in their `EnabledApps`; staff always pass. |
| `types.go`, `templates.go` | View models and manifest types; template registration. |

**Manifest resolution is per user, most-specific wins** (`resolveCollection`):

1. Per-user override (`mhs_user_progress.collection_override_id`)
2. Per-group pin (`group_app_settings.mhs_collection_id`)
3. Workspace active collection (`site_settings.mhs_active_collection_id`)
4. None

This lets a QA tester pin a specific build to their own account while the class
default is unchanged. It is also the source of the shared-device tension
described below: the manifest is per user but the cache is per device.

**Server configuration that reaches the client** (`bootstrap/config.go`):
`mhs_frozen_switch_ms` (25 s), `mhs_fallback_stall_ms` (45 s),
`mhs_tail_stall_ms` (5 min) and `mhs_keepalive_ms` (10 s) are served in the
manifest's `tuning` block; the probes are derived from the configured log and
save endpoints (`ProbesFromServices`); `mhs_download_pieces_off = true` stops
listing pieces in the manifest (a kill switch: the whole files stay on the
content server).

### Client side (PWA)

- **`internal/app/resources/assets/js/mhs-delivery.js`** — the
  `MHSDeliveryManager`. Registers the service worker, fetches the manifest,
  checks per-unit cache status and file integrity, runs the preflights,
  orchestrates downloads (with retries that never give up), monitors progress
  and stalls, prunes stale caches, probes the game services, and exposes
  Clear/Reset. This is the brain of the client.
- **`internal/app/resources/assets/js/mhs-steplog.js`** — the per-load step log
  (`MHSStepLog`): every step as a timestamped entry, the shared "Status details"
  panel (`resources/templates/mhs_steplog_panel.gohtml`), Copy report, the
  renewable CSRF token (`MHSStepLog.csrf`) every page posts through, and the
  repair of the game's local log store.
- **`templates/missionhydrosci_units.gohtml`** — the launcher: progress dots,
  the current-unit download/launch card, the next-unit status, storage, the
  active game version, the ceremony row, and the auto-download pipeline (current
  unit, then the next). It warns before Launch when the last launch on this
  device was stopped by the play gate.
- **`templates/missionhydrosci_manage.gohtml`** — the manage page and its staff
  gate.
- **`templates/missionhydrosci_play.gohtml`** — the Unity host page (see
  [The play page](#the-play-page)).
- **`templates/missionhydrosci_ceremony.gohtml`, `missionhydrosci_signedin.gohtml`,
  `devicetest_landing.gohtml`, `devicetest_run.gohtml`** — the ceremony host,
  the signed-in-again page, and the device-test pages.
- **`static/sw*.js`** — the service worker (`SW_VERSION` in `static/sw.js`;
  see `sw.go` for how the files are combined). Handles install/activate,
  content interception, both download paths, pieces, resume, and the
  Background Fetch events.

### The lifecycle of a download

1. **Sign-in.** For staff, a post-login action only registers the service
   worker. Members land on the units page, which manages downloads; nothing
   else starts a download.
2. **Units page.** `MHSDeliveryManager.init()` registers the SW, requests
   persistent storage, fetches the manifest, reconnects to in-flight downloads
   (matched by unit **and** version), **prunes stale caches** (never one a live
   play session is using), and checks each unit's cache. The auto-download
   pipeline downloads the current unit, then the next.
3. **Integrity check.** Every cached file is checked against the manifest: its
   size, the loader's text for `createUnityInstance`, and the framework's bytes
   (not zero-filled, and defining `unityFramework` when uncompressed). A damaged
   file is removed so the unit becomes `partial` and only that file downloads
   again. A file removed twice for its size that comes back with the same size
   is kept, so a wrong size in the manifest cannot cause endless downloads.
4. **Preflights.** Before the first download: the content server (smallest file,
   8 s budget), the game services, and a space check of the bytes still missing
   against the free quota (re-checked every minute while it does not fit).
5. **Download.** Two paths, both run by the service worker:
   - **Background Fetch** (Chrome/Chromebook primary) — OS-level, survives
     navigation; the SW creates it (one creation per unit even when several
     pages ask at once) and broadcasts progress to every page.
   - **Direct download** (`fallbackFetch`: Safari/iPad, guest profiles, a
     refused or silent Background Fetch) — files one at a time inside the SW,
     streaming byte progress. A Background Fetch that shows no new bytes for
     the frozen-switch time switches to it, and the device then prefers it for
     an hour (`mhs-prefer-fallback-until`).

   Files go into `missionhydrosci-unit-<id>-v<version>`. On the direct path a
   file is saved in 8 MB resume parts as it arrives; a dropped connection
   resumes with a Range request, and only attempts that saved nothing count
   toward giving up (six). **Held tail:** some school filters hold the last
   megabytes of a large file; a file stuck in its last 16 MB after two
   attempts that saved nothing, or a partial from an earlier day, is fetched
   whole again (at most twice per file per day; after that the tail resumes go
   on). **Pieces:** a file the manifest lists with `pieces`/`pieceBytes` is
   fetched as its 8 MB pieces on both paths, each piece kept as it arrives (a
   retry costs at most one piece), then assembled into the single file the
   Unity loader expects; no large object ever crosses the filter.
6. **Never give up.** A requested download is a standing intent: each failure
   schedules the next attempt (backoff 5–60 s, reset by real progress), the
   `retrying` status carries the reason, a countdown and the attempt count, and
   Retry only skips the wait.
7. **Serve.** On launch, the SW intercepts `/missionhydrosci/content/*` and
   serves from the matching unit cache; a miss falls through, as a bare GET or
   HEAD, to the content-server redirect.

### The play page

`missionhydrosci_play.gohtml` loads the build, bridges identity and the
logging/save services into the game (`__mhsBridgeConfig`), and handles the end
of a unit.

- **Play gate.** Before the loader runs, the page sends one request in the
  game's own form (POST, JSON body, Authorization header, so an OPTIONS
  preflight first) to each service's `/probe` route
  (`MHSDeliveryManager.probeServices`). A service counts as blocked by the
  school only when the device is online and there is evidence the service is
  up: its host answers a plain GET, or `/api/services-health` says the server
  reaches it. A block is confirmed with a second probe that waits up to 45 s
  (a slow network is not a block). While blocked the game does not start: the
  loading screen says why, names the hosts for IT, rechecks every 30 s and
  starts the game by itself when they answer. A device without internet, or
  an outage on our side, still plays (the game keeps its activity and sends it
  later). A block found during play is confirmed the same way and shows the
  logging bar.
- **Launch watchdog and repairs.** 30 s without loader progress logs a stalled
  launch; at 90 s, while the loader script never arrived, the page tries again
  straight from the content server, bypassing the SW. A loader without
  `createUnityInstance` is removed from the cache and the game loads from the
  content server. A framework that loads but does not define `unityFramework`
  is removed and the page reloads once (outcome `launch-repair`); a second time
  in the tab is reported as a crash. The loader script loads with
  `crossOrigin = 'anonymous'` so its errors arrive with a message.
- **Crash reporting.** A masked cross-origin `Script error.` (no source, no
  line) is a browser notice, never a crash, and does not latch the crash
  reporter; pointer-lock, fullscreen and AudioContext refusals are notices too
  (same rule as `mhssteplog.IsCrashNotice` on the server).
- **One game window per device.** The game window is named `mhs-game`; Play on
  the units page reuses it (or tells it, over the `mhs-play-windows`
  BroadcastChannel, to switch units) instead of opening a second copy.
  Device-test windows stay out of this.
- **Bars.** One bar at a time: offline (the page's own heartbeats fail),
  signed out (posts answered 401; the bar opens sign-in in another tab, which
  lands on `/signed-in`), logging (no gameplay entries reaching the log service,
  or a blocked service), and the discard notice (Chrome discarded the window for
  memory and reloaded it; `document.wasDiscarded`). The offline and signed-out
  bars take precedence over the logging bar, and the discard notice gives way to
  all of them.
- **Heartbeats.** Every 30 s while the game runs, with a closing beat on
  `pagehide`: memory, frame rate, visibility, the game's local log store size.
  The reply carries the logging-health state. A separate play-session
  heartbeat (`mhs-active-play`, 30-minute TTL) keeps the stale-cache prune
  away from the cache the game is reading. A page restored from the
  back/forward cache reloads.
- **Unit complete.** The completion overlay reports completion (a refused
  completion is queued and sent again), downloads the next unit and moves on;
  a failed next-unit download sends the student to the units page. The game's
  EndGame marks the game ended and leads to the ceremony when the collection
  has one.

### Records and diagnostics

- **Member step-log records** (`steplog_member.go`,
  `POST /missionhydrosci/api/steplog`). Launcher and play pages store their
  step log (last 300 entries) as a `member` record in `mhs_device_tests` on an
  outcome: `download-failed`, `download-complete`, `download-switched`,
  `launch-failed`, `launch-stalled`, `launch-ok`, `launch-repair`, `crash`,
  `report` (Send report, with the student's note), `ceremony-ok`,
  `ceremony-failed`, `services-blocked`. Each record gets a summary
  (`mhssteplog.Summarize`) and, when the latest `services` step failed, the
  blocked services. A launch record receives the play page's heartbeats
  (`/api/steplog/{id}/heartbeat`).
- **Logging health** (`logs_health.go`). On every tenth beat (first check about
  4.5 minutes into play, then every 5 minutes) the server asks the log service's
  database whether entries for this launch have arrived: `seen`, `none` (nothing
  arrived while the game's local store grew, i.e. events are being produced and
  not sent), `quiet` (nothing arrived and nothing is being produced) or
  `unknown`. `none` is flagged on the record only after 4 minutes of play.
- **Device status and download errors** (`device_status.go`).
- **Where they are read.** The Device Tests viewer (`viewers/views/devicetests.go`,
  Kind = Member load record), and the **MHS Activity** page and **MHS Daily
  Digest** (`features/mhsactivity`, `/mhsactivity` and `/mhsactivity/digest`),
  which summarize the records per school and day.

## Authorization model

Certain member actions require authorization by a higher-privileged user.
**Which** authorization is a workspace setting, `MHSMemberAuth`:

- `trust` — no extra check (used by the **dev** workspace so testing is frictionless).
- `keyword` — a shared keyword, validated server-side (constant-time compare).
- `staffauth` (the default) — a teacher/coordinator/admin authorizes via login id
  plus password or emailed code, yielding a single-use token bound to the
  workspace. Staff accounts that sign in with SSO cannot authorize here yet and
  fail closed (`SSO-STATUS.md`). An unknown mode also fails closed.

**Staff unlock.** In `keyword`/`staffauth` workspaces the manage page renders
nothing but its gate for a member until staff authorize on the device. Valid
credentials grant an unlock (`staffauth.UnlockStore`, `mhs_staff_unlocks`) keyed
by workspace, user and **session token**, so it dies with sign-out and cannot
follow the account to another device. The unlock lasts
`GetMHSStaffUnlockDuration()` (workspace setting `MHSStaffUnlockMinutes`,
default 10 minutes) and **slides**: every gated action under it pushes the expiry
out again. The page shows a countdown measured against the server's clock, and
"Lock now" revokes the unlock. Gated actions under an unlock are logged with who
granted it.

Every gated member action is enforced server-side by `checkMemberAuth` (an active
unlock, or fresh credentials, which grant one):

| Action | Server-enforced |
|--------|-----------------|
| Open the manage page content | **yes** (`manage.go`: no content without an unlock) |
| Set current unit / end of game (`set-unit`) | **yes** (`progress.go`) |
| Switch collection version (`collection-override`, incl. "Use default") | **yes** (`api_manifest.go`) |
| Delete online saved state / settings | **yes** (`savedata.go`) |
| Download / Clear a unit, Clear All Downloads, Reset all MHS data | via the page gate; the operations themselves are local cache operations |

**Auth throttle** (`auththrottle.go`). Failed attempts (keyword, staff token,
password, email code) and every staff-auth *start* (a start sends a code, so
repeated starts are themselves the abuse) share one per-member budget: three free
attempts, then a wait doubling from 2 s to at most 60 s, cleared by success or
10 minutes of quiet. A throttled request gets 429 with `Retry-After`, and the page
says how long to wait. It is a deterrent, not a hardened control: state is
in memory per process. Staff-auth start answers one identical message for an
unknown, non-staff, other-workspace or disabled login.

**Game-service keys.** The game calls the log and save services directly (also
offline, later), so the play page carries their keys and every browser holds
them; this is an accepted trade-off of the game-bridge design. The save
service's delete key (`GameServices.SaveAdminAuth`) is used only by this server
and never rendered into a page.

## The cross-platform / cross-use-case challenges

The same code has to behave correctly across a matrix of platforms, networks,
and usage patterns that each stress a different browser API.

### By platform

- **Chrome / Chromebook (real profile).** Background Fetch works, survives
  navigation, and reports byte-level progress. Chrome streams live progress only
  to the context that created a fetch, which is why the SW creates them and
  broadcasts progress. Managed Chromebooks sometimes refuse or silently pause
  Background Fetch; the SW announces the method actually running and the page
  switches to the direct path. 4 GB Chromebooks discard game windows for memory
  (hence one window per device and the discard notice).
- **Chromebook guest / incognito profile.** Background Fetch is disabled, so
  every download uses the direct path
  ([MHS-004](issues/MHS-004-guest-profile-background-fetch-disabled.md)), and
  site data may not survive the session.
- **Safari / iPad.** No Background Fetch at all — the direct path is the only
  path. iOS terminates service workers whose work outlives the event budget,
  suspends timers in hidden tabs, and has its own HTTP/bfcache quirks; the
  resume parts, the stall watchdog and the visibility-aware heartbeats exist for
  this. The iPad path has not had a dedicated device pass.

### By network condition

- **Slow / flaky Wi-Fi.** A healthy-but-slow download must not be aborted
  ([MHS-011](issues/MHS-011-frozen-progress-false-stall-abort.md)); a dropped
  connection must resume, not restart
  ([MHS-003](issues/MHS-003-fallback-fetch-leaves-partial-cache.md)). Partials
  are kept for resume and purged only on explicit cancellation.
- **School web filters.** A filter can block the content server (the preflight
  fails in seconds with the host to allow), hold the last megabytes of every
  large object it scans (whole-file restart, then pieces), store its own reply
  under a game file's key (the integrity checks and launch repairs), or let the
  pages through while stopping the game's log/save requests (the play gate).
- **Offline.** Once cached, units must launch with no network. A cache deleted
  under a running game
  ([MHS-013](issues/MHS-013-prune-races-live-game-session.md)) or a false "Ready
  to play" over an empty cache
  ([MHS-002](issues/MHS-002-clear-all-leaves-orphaned-versions.md),
  [MHS-016](issues/MHS-016-fallback-download-lifecycle-gaps.md)) only shows up
  when the student is offline, which is why both are guarded.

### By storage condition

- **Origin quota is finite and shared.** `navigator.storage.estimate()` is
  origin-wide, and Cache Storage is evictable unless persisted (`persist()` is
  requested at init). Old-version caches are pruned
  ([MHS-001](issues/MHS-001-stale-unit-caches-never-cleaned.md)), and a unit
  that will not fit is caught by the space preflight before downloading.

### By use case

- **QA testers.** They create a fresh account per test run and switch builds
  often, which maximizes orphaned caches on the shared per-origin pool. They
  have an in-app clean slate (Reset all MHS data,
  [MHS-009](issues/MHS-009-reset-all-mhs-data.md)) and all-version clearing.
- **Students in a class.** A class shares one collection, so there is little
  version churn; the design optimizes for this case while tolerating the QA case.
- **Shared devices, per-user manifests.** Pruning to "my manifest" means two
  accounts on one profile using different collections evict each other's unit
  caches. This is deliberate: students in a class share a collection, QA testers
  want the old data gone, and under quota pressure evicting the other collection
  is correct.
- **Mid-session version bumps.** A content deploy can bump a unit's version
  while a student is mid-game on the old one; the prune must not delete the cache
  the running game reads (the play-session heartbeat).

### By deploy lifecycle

- **Service worker updates.** The SW precaches and serves the hash-versioned app
  assets under their exact `?v=<hash>` URLs, so a new deploy is fresh on the
  first load, and the injected hashes change the SW bytes so browsers install
  the update ([MHS-012](issues/MHS-012-app-shell-assets-stale-first-load.md)).
  The install never fails on one URL (a signed-out device tester gets 401 for
  the units page). Standing rule: **keep SW message actions and broadcast status
  values backward-compatible**, because an old page can briefly drive a new
  worker (and vice versa) during any update. Broadcasts carry `detail.version`;
  pages ignore a status for a version they are not on, and untagged broadcasts
  still pass.

## The issues folder

[`issues/`](issues/) holds the download and cache issues MHS-001…021 (all
shipped; MHS-004's guest-profile verification is still open). Its own
[README](issues/README.md) has the symptom → root-cause map, each issue's status,
and the smoke-test checklist. Later field issues and their fixes are recorded in
`docs/mission-hydrosci/` (`mhs-issues-102826.md`,
`mhs-issues-remaining-plan-110926.md`, `mhs-resolution-plan-111426.md`), and the
open backlog is `docs/mission-hydrosci/mhs-remaining-work-plan.md`.

## Where state lives (and what is safe to delete)

This is *why* the Clear/Reset controls are safe:

| State | Location | Cleared by local reset? |
|-------|----------|-------------------------|
| Player progress (current/completed units, end-of-game mark, ceremony viewed) | `mhs_user_progress` (server) | **No** — server-side |
| Collection override (chosen version) | `mhs_user_progress` (server) | **No** — server-side |
| Game saves and settings | `stratasave` service | **No** — online (deleted only by the manage page's gated delete) |
| Staff unlock | `mhs_staff_unlocks` (server, TTL) | **No** — expires, or Lock now / sign-out |
| Step-log and member records | `mhs_device_tests` (server) | **No** |
| Downloaded unit files (and resume parts, pieces) | Cache Storage (per origin) | Yes |
| App shell (page, CSS, delivery and step-log JS) | Cache Storage (per origin) | Yes (Reset only) |
| Manual-download list, progress queue, progress-changed flag, play-session heartbeat | localStorage | Yes (Reset) |
| Device id (`mhs-device-id`) | localStorage | No — kept for telemetry continuity |
| Prefer-direct-download hint (`mhs-prefer-fallback-until`) | localStorage | No — a device capability hint; expires after an hour |
| Size-repair counts (`mhs-size-repairs`), last gated launch (`mhs-services-blocked`) | localStorage | No |
| Current page's step log (`mhs-steplog`) | sessionStorage | Ends with the tab |

So "Clear All Downloads" and "Reset all MHS data" are **local device
operations**. They never lose progress or saves, which the UI copy says
explicitly.

## Building, deploying, and testing

- **Build order:** `make css-prod` **then** `go build` — Tailwind CSS is embedded
  in the binary, so a new template class must be compiled before the Go build.
- **Service worker version:** `static/sw.js` carries `SW_VERSION` (bump it on
  real SW-code changes); the served worker also embeds the asset content hashes,
  so any asset change makes the SW bytes differ and browsers install the update.
- **Go tests:** `go test ./internal/app/features/missionhydrosci/`. They cover the
  auth throttle, member auth and the staff unlock (keyword grant, session scope,
  fail-closed mode, unlock/lock/status endpoints), save-data deletes, services
  health and probes, manifest pieces, device-test step derivation, heartbeats,
  the ceremony, and rendered pages (play gate, one game window, discard notice,
  masked script errors, damaged loader, signed-out bar). `play_render_test.go`
  also runs `node --check` over the units and play pages' inline scripts
  (skipped without `node`). The manage tests need a local MongoDB
  (`testutil.SetupTestDB`).
- **JS/SW checks:** `node --check` each JS file, and concatenate the three SW
  files with a stub `MHS_ASSET_VERSIONS` const and `node --check` the result
  (that is what `/sw.js` serves).
- **Real devices:** the smoke-test checklist is in
  [`issues/README.md`](issues/README.md).
