# Mission HydroSci — Code & Design Review

**Date:** 2026-07-16
**Reviewer:** Claude (multi-agent review, findings verified against source)
**Scope:** The full Mission HydroSci feature in `stratahub/` — Go server
(`internal/app/features/missionhydrosci/`, `store/mhsdevicestatus/`,
`system/staffauth/`), the client delivery brain
(`assets/js/mhs-delivery.js`), the three service-worker sources
(`static/sw-cache.js`, `sw-background-fetch.js`, `sw.js`), the page templates
(`missionhydrosci_units.gohtml`, `_play.gohtml`, `_manage.gohtml`,
`_offline.gohtml`), and the manage-page / staff-unlock redesign work.

## Update 2026-07-17 (b) — P2/P3 batch: client races, Go handlers, accessibility

Deployed to dev (SW_VERSION 1.0.11) and smoke-tested (0 console errors on units /
manage / play; visibility toggle left all unit caches intact with no duplicate
download; modals verified as labeled dialogs with focus-in).

- **P2 — visibility-vs-copy duplicate download — FIXED & VERIFIED.**
  `_recheckActiveDownloads` no longer clears tracking the instant a Background
  Fetch reports `success`; it reconciles from the cache and only finishes when
  the cache is actually complete (else it leaves tracking + lets the poller's
  grace converge), so the concurrent `checkAllCacheStatus` can't fire `'partial'`
  and trigger a duplicate download.
- **P2 — SW-side download dedupe — FIXED.** `startBackgroundFetch` now adopts an
  active sequential fallback loop for the same unit+version instead of starting a
  concurrent Background Fetch — the one dedupe point that works across pages and
  reloads. (The reverse direction is intentionally left open so the
  prefer-fallback path can still escape a paused Background Fetch.)
- **P2 — handler context timeouts — FIXED.** A `middleware.Timeout(timeouts.Long())`
  on the MHS route group bounds `r.Context()` for every handler AND its shared
  helpers (`resolveManifest`, `checkMemberAuth`, …) at once, so a stalled DB
  primary can't pin request goroutines. Static content / the SW are served
  outside the group and unaffected.
- **P2 — page-handler error surfacing — FIXED.** `ServeUnits`/`ServeManage` no
  longer swallow the progress DB error and render a misleading "new user on
  unit1" page; both (and `ServePlay`) now distinguish no-record from DB error and
  route failures through `h.ErrLog` (branded error page + central logging)
  instead of bare `http.Error`.
- **Accessibility pass — DONE (MHS-A1..A5) / PARTIAL (A6).** Dialog roles + labels
  + focus management on the units help and manage collection-picker modals;
  `aria-live`/`role` on status and gate error/loading lines; 44px play controls;
  contrast bumps; `aria-label` on the gate inputs; the misleading iPad "Press ?
  for help" hint suppressed on touch-only devices. Details and the one deferred
  item (play-overlay focus trap) are tracked in
  `docs/accessibility/accessibility-tracker.md`.

## Update 2026-07-17 — play-page P1s implemented, deployed, smoke-tested

- **P1-2 (`onResize` fullscreen ReferenceError → crash-telemetry poison) — FIXED
  & VERIFIED LIVE.** `onResize` is now exposed as `window.__mhsResize` and the
  two out-of-IIFE fullscreen handlers call that. On dev: calling
  `onFullscreenChange()` and dispatching a real `fullscreenchange` event ran
  with 0 console errors and no crash report (previously threw, killing keyboard
  focus and permanently suppressing crash reports).
- **P1-3 (final-unit overlay dead-end) — FIXED & VERIFIED LIVE.** The `isFinal`
  branch of `completeAndTransition` now hides the optimistically-shown overlay so
  the game's own end screen and the back button are reachable. Dead
  `showGameComplete()` and the now-unreachable `overlay-link` markup + its click
  handler removed. On dev: triggering completion on `unit5` left the overlay
  `display:none` (previously it stayed up as "Loading next unit…" forever).
- **P1-4 (false `'cached'` on partial cache) — FIXED (deployed; behavioral quota
  case not browser-inducible).** `handleBackgroundFetchSuccess` now broadcasts
  `'error'` unless every record actually cached (`cached === records.length`,
  and records present), instead of always claiming `'cached'`. Also dropped the
  needless `response.clone()` in the copy loop (P2-8 — OOM risk on low-end
  devices). Served worker confirmed **SW_VERSION 1.0.10** on dev with the new
  guard present.

