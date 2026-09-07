# Mission HydroSci — Remaining Work Plan

**Last updated:** 2026-09-07
**Purpose:** A living backlog of open Mission HydroSci items, to be worked through
incrementally as time and token budget allow. This is the canonical to-do list;
update the **Status** column as items are done.

> **2026-09-07:** the loading-status and Unit 2 device-test plan
> (`mhs-loading-status-and-unit2-device-test-plan.md`) is approved and under
> way. It absorbs RDY-2 and UX-5 (its step A0), supersedes DOC-3 / MHS-008
> (steps A1 and A2), and gives OPS-1 its data source (A2). The rows below
> point to it. The working tree is clean; everything marked DONE is committed.

**Where these came from:** the round-4 feature review
(`mhs-feature-review-072226.md`, dated 2026-07-22) plus the running fix log
(`mhs-code-design-review-071626.md`). The severe items from those reviews (data
loss, offline stranding, ACER downloads, play-page crashes) are already **fixed**;
what remains is collected here.

**Confidence note:** items marked *(verified 2026-07-28)* were re-checked against
current `HEAD`. Items marked *(from review 2026-07-22)* are as-reviewed —
**verify the location and that the issue still exists before editing**, because
the code has shifted since (line numbers especially).

**Standing context for whoever picks this up** (details in
[[mhs-review-fixes]] / `mhs-delete-state-settings` memory):
- Deploy: `stratahub_update/aws_update.sh` — builds + restarts the **shared**
  `adroit.games` service (dev deploy briefly bounces prod workspaces too).
- Dev smoke-test login: `sysadmin@adroit.games` (trust). localhost can't download
  (no CDN CORS) — must deploy to dev to exercise real downloads.
- JS/SW/template verify recipe: `node --check` each JS file + the concatenated SW
  (sw-cache + sw-background-fetch + sw.js with a stub `MHS_ASSET_VERSIONS`); parse
  the four `.gohtml` templates with stubbed funcs. Build order: `make css-prod`
  **then** `go build`. `SW_VERSION` is 1.0.12 (bump only on real SW-code changes).
- Priority key: **P1** = do before the September classroom launch; **P2** = should
  do; **P3** = polish. Effort: **S** (<1h), **M** (1–3h), **L** (>3h / needs
  device or coordination).

---

## Summary

