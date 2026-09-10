# Mission HydroSci — Unit Loading Status and Unit 2 Device Test — Plan

**Date:** 2026-09-07 (revised twice the same day after review: the device test is standalone, one fixed route per workspace, no accounts, no sessions, no links to manage)
**Status:** Approved 2026-09-07, not started
**Scope:** two related pieces of work in `stratahub`, feature `internal/app/features/missionhydrosci`:

- **A. Unit loading.** Answer the field reports (connection errors, downloads sitting at 0%, the long wait before the backup download method takes over) and add a visible step-by-step status so a user can see, and report, where a load fails.
- **B. Unit 2 Device Test.** One URL per workspace that a school can open, with no account and no login, that downloads and plays Unit 2 in the real Mission HydroSci context, collecting as much device, network, download and gameplay data as we can, with an admin view to inspect and download it.

---

## 1. Recommendation: A first, in two short steps, then B

Do the loading work first, but keep it to two small deliverables: **A0** (timing and a fail-fast connection check, about a day, ships alone) and **A1** (the step log and its display, two to three days). Then build the device test on top of A1.

Why this order:

1. Students are hitting the loading problem now, during the September launch. A0 is small and can go out on its own.
2. The device test is only worth building if it tells us *where* a device fails. That instrument is A1's step log with its copy/send report. Building B first would ship a test that says "failed" and nothing else, which is the very complaint in the reports.
3. A1 is shared code. The same step log drives the units page, the play page, and the device-test record. It is the first half of B, not a detour from it.
4. B's server side (setting, test records, viewer) does not depend on A. If a second person or session is available, it can start in parallel with A1.

Order: **A0 → A1 → B → A2**. A2 is explained in §3; it reuses B's storage, so it comes last, unless B slips, in which case it can follow A1 directly (see the sequencing note in §3).

---

## 2. What was verified in the code (2026-09-07)

Facts the plan depends on, with locations. Line numbers are as of HEAD `4bf18c7`; re-check before editing.

### Download timing — `internal/app/resources/assets/js/mhs-delivery.js`

- `FROZEN_SWITCH_MS = 60000` (:530). A Background Fetch that has delivered no new bytes for 60 s while the tab is visible is abandoned for the service-worker sequential fallback (`_pollDownloadOnce`, :871-885). This is the "60 seconds" in the reports. The poll runs every 5 s (:527), so the real wait is 60–65 s after the last byte.
- `STALL_THRESHOLD_MS = 150000` (:528). The fallback path (iPad, guest profiles, and every device after the switch) surfaces "Stalled" + Retry only after **2.5 minutes** of silence. That is the even longer wait on the devices that already use the fallback.
- `PROGRESS_KEEPALIVE_MS = 20000` (:59). Pages re-attach the service worker's progress broadcaster every 20 s. If the worker is restarted mid-download, a healthy download can look silent for up to about 20 s. This bounds how far the frozen-switch can safely be lowered (see A0).
- The switch is immediate, with no wait, when Background Fetch throws on start (`static/sw-background-fetch.js:156-159`), when the 24 h prefer-fallback pin is set (:542-552, :1268-1272), or when the API is absent (:1298).
- Chrome streams live byte counts only to the context that created the fetch. The SW creates it and broadcasts progress; a page that misses broadcasts sees a frozen number, often 0%, while the download is healthy (comment at :812-841). This is one plausible source of "0%" reports that are not actually stuck.

### What the user sees today

- One replaced status string per surface: "Checking...", "Downloading...", "Download stalled or paused — …", or `detail.error` (`templates/missionhydrosci_units.gohtml:457-506`), a percent-only progress bar, and a download-mode notice. The play page shows "Initializing… / Starting Unity… / Loading assets… / Compiling… / Almost ready…" (`missionhydrosci_play.gohtml:741-761`).
- **There is no per-step log, no timestamps, and nothing a user can copy.** MHS-008 (`issues/MHS-008-diagnostics-logging-gap.md`) documents this gap. Only the download-error stopgap shipped (`device_status.go:101-137`): a server log line, nothing stored, no admin view.

### Identity and telemetry

- The play page injects `window.__mhsBridgeConfig = { identity: {user_id, name}, services: {log_submit, state_save, state_load, settings_save, settings_load} }` (`missionhydrosci_play.gohtml:264-285`) and repeats identity with `SendMessage('MHSBridge', 'OnPWAReady', …)` (:772-775). `user_id` is the 24-hex `users._id` from the session (`play.go:45-48`).
- stratalog and stratasave authenticate with one static per-deployment Bearer key each (`stratalog/internal/app/system/auth/apikey.go`, same in stratasave), rendered into the launch page by StrataHub from `GameMHSLogAuth` and `GameMHSSaveAuth` (`bootstrap/appconfig.go:119-127`). The keys are never in a URL. Both services accept **any** `user_id` matching `^[0-9a-f]{24}$` and reject anything else (`stratalog/.../logapi/handler.go:24, 145-170`). No `users` row is consulted.
- Log records carry `game, user_id, eventType, eventKey, sceneName, version, data, device`. No login id, device id, or session id (`internal/app/store/logdata/store.go:15-26`).
- **mhsgrader grades whatever `user_id` appears in `logdata`.** Its scanner reads trigger events from the log collection and grades per event `UserID` (`mhsgrader/internal/app/grader/scanner.go:48-85`); it never looks at `users`. A generated id therefore gets progress points with no account. The `getOrMigrateID()` helper named in the root `CLAUDE.md` and `ARCHITECTURE.md` does not exist anywhere in code.
- A real `users._id` is a MongoDB ObjectID: its first 8 hex characters are the creation time in seconds. Every real id was created at a real time (the May 2026 migration generated them at migration time), so an id whose first 8 hex decode to an impossible date can never collide with a real one. The dev sentinel already uses this idea with `000000000000000000000001` (`bootstrap/startup.go:239`).
- The MHS dashboard joins `logdata` and grades to **members** by `user_id` hex (`mhsdashboard/dashboard.go:658-693`), so ids without a `users` row are not listed there. The device test does not need the dashboard: its own viewer queries `logdata` and the grades by id (see 4.6).