Remaining from this review after this batch: the shared game-bridge service
tokens (**P2-3 — accepted as-is, left unchanged per decision**) and the
lower-severity P2/P3 items (races, timeouts/error-surfacing on the Go handlers,
and the accessibility pass tracked in `docs/accessibility/accessibility-tracker.md`).

## Update 2026-07-16 — first batch implemented, deployed to dev, smoke-tested

The following were fixed the same day, built, deployed to dev.adroit.games, and
smoke-tested (as `sysadmin`, a `trust`-workspace staff login):

- **P0-1 (manifest failure wipes downloads) — FIXED & VERIFIED LIVE.**
  `refreshManifest` now validates `response.ok`/shape and keeps the prior
  manifest on failure, gated by a `manifestLoaded` flag that stands `pruneStaleCaches`
  and the reconnect-abort down until a real manifest loads. Verified on dev:
  routing `/api/manifest` to 500 and reloading left all four unit caches intact
  (previously this path deleted every one).
- **P1-1 (SSO staff-auth mints a token with no credential check) — FAILED CLOSED.**
  The `default`/SSO branch now returns `ErrUnsupportedAuthMethod` instead of a
  verified token. SSO is placeholder and enabled nowhere, so this was latent; it
  is now a deliberate gate. Status + the revisit checklist live in
  `missionhydrosci/SSO-STATUS.md`.
- **P2-1 (no rate limiting / non-constant-time keyword) — ADDED.** Lightweight
  per-member exponential backoff on failed keyword/token/password attempts
  (`auththrottle.go`), constant-time keyword compare, and the dead
  `/api/auth/keyword` oracle removed. (Keyword mode is not used in production;
  backoff, not lockout, per the low-effort threat.)
- **P2 (unlock countdown clock-skew reload loop) — FIXED.** The countdown now
  converts the server expiry into a deadline in the device's own clock frame via
  a server-provided "now", so a skewed device clock can't force an instant-expiry
  reload loop. (Not end-to-end smoke-testable on the `trust` dev workspace — no
  gate renders for staff; verified by parse + clean page load.)
- **P3 (unit1–5 hardcode) — FIXED & VERIFIED LIVE.** `set-unit` now validates
  against the resolved manifest. On dev: `unit2` → 200, `unit6`/`unit99` → 400
  (and `unit6` will pass automatically once a 6-unit collection exists).
- **P3 (localStorage schema drift) — FIXED.** "Reset all MHS data" now also
  purges `mhs-prefer-fallback-until` (key exported and verified present on dev).
- **P3 (dead code) — REMOVED.** Orphaned launcher per-unit button plumbing
  (`updateDownloadButtonVisibility` + suffixed lookups), the sender-less SW
  message handlers (`deleteUnit`/`checkStatus`/`getVersion`) and their
  now-unreachable cache-check helpers, unused `UnitsData` view-model fields, and
  the dead keyword endpoint. `SW_VERSION` bumped to 1.0.9. Units + manage pages
  reload with **0 console errors** on dev; served SW is 1.0.9 with the removed
  handlers absent.
- **Accessibility** findings were moved into a maintained cross-feature tracker:
  `docs/accessibility/accessibility-tracker.md` (seeded with the MHS items).

Still open from this review (not yet started): **P2-3** (shared game-bridge
service tokens — a design decision pending, see below), **P1-2/P1-3/P1-4** (play
page: `onResize` fullscreen crash, final-unit overlay dead-end, false `'cached'`
on partial cache), and the remaining P2/P3 items. All changes are currently
uncommitted on `main`.

## How to read this

This review covers work that is largely **already tracked and iterated** — the
`issues/` folder (MHS-001…021) and the `mhs-ui-redesign-plan-070926.md` addenda
document two prior review rounds and several field fixes. **Nothing already
tracked there is repeated here.** Every finding below is a *new* defect, gap, or
design concern not in that record, and the significant ones were re-verified by
reading the actual code paths (line references are current as of this date).

Baseline health is good: `go build ./...`, `go vet`, and the full MHS Go test
suite (feature + `staffauth`) all pass, and `node --check` is clean on all four
JS/SW files. The findings are about correctness under failure and adversarial
conditions, not day-one breakage.

Severity key: **P0** = data loss or core-promise breakage; **P1** = serious
functional or security defect; **P2** = real but bounded; **P3** = polish,
hardening, hygiene.

---

## P0 — must fix before the next classroom deploy

### P0-1. A failed manifest fetch wipes every downloaded unit (breaks offline)

**`mhs-delivery.js:363` (cause), `:254-300` (`pruneStaleCaches`), `:112-120` (`init` ordering)**