| ID | Item | Pri | Effort | Status |
|----|------|-----|--------|--------|
| SEC-1 | `checkMemberAuth` fails **open** on an unknown member-auth mode (no `default:`) | P1 | S | **DONE 2026-07-28** |
| SEC-2 | `/api/auth/start` is unthrottled (mail-bomb / enumeration / code-invalidation) | P1 | S | **DONE 2026-07-28** |
| SEC-3 | `ValidateAndConsumeToken` doesn't verify workspace binding | P3 | S | OPEN |
| SEC-4 | `HandleDeviceStatus` input unbounded (contrast bounded `HandleDownloadError`) | P3 | S | PARTIAL 2026-09-07 (body capped at 32 KB); field/map caps OPEN |
| SEC-5 | Unlock-window gated actions not logged with attribution | P3 | S | OPEN |
| DL-1 | SW download-creation race → BG fetch + fallback both run for one unit | P2 | M | OPEN |
| DL-2 | Play overlay `autoCleanup([nextUnit])` wipes manual pre-downloads | P2 | S | OPEN |
| DL-3 | Post-login early-download hook is a 4th, unmanaged download driver | P2 | M | OPEN |
| DL-4 | Play heartbeat stopped on `pagehide`, no `pageshow`/`persisted` re-arm | P2 | S | OPEN |
| DL-5 | `authThrottle` idle-reset uses `nextAllowed` (future) not last-attempt | P3 | S | OPEN |
| DL-6 | Two throttle-key derivations that are supposed to be one budget | P3 | S | OPEN |
| DL-7 | `groupappstore` correlated `$lookup` — DocumentDB risk, error swallowed | P2 | M | OPEN |
| DL-8 | Double collection resolution per units/manage render | P3 | S | OPEN |
| UX-1 | `aria-live` announces every download percent (~1/s) — SR spam | P2 | M | OPEN |
| UX-2 | Contrast fix (MHS-A4) nullified at runtime by JS class strings | P3 | S | OPEN |
| UX-3 | 429 backoff is a dead-end alert; no `Retry-After` / client handling | P3 | M | OPEN |
| UX-4 | `handleUnitComplete` has no `resp.ok` check (latent progress-drop) | P3 | S | OPEN |
| UX-5 | `crypto.randomUUID()` no fallback — kills device telemetry on old Chrome | P3 | S | **DONE 2026-09-07** (A0, awaiting deploy) |
| UX-6 | `quota === 0` → `NaN%` (guard only checks `!estimate`) | P3 | S | OPEN |
| UX-7 | Initial device report counts events, not units — can send incomplete | P3 | S | OPEN |
| UX-8 | Collection picker interpolates `c.id` unescaped into onclick (hardening) | P3 | S | OPEN |
| RDY-1 | **iPad smoke pass has never been done** (fallback-only platform) | P1 | L | OPEN |
| RDY-2 | MHS-005: request `navigator.storage.persist()` + report `persisted()` | P1 | S | **DONE 2026-09-07** (A0, awaiting deploy) |
| RDY-3 | MHS-006: per-unit space preflight before downloading | P1 | M | **DONE 2026-09-07** (A0: missing bytes vs free quota, re-checked every minute; awaiting deploy) |
| RDY-4 | Dev-sentinel user seeded into default/prod workspace unconditionally | P1 | S | OPEN |
| RDY-5 | `progress.go` `totalUnits = 5` fallback breaks a future 6-unit collection | P2 | S | OPEN |
| RDY-6 | MHS-004: guest-profile verification run, then close/re-scope | P3 | M | OPEN |
| ARCH-1 | Units/manage pipelines ~150 duplicated lines — refactor or freeze | P2 | L | OPEN |
| ARCH-2 | Factory for `MHSDeliveryManager` construction (3–4 sites, wrong defaults) | P3 | M | OPEN |
| ARCH-3 | Dead code: `PlayData.CDNBaseURL`, `nextUnitVersion`, others | P3 | S | OPEN |
| DOC-1 | Feature `README.md` stale (SW 1.0.7, "no Go tests", missing files/routes) | P2 | M | OPEN |
| DOC-2 | `issues/README.md` stale (1.0.7, open-issue list, smoke-test checklist) | P2 | M | OPEN |
| DOC-3 | Formally close / re-scope `issues/MHS-008` (only telemetry half shipped) | P3 | S | superseded → loading-status plan A1/A2 |
| OPS-1 | No monitoring/alert on `"mhs download error"` log; no teacher runbook | P2 | M | data source → loading-status plan A2; runbook OPEN |
| TEST-1 | Zero automated tests for the newest behaviors; no JS test harness | P2 | L | OPEN |

---

## Details

### Security / correctness

**SEC-1 — `checkMemberAuth` fails open on an unknown mode.** ✅ **DONE 2026-07-28.**
Added a `default:` case to the `switch mode` in `checkMemberAuth`
(`api_manifest.go`) that logs and returns 403 — an unrecognized member-auth mode
now fails **closed** instead of granting an unlock with no credential check. Unit
test `TestCheckMemberAuth_UnknownModeFailsClosed` in `manage_test.go` asserts a
`"bogus"` mode → 403 and no unlock. Verified passing.

**SEC-2 — `/api/auth/start` unthrottled.** ✅ **DONE 2026-07-28.**
`HandleStaffAuthStart` (`staffauth.go`) now applies `h.authThrottle` (same
per-member key as verify/checkMemberAuth): `retryAfter` → 429; every attempt
(success *and* failure) counts toward the backoff, so repeated starts can't
mail-bomb an email-method staff account or invalidate a code — a completed verify
clears the budget. The not-found / not-staff / wrong-workspace / disabled
responses are collapsed into one generic 403 to stop staff-account enumeration.
Verified on dev: 4 bogus starts → identical generic 403, then 429. (Also removed
the now-dead `loadSettings` helper + its orphaned imports while in the file.)

**SEC-3 — token workspace binding.** *(from review 2026-07-22)*
`system/staffauth/store.go` `ValidateAndConsumeToken` matches only
`{token, verified, unused, unexpired}`; `checkMemberAuth` never compares
`ch.WorkspaceID` to the request workspace. Narrow today (minting is
workspace-scoped) but a cheap invariant to enforce.

