# Mission HydroSci End-of-Game Ceremony — Integration Plan

*Drafted 2026-09-21. Status: DECIDED (Dale, same day); two grader-side details
remain open in §7 and do not block the phases. Nothing has been implemented.*

## 0. Status and how to resume

- Written from a read of `mhs-gameplay-end` (dist `v0.1.7`, HEAD `72921e0`
  "put ceremony work on hold"), `stratahub` (main `9ebc8ea`), `mhsgrader`
  (v3 rules), and the MHSBridge hand-off in `mhs-updates`.
- **Decisions taken 2026-09-21 (Dale):** D1 component with id `end`; D2 CDN;
  D3 host page + embed API (ceremony v0.1.8); D4 stream at the end, no
  pre-download; D5 the grader computes all 23 EA checkpoints and the stars;
  D6 replay any time after completion, staff per-student preview added, no
  fixture-preview UI in stratahub (the bundle's own launcher on the CDN is the
  preview); D7 no waiting: the show starts at once and each unit's scores are
  read just before that unit's scenes; D8 viewed-at marks on progress; D9 Dale
  signs off on visuals; D10 as written; scope = Unit 5 finishers only; old
  builds use the units-page button.
- **Still open (grader side, non-blocking):** U2.C3 composition and the
  U5.C4 per-selection detection (§7 Q11), and the U3.C5 threshold mismatch
  (§7 Q13).
- **Progress 2026-09-21 (evening):** the team document with the EA-score
  decisions and questions is at `mhsgrading/docs/ea-scores-team-questions-2026-09.md`
  (the grader brief `mhsgrader/docs/updates/ea-scores.md` points at it).
  Ceremony Phase 1 (E1–E4, E7) is DONE: `mhs-gameplay-end` v0.1.8 built,
  verified in a browser (standalone + late-binding harness), committed, and
  staged locally as `dist/v0.1.8` (103 files, 71 MB incl. the 9.4 MB vendored
  runtime); **not uploaded** (no AWS credentials on this machine — Dale
  uploads with `aws s3 sync`, then snapshots the plain pair to `_v7`). The
  host-page contract stratahub builds against: `mhs-gameplay-end/docs/embed-api.md`.
- **2026-09-22:** v0.1.8 is live on the CDN (`<cdn>/mhs/end/v0.1.8/`, 103/103
  files, browser-verified; archived as `_v7`). stratahub **S1, S2, S4, S5,
  S6, S7 implemented** (see §4 status notes): ceremony build kind + sync,
  collection reference + forms, manifest block, host page, ea-scores endpoint,
  viewed marks, play/units hooks, step-log outcomes. Verified: CORS from the
  workspace origin to the CDN works directly and through the content redirect
  (so no service-worker change is needed). Dashboard link + trophy mark done
  the same day. Deployed to production 2026-09-22 (the ceremony stays off until
  a collection selects a version). Not yet: docs screenshots, e2e; grader G1/G2.
- **2026-09-22 (later):** Dale registered v0.1.8 and put it on the active
  Dev MHS collection; the units list gained the ceremony row (staff: Open
  (preview)). **Grader G1 done** (mhsgrader `293a5fb`): the nine checkpoints
  as `eaScores` on finished attempts and interim `eaStars` per unit, harness
  26/26 on all fixtures, binary deployed. **Backfill done** 2026-09-22 07:53–08:06 UTC (Dale's go): 795 grade
  documents wiped and replayed in 47 batches, zero errors; afterwards 584
  documents carry EA scores, 421 a unit star, 116 all four units; the cursor
  sits at the latest trigger event.
- **2026-09-22 (v0.1.9):** the first host-page run froze after the cold open:
  the bundle's holo images were plain image loads, tainted by the content
  path's redirect to the CDN, and Babylon's texture upload threw at Toppo's
  first line. Fixed in the bundle (player v8: CORS image loads;
  `mhs-gameplay-end/docs/v0.1.9-plan.md`), v0.1.9 uploaded by Dale, synced and
  selected on Dev MHS; Dale confirmed Open (preview) plays through. Hosting
  rule recorded in the embed contract: every asset is a CORS load, so the CDN
  must allow the host page's origin.
- **2026-09-22 (v0.2.0, D9 done):** the bundle is 37 MB (was 71) with no
  visible change — unused clips and orphan samplers dropped, clips resampled,
  WebP textures ≤ 1024 px, voice 64 kbps, music 128 kbps, holo ≤ 1280 px, WebP
  logo (`mhs-gameplay-end/docs/v0.2.0-plan.md`, pipeline `tools/optimize.sh`).
  Staged; Dale uploads, syncs and selects v0.2.0. A member in Unit 5 is testing
  the end-to-end path on Dev MHS; the team's EA questions have been sent.
- **Next step:** the member's result; grader G2 (the other fourteen
  checkpoints, true star totals) as the team's answers arrive.
