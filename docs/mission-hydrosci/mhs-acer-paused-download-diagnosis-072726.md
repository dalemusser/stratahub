# MHS — ACER "12% then stalled" paused-download diagnosis

**Date:** 2026-07-27
**Device:** ACER Chromebook with a persistent ChromeOS "Profile error occurred"
(survived account deletion). Tester: Kyle Cutler. QA account
`adroitqateam@gmail.com`.
**Status:** RESOLVED. Diagnosed, then all three fixes + a download-mode indicator
implemented and deployed; a first ACER retest exposed one remaining gap (the
auto-switch still required page control), fixed in a follow-up. **Kyle confirmed
on 2026-07-28: "The fallback on the manage page seems to be working correctly."**
Committed (HEAD `9406908` at time of writing) and pushed. Background Fetch remains
the default path for healthy devices; the changes only affect devices that are
actually failing. See "Implementation" and "Follow-up after ACER retest" below.

## The symptom

Every download on this device: **pauses immediately, jumps to ~12%, then
stalls there.** The ChromeOS "Paused Downloading…" notification appears. Retry
sometimes downloads instantly and cleanly; sometimes it pauses and re-stalls at
12%. Switching to a different collection sometimes downloads smoothly, but
hitting **Reset all MHS Data** immediately puts it back to the 12% stall.
Clearing the CloudFront cache and hard-refreshing (Ctrl+Shift+R) did not help.

## The headline

**The SW sequential fallback (regular `fetch()`, immune to Chrome's
download-service pausing) works correctly on this device** — every time the code
actually reached it, the download "went smoothly." The problem is **not** that
the fallback is broken; it is that **three defects keep routing the download
back onto the pausable Background Fetch path instead of the fallback.** Fix the
routing and this device works with no Powerwash.

The pausing itself is Chrome behaviour (metered connection / battery saver /
corrupt download-service DB in the bad profile). We can't stop Chrome from
pausing Background Fetches — the whole point of the fallback is to *not use*
Background Fetch on such devices. It just isn't engaging.

## Root causes (all confirmed in code)

### A. The auto-switch to fallback only fires on **zero** bytes — but this device delivers ~12% before pausing

`internal/app/resources/assets/js/mhs-delivery.js:808`

```js
if (downloaded === 0 && fresh.result === '' &&
    Date.now() - state.startedAt > FIRST_BYTE_TIMEOUT_MS &&
    navigator.serviceWorker && navigator.serviceWorker.controller) {
  ... this._preferFallback(true); await fresh.abort(); this._startFallbackDownload(...)
}
```