**SEC-4 — `HandleDeviceStatus` input unbounded.** *(from review)*
`device_status.go` `HandleDeviceStatus` accepts arbitrary `device_details` map +
unbounded distinct `device_id`s (one doc each). Contrast the newer bounded
`HandleDownloadError` (8 KB limit, clipped fields). Cap lengths/map size.

**SEC-5 — unlock-window attribution.** *(from review; plan deviation)*
The redesign plan promised gated actions during an unlock be logged "authorized
by <staff>". `api_manifest.go` `checkMemberAuth` logs the grant but the
unlock-pass path returns silently, discarding `GrantedBy`. Log at unlock-pass (or
return `grantedBy` so action handlers log it).

### Download / client correctness

**DL-1 — SW download-creation race.** *(verified 2026-07-28)*
`static/sw-background-fetch.js` `startBackgroundFetch` awaits `get(fetchId)` then
`fetch(fetchId)`, and the SW message handler runs `download` actions
concurrently; two near-simultaneous `download` messages (session-restore
reopening units+manage tabs; poller self-heal while a cold SW is mid-create) both
miss `get()`, the loser's duplicate-ID `TypeError` is caught as "BG unavailable"
→ launches `fallbackFetch` — a second full ~100 MB download racing the first. Fix:
a `bgFetchCreationsInFlight` map keyed by fetchId (mirror of `fallbackRuns`); or
re-`get()` in the catch and return if an in-progress registration now exists.

**DL-2 — overlay wipes manual downloads.** *(verified 2026-07-28)*
`missionhydrosci_play.gohtml` — `autoCleanup([nextUnit])` (two sites) keeps only
the next unit; units/manage keep `current + next + mhs-manual-downloads`. Completing
a unit deletes any manually pre-downloaded unit. Fix: parse `mhs-manual-downloads`
and add to the keep-list, matching the other pages.

**DL-3 — fourth, unmanaged download driver.** *(from review)*
`bootstrap/routes.go` post-login early-download action posts a raw `download`
message to the SW with **no `MHSDeliveryManager`** — no stall watchdog, no
zero-byte/frozen fallback switch, no prefer-fallback pin, no telemetry, no dedupe
with the units page the user lands on. Best folded into the ARCH-2 factory.

**DL-4 — play heartbeat not re-armed after `pagehide`.** *(from review)*
`missionhydrosci_play.gohtml` stops the play heartbeat on `pagehide` with no
`event.persisted`/`pageshow` handling; an iOS bfcache-restored game runs
unprotected, re-opening the MHS-013 prune-under-live-game window after the 30-min
TTL. Fix: don't stop when `event.persisted`; restart on `pageshow`.

**DL-5 — throttle idle-reset reference time.** *(from review)*
`auththrottle.go` compares `now` against `nextAllowed` (a future time during
backoff), not the last attempt, so "gone quiet 10 min" is really "10 min after
the backoff window ends". Benign but wrong semantics. Track `lastAttempt`.

**DL-6 — throttle key derivation divergence.** *(from review)*
`checkMemberAuth` builds `userOID.Hex()+":"+wsOID.Hex()`; `authThrottleKey`
(used by verify) uses raw `user.ID + ":" + ws.Hex()`. Equal today, but the
"shared budget" claim depends on it. Have `checkMemberAuth` call
`authThrottleKey`.

**DL-7 — correlated `$lookup` DocumentDB risk.** *(from review)*
`store/groupapps/groupappstore.go` `GetMHSCollectionForUser` (runs on every
manifest resolution) uses a `$lookup` with an inline `let`/`$expr` sub-pipeline —
restricted on older DocumentDB — and swallows the error (`return nil`), silently
masking group pins. Rewrite as a two-step `$in` query (or the equal-field
`$lookup`) and log the error.

**DL-8 — double collection resolution per render.** *(from review)*
`ServeUnits`/`ServeManage` each call `resolveManifest` + `resolveEffectiveCollectionInfo`,
both of which call `resolveCollection` → the group-pin aggregation + progress
upsert run twice per page load. Resolve once and share.

### Accessibility / UX

**UX-1 — aria-live progress spam.** *(from review; a regression from the A2 fix)*
`missionhydrosci_units.gohtml` `#progress-text`/`#status-<id>`/`#next-unit-status`
are `aria-live="polite"` and get a new percent up to 1/s for minutes — two live
regions announcing at once. Fix: keep live regions for *transitions* only; put
`role="progressbar"` + `aria-valuenow/min/max` on the bar; or announce coarse
milestones (every 25%). Update accessibility-tracker MHS-A2.

