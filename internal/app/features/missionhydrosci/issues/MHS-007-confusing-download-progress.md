# MHS-007 — Confusing 0%→100% progress; `downloadTotal: 0` + per-file granularity

**Priority:** P2
**Status:** Implemented (2026-07-05)

## Summary

Two independent things make download progress look broken: the browser's own
download indicator is indeterminate because Background Fetch is started with
`downloadTotal: 0`; and the in-app percentage on the fallback path only advances
at **file boundaries**, so it sits at 0% through the one dominant `.data.unityweb`
file and then jumps to ~100%.

## Evidence / symptoms

- Observed directly: "download percentage at 0% but the download icon on the
  browser looks like something is downloading and then eventually it jumps to
  100%."

## Root cause

1. **Browser indicator indeterminate:** `backgroundFetch.fetch()` is called with
   `downloadTotal: 0`, which tells the browser the total size is unknown, so its
   notification/progress UI is indeterminate:
   - `assets/js/mhs-delivery.js:461`
   - `static/sw-background-fetch.js:42`
   (Our own percentage is computed from `unit.totalSize`, so this only affects the
   browser's native UI — but that is what the tester sees "spinning".)

2. **In-app bar jumps on the fallback path:** `fallbackFetch` broadcasts progress
   only **after each file completes**. A unit is dominated by one large file, so
   the bar reports 0% during that download then jumps near 100%:
   - `static/sw-background-fetch.js:76-110`
   (The Background Fetch path reports byte-level progress and is smoother:
   `assets/js/mhs-delivery.js:256-272`.)

## Affected code

- `internal/app/resources/assets/js/mhs-delivery.js`
- `internal/app/features/missionhydrosci/static/sw-background-fetch.js`

## Proposed fix

- Pass the real total to Background Fetch: `downloadTotal: totalSize` (we already
  compute `totalSize`), so the browser's native indicator is accurate.
- In `fallbackFetch`, stream the response with a `ReadableStream` reader and
  report intra-file byte progress for the large file(s), so the in-app bar moves
  smoothly instead of jumping. Accumulate `response.body` chunks, tracking bytes,
  then `cache.put` the assembled response.

## Risk / notes

- **Scope split with MHS-011:** this issue covers the *reporting* deficiencies
  (`downloadTotal: 0`, per-file granularity on the fallback path). The frozen
  live-progress stream on the Background Fetch path — and the stall detector
  aborting healthy downloads because of it — is
  [MHS-011](MHS-011-frozen-progress-false-stall-abort.md). Fix both together;
  same file, same functions.
- Cosmetic/UX; does not by itself fix failures, but removes a major source of
  "is it stuck?" confusion and false stall reports.
- Streaming reassembly must preserve headers/status so cached responses still
  serve correctly to the Unity loader.
</content>
