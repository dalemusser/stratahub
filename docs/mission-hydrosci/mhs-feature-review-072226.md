# Mission HydroSci — Feature Review (Round 4)

**Date:** 2026-07-22
**Target:** HEAD = `94b9aba` (all prior fix waves committed; working tree clean)
**Reviewer:** Claude — four parallel deep reviews (Go server, client JS/service
worker, templates, design/docs/readiness); every significant finding re-verified
against source by hand. **No changes made** — this is findings and
recommendations only.

**Baseline health:** `go build ./...`, `go vet`, all MHS + staffauth Go tests,
and `node --check` on all four JS/SW files pass. Served worker is
`SW_VERSION 1.0.12`.

## How to read this

This is the fourth review round. Prior rounds are recorded in
`mhs-code-design-review-071626.md` (the running fix log) and `issues/`
(MHS-001…021). **Nothing already tracked and closed is repeated here.** The
recurring pattern held again this round: several findings are defects or gaps
introduced (or amplified) by the previous rounds' own fixes — including code I
wrote. Those are called out plainly.

Two standing decisions are respected and not re-litigated: the shared
game-bridge service tokens in the play page (accepted as-is), and SSO staff-auth
failing closed (deliberate; see `SSO-STATUS.md`).

Severity: **P1** = student-facing dead end or security hole reachable today;
**P2** = real defect/risk, bounded; **P3** = polish, hardening, hygiene.

---

## 1. Priority findings (all hand-verified)

### P1-1. End-of-unit transition strands the student when the manifest can't resolve the next unit — including the flagship offline case

*Found independently by two reviewers; every link verified.*

The play overlay's transition depends on a **fresh manifest fetch** by a **new
`MHSDeliveryManager`**, and fails silently when that can't produce the next
unit:

- The service worker does **not** cache or intercept
  `/missionhydrosci/api/manifest` (`static/sw.js:45-74` handles content,
  `/units`, `/play/*`, and versioned assets only). Offline, the overlay
  manager's `refreshManifest()` fails and leaves an empty manifest.
- `isCached(nextUnit)` returns `'not_cached'` when the unit isn't in the
  manifest — **regardless of actual cache contents** (`mhs-delivery.js`
  `isCached`).
- The flow therefore enters `showDownloadProgress` → `downloadUnit(nextUnit)`,
  which **early-returns with only a `console.error`** when the manifest is
  empty or the unit is missing (`mhs-delivery.js:1127-1137`). No status ever
  fires; the overlay's exit paths all hang off statuses.