**UX-2 — contrast fix nullified at runtime.** *(from review)*
The template bumped state text to gray-500, but the JS status branches reset it to
`text-gray-400 dark:text-gray-500` (`units.gohtml` `not_cached`/next-queued;
`manage.gohtml` likewise), so "Not downloaded" *displays* below AA. Fix the JS
class strings + the version line; update tracker MHS-A4.

**UX-3 — 429 UI dead-end.** *(from review)*
The client special-cases only 403; a throttle 429 becomes a raw alert with no
wait affordance, and the server never sends `Retry-After` though it knows the
exact wait. Send the header; handle 429 in `handleAuthFailure`/the gate.

**UX-4 — no `resp.ok` in `handleUnitComplete`.** *(from review)*
`missionhydrosci_play.gohtml` does `resp.json()` without an ok check; works today
only because completion errors are plain-text (parse throws → offline queue). If
that endpoint ever returns JSON errors (this feature does elsewhere), a 4xx/5xx
parses cleanly, reads as final, and **drops the completion without queuing**. Add
`if (!resp.ok) throw`.

**UX-5 — `crypto.randomUUID()` no fallback.** *(from review)*
`units.gohtml` uses `crypto.randomUUID()` for the device id; throws on Chrome <92
(plausible on aged Chromebooks), silently killing the device telemetry we rely on
to diagnose those very devices. Add a `Math.random`-based fallback.

**UX-6 — `quota === 0` → `NaN%`.** *(from review)*
`units.gohtml` + `manage.gohtml` storage math guards only `!estimate`; a `quota`
of 0 yields "NaN%" and skips low-storage logic. Guard `if (!estimate ||
!estimate.quota) return;`.

**UX-7 — initial device report counts events, not units.** *(from review)*
`units.gohtml` `reportedCount++` increments per status event, so rapid
`downloading` events for one unit can trip the initial report before other units
report. Count `Object.keys(initialReport).length`.

**UX-8 — picker `c.id` unescaped (hardening).** *(from review)*
`manage.gohtml` collection picker interpolates `c.id` into an onclick string
unescaped (`c.name`/`c.units_summary` go through `escapeHtml`). Server-controlled
hex today — hardening only. Build via `createElement` + `dataset`.

### September launch readiness

**RDY-1 — iPad smoke pass (never done).** *(from review; highest-risk gap)*
iPads are a named launch platform and the **only** fallback-only path; the
MHS-014/016/018 iOS SW-termination fixes have never run on real hardware. Needs
device time. Exercise: download a unit, unit-complete transition, offline launch,
Clear/Reset, a member collection-switch under staffauth, the download-mode
notices. This is the biggest unknown for September.

**RDY-2 — MHS-005 storage persist.** *(open issue)*
Request `navigator.storage.persist()` in `init()` and report `persisted()` in
device status. Field disks run near-full (incident device 94%); best-effort
storage is evictable. Trivial + high value. (`issues/MHS-005-*.md`.)

**RDY-3 — MHS-006 per-unit space preflight.** *(open issue)*
Check available quota vs. unit size before downloading; warn/gate rather than
fail mid-download. The only remaining unmitigated engine for quota failures; the
new quota-classified error softens the landing but doesn't prevent it. Pairs with
RDY-2. (`issues/MHS-006-*.md`.)

**RDY-4 — dev-sentinel hardening.** *(from review)*
`bootstrap/startup.go` `ensureMHSDevSentinel` seeds a `trust`/`member`/`active`
user with the published login `mhs-developer-sentinel` into the **default/prod**
workspace unconditionally. If `trust` login is reachable there, anyone can sign in
as it. Gate seeding to non-prod, or make the sentinel non-loginable, before
classes start.

**RDY-5 — `totalUnits = 5` fallback.** *(from review)*
`progress.go` `HandleCompleteUnit` falls back to `totalUnits = 5` when the
manifest resolves empty; on a future 6-unit collection a manifest outage would
mark a student complete after unit 5. Fail the request instead of guessing.

**RDY-6 — MHS-004 guest-profile close-out.** *(open issue, mostly done)*
Fallback now has retries/resume/watchdog/lifecycle fixes; remaining is one
guest/incognito verification run + adding Background-Fetch availability to
telemetry, then close or re-scope. (`issues/MHS-004-*.md`.)