- Related documents: `mhs-gameplay-end/docs/implementation-plan.md` (Phase 5
  is the original stratahub integration spec), `mhs-gameplay-end/docs/partial-nodata-plan.md`
  (on hold), `mhsgrader/docs/updates/ea-scores.md` (the grader brief, decision
  of 2026-08-03), `docs/mission-hydrosci/mhs-remaining-work-plan.md` (RDY-5,
  DEV-1), `docs/mission-hydrosci/mhs-builds-admin-guide.md`.
- Real hostnames and bucket names never appear in this document; the CDN is
  `<cdn>` and the bucket `<bucket>` (stratahub is a public repository).

## 1. What exists today

### 1.1 The ceremony bundle (`mhs-gameplay-end/dist/v0.1.7`)

- 139 files, 66.3 MB. Entry page `ceremony_v6.html`; from v0.1.8 the plain
  names `ceremony.html` + `lib/player.js` are the show and the `_vN` files
  become the archive (`docs/visual-v2-plan.md` §"plain-name convention").
- Rendering is Babylon.js loaded from the **unpinned** public Babylon CDN, plus
  four textures fetched from Babylon's playground host
  (`lib/player_v6.js:162, 406, 609, 816`). Every other path is relative; the
  bundle is relocatable under any prefix. No storage, cookies, or analytics.
- Contents: five character GLBs 44.1 MB (animation is 59–84 % of each file: 25
  clips per rig, about 12 used; the body normal maps are 2.2–2.8 MB
  uncompressed PNG each), music 6.6 MB (192 kbps stereo), voice 4.5 MB
  (128 kbps mono), holo images 5.5 MB, logo 0.6 MB PNG, plus 4.55 MB of stale
  files (`assets/audio/`, archive players and pages) not used by the v6 show.
- **Data contract** (`lib/resolver.js`): `{ items: { "U2.C2": {score, max}, … },
  stars: { unit2..unit5: 0–3 } }`. Nine checkpoint keys drive eight A/B
  sections: `U2.C2+U2.C3 ≥ 2`, `U2.C5 ≥ 4`, `U2.C7 ≥ 2`, `U3.C1 ≥ 2`,
  `U3.C5 ≥ 3`, `U4.C6 ≥ 2`, `U5.C3 > 0`, `U5.C4 > 0`. A missing key means
  *unknown* and plays the gentle B variant; a fabricated 0 would assert poor
  performance. There is no status enum. `game`, `user_id`, `generatedAt`,
  `currentUnit` are carried but not read.
- **No production data path exists.** The boot block only understands
  `?profile=<fixture>`; without it every section plays B with dark stars. The
  comment names `/missionhydrosci/api/ea-scores` as the intended source.
- **No exit signal.** The show ends on a black screen with the logo, a
  congratulation line, and Restart/Replay. The Begin button is mandatory
  (browser audio unlock), so the ceremony cannot autoplay.
- Already staged on the CDN at `<cdn>/mhs/end/v0.1.7/` (bucket layout
  `mhs/end/vX.Y.Z/` mirrors `mhs/unitN/vX.Y.Z/`); version folders are
  immutable; `dist/` is git-ignored and hand-staged with `aws s3 sync`.
- The repo is on hold pending designer answers on partial and no-data players.

### 1.2 stratahub

- **Builds and collections.** `mhs_builds` holds one record per
  `(unit_id, version)` with relative S3 keys and the three Unity key files
  (`models/mhs_build.go`); `mhs_collections` holds unit → version references
  (`models/mhs_collection.go`). Resolution: per-user override → group pin →
  workspace active collection (`missionhydrosci/api_manifest.go:30-90`). The
  S3 sync (`mhsbuilds/sync.go`) only recognises `unitN/vX.Y.Z/`; the zip
  analyser only `unitN/` folders (`upload.go:30`). Unit titles are a hardcoded
  map that already reserves `unit6` (`upload.go:367-382`).