`refreshManifest()` swallows any fetch failure and fabricates a *valid-looking*
empty manifest:

```js
} catch (err) {
  console.error('Failed to fetch MHS content manifest:', err);
  this.manifest = { cdnBaseUrl: '', units: [] };
}
```

`init()` then calls `pruneStaleCaches()`, whose guard `if (!this.manifest ||
!this.manifest.units) return;` **passes** (an empty array is truthy), builds an
empty `valid` set, and — with no active Background Fetch and no play heartbeat —
deletes **every** `missionhydrosci-unit-*` cache.

**Failure scenario:** A student opens the installed PWA **offline** (the exact
case the feature exists to support). The units page is served from cache,
`init()` runs, the manifest request fails because there's no network, and the
prune deletes the fully-downloaded unit the student was about to play. It cannot
re-download offline. The same one-shot manifest failure online (timeout, 500,
flaky Wi-Fi) silently discards potentially gigabytes of cached units. Secondary
damage: with the empty manifest, `_reconnectActiveDownloads` misclassifies a
healthy in-flight fetch as "no manifest match" and aborts it; and the play
overlay's own `mgr.init()` (`_play.gohtml:485`) can clobber a good manifest
loaded seconds earlier, stranding the end-of-unit transition.

**Fix:** On fetch failure, keep the previous manifest (only fall back to the
empty object when `this.manifest` is still `null`); add a `response.ok` check;
and gate `pruneStaleCaches` behind a `manifestLoaded === true` flag **and**
`units.length > 0`. This is pre-existing code that neither prior review round
exercised with a failing manifest fetch — it is trivially reproducible by
loading the units page with the manifest endpoint blocked.

---

## P1 — serious; fix before broad rollout

### P1-1. Staff-auth mints a verified unlock token with no credential check for SSO/trust staff

**`system/staffauth/staffauth.go:182-194`; consumed by the manage gate `_manage.gohtml:140-141`**
*(independently found by two reviewers)*

`StartAuth` switches on the staff member's `AuthMethod`. `password` and `email`
issue a challenge that must be verified; but the `default` branch — which
catches **google, microsoft, classlink, clever** — and the `trust` branch both
return a fully verified token immediately:

```go
default:
    // For SSO methods ... fall back to simple confirmation ...
    token, err := v.Store.CreateVerifiedToken(ctx, wsID, user.ID, user.FullName)
```

**Failure scenario:** Google OAuth is the platform's primary staff auth, so most
teachers/admins have `auth_method: "google"`. In a `staffauth` workspace, a
student on the manage gate types a teacher's login ID (usually their email,
which students can see) → `/api/auth/start` returns `{method:"trust",
token:...}` → the gate JS auto-spends it (`if (data.method === 'trust') {
gateUnlock(...) }`) → instant 10-minute unlock covering *every* gated action,
including the irreversible stratasave deletes — **and the audit trail attributes
it to the teacher.** This is exactly the class of bypass MHS-010 was fixed to
close server-side, re-opened through the auth-method fallback. The redesign
*amplifies* it: what used to grant one action now grants a full sliding session.

**Fix:** For SSO-method staff, fall back to the email-code challenge (the SSO
identity *is* an email) rather than `CreateVerifiedToken`; never return a
pre-verified token from `/api/auth/start`. Document that accounts used to
authorize must be password- or email-capable.

### P1-2. `onResize()` is called outside its defining scope → ReferenceError on every fullscreen exit, which also poisons crash telemetry

**`_play.gohtml:808, 823` call it; it's defined at `:673` inside the IIFE that closes at `:765`**

`onResize` lives inside the loader IIFE (673–765). `toggleFullscreen` (775) and
`onFullscreenChange` (814) are top-level and both call `onResize()` — a
`ReferenceError` at runtime.

**Failure scenario:** A student exits fullscreen on a Chromebook →
`onFullscreenChange` throws. Three consequences: (a) the queued
`setTimeout(focusCanvas, 150)` never runs, so **keyboard input (WASD) is dead
until the student clicks the canvas**; (b) the throw reaches `window.onerror` →
`reportCrash('runtime_error', …)`, which fires a bogus crash report **and sets
`crashReported = true` (`:590-591`), permanently suppressing every real crash
report for the rest of the session** — it poisons the very telemetry this page
was built to collect; (c) the canvas can stay mis-sized.

**Fix:** Hoist `onResize` (and canvas sizing) to the script's top level, or
expose it (`window.__mhsOnResize = onResize`) and call that. Add "enter+exit
fullscreen, then press W" to the smoke-test checklist.