### Architecture

**ARCH-1 — pipeline duplication (refactor or freeze).** *(from review)*
~150 near-verbatim lines duplicated between `missionhydrosci_units.gohtml` and
`missionhydrosci_manage.gohtml` (manual-download helpers, `buildKeepList`,
`runPipeline`, self-heal conditions, storage throttle, `updateStorageInfo`),
hand-mirrored through many edits and already divergent in places. The deferred
MHS-020 status→UI-table refactor is the right fix; doing it late is risky.
**Decision needed:** land the refactor now, or declare a pipeline freeze until
after launch.

**ARCH-2 — manager construction factory.** *(from review)*
`MHSDeliveryManager` is constructed at 3–4 sites (units, manage, play overlay,
and the DL-3 early-download hook which uses no manager at all). The `mhs-delivery.js`
DEFAULT_* constants are wrong for every real deployment, and the play overlay pokes
private fields post-construction. A single factory with canonical config removes
those traps and absorbs DL-3.

**ARCH-3 — dead code.** *(from review)*
`PlayData.CDNBaseURL` (populated, unread), `nextUnitVersion` in the play template
(assigned, unread), and any stragglers. Remove.

### Docs / ops / tests

**DOC-1 — feature README refresh.** The `README.md` describes the system at SW
1.0.7 / 2026-07-07: missing `manage.go`/`savedata.go`/`auththrottle.go` and newer
routes; auth model lacks the sliding unlock + `MHSStaffUnlockDuration`; "no Go
unit tests" is now false; "two templates" is now four; `mhs-prefer-fallback-until`
missing from the state table; also record the "shared game-bridge tokens accepted
as-is" decision here.

**DOC-2 — `issues/README.md` refresh.** "in QA 2026-07-07", "SW_VERSION 1.0.7",
the open-issue list, the obsolete member-auth "Client gate (modal)" table, and the
smoke-test checklist (missing every post-07-07 behavior: manage gate/unlock, Set
current in place, zero-byte/frozen fallback switch, fullscreen+WASD, final-unit
overlay hide, friendly error + telemetry, download-mode notices).

**DOC-3 — close/re-scope MHS-008.** `issues/MHS-008-*.md` still "Documented".
Only the download-failure-reason telemetry shipped; cache-inventory,
`storage_persisted`, BG-availability, and admin-view proposals were not built.
Close explicitly and note what remains (folds into RDY-2/RDY-6).

**OPS-1 — telemetry monitoring + runbook.** Nothing watches the
`"mhs download error"` log line (no saved query/dashboard/alert; the grep string
lives only in the review doc), and no teacher/support runbook maps the friendly
error messages ("network or security-software", "not enough space", the
background-vs-keep-tab-open modes) to actions. Minimum: a documented journalctl/grep
recipe + a weekly review ritual, and a short support page.

**TEST-1 — test coverage.** No automated tests for the newest behaviors:
`HandleDownloadError`; set-unit manifest validation; `checkMemberAuth` in
staffauth mode (single-use consumption, `GrantedBy`); the `ServeManage` "content
never sent while locked" guarantee; and **the entire client JS** (no test
harness at all — the P0 manifest guard, skew fix, `classifyDownloadError`,
frozen-switch, and download-mode logic rest on `node --check` + manual smoke). A
minimal node harness over `classifyDownloadError`/`parseFetchId`/the throttle math
would be cheap and real.

---

## Suggested sequencing

1. **Cheap security P1s first:** SEC-1 (fail-closed default) and SEC-2 (throttle
   `/api/auth/start`). Both small, both reachable by any gated member.
2. **September verification, in parallel with device availability:** RDY-1 (iPad
   pass — the biggest unknown), RDY-2 + RDY-3 (storage persist + preflight),
   RDY-4 (dev-sentinel), RDY-5 (`totalUnits=5`).
3. **Client-correctness P2s:** DL-1 (creation race), DL-2 (manual-download wipe),
   DL-7 (DocumentDB `$lookup`), UX-1 (aria-live spam).
4. **Decide ARCH-1** (refactor vs. freeze) before touching either pipeline again;
   ARCH-2 factory naturally absorbs DL-3.
5. **P3 batch + docs/ops** (SEC-3/4/5, DL-4/5/6/8, UX-2…8, DOC-1/2/3, OPS-1,
   TEST-1) as budget allows — several are one-line fixes worth batching.