- **Delivery.** `GET /missionhydrosci/api/manifest` lists units with files; the
  service worker downloads from `cdnBaseUrl + '/' + path` and caches under
  same-origin `/missionhydrosci/content/<id>/v<ver>/…` in cache
  `missionhydrosci-unit-<id>-v<ver>`; a miss is a 302 to the CDN
  (`missionhydrosci/content.go`). The service worker, delivery manager and
  manifest accept any id string; nothing there parses `unitN`.
- **Sequencing.** The play page is a full-page navigation with identity injected
  server-side (`missionhydrosci_play.gohtml:366-379`). The game calls
  `window.mhsUnitComplete(unitId)` → `POST /api/progress/complete` →
  `mhs_user_progress.current_unit` becomes `"complete"` after the last manifest
  unit (`mhsuserprogress/store.go:203-208`). For the final unit
  `completeAndTransition` hides the overlay and leaves the game's own end screen
  running (`missionhydrosci_play.gohtml:502-518`); the game then calls
  `window.mhsEndGame()` which navigates to `/missionhydrosci/units`
  (`:692-695`), where the "Mission Complete" card is the only end state
  (`missionhydrosci_units.gohtml:75-80`). Old builds never call EndGame; the
  player uses Back.
- **Grades.** Only the dashboard reads `mhsgrader.progress_point_grades`
  (leader/admin/coordinator); `deps.MHSGraderDatabase` is already wired in
  bootstrap. There is no member-facing grades API.
- **Reporting.** `POST /api/device-status` (per device `unit_status` map) and
  `POST /api/steplog` (+ heartbeat) with a fixed outcome vocabulary
  (`steplog_member.go:25-34`).

### 1.3 mhsgrader

- `progress_point_grades`: `grades[pointID] → []{attempt, status
  active|passed|flagged, metrics, reasons, times}`, `currentUnit` set by unit
  *start* events only. No EA scores, no stars, no unit- or game-complete flag.
- Polls `stratalog.logdata` every 5 s on a single global cursor; one poisoned
  event stalls everyone. Device-test ids (`ffffffff…`) are graded like students.