### Routing and reusable patterns

- Everything under `/missionhydrosci` requires a session, a role, and `RequireApp("missionhydrosci")` (`routes.go:24-26`). Already public, registered at the root router beside that mount: `/sw.js`, `/manifest.json`, `/missionhydrosci/content/*` (302 to the CDN) (`bootstrap/routes.go:313-315`). A more specific path such as `/missionhydrosci/devicetest/*` can be registered the same way and wins over the mount.
- Every page registers the service worker with scope `/` (`missionhydrosci_units.gohtml:369`, `_manage.gohtml:499`, `_play.gohtml:509, 862`), and the worker intercepts `/missionhydrosci/content/*` for any controlled same-origin page (`static/sw.js:48-52`). A standalone page anywhere on the origin therefore gets Unit 2 from the cache after download.
- The workspace comes from the host (`system/workspace/workspace.go` middleware), with no session involved, so one route serves every workspace on its own host. `resolveCollection` (`api_manifest.go:37-90`) falls through to the workspace's active collection when there is no signed-in user; `collectionToManifest` (:415-463) turns unit + version pairs into file lists from `mhs_builds`, which is the record of what is on the CDN (`domain/models/mhs_build.go`).
- The workspace's MHS settings live in `site_settings` (`SiteSettings.MHSActiveCollectionID`, `models/sitesettings.go:50`) and are edited on `/settings`; handlers overlay the form onto the stored document, so adding fields is routine.
- gorilla/csrf issues tokens to anonymous pages (the `/login` form relies on it), so public device-test pages can POST with the normal `X-CSRF-Token` header. Template for a public rate-limited endpoint: `features/memberstatusapi` (`ratelimit.New` + `ClientIP`, `io.LimitReader`, `maintenance.isExemptPath`).
- Viewers framework (`docs/viewers/adding-a-viewer.md`): one Go file per viewer, a `{workspace_id, sortField, _id}` compound index, CSV export for free. `viewscope` hides rows without `user_id`/`organization_id` from coordinators and leaders; admins and analysts see everything.

---

## 3. Workstream A — unit loading

### A0. Fast fixes (about 1 day; ships alone)