### P1-3. Final-unit completion strands the student behind a permanent overlay

**`_play.gohtml:337-338` (show), `:367-378` (early return), `:494` (`showGameComplete` is dead code)**

`handleUnitComplete` reveals the "Unit Complete / Loading next unit…" overlay
*before* it knows whether the unit is final:

```js
var overlay = document.getElementById('unit-complete-overlay');
if (overlay) overlay.classList.remove('hidden');
```

For the final unit, `completeAndTransition` early-returns without ever hiding it
("Don't clean up Unity or show an overlay" — but it's *already shown*). The
overlay sits at `z-index:100` over the game's end screen; the back button is at
`z-index:20`, underneath and unreachable. `showGameComplete()` (the function
meant to give this state an exit) is **never called anywhere** (grep-confirmed).
The offline path `completeAndTransition(nextUnitId, !nextUnitId)` hits the same
dead end.

**Failure scenario:** A student finishing the last unit sits on a black screen
reading "Loading next unit…" forever; only closing the tab escapes.

**Fix:** In the `isFinal` branch, either `overlay.classList.add('hidden')` (new
builds — let the game's end screen show) or call `showGameComplete()` (old
builds).

### P1-4. Background Fetch broadcasts `'cached'` even when files failed to cache

**`sw-background-fetch.js:370-394`**

The record→cache copy loop swallows per-record errors and then broadcasts
success unconditionally:

```js
} catch (err) {
  console.error('Failed to cache record:', record.request.url, err);
}
...
broadcastStatus(unitId, 'cached', { version: version });   // even if cached < records.length
```

**Failure scenario:** Storage exhaustion (the documented #1 field failure) makes
`cache.put` throw `QuotaExceededError` on the large `.data` file. The catch
swallows it, the SW still broadcasts `'cached'`, the UI shows "Ready to play",
and the gap only surfaces later — worst case when the student is offline and the
cache-miss→CDN redirect can't save them. The same silent skip happens when the
`'/'+unitId+'/'` URL marker isn't found (`idx === -1`).

**Fix:** Count failures; if `cached !== records.length`, broadcast `'error'`
(with `version` and a quota-aware message), or run the real cache-status check
and broadcast its result.

---

## P2 — real but bounded

### P2-1. No rate limiting / lockout on keyword and password unlock attempts; keyword compare isn't constant-time

**`api_manifest.go:172-175` (keyword), `system/staffauth/staffauth.go:204-216` (password)**

`POST /missionhydrosci/api/manage/unlock` accepts unlimited keyword guesses with
no attempt counter, no delay, no failure logging, and a plain `!=` compare:

```go
case "keyword":
    if keyword == "" || keyword != settings.MHSMemberAuthKeyword {
        return http.StatusForbidden, "invalid keyword"
    }
```

The staff-auth `password` branch is likewise uncounted, while the `email` branch
*does* enforce `ErrTooManyAttempts` — an inconsistency. Classroom keywords are
short and memorable; the sliding unlock makes a successful guess more valuable
than before (it now covers the permanent deletes).

**Fix:** Per-session/per-user failure counter with lockout or backoff (reuse the
`emailverify` `ErrTooManyAttempts` pattern), log failures with attribution, and
use `subtle.ConstantTimeCompare` for the keyword. Also: `HandleKeywordVerify`
(`/api/auth/keyword`) is no longer referenced by any template — it's a second
unthrottled keyword oracle; remove it or fold it into the same limiter.

### P2-2. Unlock countdown can enter an infinite reload loop on a skewed device clock

**`_manage.gohtml:691-704`**

The countdown compares the server-issued expiry against the *client* clock:

```js
var remaining = unlockExpiresAtMs - Date.now();
if (remaining <= 0) { window.location.reload(); return; }
```

**Failure scenario:** A Chromebook whose clock runs fast by more than the unlock
duration computes `remaining <= 0` immediately, reloads, the server re-renders
the still-unlocked page with a fresh server-time expiry, and the skewed client
reloads again — a 1-second reload loop that makes the tools unusable. Wrong
device clocks are squarely in this feature's field population (see the ACER
report in the redesign plan).

**Fix:** Emit a server "now" alongside the expiry and count down against
elapsed-since-render, or on `remaining <= 0` call the existing
`/api/manage/status` and only reload if the server confirms the unlock is gone.

### P2-3. Shared service bearer tokens are embedded in the play page for every student

**`play.go:102,107`, `types.go:55,60`, `_play.gohtml:280-285`**
*(independently flagged by three reviewers)*

The stratalog/stratasave bearer tokens are static, shared by all users, and
rendered verbatim into page source:

```js
log_submit:    { url: '{{ .LogSubmitURL }}',    auth: '{{ .LogAuth }}' },
state_save:    { url: '{{ .StateSaveURL }}',    auth: '{{ .SaveAuth }}' },
```

**Failure scenario:** Any student can view-source, take the token, and POST logs
or saves **as any `user_id`** directly against stratalog/stratasave — deleting
or forging another player's saved state — bypassing every server-side MHS gate.

This may be an accepted trade-off of the offline game-bridge design (the game
needs to call the services directly, offline-capably). If so, it should be
documented as an accepted risk; if not, mint per-user, short-lived save/log
tokens at page render, or proxy those calls through stratahub where the session
identity is enforced.

### P2-4. `pruneStaleCaches` / prune paths can wipe or duplicate downloads via several narrower races

Beyond P0-1, the client has a cluster of prune/cancel/reconnect races worth
fixing together:

- **`_recheckActiveDownloads` clears tracking on `result==='success'` with no
  copy-grace (`mhs-delivery.js:1040-1046`).** A BG fetch that completes while the
  tab is hidden is still copying records when the user returns; the visibility
  handler clears tracking → `'partial'` fires → self-heal starts a *duplicate*
  download that collides with the in-flight copy and falls to `fallbackFetch`,
  re-downloading hundreds of MB. The poller path already has a two-grace
  reconcile for exactly this; the visibility path doesn't.
- **No SW-side dedupe between the two download paths
  (`sw-background-fetch.js:74-116` vs `:228-249`).** `startBackgroundFetch` only
  checks `backgroundFetch.get()`; `fallbackFetch` only checks `fallbackRuns`.
  After an SW-internal switch to fallback plus a slow `getActiveFallbacks`
  round-trip (>3s timeout → `[]`), the page can start a real BG fetch while the
  SW's own fallback loop is mid-unit — two full downloads competing on one flaky
  link. The SW is the only context shared across pages/reloads and should own
  this dedupe.
- **Play overlay's `autoCleanup([nextUnit])` deletes manual downloads
  (`_play.gohtml:394,458`).** The units and manage pages keep
  `current+next+manualDownloads`; the overlay keeps only `next`, so completing a
  unit wipes any unit a student/tester pre-downloaded.

**Fix:** Give the success branch of `_recheckActiveDownloads` the same grace as
the poller; make `startBackgroundFetch` consult `fallbackRuns` (adopt/return);
build the overlay keep-list from `[nextUnit]` plus the parsed
`mhs-manual-downloads` key.

### P2-5. `downloadUnit` / `retryDownload` early-returns fire no status → callers strand

**`mhs-delivery.js:1078-1088` and `:843-846`**

Both bail silently when the manifest or unit is missing (`if (!this.manifest)
return; … if (!unit) return;`). Combined with P0-1's manifest clobber, the play
overlay's `downloadUnit(nextUnit)` no-ops and the overlay says "Downloading next
unit…" forever with Unity already torn down (the MHS-017 dead-end class,
re-opened via a new entry point). On the units page, `mhsDownload` disables
Retry *before* calling `retryDownload`, which then fires nothing and leaves the
button stuck disabled on "Stalled."