- `docs/updates/ea-scores.md` (2026-08-03, Dale's decision): the grader
  computes EA checkpoint scores per attempt (`Grade.EAScores`) and stratahub
  serves them; the crosswalk covers the nine ceremony checkpoints (eight are
  bandings or pass-throughs of metrics the rules already store; **U5.C4 needs
  new per-selection detection**; U2.C3's composition is still an open designer
  question); stars are unit totals over all 23 checkpoints banded per the EA
  document; a backfill command is required for existing students.

## 2. Decisions

Each item states the options, the recommendation, and what it changes.

### D1 — Is the ceremony a unit? **No: a distinct "ceremony" component.**

Treating it as `unit6` looks free (upload, sync, manifest, service worker all
flow) but breaks sequencing: `CompleteUnit` counts manifest units, so a student
would only be "complete" after completing the ceremony; `unitNumber` requires
`unit<N>`; the units page would list it with Download/Launch and the play page
would try to run Unity on it; `unit6` is reserved for a possible sixth game
unit (RDY-5); and the dashboard's unit list comes from
`mhs_progress_points.json`, not the manifest.

Recommended shape:

- `MHSBuild` gains `Kind` (`"unit"` when empty, `"ceremony"`) and `EntryFile`
  (the page or embed script). The ceremony's id is **`end`**, matching the
  existing CDN path `mhs/end/vX.Y.Z/`. The S3 sync accepts `end/vX.Y.Z/` and
  detects the entry file instead of Unity key files. Zip upload for the
  ceremony is unnecessary (it is staged with `aws s3 sync`; the sync registers
  it). The storage page shows a Ceremony section.
- `MHSCollection` gains `Ceremony *MHSCollectionComponent{Version,
  BuildIdentifier}`; nil means no ceremony (today's behaviour). The manual
  collection form gets a Ceremony version select; upload-created collections
  inherit the previous collection's ceremony reference; the manage modal and
  the collection detail page show it.
- The manifest gains a top-level `ceremony` block (`{id: "end", version,
  entry, files, totalSize}`) **beside** `units`, so `totalUnits`, the units
  page and the play page are untouched.

### D2 — Hosting. **CDN/S3 next to the units, as already staged.**

Under `<prefix>/end/vX.Y.Z/`, immutable per version, registered by the S3
sync. Rejected: embedding in the stratahub binary (60 MB of assets, content
releases coupled to deploys) and a separate bucket or content host (a second
CORS/redirect path to maintain).

### D3 — How the page is served. **A stratahub host page with an embed API.**

`GET /missionhydrosci/ceremony` inside the session-gated MHS router renders a
bare-layout page that loads the ceremony from same-origin
`/missionhydrosci/content/end/v<ver>/…` (service-worker cache when present,
CDN redirect otherwise) and mounts it:

```html
<div id="ceremony"></div>
<script src="/missionhydrosci/content/end/v0.1.8/lib/embed.js"></script>
<script>
  MHSCeremony.mount(document.getElementById('ceremony'), {
    base: '/missionhydrosci/content/end/v0.1.8/',
    scoresUrl: '/missionhydrosci/api/ea-scores',   // polled until ready
    returnUrl: '/missionhydrosci/units',
    onEvent: function (e) { /* steplog / device-status */ }
  });
</script>
```

The ceremony bundle (v0.1.8) supplies `lib/embed.js`: it builds the DOM the
show needs (today's `ceremony_v6.html` markup and CSS, about 5 KB), prefixes
every asset path with `base`, fetches or accepts scores, and reports
`started`, `ready`, `finished`, `failed` events. Its own `ceremony.html` keeps
working as the dev harness by calling the same API with `?profile=`.

Rejected: navigating to the CDN's HTML (cross-origin scores fetch, no session,
no exit control, and the page origin differs between the cached and uncached
cases); copying the ceremony markup into a Go template (every ceremony release
would need a stratahub deploy); server-side fetch-and-wrap with `<base href>`
(works but is opaque and still needs a boot-block change).

Babylon.js (pinned version) and the two playground textures are vendored into
the bundle (`lib/vendor/`, `assets/env/`). This makes the page fully
same-origin, offline-capable once cached, and immune to upstream Babylon
changes. v0.1.7 on the CDN is exposed to that risk today.

### D4 — Delivery. **Stream at the end; no pre-download.** *(decided 2026-09-21)*

The host page loads the ceremony through the same-origin content path
`/missionhydrosci/content/end/v<ver>/…`. With nothing cached, each request is
the usual 302 to the CDN, exactly as an uncached unit loads today, so the
service worker, the units-page pipeline, `autoCleanup` and device status need
no change. Students reach the ceremony at different times, unlike a class
starting a unit together, so the pre-download's additions (pipeline, keep
lists, a device-status entry) are not worth their complexity. The ceremony's
own load gate is the waiting experience: Begin enables once the characters
have loaded, with a progress line. This makes the size work in D9 the main
lever for a good first impression.

Fallback if streaming proves too slow on school networks: download the
ceremony when Unit 5 becomes the current unit, through the existing pipeline,
in cache `missionhydrosci-unit-end-v<ver>`.

### D5 — Data. **The grader computes all 23 EA checkpoints and the stars.** *(decided 2026-09-21)*

This follows the 2026-08-03 decision and the brief in
`mhsgrader/docs/updates/ea-scores.md`. The dialogue variants need nine
checkpoints, but the star board is defined as each unit's **total over all of
that unit's checkpoints** banded per the EA document (23 checkpoints across
Units 2–5), so the grader implements the full set. The nine can land first
(G1) so the dialogue can be verified end to end; the remaining fourteen and the
stars (G2) follow; the release waits for both.