The "no bytes in 90s → switch to fallback" mitigation (added for the earlier
ACER report) assumed Chrome pausing looks like **nothing ever downloads**. The
actual field failure is **a little downloads, then Chrome pauses indefinitely**:
the download service lets ~12% (one or more small files) through, then freezes.
Because `downloaded === 0` is false (it's 12%), the auto-switch **never fires**.
The download then sits at 12% until the 2.5-minute stall watchdog
(`STALL_THRESHOLD_MS = 150000`) surfaces "Stalled" and a manual **Retry** — so
the "automatic" fallback is not automatic for the real failure mode; it requires
a human to hit Retry every single time.

*(The "jump to 12%" itself is the poller's completed-records floor at
`:780-796`: it sums the sizes of Background-Fetch files that finished before the
pause. Working as intended; it's just reporting a frozen download.)*

### B. "Reset all MHS Data" wipes the prefer-fallback pin — re-breaking the device on every reset

`internal/app/features/missionhydrosci/templates/missionhydrosci_manage.gohtml:797`

```js
manager.purgeAllMHSData([MANUAL_DL_KEY, 'mhs-progress-queue',
  'missionhydrosci-progress-changed', MHSDeliveryManager.ACTIVE_PLAY_KEY,
  MHSDeliveryManager.PREFER_FALLBACK_KEY]).then(...)
```

When a Retry finally uses the fallback, it sets `mhs-prefer-fallback-until`
(24 h) so subsequent downloads skip Background Fetch entirely
(`downloadUnit` at `mhs-delivery.js:1182` gates the fallback on that pin). **But
Reset all MHS Data purges that key** — so the next download reverts to the
pausable Background Fetch and stalls at 12% again.

This is a regression I introduced in the telemetry batch (2026-07-22). The
purge was added as a *tester convenience* ("reset to re-test the Background
Fetch path"), but in the field it directly causes the reported failure: the pin
is a **device-capability hint, not user data**, and the tester's workflow is
Reset-heavy.

**The tester unknowingly ran the exact experiment that isolates this:** switch
collection → downloads **smoothly** (pin still set) → hit Reset all MHS Data →
**stalls at 12%** (pin wiped). The only thing that changed was the Reset, and its
only relevant side effect is deleting the pin. This is the single clearest
reason the earlier "fix doesn't appear to work" — it *does* work, and then Reset
immediately un-works it.

### C. The fallback path requires the SW to be **controlling** the page — a hard refresh strips that

`internal/app/resources/assets/js/mhs-delivery.js:559`

```js
MHSDeliveryManager.prototype._startFallbackDownload = function(unitId, unit) {
  if (!navigator.serviceWorker || !navigator.serviceWorker.controller) return false;
  ...
```

`_startFallbackDownload` needs `navigator.serviceWorker.controller`. Right after
a hard refresh (Ctrl+Shift+R — which Dale suggested, and the tester used), the
page has an active SW *registration* but is **not yet controlled** (the
`controllerchange` hasn't landed). So even with the pin set, the fallback gate
at `:1182` short-circuits to false, and `downloadUnit` falls through to the
Background Fetch branch at `:1194` (which only needs `reg.active`, not
`controller`). Result: pin set, fallback preferred, but the code still starts a
pausable Background Fetch → pauses → 12%.

This is why the stall persisted "after hard refresh" and why **Retry is
inconsistent**: `retryDownload` at `:912` also gates the forced-fallback on
`navigator.serviceWorker.controller`, and when it's null it falls through to
`downloadUnit` → Background Fetch. Retry works when the page is controlled,
re-stalls when it isn't (fresh after a refresh).

## How the three defects reproduce the tester's session exactly

| Tester action | What the code did | Observed |
|---|---|---|
| Set U1 current → Reset → Clear (initial DL) | Reset wiped pin → BG fetch → Chrome paused after 12% → zero-byte switch can't fire (12≠0) → stall watchdog @2.5 min | "immediate pause, jump to 12%, stall" |
| Retry (worked) | controller present → forced fallback + pin set → regular fetch, no pausing | "downloaded quickly, then next chapter too" |
| Ctrl+Shift+R, download | controller null after hard refresh → fallback gate false despite pin → BG fetch → pause | "same… pause then 12%" |
| Retry (stalled) | controller still null → retry falls through to BG fetch | "paused and 12%ed on retry" |
| Switch collection (smooth) | controller now settled + pin set → fallback | "downloaded U1 then U2 smoothly" |
| **Reset all MHS Data** (stalled) | **Reset wiped pin** → BG fetch → pause | "after Reset, stalled at 12%" |
| CloudFront clear + refresh + Reset | irrelevant clear; refresh nulls controller; Reset wipes pin → BG fetch everywhere | "all stall at 12%" |

Every observation is accounted for. Nothing here requires the CDN or the CloudFront
cache to be at fault (they weren't — the fallback pulls the same files fine).

## Why this matters beyond one broken Chromebook

The corrupt profile makes Chrome pause **every** fetch, so this device is a
*magnifier*, not an outlier. The same pausing triggers on **healthy** devices
under metered connections or battery saver — common in classrooms. Any such
device hits the identical "12% then stuck, Reset re-breaks it" trap. Fixing
A/B/C is defense-in-depth for the whole low-end fleet, independent of whether
this particular ACER later gets Powerwashed.

## Recommended fixes (in priority order)

1. **B — stop Reset from wiping the pin (one line, do first).** Remove
   `MHSDeliveryManager.PREFER_FALLBACK_KEY` from the Reset purge list
   (`manage.gohtml:797`). It's a device hint, not user data. If testers need to
   re-exercise the Background Fetch path, add a separate explicit control or
   rely on the 24 h TTL. This alone stops the device from re-breaking itself and
   is almost certainly why the prior fix looked ineffective.

2. **A — broaden the auto-switch from "zero bytes" to "frozen download".**
   Switch to fallback when a Background Fetch's progress hasn't advanced for the
   timeout window **regardless of whether it's at 0% or 12%**, while
   `fresh.result === ''`. The stall state already tracks `maxDownloaded` /
   `lastProgressAt`, so the change is small: trigger on
   `Date.now() - state.lastProgressAt > FIRST_BYTE_TIMEOUT_MS` (a paused fetch),
   not only `downloaded === 0`. This makes the fallback *automatic* for the real
   failure mode instead of requiring a manual Retry. (The ~12% already pulled by
   the download service isn't in our cache yet — Background Fetch caches
   atomically on success — so aborting loses only cheaply re-fetchable bytes; the
   "nothing is lost" comment should be updated accordingly.) Guard against
   aborting a genuinely slow-but-progressing download by keeping the timeout
   generous and requiring truly zero *new* bytes across the window.

3. **C — don't require live SW control to choose the fallback.** When the pin is
   set (or a stall is detected) but `navigator.serviceWorker.controller` is null,
   wait for control (`navigator.serviceWorker.ready` / a `controllerchange`)
   before starting the download instead of silently falling through to
   Background Fetch. Removes the hard-refresh inconsistency and the flaky Retry.

**Risk note:** #2 is the one with regression surface — over-eager switching could
abort a slow-but-healthy Background Fetch on a good-but-slow network. Keep the
window conservative and base it on zero *new* bytes (not absolute rate). #1 and
#3 are low-risk.

**Bigger question for Dale (not urgent):** given how much grief Background Fetch
causes on the target fleet, is it worth keeping it as the *default* path, or
should the fallback be the default with Background Fetch as the opt-in
optimization? That's an architecture decision, not a bug fix — flagging it, not
recommending it here.

## Implementation (2026-07-27, deployed to dev, smoke-tested)

All in `mhs-delivery.js` + the two templates; **no service worker logic changed**
(the new delivery JS propagates via its content-hash in the SW precache), and
**Background Fetch stays the default** — the escalation/messaging only affect a
device once it actually experiences pausing.

- **B — Reset no longer purges the pin.** Removed `PREFER_FALLBACK_KEY` from the
  Reset all MHS Data purge list (`manage.gohtml`). The pin is a device-capability
  hint, not user data; it self-expires after 24 h. *Verified on dev: the manage
  purge list no longer references the key.*
- **A — auto-switch on frozen, not just zero, progress.** The poller now
  escalates to fallback when a Background Fetch has **no new bytes for 90 s while
  unfinished** (`downloaded <= maxDownloaded` this tick + aged `lastProgressAt`),
  which catches "paused at 12%", not only "never started". A healthy slow
  download keeps delivering bytes, so it is never touched. Makes the switch
  automatic — no manual Retry needed.
- **C — fallback no longer needs page *control*.** `_startFallbackDownload` is now
  async and messages an **active** worker (via `_waitForSW`) rather than requiring
  `navigator.serviceWorker.controller`, which a hard refresh strips. All call
  sites updated to await. *Verified on dev: `_startFallbackDownload` is an
  AsyncFunction; default mode is `background`, pin flips it to `fallback`.*
- **Download-mode indicator.** New `getDownloadMode()` / `hasActiveDownload()` /
  `DOWNLOAD_MODE_MESSAGES` on the manager; a notice on the launcher and manage
  page shows, while a download is active, either the blue *"Downloading in the
  background — you can switch tabs… ready when you come back"* (default) or the
  amber *"Keep this tab open… downloads pause if the tab is closed"* (fallback).
  *Verified on dev: blue notice on the default path, amber when pinned to
  fallback; 0 console errors.*

What could **not** be verified on dev: the actual ChromeOS pausing — that needs
the ACER. The mechanisms are confirmed wired and correct; the field behavior
needs the device.

### Follow-up after ACER retest (2026-07-28)

Kyle's retest confirmed B and C work (Reset keeps the fallback; hard refresh
keeps the fallback; Retry → fallback), but the **automatic** switch (A) did
**not** fire when the download started on the manage page after a hard refresh /
site-data clear — it only switched when he navigated to the units page or hit
Retry. Root cause: **I removed the `navigator.serviceWorker.controller`
requirement in three places for Fix C but left it on the auto-switch condition
itself.** A page loaded uncontrolled (hard refresh, or first load after clearing
site data — the ACER's state) has `controller === null`, so the auto-switch
couldn't fire there; navigating to units (a controlled in-app load) could.
Classic "the fix left one gap."

Fixes (all `mhs-delivery.js`, deployed 2026-07-28):
- **Removed the controller requirement from the auto-switch** and reordered it to
  start the fallback FIRST (via `_startFallbackDownload`, which needs only an
  active worker) and abort the paused Background Fetch only once the fallback has
  taken over — never stranding the unit.
- **Added a `document.visibilityState === 'visible'` guard** to the auto-switch.
  This is a *correctness* guard, not cosmetic: the broadened "no new bytes for N
  seconds" detection (unlike the old zero-byte-only check) could otherwise
  misfire on a healthy Background Fetch pre-downloading with the tab HIDDEN (the
  pre-class case), where the SW can go quiet between keepalives and look
  "frozen". Switching only while the page is in the foreground both prevents
  that false positive and matches intent — the page-open fallback is only useful
  when the page is open.
- **Lowered the window 90s → 60s** (`FROZEN_SWITCH_MS`) so the auto-switch is
  more responsive for a user watching a stuck download.

## Test recipe once a build is ready

On the ACER (profile error still present), without Powerwash:
1. Reset all MHS Data, then start a download → should **auto-switch to fallback**
   and complete **without a manual Retry** (validates A + C).
2. After it completes, hit **Reset all MHS Data** again and re-download → should
   **stay on fallback** and complete smoothly (validates B — the pin survives
   Reset).
3. Hard-refresh (Ctrl+Shift+R), then download → should still use fallback
   (validates C).