**Failure scenario (offline — the feature's core promise):** a student finishes
a unit offline with the next unit already fully downloaded. The completion POST
fails → offline queue path → `completeAndTransition(nextUnitId, false)` →
Unity is already torn down → the overlay (z-index 100, covering the back
button at z-index 20) shows "Downloading next unit..." forever. Only a manual
reload escapes.

**Failure scenario (online — concrete server-side trigger):**
`api_manifest.go:420-426` *excludes* a collection unit from the manifest when
its build record is missing, while `/api/progress/complete` still returns that
unit as `next_unit`. Same dead end, fully online. Also reachable via a
collection switch between page load and unit completion.

**Why this survived three rounds:** this is review-doc **P2-5** ("fire
`_fireStatus` from every early return") — it was analyzed and specced but never
implemented; the surrounding fixes' update notes made the doc read as if the
area was done. The play page's own comment ("Every terminal state must
navigate", `_play.gohtml:419-424`) is only true if statuses actually fire.

**Fix (three small parts):**
1. Implement P2-5: `_fireStatus(unitId, 'error', {error: 'Unit not available.'})`
   from both silent early-returns of `downloadUnit` **and** `retryDownload`
   (the latter also still leaves the units-page Retry button stuck disabled).
2. In `completeAndTransition`, after `refreshManifest()`, check
   `mgr.manifestLoaded`; if false, `navigateToNext(nextUnit)` immediately —
   `/play/*` is SW-cached network-first, so a previously visited play page
   works offline.
3. Add `.catch()` to the overlay's `mgr.init().then(...)` chain
   (`_play.gohtml:485`).

### P2-1. `checkMemberAuth` fails OPEN on an unrecognized member-auth mode — and now grants a sliding unlock

`api_manifest.go` — the mode switch has `case "staffauth"` and
`case "keyword"` but **no `default`**, and `GetMHSMemberAuth()`
(`sitesettings.go:103-108`) returns any non-empty stored string verbatim. An
out-of-band value (direct DB edit, migration, or a future mode like `"sso"`)
makes every gated member action pass with **empty credentials** — and since the
round-3 unlock refactor, it also **grants a 10-minute sliding unlock** with
`granted_by: ""` and clears the member's throttle state. The old code had the
same missing default but authorized one action; the refactor amplified it to a
session. This contradicts the fail-closed stance deliberately adopted in
`staffauth.go`'s SSO branch.

Not reachable through the settings UI (it validates the enum); reachable
through any non-UI write. Cheap insurance with real payoff.

**Fix:** `default: return http.StatusForbidden, "member authorization is
misconfigured for this workspace"` + an Error log — mirroring the staffauth
default branch.

### P2-2. `/api/auth/start` is unthrottled: teacher mail-bomb, staff enumeration oracle, and code invalidation

The round-3 backoff was wired into `/api/auth/verify` and `checkMemberAuth` but
**not** `HandleStaffAuthStart` (`staffauth.go:41-80` — verified: no throttle
call). For an email-method staff account, every `start` call sends a mail
(`emailverify.Create(..., isResend=false)` — and the non-resend path has **no
rate limit**, `store/emailverify/store.go:129`; it also `DeleteMany`s the prior
verification first). Any gated member today can:
(a) loop `POST /api/auth/start {login_id: <teacher email>}` → mail-bomb the
teacher / burn SMTP quota; (b) use the distinct 404/403/400 responses
(`ErrUserNotFound` / `ErrUserNotStaff` / `ErrUnsupportedAuthMethod`) as a
staff-account enumeration oracle; (c) repeatedly invalidate the code a
legitimate staffer is trying to type.

**Fix:** apply `h.authThrottle` (same key as verify) to `HandleStaffAuthStart`;
return a generic message for not-found/not-staff.

### P2-3. SW download-creation race can run a Background Fetch AND a fallback loop concurrently for the same unit

`startBackgroundFetch` awaits `get(fetchId)` then `fetch(fetchId)`, and the SW
message handler runs `download` actions **concurrently**
(`sw.js:181-190`, `event.waitUntil` per message — verified). Two `download`
messages inside the creation window (session-restore reopening units+manage
tabs whose pipelines both fire at init; or the page poller self-healing while a
cold SW is still inside `backgroundFetch.fetch()`) both miss the `get()`, the
loser throws the duplicate-ID `TypeError`, and the catch — which treats *every*
error as "Background Fetch unavailable" — launches `fallbackFetch`: a second
full download of the same multi-hundred-MB unit racing the first over school
Wi-Fi. The round-3 `fallbackRuns` dedupe only covers the opposite ordering.

**Fix:** keep a `bgFetchCreationsInFlight` map keyed by fetchId (mirror of
`fallbackRuns`); or in the catch, re-`get(fetchId)` and return true if an
in-progress registration now exists before falling back.

### P2-4. The aria-live pass made downloads absurdly chatty for screen readers (regression from the MHS-A2 fix)

`#progress-text`, `#status-<id>`, and `#next-unit-status` are all
`aria-live="polite"` **and** receive a new percentage up to once per second for
many minutes per unit — two live regions announcing simultaneously for the
current unit, plus a burst at page load as 5-6 rows flip from "Checking...".
The correct pattern: keep live regions for state *transitions* only
(`#unit-status` already does this — stable text), put
`role="progressbar"` + `aria-valuenow/min/max` on the bar so progress is
queryable on demand, or announce coarse milestones (every 25%) from one
visually-hidden region. Update tracker MHS-A2 when revised.

### P2-5. Play overlay still wipes manual downloads (the unimplemented third bullet of review-doc P2-4)

`_play.gohtml:394,458` — `autoCleanup([nextUnit])`; `mhs-manual-downloads` is
not referenced anywhere in the play template (verified by grep). Units and
manage keep `current + next + manual`; completing a unit deletes any unit a
tester or teacher deliberately pre-downloaded (hundreds of MB to re-fetch, or
unplayable if now offline). The review doc's 07-17(b) update fixed bullets 1-2
of P2-4 but this one never landed. **Fix as originally specced:** parse
`mhs-manual-downloads` in the overlay path and append to the keep-list.

---

## 2. Secondary findings (P3)

### Go server

- **Throttle idle-reset uses the wrong reference time** (`auththrottle.go:61-65`):
  compares `now` against `nextAllowed` (a future time during backoff) instead of
  the last attempt, so "gone quiet for 10 min" is actually "10 min after the
  backoff window ends". Benign for the deterrent purpose; semantics don't match
  the comment. Track `lastAttempt` explicitly.
- **Throttle keys derived two ways** — `checkMemberAuth` builds
  `userOID.Hex()+":"+wsOID.Hex()` while `authThrottleKey` uses the raw session
  `user.ID`; equal today, but the "shared budget" claim depends on it. Have
  `checkMemberAuth` call `authThrottleKey`.
- **429s are invisible/dead-end in the UI**: the client special-cases only 403;
  a throttled student gets a raw alert with no wait affordance, and the server
  never sends `Retry-After` (it knows the exact wait). Send the header; handle
  429 in `handleAuthFailure`/the gate.
- **`HandleDownloadError` has no server-side rate limit** — the once-per-
  unit+class dedupe is client JS only. Bounded (8 KB body, clipped fields,
  Warn-level log line), matching its documented threat model; add a
  per-user/minute cap only if log volume ever matters.
- **Group `middleware.Timeout` interactions verified safe** — no streaming
  handlers in the group; SW/manifest/content served outside it; chi's Timeout
  only cancels the context (never writes a mid-response 504). No defect.

### Client JS / service worker

- **Success-reconcile can fire a false `'error'` after a genuine `'cached'`**
  (`mhs-delivery.js:729-747`): the second reconcile's `_checkUnitCache` await
  can straddle the SW's final `cache.put` + `'cached'` broadcast; the stale tick
  then flips the UI to Retry over a fully cached unit and logs a phantom
  telemetry event. Guard after each await:
  `if (this._stallState[unitId] !== state) return;` (same guard useful in
  `_recheckActiveDownloads`).
- **`_recheckActiveDownloads` "disappeared" branch clears tracking with no
  status** and lacks the fallback-adoption check `_pollDownloadOnce` has — a
  unit can stick at "Downloading N%" until the next tab switch, and an
  SW-side fallback switch that happened while hidden gets cleared instead of
  adopted. Mirror the poller's disappeared handling.
- **Dedupe-adopt gaps on uncontrolled pages**: after a hard/shift reload
  `navigator.serviceWorker.controller` is null, so `_getActiveFallbacks`
  returns `[]` and the poller flaps partial/downloading every ~5s against the
  SW's dedupe (no duplicate download; visible churn). Also the SW's
  fallback-adopt early return rebroadcasts nothing, so the requesting page sits
  at 0% up to ~6s. Rebroadcast a `'downloading'` snapshot from the dedupe
  branch; let `_getActiveFallbacks` fall back to `reg.active`.
- **`classifyDownloadError` maps HTTP 4xx/5xx to `network`** with "not your
  account — check your connection" copy: a broken CDN deploy (persistent 404s)
  or CDN outage tells every student to blame their Wi-Fi and skews the
  telemetry class data. `rawError` preserves the status (greppable), but
  classify `http-<status>` distinctly with server-fault copy for 5xx/404.
- **Telemetry fidelity:** page-originated error reports never set `version`
  (in scope at every call site); the same BG failure can log twice under two
  classes (`bgfetch-<reason>` from the SW broadcast + `generic` from the page
  poller backstop — dedupe key is unit+class); `device_type` is parsed
  server-side but never sent. All small; fix together.
- **`units: null` contract break (interacts with the round-3 P0 fix):** a
  no-active-collection workspace serves `ContentManifest{CDNBaseURL}` → nil
  slice → `"units": null` (`api_manifest.go:459`, no `omitempty` in
  `types.go:87`) → the client's `!Array.isArray(parsed.units)` validation
  rejects it as a *failed* load. Every page load there logs a spurious manifest
  error and `manifestLoaded` can never become true (the intended
  loaded-but-empty branch is unreachable). Behavior is coincidentally safe —
  prune stands down either way — but fix the contract: `Units:
  []ContentManifestUnit{}` in the `!ok` branch (and/or accept `null` client-side).
- **Nits:** default constants (`mhs-unit-`, `/mhs/content/`…) mismatch every
  real deployment — make the real names the defaults and delete the play page's
  private-field pokes; duplicate `var self = this;` in `init`;
  `_activeDownloads[unitId] = false` (vs `delete`) in the downloadUnit catch;
  `SW_VERSION` now has no runtime reader (comment it as update-trigger-only);
  progress-floor substring matching can credit the wrong file (cosmetic).

### Templates

- **The contrast fix (MHS-A4) is nullified at runtime**: JS status branches
  re-set `className` to `text-gray-400 dark:text-gray-500`
  (`units.gohtml:401,526`; `manage.gohtml:287,534`), so "Not downloaded" lines
  *display* below AA within ~1s of load despite the template bump; the version
  line (`units.gohtml:133`) is also gray-400. Fix the four JS class strings and
  two template spots; update tracker A4.
- **`handleUnitComplete` lacks a `resp.ok` check** (`_play.gohtml:348-358`):
  works today only because errors are plain-text (json parse throws → offline
  queue). If the endpoint ever returns JSON errors (this feature already does
  elsewhere), a 4xx/5xx would parse cleanly, read as "final", and **drop the
  completion without queuing**. Add `if (!resp.ok) throw ...`.
- **Collection picker interpolates `c.id` unescaped** into onclick HTML
  (`manage.gohtml:940,948`). Server-controlled hex today — hardening only:
  build via `createElement` + `dataset`.
- **Focus-trap gap (minor):** if focus lands on `<body>` (click on dialog
  padding), Tab can escape both traps — known limitation of the
  first/last-element pattern; fix opportunistically.
- **Dead code:** `nextUnitVersion` (`_play.gohtml:294`) and
  `PlayData.CDNBaseURL` are populated but never read.
- Verified clean: zero dangling element-ID references both ways across all four
  templates; VM/template consistency after the field removals; `csrfToken`
  wiring on all three pages; clock-skew countdown sound (fallback path
  unreachable in practice); final-unit `completionHandled` semantics correct;
  dark-mode coverage on all new markup; no template XSS (contextual
  autoescaping verified throughout).

---

## 3. Review-doc items that never landed (now made explicit)

The running review doc's body lists these as findings with specced fixes, but
no Update section implemented them — they are **still open** and easy to lose
(this round's P1-1 is exactly one of them resurfacing at higher severity):

- **P2-5** — status-less `downloadUnit`/`retryDownload` early returns (→ this
  round's P1-1).
- **P2-4 bullet 3** — overlay keep-list ignores manual downloads (→ this
  round's P2-5).
- **P2-8 tail** — overlay `autoCleanup(...).then(navigate)` chains still have
  no `.catch`; `deleteUnit`'s unguarded `caches.delete`.
- **P3** — play heartbeat permanently stopped on `pagehide` (no
  `event.persisted`/`pageshow` re-arm; bfcache-restored iOS game unprotected).
- **P2-9** — correlated `$lookup` sub-pipeline in `groupappstore.go:131`
  (DocumentDB risk on every manifest resolution; error swallowed).
- **P3** — double collection resolution per units/manage render.
- **P3** — unlock-window actions not logged with attribution (plan promised
  "authorized by <staff>" per action).
- **P3** — `ValidateAndConsumeToken` lacks workspace binding.
- **P3** — `HandleDeviceStatus` input unbounded (contrast the bounded
  download-error handler).
- **P3 batch** — `crypto.randomUUID()` without fallback (units:251; throws on
  Chrome <92 — silently kills device telemetry on exactly the aged fleet);
  `quota === 0` NaN guard (units:687, manage:663); initial device report
  per-event counting (units:349-360).

**Recommendation:** fold the survivors into `issues/` (individual files or one
MHS-022 hygiene issue) so `issues/README.md` is again the single tracker.

---

## 4. Documentation drift

The code has outrun the docs by five deploy waves. Stale items (verified):

**`issues/README.md`:** "in QA as of 2026-07-07"; "SW_VERSION is 1.0.7" (now
1.0.12); "Still open: MHS-004, -005, -006, -008" (008's failure-reason half
closed 2026-07-22); the smoke-test checklist omits everything added since 07-07
(manage gate/unlock/countdown/Lock-now, Set-current in place, Clear/Reset
staying on /manage, zero-byte fallback switch + 24h pin, fullscreen-exit +
WASD check, final-unit overlay hide, friendly error + telemetry log line); the
member-auth table's "Client gate (modal)" column is obsolete and the table is
missing the save-data deletes and the /manage entry gate.

**Feature `README.md`:** server-file table missing `manage.go`, `savedata.go`,
`auththrottle.go` and the newer routes; client inventory missing the manage
template; authorization model missing the sliding unlock session +
`MHSStaffUnlockDuration`; "SW_VERSION is 1.0.7"; "still open …-008"; "state
lives" table missing `mhs-prefer-fallback-until`; "this feature has no Go unit
tests" now false (14 test funcs across 4 files); "parse both templates" — there
are four.

**`issues/MHS-008-*.md`:** still "Status: Documented" — should be closed/
re-scoped explicitly (only failure-reason telemetry shipped; cache-inventory,
`storage_persisted`, BG-availability, and admin-view proposals were not built).

**Accurate:** `SSO-STATUS.md`, `accessibility-tracker.md` (modulo the A4
runtime nullification above), the redesign plan (except its "uncommitted"
header), and the review doc itself.

Also: the "shared game-bridge tokens accepted as-is" decision lives only in the
review doc — it belongs in the README's authorization section so it isn't
re-flagged forever.

---

## 5. September launch readiness (prioritized)

1. **Fix P1-1 + P2-1 + P2-2 + P2-3** (this round's headline fixes — all small).
2. **MHS-005 `storage.persist()`** — trivial (one call in init + report
   `persisted()` in device status). Field disks run near-full (the 07-22
   incident device was 94% full); best-effort storage silently evicts units.
3. **MHS-006 per-unit space preflight** — small/medium; the only remaining
   unmitigated engine for mid-download quota failures. Pairs with 005.
4. **iPad smoke pass — has never been done.** iPads are a named launch platform
   and the *only* fallback-only platform; the MHS-014/016/018 fixes have never
   been validated on real iOS. Highest-risk untested surface; needs only device
   time.
5. **Managed-Chromebook (ACER-class) retest** of the pausing mitigations on a
   school-managed, possibly-metered device.
6. **Low-end-device P3 batch** (~1-2h): `crypto.randomUUID()` fallback, quota
   NaN guards, device-report counting.
7. **MHS-004 closure** — mostly done by attrition (fallback now has
   retries/resume/watchdog); one guest-profile verification run + BG-fetch
   availability in telemetry, then close or re-scope.
8. **Dev-sentinel hardening** — `startup.go` still unconditionally seeds a
   `trust`/`member`/`active` user with a published login ID into the default
   workspace. Gate to non-prod before classes.
9. **Refactor-or-freeze decision** (see §6): either land the deferred
   status→UI-table refactor now or declare a pipeline freeze until after
   launch.
10. **SSO sequencing** — if SSO ships before September, MHS hard-blocks SSO
    teachers until the email-code path in `SSO-STATUS.md` is built.
11. **6-unit residue** — `progress.go` falls back to `totalUnits = 5` when the
    manifest resolves empty; on a future 6-unit collection a manifest outage
    would mark students complete after unit5. Fail the request instead of
    guessing.

---

## 6. Architecture & operations concerns

- **Pipeline duplication is drifting now, not hypothetically.** ~150 lines
  near-verbatim duplicated between the units and manage templates (manual-
  download helpers, `buildKeepList`, byte-identical `runPipeline`, self-heal
  conditions, storage throttle, `updateStorageInfo`), hand-mirrored through six
  waves of edits, already divergent in places nobody chose (e.g. `partial`
  renders differently). The play overlay is a third, differently-shaped
  `onStatus` policy. The deferred MHS-020 status→UI-table refactor is the right
  fix; doing it late is risky — decide now (refactor or freeze).
- **There are FOUR download-driving sites, not three.** Besides units/manage/
  play-overlay, the post-login early-download hook
  (`bootstrap/routes.go:331-397`) posts a raw `download` message with **no
  manager at all** — no stall watchdog, no zero-byte fallback switch, no
  prefer-fallback pin, no telemetry, no page-side dedupe with the units page
  the user lands on. A single factory with canonical config (which would also
  fix the wrong default constants and the overlay's private-field pokes) is the
  missing piece.
- **Telemetry has no operator loop.** One `zap.Warn("mhs download error")` line
  per event; no saved query, dashboard, or alert — the grep string appears only
  in the review doc. Structural blind spot: reporting rides a client POST, so
  fully-offline failures can never report. Minimum before September: a
  documented journalctl/grep recipe + a weekly review ritual, and a
  teacher/support runbook mapping the two new friendly messages to actions
  (nothing in `docs/stratahub-user-documentation/` mentions them).
- **Single-instance couplings are acceptable and honest** — `authThrottle`
  documents its in-memory scope; unlock sessions/challenges/device status are
  Mongo-backed. The log-only telemetry is the other single-instance coupling.
- **Test coverage gaps (newest behaviors with zero tests):**
  `HandleDownloadError`; set-unit manifest validation; `checkMemberAuth` in
  staffauth mode (single-use consumption, GrantedBy); the ServeManage
  "content never sent while locked" guarantee; and the entire client JS — there
  is no JS test infrastructure at all, so the P0-1 manifest guard, the skew
  fix, `classifyDownloadError`, and the fallback switches rest on `node --check`
  plus manual smoke. Even a minimal node-based harness exercising
  `classifyDownloadError`, `parseFetchId`, and the throttle/backoff logic
  would be cheap and real.

---

## 7. What held up (verified clean this round)

The round-3 work broadly survived adversarial re-review: the `manifestLoaded`
gating of prune/reconnect-abort (no stale-prune hole; correct pairing of
keep-old-manifest with stand-down), the success cache-confirmation, SW↔page
compat in both deploy-window directions (old page/new worker and vice versa),
cancel paths (user cancels never produce error broadcasts or telemetry),
`cache.put` without clone, the final-unit overlay fix, `__mhsResize`, the
clock-skew countdown, the a11y dialog/focus work (except the two notes above),
CSRF wiring, `ErrLog` conversions (correct argument order; ErrLog always
populated), and the staffauth fail-closed default. The unlock store, throttle
tests, and savedata flow remain solid.

---

## 8. Recommended action plan

**Batch A — before the next deploy (small, high value):**
P1-1 (three-part fix), P2-1 (`default` case), P2-2 (throttle `start`),
P2-3 (creation-race guard), P2-5 (overlay keep-list), the `resp.ok` check,
and the `units: null` contract fix.

**Batch B — accessibility/UX corrections:**
aria-live de-chatter (progressbar pattern), the runtime contrast fix,
429 `Retry-After` + client handling.

**Batch C — September readiness:**
MHS-005 persist + MHS-006 preflight, iPad smoke pass, ACER retest, low-end P3
batch, sentinel gating, MHS-004 closure.

**Batch D — hygiene & structure:**
Fold §3 survivors into `issues/`; refresh both READMEs + close MHS-008's file;
telemetry fidelity fixes; refactor-or-freeze decision on the pipelines; factory
for manager construction (absorbs the early-download hook); minimal JS test
harness; support runbook + log-watch recipe.
