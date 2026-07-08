# MHS-004 — Guest/Incognito disables Background Fetch → fragile sequential path

**Priority:** P1
**Status:** Documented (Background-Fetch-in-guest behavior should be verified)

## Summary

Chrome does not expose the Background Fetch API in Guest and Incognito profiles.
On those profiles `reg.backgroundFetch` is undefined, so downloads fall back to
the SW sequential `fallbackFetch`, which is far less robust on unstable networks
and interacts badly with the small, ephemeral guest quota. This is the leading
explanation for "guest profile fails, real profile works."

## Evidence / symptoms

- "Real profile on the chromebook can download all 5 units without issue, seems
  to be related specifically to the guest profile."
- Guest/Chromebook downloads fail; DevTools "move to unit" workaround used.
- University wifi (unstable) made it worse — consistent with a sequential fetch
  that aborts the whole unit on a single failed file.

## Root cause

The delivery manager only uses Background Fetch when `reg.backgroundFetch` is
present, otherwise it posts a `fallbackDownload` message to the SW:

- `assets/js/mhs-delivery.js:435-485` (`downloadUnit` — BG-fetch-or-fallback)
- `static/sw-background-fetch.js:68-119` (`fallbackFetch` — sequential, no
  per-file retry, no partial cleanup)

`fallbackFetch` weaknesses on constrained/guest devices:
- One failed file throws and aborts the entire unit (no retry).
- No partial-cache cleanup on failure (see MHS-003).
- Runs against a small, ephemeral guest quota that is also non-persistable
  (see MHS-005).

> Note: that Background Fetch is unavailable in guest mode is documented Chrome
> behavior (guest/Incognito are off-the-record, ephemeral profiles; Background
> Fetch requires persistent registrations that outlive the session and shows
> profile-tied OS notifications). The **conclusion** — guest mode loses Background
> Fetch and drops to the fragile fallback — is well-supported. The **exact
> failure mode** (`reg.backgroundFetch` is `undefined` vs. `.fetch()` rejecting)
> has not been empirically confirmed on the actual Chromebook builds; the code
> falls back correctly either way. The device-status telemetry (MHS-008) can
> record whether `backgroundFetch` was available per session to confirm with
> data. The same ephemerality also causes `storage.persist()` to be denied in
> guest mode (see MHS-005).

## Affected code

- `internal/app/resources/assets/js/mhs-delivery.js`
- `internal/app/features/missionhydrosci/static/sw-background-fetch.js`

## Proposed fix

Harden the fallback path so it can succeed where Background Fetch is unavailable:
- Retry each file a small number of times with backoff before failing the unit
  (see MHS-006).
- Purge the partial cache on terminal failure (MHS-003).
- Combine with per-unit space pre-flight (MHS-006) and `persist()` (MHS-005) so
  constrained profiles fail gracefully with a clear message instead of silently.

Also record, via device-status telemetry, whether Background Fetch was available
and which path was used, so we can confirm the guest-mode hypothesis with data.

## Risk / notes

- We cannot make Background Fetch appear in guest mode; the goal is to make the
  fallback path reliable enough that guest profiles work anyway for the unit
  sizes involved.
- For QA specifically, recommend testing on **real (non-guest) profiles** with an
  explicit in-app reset (MHS-002) between runs, rather than relying on guest-mode
  auto-clear.
</content>