**Fix:** Fire `_fireStatus(unitId, 'error', …)` from every early return (it also
clears tracking), and add a timeout guard around the play overlay's
`mgr.init().then(downloadUnit)`.

### P2-6. Missing context timeouts and error-handler inconsistency on the MHS DB handlers

**`progress.go`, `device_status.go`, `units.go`, `play.go`, `manage.go`**

Most MHS handlers use bare `r.Context()` (no timeout), unlike the two collection
handlers that already wrap a 30s `context.WithTimeout`. `device_status` is
POSTed frequently by every device, so a stalled DocumentDB primary during
failover can pin goroutines for as long as clients hold connections. Separately,
these handlers use `http.Error` + `h.Log` rather than the `h.ErrLog`
(`ErrorLogger`) the rest of the codebase uses — so a DB error on `ServeUnits` /
`ServePlay` / `ServeManage` returns a bare plaintext 500 instead of the branded
error page, and doesn't flow through the central logger. The `Handler` even
carries an unused `ErrLog` field.

**Fix:** Apply a consistent `context.WithTimeout` across the MHS DB handlers, and
route the HTML page handlers through `h.ErrLog`.

### P2-7. `ServeUnits` / `ServeManage` swallow manifest and progress errors → misleading state

**`units.go:16-35`, `manage.go:83`**