stratahub adds `GET /missionhydrosci/api/ea-scores` (session, self-only for
members, device-test ids refused) that takes the latest *finished* attempt per
point (the dashboard's rule), merges `eaScores` across points, adds `stars`,
`status` and `completedUnits` (from `mhs_user_progress`), and returns the
contract in §3. Staff may pass `user_id` for a student within their reach
(`viewscope`, the same rule as the dashboard) to drive the per-student preview
in D6.

### D6 — Trigger and gating. **`mhsEndGame` → ceremony; replay from the units page.** *(decided 2026-09-21)*

- `window.mhsEndGame` navigates to `/missionhydrosci/ceremony` when the
  resolved collection has a ceremony, otherwise to the units page as today.
  The `isFinal` branch stays as it is (it exists to keep the game's end screen
  reachable).
- Old builds without EndGame: the units page "Mission Complete" card gains a
  **Watch your ceremony** button. The same button is the replay entry point,
  available any time after completion.
- Gate: members may open the ceremony only when
  `mhs_user_progress.current_unit == "complete"`. This keeps the on-hold
  "partial player" question out of this plan (see D7).
- Staff per-student preview (leader, coordinator, admin): the dashboard's
  student row gets a "View ceremony" link to
  `/missionhydrosci/ceremony?user_id=<hex>`; the host page and the scores
  endpoint accept the parameter for staff within their reach. Small addition
  on top of the member path, so it is included.
- Fixture preview: no stratahub route or UI. The bundle's own launcher
  (`index.html` → `ceremony.html?profile=<name>`) stays in every staged
  version and is reachable directly on the CDN by anyone who knows the URL;
  that is the reviewer's tool (decided 2026-09-21).

### D7 — Pending and partial data. **No waiting: scores are read just before they are needed.** *(decided 2026-09-21)*

Scope is students who finished Unit 5. "Partial" (class ended after Unit 3)
stays on hold with the designers; the gate in D6 keeps such students out.

Pending is real: the u5p4 grade lags EndGame by log upload plus the 5 s poll,
and logs can be lost (the September logging incident). A holding screen was
rejected: 45 s is far too long for middle-school students. Instead:

- The host page mounts the ceremony immediately with whatever the first
  scores call returns and keeps polling `/api/ea-scores` in the background
  (every 5 s until `status: ready` or the show ends).
- The player resolves each unit's sections **when that unit's character walks
  up**, using the latest data, rather than resolving the whole script at Begin.
  Unit 2's lines play about half a minute after Begin (after Toppo's intro);
  Unit 5's lines and the star board come several minutes in, by which time
  the last grade has almost always landed. The resolver keeps its whole-script
  `resolve` for fixtures and tests and gains a per-unit variant.
- The celebration reads the latest `stars` when it starts. A unit's star entry
  is present only when that unit is finished by the dashboard's rule (every
  point passed or flagged); a still-pending unit shows a dark row rather than
  an understated one. Missing checkpoint scores play the B variant as today.
- A student with no grades at all still gets the full show on B variants and
  dark stars; nothing ever blocks the celebration.

### D8 — Exit and reporting. *(decided 2026-09-21)*

- The embed's Exit control (top-right, always visible, label from the host
  page, e.g. "Back to Mission HydroSci") covers the end screen, a student who
  must leave early, and a device that cannot play; it navigates to
  `returnUrl` (the units page). The end-screen text names it.
- Reporting through existing endpoints: `POST /api/steplog` with new outcomes
  `ceremony-ok` / `ceremony-failed` (load failure, WebGL unavailable, scores
  never ready) carrying the resolved variant set and stars in the note.
- Viewed marks on `mhs_user_progress`: `ceremony_started_at` (Begin pressed),
  `ceremony_finished_at` (the show reached its end), `ceremony_version`, and
  `ceremony_view_count`, written by the host page through a small
  `POST /missionhydrosci/api/ceremony/viewed` call. The dashboard shows a
  trophy mark beside the student's name once started, with the date, version
  and finished/unfinished state in its tooltip; the Members Report is not
  changed.

### D9 — Size reduction. **Target ≤ 25 MB, in measured steps, shipped as v0.1.8+.**

| Step | Tool | Expected saving |
|---|---|---|
| Drop `assets/audio/`, archive players and pages from the staged bundle | rsync excludes | 4.5 MB |
| Strip unused animation clips (about 13 of 25 per rig) | `gltf-transform` (Node CLI) | 13–14 MB |
| Share one animation library across the five rigs (same clip byte sizes in every file; bone names must match) | `gltf-transform` + a Babylon retarget step | up to 16 MB more, riskier |
| Body normal maps PNG → WebP lossless or KTX2 | `gltf-transform webp` / `ktx` | 8–10 MB |
| Voice 128 → 64 kbps mono; music 192 → 128 kbps | ffmpeg | 4.4 MB |
| Logo → WebP at 1024 px; holo JPEGs re-encoded at 1280 px | ImageMagick / cwebp | 1–2 MB |
| Geometry `prune`, `dedup`, `quantize` | `gltf-transform` | small |

