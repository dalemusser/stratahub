# MHS-017 — Play overlay dead ends: 'not_cached' unhandled, 'stalled' has no exit

**Priority:** P1
**Status:** **Fixed 2026-07-06.** 17a: 'not_cached'/'partial' now redirect
like 'error' ("Download interrupted. Redirecting…"), gated on a
`downloadStarted` flag — `mgr.init()`'s checkAllCacheStatus fires
'not_cached' for the next unit BEFORE the download begins, and redirecting
on that would skip the download entirely. 17b: 'stalled' arms a one-shot 30s
timer that redirects unless any 'downloading'/'cached' arrives first (timer
cleared on progress); a `redirecting` latch prevents double navigation.
**Area:** `internal/app/features/missionhydrosci/templates/missionhydrosci_play.gohtml`
(`showDownloadProgress` onStatus chain)

Both CONFIRMED. The completion overlay ("Unit Complete → Downloading next
unit…") handles `downloading` / `cached` / `stalled` / `error` only, and by
this point `cleanupUnity()` has already torn the game down — so any
unhandled terminal state strands the student on a frozen overlay whose only
escape is manual reload/back.

## 17a — 'not_cached' (and 'partial') freeze the overlay

Round 1 made `'not_cached'` newly reachable mid-download: a canceled
fallback loop broadcasts it to every page, and a cross-tab `deleteUnit`
surfaces it via the poller's `!fresh` backstop (which can also fire
`'partial'`). `_fireStatus` treats `'not_cached'` as terminal — tracking and
watchdog cleared — so **no further status ever arrives**; the overlay's
if/else chain matches nothing, the text freezes at "Downloading next
unit... X%", and `navigateToNext` is never called.

**Fix:** add a branch for `'not_cached'`/`'partial'` mirroring the `'error'`
branch: brief message ("Download interrupted. Redirecting…") then
`navigateToNext` — the play page's cache-miss path falls through to the CDN
redirect, so navigating is the correct degraded behavior.

## 17b — 'stalled' is a message with no exit

The `'stalled'` branch only sets "Download stalled — still trying...". The
overlay has no Retry control and, unlike `'error'`, no navigation timeout.
`_monitorFallbackDownload` fires 'stalled' exactly once and then stays
silent; a dead SW loop never emits another status. The student is stranded
indefinitely.

**Fix:** on 'stalled', start a one-shot timer (~30s); if no
'downloading'/'cached' arrives before it fires, treat as the error path
("Download stalled. Redirecting…" → `navigateToNext`). Clear the timer on
any subsequent progress. (A Retry affordance on the overlay would also
work but adds UI; the redirect matches the existing error behavior.)