`manifest, _ := h.resolveManifest(r)` discards the resolution flag, and
`ServeUnits` ignores the `GetOrCreate` error (`if err == nil { … }`), so a
transient DB error renders the page as if the student is a brand-new user on
unit 1.

**Failure scenario:** A student who is really on unit 3 sees unit 1 as "current"
and their completed units as "future," with no error surfaced — and could be
gated out of their actual unit or re-download from scratch. On the manage page
the same swallow renders an empty units table and burns the member's unlock time
on a broken page.

**Fix:** Distinguish "no record" from "DB error"; on error, render the error
page (and log it) rather than a misleading default.

### P2-8. Needless `response.clone()` in the SW cache-copy loop risks OOM on low-end devices

**`sw-background-fetch.js:385` (and similar at `mhs-delivery.js:443`)**

`await cache.put(cacheKey, response.clone());` — the original `response` is never
read again. `clone()` tees the body; as `cache.put` drains one branch, the
unread branch buffers the full body with no backpressure — a multi-hundred-MB
spike per record, in the SW process the OS kills first, on exactly the low-end
Chromebooks this targets.

**Fix:** `cache.put(cacheKey, response)` directly.

### P2-9. `GetMHSCollectionForUser` uses a correlated `$lookup` sub-pipeline (DocumentDB risk), and swallows errors

**`store/groupapps/groupappstore.go:120-149`**

This runs on every manifest resolution and uses a `$lookup` with an inline
`let`/`$expr` sub-pipeline — on the same restricted feature set as
`$graphLookup`/`$facet` and unreliable on older DocumentDB engines. The error is
swallowed (`return nil`), so on an unsupporting cluster every group pin silently
falls through to the workspace-active collection with no log. The sibling
`EnabledAppIDsForUser` just above uses the safe `localField/foreignField` form.

**Fix:** Rewrite as two steps (member group_ids, then `Find` with `$in`) or the
equal-field `$lookup` form, and log the error instead of returning nil.

---

## P3 — hardening, hygiene, and polish

**Correctness / consistency**

- **`unit1..unit5` is hard-coded in `progress.go:88`** while collections are
  data-driven and the curriculum has **6 units**; `CompleteUnit` advances
  dynamically to `totalUnits`. The manage table renders a `Set current` button
  for every manifest unit, so a `unit6` row always fails with 400 "invalid unit"
  after the confirm dialog. Derive valid units from the resolved manifest.
  *(found by two reviewers)*
- **`_pollDownloadOnce` progress floor can overcount / exceed 100%
  (`mhs-delivery.js:762-770`):** first-match `indexOf` on undecoded URLs
  double-counts when one file path is a substring of another's URL, and percent
  is uncapped. Match on decoded pathname-suffix equality and clamp.
- **`quota === 0` → `NaN%` (`units.gohtml:738-741`)** and skips low-storage
  logic; guard `if (!estimate || !estimate.quota) return;`.
- **`crypto.randomUUID()` with no fallback (`units.gohtml:251`)** throws on old
  Chrome (<92, plausible on aged Chromebooks), silently killing the device
  report. Add a `Math.random` fallback.
- **Initial device report can be incomplete (`units.gohtml:353-360`):**
  `reportedCount` increments per *event*, not per unique unit, so rapid
  `downloading` events for one unit can trip the send early with units missing.
  Count `Object.keys(initialReport).length`.
- **`_reconnectActiveDownloads` (`mhs-delivery.js:956`):** a single throwing
  `bgFetch.abort()` abandons the remaining reconnections (only the outer catch
  exists). Use per-iteration try/catch like `_abortAllBGFetches`.
- **`ServeUnits`/`ServeManage` resolve the collection twice per render**
  (`resolveManifest` + `resolveEffectiveCollectionInfo` both call
  `resolveCollection`), doubling the group-pin aggregation and progress upsert on
  the hottest pages. Resolve once and share.

**Auth / audit / data hardening**

- **Plan deviation — gated actions aren't logged with attribution**
  (`api_manifest.go:148-159`): the redesign plan promises "authorized by
  <staff>" logging during the unlock window; only grant/revoke are logged, and
  the unlock-pass path discards `GrantedBy`. A disputed deletion can't be traced
  to the action that ran under the unlock.
- **Token consumption doesn't verify workspace binding**
  (`system/staffauth/store.go:173-186`): `ValidateAndConsumeToken` matches only
  `{token, verified, unused, unexpired}`; add `ch.WorkspaceID == wsID` as a cheap
  invariant.