> **Status (2026-09-07): implemented, committed (`5c12cde`) and deployed to the shared adroit.games service at 16:16 PDT; device verification still pending.**
> Items 1–6 below, the never-give-up retry scheduler and the space preflight are live. Verified locally before deploy:
> `node --check` on the delivery JS and the concatenated worker, all four
> templates parse, `go build`, `go vet`, and the feature and bootstrap Go tests
> pass. After deploy: `/health` 200, the served delivery script carries the new code, the worker serves a new asset hash, no errors in the service log. Next: run the device recipe at the end of this section.
> Server-side additions: config keys `mhs_frozen_switch_ms`,
> `mhs_fallback_stall_ms`, `mhs_keepalive_ms` (defaults 25000 / 45000 / 10000)
> served in the manifest's `tuning` block; the manifest also carries `probes`
> (the log and save services' `/health` URLs) for the preflight; device-status
> reports gain `storage_persisted` and `background_fetch_available`;
> download-error telemetry gains a one-line `preflight` summary. The service
> worker is unchanged (`SW_VERSION` stays 1.0.12; the delivery JS hash change
> is what triggers the worker update).

Goal: cut the dead time before the backup path starts, fail fast with a specific message when the content server is unreachable, and make any remaining wait visible.

**The three timing values this step changes** (all constants in `mhs-delivery.js`; what they mean in plain terms):

| Value | Today | Proposed | What it controls |
|---|---|---|---|
| Frozen-switch | 60 s | 25 s | How long a Chrome background download may show **no new bytes** (with the tab visible) before we give up on it and start the direct download instead. This is the wait the reports complain about. |
| Fallback stall | 150 s | 45 s | How long the **direct** download (iPad, guest profiles, and any device after a switch) may go silent before we act. Today it is 2.5 minutes before "Stalled" appears; now: after 45 s it restarts automatically, every time, resuming from the files already cached. |
| Keepalive | 20 s | 10 s | How often the page nudges the service worker so it keeps reporting progress. Matters because a healthy download can *look* frozen for one keepalive interval after the worker restarts; the frozen-switch must be longer than this or it would abort good downloads. Halving it is what makes 25 s safe. |

1. **Frozen-switch 60 s → 25 s and keepalive 20 s → 10 s.** `FROZEN_SWITCH_MS` and `PROGRESS_KEEPALIVE_MS`. Keep the existing guards: visible tab, `result === ''`, no new bytes this tick. A false switch is cheap: Background Fetch caches atomically so nothing already downloaded is lost, and the direct path works everywhere; the cost is only "keep the tab open".
2. **Never give up.** Split `STALL_THRESHOLD_MS`: `BG_STALL_MS` stays 150 s (the frozen-switch fires long before it), `FALLBACK_STALL_MS` becomes 45 s. The fallback broadcasts at least once a second while bytes move (`sw-background-fetch.js:342-346`); 45 s of silence with the tab visible means a dead worker loop or a black-holed connection, and the download restarts automatically, every time, resuming from the files already cached. More generally, a requested download is now a standing intent: **every** failure (network error, blocked content server, out of space, manifest not loading, Background Fetch failure) schedules the next attempt itself with a short backoff (5, 10, 20, 30, then 60 s, repeating; reset by real progress), a `retrying` status carries a live countdown and attempt count, and the Retry button only skips the wait. Nothing asks the user to notice that the network came back. Retries stop only when the unit is cached or the user removes it. Repeated Background Fetch failures switch the device to the direct path. Decision (Dale, 2026-09-07): giving up costs the user a click they don't know to make; retrying costs nothing as long as the page says what it is doing.
   Also added here: a per-unit **space preflight** (MHS-006 / RDY-3): before each attempt the bytes still missing from the unit's cache are compared with the free quota; if they do not fit, the page says how much is free and how much is needed, and re-checks every minute so freeing space resumes the download by itself.
3. **Content-server preflight, so "connection errors" fail fast and specifically.** Before the first download in a page session, fetch the unit's smallest file from `cdnBaseUrl` with `mode: 'cors', cache: 'no-store'` and an 8 s `AbortController`. Success records latency. Failure or timeout fires `'error'` with `errorClass: 'cdn-unreachable'` and the message "Cannot reach the game content server (cdn.adroit.games). A school firewall or content filter may be blocking it." and **skips the 25 s wait entirely**. Also probe the log and save service hosts (their health or ping route; use `mode: 'no-cors'` if those routes lack CORS, since an opaque response still proves reachability) and record the result. A blocked game service is the likeliest cause of an in-game "connection error", and nothing reports it today.
4. **Make the wait legible now** (interim, replaced by A1). The hero status shows bytes and elapsed time and, while frozen, a countdown: "Waiting for the download to start… 12 s. If nothing arrives in 13 s, switching to the direct download method." It reads the existing stall-state timestamps.
5. **Fold in two one-liners from the backlog** because the device test will report them: `navigator.storage.persist()` plus `persisted()` (RDY-2 / MHS-005) and the `crypto.randomUUID` fallback for the device id (UX-5).
6. **Done:** the three values are served through the manifest JSON (`tuning: {frozenSwitchMs, fallbackStallMs, keepaliveMs}`) from the config keys above, so they can be adjusted per deployment with a config edit and restart instead of a JS deploy. (Per-workspace values would be a Site Settings addition later if ever needed.)

Verification: `node --check` on the JS and the concatenated SW; template parse; deploy to dev. Then the ACER recipe from `docs/mission-hydrosci/mhs-acer-paused-download-diagnosis-072726.md` (Reset, download, must auto-switch with no Retry, now within about 30 s). A healthy slow download must **not** switch: throttle to Slow 3G in DevTools and confirm bytes keep flowing and no switch fires. iPad fallback stall: cut Wi-Fi mid-download, expect one auto-resume, then Stalled. Items 1–4 need no SW logic change (`SW_VERSION` unchanged); the probe is page-side.

### A1. Step/status log and display (2–3 days)

> **Status (2026-09-07): implemented.** `mhs-steplog.js` (hash-versioned and
> precached like the delivery script; `SW_VERSION` 1.0.13 adds the
> `getVersion` reply), the shared `mhs_steplog_panel` snippet in the shared
> templates set (each page compiles against the shared set only, so a partial
> used by three pages has to live there), delivery-manager hooks for every step
> below, and the panel on the units, manage and play pages (the play page shows
> the current step under the loading bar and reveals the panel when a launch
> step fails). Copy report is in; Send report waits for B's storage as planned.
> Verified: JS syntax, concatenated worker, template parse with the snippet,
> `go build`/`vet`/tests, a local server start that compiles every page.

Goal: every load, on every surface, produces a human-readable, timestamped step list that the user can see and copy, and that the server can store.

**Client module** `internal/app/resources/assets/js/mhs-steplog.js` (new; add to the precache list in `sw.go` so it is versioned like `mhs-delivery.js`, per MHS-012):

- API: `MHSStepLog.start(context)`, `record(stepId, state, message, detail)`, `onChange(cb)`, `toText()`, `toJSON()`, `flush()`.
- Entry shape: `{t: msSinceStart, at: ISO, step, state: running|ok|warn|fail|info, msg, detail}`. Capped at 300 entries. Mirrored to `sessionStorage` so a reload keeps the log. The same log object is handed to the delivery manager (`manager.setStepLog(log)`) so every hook below writes into one place.
- Steps, with fixed ids (messages are the user-facing text):
  1. `sw` — registering → active (worker version via a new `getVersion` SW message with a 2 s timeout; older workers report "unknown").
  2. `manifest` — loading → loaded (collection name, unit, version, size) or failed.
  3. `storage` — estimate, persisted, low-storage verdict, cache inventory summary (count and bytes of `missionhydrosci-*` caches).
  4. `cdn` — the A0 preflight: latency, or the blocked message.
  5. `services` — log and save service reachability.
  6. `method` — "Background download (Chrome)" or "Direct download (keep this tab open)" and *why* (no API, a prior pause on this device, Background Fetch failed to start).
  7. `download` — started → progress samples every 5 s (bytes, rate, ETA) → frozen countdown → switched → stalled or auto-resumed → complete (bytes, elapsed, average rate) or error (`errorClass`, `failureReason`, `rawError`).
  8. `verify` — the size-check result (existing `_checkUnitCache` outcome).
  9. `launch` — play page: loader script fetched (cache or network), Unity progress milestones with timings, `createUnityInstance` resolved, `OnPWAReady` sent, first frame observed.
  10. `game` — page-side events the game already surfaces: crash reports, unit complete.
- Hook points: `init()`, `refreshManifest()`, `downloadUnit()` / `retryDownload()` / `_startFallbackDownload()`, `_pollDownloadOnce()` (frozen countdown, switch), `_maybeFireStalled()`, terminal statuses in `_fireStatus()`, and the play page's loader, progress, instance and `reportCrash` callbacks.

**UI**

- Units page: a "Status details" panel under the hero card, collapsed by default, auto-expanded on any `warn` or `fail`, always expanded in device-test mode. One row per step: icon, name, current message, elapsed. A live line for the running step. A **Copy report** button producing plain text: a header (date, host/workspace, device id, SW version, asset hash, user agent, unit/version/collection) followed by the step lines, ready to paste into Discord or an email. **Send report** (log plus an optional note to the server) arrives with B's storage.
- Play page: the loading overlay gets a second line with the current step and elapsed time. On failure it shows the step list and Copy report, since the overlay is the only thing on screen at that point.
- Manage page: the same panel. The pipelines are already duplicated between units and manage (ARCH-1); keep the panel a shared snippet template and the logic in the module so this does not add a third copy.

Verification: the smoke checklist in `issues/README.md`, plus: reload mid-download keeps the log; Copy report yields the full text on Chromebook and iPad (clipboard API with a textarea fallback); both themes.

### A2. Also keep students' step logs on the server (about 1 day) — implemented 2026-09-07

> **Status (2026-09-07): implemented.** `POST /missionhydrosci/api/steplog`
> (session-gated) stores a member's step log as a kind "member" record in
> `mhs_device_tests`: the delivery manager sends it when a download completes
> (once per unit) or fails (first time, then at most every 10 minutes while
> the automatic retries continue), and the play page sends it when a launch
> fails, when the game renders its first frame, or on a crash while loading.
> The Device Tests view shows these rows under Kind = "Member load record",
> with the member's name and organization and the outcome. With this, every
> step of the plan is in place; device verification remains.

**What this means.** After A1, every student's units and play pages build the same step log the device test builds, but for students it only lives in the browser (they can copy it). A2 makes the page send that log to StrataHub when a download or launch *fails* or *finishes*, and stores it beside the device-test runs (same collection, `kind: "member"`, same viewer with a kind filter). The result is that a field report like "connection error on a Chromebook in room 12" can be looked up by member, device and time and read step by step, without asking the student to copy anything and without a device test. It is bounded (one record per failure or completion, at most 300 entries, 16 KB) and needs no new UI beyond the viewer filter.

Endpoint: `POST /missionhydrosci/api/steplog` (session-gated, CSRF token as today), deduped per device, unit and outcome per page session. This closes MHS-008 (DOC-3) and gives OPS-1 something concrete to watch.

**Sequencing note.** A2 shares B's collection and viewer, so the natural slot is after B. If B slips, A2 can follow A1 directly: create the `mhs_device_tests` collection and the viewer then (student rows only), and let B add the form fields and public routes later. Either way the diagnostic value starts the moment students' failures are being stored.

---

## 4. Workstream B — Unit 2 Device Test (standalone, one route per workspace)

> **Status (2026-09-07): implemented.** Site Settings gained the enable switch
> and the Unit 2 build selector; the public routes live under
> `/missionhydrosci/devicetest` (landing, start, run page, one-unit manifest,
> play, diagnostics, steps, summary, report, complete), registered at the root
> router beside the content fallback; the play template has a device-test
> mode; `mhs_device_tests` holds the runs (store, indexes, marked ids via
> `models.NewMHSDeviceTestUserID`); the **Device Tests** viewer (admin,
> analyst) shows runs with the step timeline, diagnostics and the run's game
> telemetry, plus CSV, JSON and per-run JSON export (the JSON export is a new
> optional capability of the viewers framework). Admin guide:
> `../mhs-device-test/admin-guide.md` (with a tester guide beside it). Verified: build, vet, Go tests (settings render,
> viewers, feature helpers, id marker), template parse, a local server run
> (landing renders, bogus run id 404, viewer and settings behind login).
> Device verification pending with the rest.
>
> **Addendum (2026-09-08): heartbeats.** A real run on a Mac crashed the tab
> ("Aw, Snap", error code 5) on Unit 2's second screen, and a crashed page
> can report nothing, so the run only showed "Gameplay". The play page now
> sends a heartbeat every 30 s while the game runs (Unity WebAssembly heap,
> JavaScript heap, frame rate, tab visibility) and a closing beat on
> pagehide; the store keeps the last 240 beats per run. A run whose beats
> stop for two minutes without a closing beat or a completion is shown as
> "Page stopped responding at …" with its last memory figures, and the
> detail lists the trend. Members' launches get the same through their
> launch record (`/api/steplog/{id}/heartbeat`).
>
> **Addendum (2026-09-08): post-play questionnaire.** Once the game has been
> launched, the run page shows five questions (sound, controls, picture,
> performance, how far) plus notes; answers are stored on the run record
> (`questionnaire`), can be updated, and appear as a Sound column and filter
> and in the detail and exports. State is server-side only: the run URL
> identifies the run, and the page renders the questionnaire from the
> record's stage and answers.

### 4.1 Goals and non-goals

- **One fixed URL per workspace:** `https://<workspace-host>/missionhydrosci/devicetest`. There is nothing to create or manage; the workspace is resolved from the host, so the same route works in every workspace.
- A school opens it, fills in a short form, and Unit 2 downloads and launches exactly as it would for a student: same service worker and cache, same CDN, same log and save services, same play page and bridge configuration.
- Which Unit 2 runs is chosen in the workspace's settings from the builds already on the CDN (4.3). Only that unit exists in the test; there is no other unit to fall back to and no progress record to reset.
- **No account, no login, no StrataHub session.** Nothing is written to `users`, organizations, groups or `mhs_user_progress`. Credential handling in StrataHub is untouched, so a tester's email can later become a normal study account with no overlap.
- Every run gets its own game id that cannot collide with a real user id and is recognizable as a test id by its value (4.4).
- We can see who ran it (school, person, device) and what happened at every step, and download all of it. Everything is kept; deletion or management tools can be added later if wanted.
- Non-goals: per-user game-service credentials; testing units other than the chosen one; offline replay of the test.

### 4.2 Flow

1. An admin turns the device test on for the workspace in Site Settings and picks the Unit 2 build (default: the active collection's Unit 2).
2. The tester opens `https://<workspace-host>/missionhydrosci/devicetest` and sees a public landing page: what the test does, what is collected, and the form: school/district, name, role, email (optional), device type, whether the device is school-managed, network (school Wi-Fi or other), notes, a consent line, and "Start Unit 2 test".
3. The server creates a **test record** with a fresh test id and redirects to the run page `…/devicetest/run/<test-id>`. The id is unguessable and doubles as the game's `user_id` (4.4); no cookie is needed.
4. The **run page** shows one card, Unit 2, with the A1 status panel open. It registers the service worker, fetches a one-unit manifest, downloads, verifies, and shows Launch. Every step-log entry streams to the server.
5. **Launch** opens `…/devicetest/run/<test-id>/play`: the existing play template in device-test mode. StrataHub renders the game-service URLs and Bearer keys into the page exactly as it does for students, with `identity.user_id` set to the test id. The game logs to stratalog and saves to stratasave as usual.
6. When the game reports unit completion, the page records it on the test and shows a "Test complete" screen (with the short id to quote back to us) instead of transitioning to another unit. Reaching gameplay, crashes and completion are all recorded on the test.
7. Admins and analysts review runs at `/views/device-tests`: filter, drill in, CSV, JSON.

### 4.3 Workspace setting: which Unit 2

Two fields on the workspace's Site Settings page (MHS section, beside the active collection), stored in `site_settings`:

- `mhs_device_test_enabled` (bool, default off). When off, the route renders "The device test is not enabled for this workspace."
- `mhs_device_test_unit` (`{unit_id, version}`, default empty). The selector lists every `mhs_builds` record for `unit2` (version, build identifier, upload date, and which collections contain it) so the choice is limited to builds that are already on the CDN. Empty means "the active collection's Unit 2". The selector can be widened to other units later without changing anything else.

The run page's manifest is built from that one build with `collectionToManifest`'s per-unit logic, plus the A0 tuning block.

### 4.4 Identity: test ids that cannot be real ids

- The game needs a 24-hex `user_id`; stratalog, stratasave and mhsgrader accept any well-formed value and never check `users`. The requirement is therefore that test ids can never equal a real id and can be recognized on sight.
- **Format:** a fixed 8-hex marker followed by 16 random hex characters (64 random bits): `<marker><16 random hex>`. The first 8 hex of a real ObjectID are its creation time in seconds, so a marker that decodes to an impossible date guarantees no overlap. Proposed marker `ffffffff` (decodes to the year 2106; visually unmistakable in any log or export). Constant `models.MHSDeviceTestUserIDPrefix` with helper `models.IsMHSDeviceTestUserID(hex)`; document it in the device-test guide and the stratalog and stratasave READMEs so downstream consumers (Abt exports, the grader, support tooling) can filter on it. Any 8-hex marker with an impossible date works; the value is a one-line choice.
- **One id per run, used everywhere:** the test record's `_id`, the id in the run URL, and the game's `user_id` are the same value. Any `logdata`, `player_states`, `player_settings` or grades document carrying an id with the marker is test data, and the `mhs_device_tests` collection says which run it was.
- No `users` row. The MHS dashboard will not list these ids (it joins by member). That is intended; the device-test viewer shows the same gameplay data by querying `logdata` and the grades directly by id (4.6).
- A fresh id means an empty save, so Unit 2 starts clean every time, the same as a staff "jump to unit". Because the test has no progress record, "Reset all MHS data" or a re-run cannot land anyone in Unit 1: the run page is the only entry point and its manifest has one unit.

### 4.5 Data model (as built)

`mhs_device_tests`, one doc per device-test run (kind `devicetest`) or per member load record (kind `member`, plan step A2):

- identity: `_id` (the 24-hex test id, also the game's `user_id`), `workspace_id`, `kind`; member records add `user_id` and `organization_id`; `unit_id`, `unit_version`, `build_identifier`, `collection_name`, `device_id`
- form: `school, tester_name, tester_role, tester_email?, device_type, managed_device?, network_type, notes`
- server context: `remote_ip, user_agent, started_at, last_seen_at, ended_at?, expires_at` (24 h for a run; 6 h for a member launch record)
- device summary columns `device_type, platform, browser, network` plus the full `diagnostics` snapshot (bounded: 120 keys, 500-character values; also holds `steplog_context` and, for member records, `outcome`)
- outcome: `stage` (run | downloading | downloaded | launching | gameplay | completed | failed), `reached_stage` (the furthest stage, kept while `stage` is failed), `failed_step`, `failed_reason`, `download {path, bytes, seconds, avg_bps, switched, stalls, retries}`, `launch {loader_ms, unity_ms, first_frame_ms}`, `gameplay_reached_at`, `unit_completed_at`, `crash_count`, `end_reason` (completed | closed)
- `steps`: the step-log entries, newest 300 kept (`$push` / `$each` / `$slice`, DocumentDB-safe)
- `problem_reports`: notes sent through Send report (newest 50)
- `questionnaire {sound, controls, display, performance, progress, notes, answered_at}`: the post-play answers, replaced on update
- `heartbeats` (newest 240), `last_heartbeat`, `last_heartbeat_at`, `heartbeat_count`: the play page's 30-second samples

Indexes: `{workspace_id, started_at: -1, _id: -1}` for the viewer cursor; `{workspace_id, form.school}` for the school filter; `{workspace_id, user_id, started_at: -1}` for member records. No TTL: everything is kept.

Site settings: `mhs_device_test_enabled` and `mhs_device_test_unit` (4.3).

### 4.6 Data collected (as built)

| Area | Fields | Source |
|---|---|---|
| Who | school or district, name, role, email (optional), device type, school-managed or not, network type, notes | the form before the test |
| Survey answers | did the sound play (required), did the keyboard and pointer work, did the picture look right, how did it run, how far they got, notes, when answered | the questionnaire on the run page after launch |
| Where and when | workspace, client IP, user agent, start / last activity / end / expiry times; time zone, languages | server and page |
| Device | user agent and client hints (platform and version, model, architecture, bitness, brands, full versions), screen size and pixel ratio, viewport, orientation, touch points, CPU cores, device memory, battery level and charging when available | run page snapshot |
| Browser | installed-app mode, service worker supported and controlling, Background Fetch API, Cache API, cookies enabled, cross-origin isolation, SharedArrayBuffer, WebAssembly, BroadcastChannel, WebGL 2 / 1 / none with GPU vendor and renderer, maximum texture size, page load and time-to-first-byte | run page snapshot |
| Storage | usage and quota, persisted or not, inventory of the Mission HydroSci caches with file counts, the space check against the unit's missing bytes | run page snapshot and step log |
| Network | connection type, effective type, downlink, round-trip time, data-saver flag, online state; content-server probe latency or failure; log and save service reachability | run page snapshot and step log |
| Download | path chosen and why; progress samples (every 5 s or 10 %) with rate and time left; waiting countdowns; the switch to the direct path; stalls; every automatic retry with reason, delay and attempt; errors with class, raw text and failure reason; completion with size, duration and speed; file verification; and the summary (path, bytes, seconds, average speed, switched, stall and retry counts) | step log and summary |
| Launch | loader fetched and from where, Unity milestones, Unity start time, identity hand-off, first frame; loader / Unity-start / first-frame timings; the moment gameplay was reached | play page |
| While playing | every 30 s: Unity WebAssembly heap, JavaScript heap used and total, frame rate over the interval, tab visibility, time since launch; a closing beat on leaving; from these, "page stopped responding" when beats stop without a closing beat or completion | play page heartbeats |
| Game events on the page | crash reports (type, phase, message) and the crash count; unit completion; tester reports | play and run pages |
| Game telemetry (not stored on the run; shown in the detail by id) | stratalog event count, first and last event, scenes seen; grader progress points and current unit; save and settings data live under the id in stratasave | viewer queries the game services by test id |
| Run | stage and furthest stage, last problem, how it ended (completed, closed, stopped responding), duration | server |

### 4.7 Routes, pages and the viewer (as built)

All device-test routes are public, registered at the root router beside `/missionhydrosci/content/*`, outside the session-gated mount, under `/missionhydrosci/devicetest/`. Pages render with the normal CSRF token; POSTs send `X-CSRF-Token`; every body is bounded (64 KB); the start POST is throttled per IP (`mhs_device_test_start_limit` per `mhs_device_test_start_window`, defaults 10 per 10 minutes); the path is exempt from maintenance mode, as is the content route.

- `GET  /missionhydrosci/devicetest` — landing page and form (or the "not enabled" page).
- `POST /missionhydrosci/devicetest/start` — validate the form, create the run with a marked id, redirect to the run page.
- `GET  /missionhydrosci/devicetest/run/{testId}` — the run page (`devicetest_run.gohtml`): one Unit 2 card, Download / Retry now / Launch, the status panel open, the download-mode notice and storage bar, Send report, and, once launched, the post-play questionnaire. The delivery manager runs in isolated mode (no pruning or aborting of other units) with the run's manifest URL; the step log streams every 5 s.
- `GET  …/run/{testId}/manifest` — the one-unit manifest pinned to the run's build, with the tuning and probes blocks.
- `GET  …/run/{testId}/play` — the play template in `DeviceTest` mode: identity = the test id and "Device Test", the same game-service URLs and keys, no next unit, the back arrow and the Test complete link return to the run page's questionnaire, completion posted to the run.
- `POST …/run/{testId}/diagnostics` — the device / browser / storage / network snapshot and summary columns.
- `POST …/run/{testId}/steps` — batched step entries; the stage, furthest stage, last problem and gameplay time are derived from them.
- `POST …/run/{testId}/summary` — download and launch summaries, gameplay reached, crash count.
- `POST …/run/{testId}/report` — a tester note.
- `POST …/run/{testId}/complete` — the game's unit-complete callback; stamps completion.
- `POST …/run/{testId}/heartbeat` — the play page's 30-second sample or closing beat.
- `POST …/run/{testId}/questionnaire` — the post-play answers (a normal form post; the sound answer is required).

Members (session-gated, in the existing MHS route group): `POST /missionhydrosci/api/steplog` stores a launcher or play page's step log on a download or launch outcome, a crash, or Send report, and returns the record id; `POST /missionhydrosci/api/steplog/{id}/heartbeat` adds heartbeats to a launch record.

Service worker: unchanged apart from the `getVersion` reply. The pages are same-origin and in scope, so downloads and content interception work as for members; the run and play pages themselves are not cached for offline use.

Admin:

- Site Settings: the enable checkbox, the Unit 2 build selector, and the link to give a school (4.3).
- Viewer `/views/device-tests` (roles admin, analyst, superadmin). Columns: started, school, tester, device, network, path, download, stage, sound, last problem, duration. Filters: started (date range), stage, kind, device, school, sound, test id. Chips: runs, reached gameplay, completed, failed now, last run. The detail (an expandable row): the form, detected device and network, download and launch summaries, how it ended and the last heartbeat, the tester's answers and notes, the game telemetry for the run, tester reports, the heartbeat table, the step timeline, the diagnostics snapshot, and a per-run JSON download. Exports: CSV (the table) and JSON (every matching run in full), both new optional capabilities of the viewers framework.

**Panel after Launch and on a cache hit (2026-09-08).** The game tab relays
its step entries to the run page over a BroadcastChannel
(`MHSStepLog.relayTo` / `listenTo`, channel per test id); relayed entries are
tagged "game tab", shown in the panel and in Copy report, and never flushed
to the server by the run page. With the manager's `probeCached` option
(on for the units, manage and device-test pages) a unit already on the
device is noted in the download-method and download rows, its files
verified and the servers probed softly (an unreachable content server is a
warning, since the unit can still be played), so those rows are filled on
every visit. A final flush (tab hidden, then closed) no
longer resends what an in-flight request already carries, and the steps
endpoint drops any entry the record already holds.

**Renewable CSRF token (2026-09-08).** A device-test page can outlive the
CSRF cookie behind its token (twelve-hour lifetime; also cleared by a
sign-out in the same browser), after which every post from the page is
refused with 403. `MHSStepLog.csrf` keeps the token, renews it by
re-reading the page's own HTML, and retries a refused post once; the
status-log flush renews on 403 and resends on its next tick; the
questionnaire form renews right before submitting. Both the run page and
the play page use it for every post.

**Cookie lifetime, renewal everywhere, launch watchdog (2026-09-09).** The
CSRF cookie follows `session_max_age` (30 days in production; the library
default was 12 h), so a signed-in browser never holds a stale secret. The
units and manage pages, the delivery manager's telemetry, and the layout's
heartbeat and HTMX requests renew a refused token as the device-test pages
already did. The play page gained a launch watchdog: 30 s without progress
→ warn entry with a loader probe, panel shown, `launch-stalled` record;
90 s → retry straight from the CDN if the loader script never arrived,
else `launch` fail + `launch-failed` record with reset advice.

**Regressions found and fixed 2026-09-09 evening.** (1) The play page's
token keeper was defined in one `<script>` block and used in another, so
every device-test play page died on load ("Initializing…" forever, nothing
logged) from the 2026-09-08 deploy until this fix, and member launches
stopped recording `launch-ok`. (2) The service worker's install pre-cached
`/missionhydrosci/units` with `cache.addAll`, which rejects on the 401 a
signed-out device tester gets, so on a device that had never signed in the
worker never installed and the device test could not download at all
("Service worker not ready"). SW 1.0.14 pre-caches each URL individually
and never lets a failure abort the install. Lesson recorded: verify the
device-test play page, not only the member play page, after any play
template change.

**Download path review (2026-09-09).** Thirty days of telemetry: 0
Background Fetch failures reported by Chrome; 9 of 10 Chromebooks only
ever downloaded in the background; all 150 download errors were on the
direct path, 135 of them a connection dropping mid-file. Changes, all in
the shared code so the launcher pages and the device test behave the same:
(1) the frozen-switch records its event with device state
(`download-switched` record, `bgfetch-frozen` telemetry); (2) the "prefer
direct" memory is 1 h and clears on a later background success; (3) the
direct path resumes a broken file from 8 MB parts with a Range request
(SW 1.0.15). Verified in a Node harness (clean, one break, two breaks,
server ignoring Range) byte-for-byte.

**Reset this device (2026-09-09).** Run-page button: deletes the test
unit's cache (isolated: other units untouched), clears the prefer-direct
memory, unregisters the service worker, reloads; the reset is logged as a
step and flushed before the reload. Server side, `POST …/reset` sets
`last_reset_at`, `resets++`, stage "run", end reason "reset" (reopened by
the next heartbeat); `LaunchedSinceReset()` (last_launch_at / gameplay /
heartbeat after the reset) decides whether the run page shows the
questionnaire. Save and log data are never touched (Dale). The play page's
stalled-launch advice for the device test points to the button.

### 4.8 Security and privacy

- **Game-service keys.** The play page renders the static stratalog and stratasave Bearer keys server-side, exactly as it does for students today; the URL never contains them. Anyone who reaches the play page can read them from the HTML, which is already true of every student browser. The enable switch turns the route off for workspaces that do not use it.
- **PII.** Tester name, email, IP and the questionnaire notes live only in `mhs_device_tests`, are shown only in the admin/analyst viewer, and never appear in `users` or in `logdata`. The landing page states what is collected. No members report or dashboard is affected because no members are created.
- **Bounds.** Every POST has a bounded body (the SEC-4 lesson); steps, reports and heartbeats are capped server-side; the start POST is throttled per IP. Steps, diagnostics, summaries and reports stop being accepted once the unit is completed or 24 h after the start; heartbeats and the questionnaire are accepted until the 24-hour expiry, completed or not. Test ids are unguessable (64 random bits).
- **Authenticity.** The test runs on the workspace's own host with its own settings, CDN and services, so what a school sees is what its students will get.

### 4.9 Implementation order

1. Model, store and indexes (`mhs_device_tests`); the id marker constant and helper; the two site-settings fields and their form controls.
2. Public handlers: landing/form, start (throttle), run page, manifest (one-unit build + tuning), diagnostics/steps/report/complete; root-router registration; maintenance exemption.
3. Play template `DeviceTest` mode (identity, endpoints, completion screen); the run template; the shared step-log panel from A1.
4. Viewer with the gameplay lookup, JSON export, menu entry.
5. Docs: `docs/mhs-device-test/admin-guide.md` and `tester-guide.md`/`.html` (how to enable it, the URL to give a school, what the school sees, how to read a run, the id marker); refresh the feature `README.md` and `issues/README.md` (DOC-1, DOC-2); note the marker in the stratalog and stratasave READMEs.

Verification: Go tests for the id format and helper, the settings resolution (configured build vs active collection), throttle, manifest filtering, expiry, and the completion stamp. End to end on dev: enable the test, open the URL in a fresh browser profile with no cookies, fill the form, download, launch, reach gameplay, complete, and confirm the run shows every stage and the gameplay section shows events and points; the disabled state; writes to a completed or expired test id are rejected; a coordinator cannot open the viewer; CSV and JSON export. Then real devices: a Chromebook in a real profile and a guest profile, an iPad, a Windows laptop, and one deliberately blocked network (block the CDN host in the hosts file) to see the fast, specific failure.

---

## 5. Sequencing and estimates

| Step | What | Effort | Depends on |
|---|---|---|---|
| A0 | the three timing values, preflight, countdown, persist and randomUUID one-liners | 1 day plus device checks | none |
| A1 | step log module, panel, Copy report | 2–3 days | A0 |
| B1–B2 | model, settings, public handlers, manifest | 1.5 days | none (can run in parallel with A1) |
| B3–B4 | play mode, run page, viewer | 2 days | A1 and B1–B2 |
| B5, A2 | docs, students' step-log storage and viewer filter | 1 day | B |

About 7–8 working days sequential, or about 6 with B's server side in parallel.

---

## 6. Decisions (all settled 2026-09-07)

- **Timing values:** 25 s frozen-switch, 45 s fallback stall, 10 s keepalive (§3 A0 table), exposed through the manifest `tuning` block so they can be adjusted per workspace without a JS deploy.
- **Test-id marker:** `ffffffff` (4.4).
- **Device-test switch:** the public URL only works in a workspace whose admin has ticked "Enable device test" in Site Settings (default off). Without the switch the URL would be live in every workspace that has an active collection, including ones that never run device tests; the checkbox keeps it explicit and is also the way to turn it off after a school's testing window.
- **Unit 2 starts cleanly with no Unit 1 save:** confirmed by Dale. Every unit starts cleanly; Mission HydroSci already supports switching between units on demand with proper authorization.
- **A2** (storing students' step logs on failure or completion for review): approved as a standing diagnostic tool; implemented 2026-09-07.
- One fixed route per workspace, no links to manage; no accounts or sessions; the Unit 2 build is chosen in workspace settings from builds already on the CDN; the game-service keys are rendered into the launch page as today; everything is kept.

The plan is complete and ready to implement in the order in §5.

---

## 7. Risks

- A0's lower thresholds can abort a healthy download if the worker stays silent longer than expected. Mitigated by the keepalive change and the visible-tab guard; verified under Slow 3G throttling; keep the values server-tunable.
- A1 touches the two duplicated pipelines (ARCH-1). Keep the panel a shared snippet and the logic in the module so no third copy appears.
- The play template gains a mode flag. Keep the device-test branches small (identity, endpoints, completion screen) so the member path stays byte-for-byte the same.
- The route is public. Runs are cheap for us (one record each) but the download is not cheap for the school's network; the enable switch and the per-IP throttle are the controls, and the landing page makes clear what starting a run does.
- The MHS dashboard will not show test ids. The device-test viewer is the place to look; the guide must say so.
- Unit 2 is about 212 MB with a single ~199 MB file; on iPad (fallback only) the worker may be terminated mid-file. A1's log will finally show when and where, and A0's auto-resume helps.

---

## 8. Files (expected)

New: `features/missionhydrosci/devicetest.go` (public handlers), `store/devicetests/`, `domain/models/devicetest.go` (record, id marker, helper), `resources/assets/js/mhs-steplog.js`, templates `devicetest_landing.gohtml`, `devicetest_run.gohtml`, `mhs_steplog_panel.gohtml`, `viewers/views/devicetests.go`, `docs/mhs-device-test/` (admin guide, tester guide as Markdown and HTML).

Modified: `mhs-delivery.js` (constants, hooks, probes, configurable manifest and telemetry URLs); `sw.go` (precache the new asset) and `static/sw.js` (`getVersion` message); `missionhydrosci_units.gohtml`, `missionhydrosci_play.gohtml` (DeviceTest mode), `missionhydrosci_manage.gohtml`; `api_manifest.go` (one-unit manifest helper, tuning block); `types.go`; `domain/models/sitesettings.go` and the settings feature (two fields, form controls); `bootstrap/routes.go` (public registration, viewer registration); `system/maintenance/maintenance.go` (exempt path); `system/indexes/indexes.go`; `resources/templates/menu.gohtml`; `issues/README.md`; feature `README.md`; stratalog and stratasave READMEs (id marker note).