Guardrails: morph target and bone names are the expression layer's API and
must not change; each step gets a visual check on a Chromebook and the
harness's fixture run; the pipeline lives in `tools/optimize.sh` in the
ceremony repo so it is repeatable for every version. Dale signs off on the
visual result (decided 2026-09-21).

### D10 — Versioning.

Ceremony versions are pinned per collection like units. Upgrading is: stage a
new immutable folder, S3 sync, edit or create the collection, activate. The
embed API carries `contractVersion`; the host page refuses a mismatch with a
clear message rather than a broken show. The `?v=` cache busters become
redundant under immutable folders (harmless to keep).

## 3. Data contract (proposed final)

```json
{
  "game": "mhs",
  "user_id": "665f1a2b3c4d5e6f7a8b9c0d",
  "generatedAt": "2026-09-21T18:00:00Z",
  "status": "ready",
  "currentUnit": "complete",
  "completedUnits": ["unit1", "unit2", "unit3", "unit4", "unit5"],
  "items": {
    "U2.C2": { "score": 1.0, "max": 1.0 },
    "U2.C3": { "score": 0.5, "max": 1.5 },
    "U2.C5": { "score": 4.3, "max": 6.0 },
    "U2.C7": { "score": 2.0, "max": 3.0 },
    "U3.C1": { "score": 3.0, "max": 3.0 },
    "U3.C5": { "score": 2.0, "max": 4.0 },
    "U4.C6": { "score": 3.0, "max": 3.0 },
    "U5.C3": { "score": 3.0, "max": 3.0 },
    "U5.C4": { "score": 1.5, "max": 1.5 }
  },
  "stars": { "unit2": 2, "unit3": 3, "unit4": 1, "unit5": 2 }
}
```

Rules: an absent item means unknown (never a fabricated 0); items come from the
latest finished attempt per point; `status` is `pending` until u5p4 has a
finished grade (or the grade document is missing); `stars` are the EA unit
totals banded per the EA document, absent for a unit with no checkpoint
scores; `completedUnits` and `currentUnit` come from `mhs_user_progress`;
a device-test id gets 403.

## 4. Work breakdown

### mhs-gameplay-end (release v0.1.8)

- E1 Embed API (`lib/embed.js`): mount, `base` prefixing, scores by value or
  polled URL with `status` handling, events, `contractVersion`.
- E2 Vendor Babylon (pinned) and the environment/flare textures.
- E3 Late binding: background polling of the scores URL; per-unit resolution
  at walk-up; celebration reads the latest stars (no hold screen).
- E4 Exit control and finished event; skip affordance.
- E5 Size pipeline (`tools/optimize.sh`) with before/after table; Chromebook
  visual check.