- **`ensureMHSDevSentinel` seeds unconditionally into the default/prod workspace
  (`bootstrap/startup.go:242-305`):** a `member`/`trust`/`active` user with the
  published login ID `mhs-developer-sentinel`. If `trust` login is reachable in
  that workspace, anyone can sign in as it. Gate seeding to non-prod, or make the
  sentinel non-loginable.
- **`HandleDeviceStatus` has no input bounds (`device_status.go:16-25`):**
  arbitrary `device_details` map and unbounded distinct `device_id`s per user →
  unbounded document growth. Cap lengths/map size.

**localStorage / lifecycle hygiene**

- **"Reset all MHS data" doesn't clear `mhs-prefer-fallback-until`**
  (`_manage.gohtml:779` purge list vs `mhs-delivery.js:506`): a QA tester who
  resets to test the Background Fetch path stays silently pinned to the fallback
  path for 24h.
- **Adopted fallback loops never start `_startProgressKeepalive`**
  (`mhs-delivery.js:973-991`): unlike every other fallback start site, so a
  reload-adopted loop loses the keepalive that prevents SW idle-termination.
- **Play heartbeat is permanently stopped on `pagehide`
  (`mhs-delivery.js:346-351`):** a bfcache-restored game on iOS has no
  heartbeat, re-opening the MHS-013 prune-under-live-game window after the 30-min
  TTL. Don't stop when `event.persisted`; restart on `pageshow`.
- **`deleteUnit`'s `caches.delete` is unguarded (`mhs-delivery.js:1200`)** and
  the overlay's `autoCleanup(...).then(navigate)` chains have no `.catch`
  (`_play.gohtml:394,458`) — a storage-layer rejection leaves the student on
  "Ready!" forever.

**Accessibility & touch (grades 6–9 on tablets)**

- Modals across all three pages lack `role="dialog"`/`aria-modal`, focus
  trapping, and focus-move-on-open (units help `units.gohtml:40-51`; play
  overlays `_play.gohtml:180,247`; manage picker `_manage.gohtml:415-426`).
- No `aria-live` on the primary status lines (`#unit-status`, `#progress-text`,
  `#sw-status`, play loading text) — download/launch state changes are silent to
  screen readers.
- Touch targets below 44px: units `?` (24px) and install-dismiss `×`; play
  fullscreen (32px) / back (36px).
- Low contrast on state-bearing text (`text-[10px] text-gray-400` dot labels
  ≈2.8:1; several `text-xs text-gray-400` status lines). Bump to gray-500 /
  dark:gray-400.
- iPad can't open the play help overlay — it's `?`-key-only (`_play.gohtml:966`),
  yet the "Press ? for help" hint shows on touch-only devices where it's a lie.

**UX dead-ends & copy**

- Loader failure in browser-tab mode offers no way out (`_play.gohtml:758`): the
  back button is `display:none` outside standalone PWA, so a failed load is a
  black screen with a question and no answer. Show a visible back link
  regardless of display mode, with student-appropriate copy.
- Unresolvable manifest / out-of-manifest `currentUnit` renders a blank hero with
  no message (`units.go:17` + `units.gohtml:83-121`) — add an `{{ else }}`
  fallback ("Your unit isn't available in the current game version — ask your
  teacher").
- Over-broad completion interception (`_play.gohtml:303-330`): `shouldIntercept`
  treats *any* navigation to `*adroit.games*`/`*cloudfront.net*` as unit
  completion, so a build that ever opens a legit external link via
  `Application.OpenURL` would silently mark the unit complete.

**Dead / drifting code**

- ~80 lines of dead per-unit button plumbing remain on the launcher post-redesign
  (`download-btn-<id>`/`clear-btn-<id>` lookups, `updateDownloadButtonVisibility`
  — `units.gohtml:381-429,600-632`); all `if (btn)` guards hold so it's harmless,
  but the "hide Download when it won't fit" protection now protects nothing.
- SW `deleteUnit`/`checkStatus`/`getVersion` message actions (`sw.js:220-236`)
  have **no sender** — either keep for the update-window compat rule or remove
  the ~80 lines of hand-maintained duplication of `_checkUnitCache`.
- Unused view-model fields post-redesign (`types.go:20-35`: `MHSMemberAuth`,
  `CDNBaseURL`, `CompletedUnits`, `UserIDHex`, etc.; `PlayData.NextUnitVersion`).
- Play page pokes private fields after construction (`_play.gohtml:482-484`:
  `mgr._swUrl`, `_channelName`) though the constructor accepts them as opts — one
  refactor away from listening on the wrong BroadcastChannel. The
  `mhs-delivery.js` defaults are wrong for every real deployment; only the
  templates' overrides save it.
- `sw.go:17-18` comment says `/missionhydrosci-sw.js`; it's mounted at `/sw.js`.

---

## Test-coverage gaps

The manage/unlock tests cover the keyword lifecycle well, but not the two
central claims of the redesign:

- No test drives `ServeManage` and asserts the gate response **omits page
  content** for a locked member and includes it once unlocked (the "content is
  never sent" guarantee).
- No test exercises `checkMemberAuth` in `staffauth` mode (token consumed
  single-use, `GrantedBy` set, reused token rejected).
- `TestUnlockIsSessionScoped_DifferentUserBlocked` actually tests *user*
  scoping — `testutil.TestUser` has no session-token field, so handler tests all
  derive keys from session token `""`. Give `WithUser` an optional session token
  to cover same-user / different-session scoping directly.
- No JS/SW test reproduces P0-1 (units page with the manifest endpoint blocked)
  or P1-4 (partial cache → false `'cached'`). Both are cheap to add and guard the
  two highest-impact bugs.

---

## Design assessment

**What's genuinely strong** — and should be preserved through any refactor:

- The **staff-unlock store** (`system/staffauth/unlock.go`) is well designed:
  key = SHA-256(ws|user|sessionToken) so it's session-scoped and dies at logout;
  hashed at rest; TTL index *plus* a query-side `expires_at > now` so it doesn't
  rely on Mongo's lazy TTL sweep; `Refresh` can't resurrect an expired record;
  `Grant` upserts atomically; tokens are consumed via atomic `FindOneAndUpdate`.
- **Fail-closed defaults** throughout the settings path (load error → `staffauth`;
  keyword mode can't be saved without a keyword), **CSRF** on all mutating routes
  with matching `X-CSRF-Token` headers, and **correct contextual autoescaping**
  everywhere Go values enter `<script>` (no XSS paths found in any template).
- The client **download state machine** is mostly coherent: monotonic/sticky
  stall ownership, version-tagged broadcast filtering with untagged pass-through
  for the compat rule, the `{promise, aborter}` fallback dedupe, the
  success-reconcile double-grace, the zero-byte→fallback switch, and the
  exact-hash versioned-asset strategy are all thoughtful and correctly
  implemented. Every page-sent SW action is handled and every broadcast status is
  consumed — the compat rule holds.
- `mhsdevicestatus.Upsert` handles the concurrent-first-insert race, and
  `CompleteUnit` is properly idempotent.

**Where the structure invites the bugs above:**

1. **Terminal-status side effects live in `_fireStatus`, but several paths mutate
   tracking or return without any status** (P0-1's abort, P1-3/P2-5's stranding).
   A single `_finishDownload(unitId, status, detail)` owner would close that
   whole class of hole.
2. **Dedupe is page-scoped but the system is multi-page** (units + manage + play
   overlay + bootstrap early-download all drive one SW). BG-fetch dedupe works by
   luck because the registration is shared; fallback-vs-BG dedupe (P2-4) does
   not. The SW is the only shared authority and should own dedupe.
3. **Duplication that will drift:** the cache-check/verify logic exists in three
   near-parallel shapes (page, SW, dead SW message path); the auto-download
   pipeline + `onStatus` switch are copy-pasted across `units.gohtml` and
   `manage.gohtml` (the deferred MHS-020 status→UI-table refactor would absorb
   both); the play overlay builds its own half-configured manager instead of
   using a factory with the canonical config.
4. **Manifest-load failure is treated as "empty game" rather than "unknown
   state,"** which is the root of P0-1 and its secondary damage. An explicit
   "manifest not loaded" state that suppresses destructive operations (prune,
   reconnect-abort) is the structural fix.

---

## Suggested fix order

1. **P0-1** (manifest-failure prune wipe) — small change, catastrophic blast
   radius, trivially testable.
2. **P1-1** (staff-auth SSO bypass) — security; defeats the gate the redesign
   was built around.
3. **P1-2, P1-3, P1-4** (fullscreen crash-telemetry poison; final-unit dead-end;
   false `'cached'`) — each is a localized fix with clear user impact.
4. **P2-1, P2-2, P2-3** (brute-force; clock-skew reload loop; shared tokens
   decision) before broad rollout.
5. The remaining P2s (races, timeouts, error surfacing), then the P3 batch and
   the test-coverage additions, then the structural refactors (single
   `_finishDownload`, SW-owned dedupe, the status→UI-table dedupe).