- E6 Stage under the plain-name convention, upload, 200-sweep, tag.
- E7 Tests: resolver suite stays green (whole-script `resolve` unchanged,
  per-unit resolution covered by new cases including "scores arrive between
  units"); a Playwright smoke of the embed with fixtures; `index.html` and
  `ceremony.html` keep working standalone on the CDN as the reviewer's tool.

### mhsgrader

- G1 `EAScore` + `Grade.EAScores`; eight checkpoint computations per the
  brief (U5.C4 follows when its per-selection detection exists). EA scores use
  the same attempt windows as the colour rules, so they move together when
  the grading team's start-and-end anchoring lands (their answer A1).
- G2 The remaining fourteen checkpoints and the per-unit stars (banded unit
  totals), stored on the grade document so the reader does not re-derive them.
- G3 Backfill command; run on production after deploy.
- G4 `docs/statistics_and_data_collection.md`; fixture replay stays 26/26.

### stratahub

- S1 Models/stores: `MHSBuild.Kind`, `EntryFile`; `MHSCollection.Ceremony`;
  sync accepts `end/`; storage page, manual collection form, manage modal,
  collection detail; indexes unchanged (unique `(unit_id, version)` still
  holds with id `end`).
- S2 Manifest `ceremony` block; delivery manager looks up `end` there.
- S3 Delivery: none (streamed at the end through the content redirect).
  Verify once that the service worker passes an uncached
  `/missionhydrosci/content/end/…` request through to the redirect as it does
  for units.
- S4 Host page `/missionhydrosci/ceremony`: gate, version from the resolved
  collection, bare layout, CSRF via `MHSStepLog.csrf`; staff `?user_id=`.
- S5 `GET /missionhydrosci/api/ea-scores`: handler with `GradesDB`, self-only
  for members, staff `user_id` within `viewscope`, device-test guard,
  `status`/`completedUnits`; `POST /api/ceremony/viewed` for the marks.
- S6 Play page `mhsEndGame` target; units page Mission Complete card button;
  dashboard "View ceremony" link and trophy mark.
- S7 Steplog outcomes `ceremony-ok` / `ceremony-failed`; Device Tests viewer
  labels.
- S8 Docs: admin guide (ceremony build and collection), teacher guide note,
  `ai/context.md`, remaining-work plan entries.
- S9 Tests: Go (manifest with ceremony, gate, ea-scores merge, sync parsing);
  e2e member journey (complete → ceremony page mounts with fixture scores);
  manual Chromebook pass.

## 5. Phases and critical path

0. Decisions (§7).
1. In parallel: E1–E4, S1–S4, G1. End of phase: the ceremony mounts on dev
   with fixture scores through the host page.
2. S5 + E3 wired end to end on dev with real grades; the late-binding path
   exercised by finishing Unit 5 on a dev account and watching Aryn's lines
   and the star board pick up the last grade.
3. G2, G3 (remaining checkpoints, stars, backfill) and E5, E6 (size); S6–S9.
4. Chromebook validation, docs, production: grader deploy + backfill, S3 sync,
   collection with the ceremony, activate per workspace.

The grader work (G1) is the critical path for correct dialogue variants;
everything else can proceed against fixtures.

## 6. Risks

- Grades late or never (logging incident class): late binding covers the
  usual lag; a lost log means B variants and a dark star row, with no message
  that blames the student.
- Chromebooks: 60 MB today, streamed at the end; D9 is the mitigation. Babylon needs WebGL;
  the host page must detect a missing context and report `ceremony-failed`
  with a plain message. Unity is torn down by the navigation, so memory
  pressure from the game does not carry over.
- Old game builds without EndGame: covered by the units page button.
- Device-test runs must never reach the ceremony (the devicetest page
  short-circuits completion; ea-scores refuses `ffffffff…` ids).
- Public repository: no real CDN host in committed docs or code.
  `mhs-gameplay-end/tools/voices.json` carries live ElevenLabs voice ids;
  check that repo's visibility before mirroring anything from it.
- Unpinned Babylon in v0.1.7: the staged bundle can break retroactively;
  whatever version ships must be vendored.

## 7. Questions and answers

Answered 2026-09-21 (Dale): 1 yes; 2 yes; 3 stream at the end; 4 the star
board is required and the grader will provide whatever scores it needs;
6 replay any time, per-student preview added since it is small; 8 viewed
marks wanted; 9 Dale signs off on visuals; 10 Unit 5 finishers only for now,
partial finishers later; 12 acceptable.

Also answered 2026-09-21: 5 no holding screen, start at once and read each
unit's scores just before its scenes (D7); 7 no stratahub preview UI, the
bundle's launcher on the CDN is enough (D6); 11 Dale pointed at
`mhsgrading/docs/grading-team-questions-2026-09-answers.md`, which answers the
September grading-sync questions (windows, anchors, U3P5 at 2.5, U4P4, U5P2
types) rather than the two EA-score questions, so those stay open below.

Still open (grader side; none blocks Phase 1):

11. **EA-score details.** (a) U2.C3: Tera only (max 1.5) or Tera and Aryn
    (max 3)? Only the max and banding change; the ceremony sums C2 + C3
    against 2 either way. Ask the grading team with the next batch of
    questions. (b) U5.C4: the three half-point selections (Tilted Out, the X
    for no extra converting, cold glass roof) need their log events
    identified from the fixture logs or the dialogue export; a grader task,
    not a designer question. Until it lands, U5.C4 is absent and Aryn plays
    her B line.
13. **U3.C5 threshold.** The grading team set the superfruit garden's green
    bar at 2.5 (one wrong planting allowed, answer A5); Tera's happy variant
    in the ceremony needs `U3.C5 ≥ 3`. A student with one wrong planting is
    green on the dashboard but hears the gentle line. Ask the designers
    whether the ceremony threshold should follow the rubric's band (≥ 2.5).
