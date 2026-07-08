# MHS-021 — Stale/paused Background Fetch strands current unit at 0% (cross-device switch)

**Priority:** P1
**Status:** **Fixed 2026-07-06** (found in the round-2 device smoke test)
**Area:** `internal/app/resources/assets/js/mhs-delivery.js`
(`_reconnectActiveDownloads`, `_parseFetchId`),
`internal/app/features/missionhydrosci/templates/missionhydrosci_units.gohtml`
(pipeline re-entry)

## Field report (2026-07-06, HP Chromebook)

Same account used on a MacBook (collection switches, unit jumps, Reset) and
then on an HP Chromebook. On the Chromebook, Unit 4 (current unit) showed
0% and never moved; the ChromeOS tray showed the download **paused at 0%**;
manually resuming it un-paused the tray entry but nothing downloaded, and no
'stalled'/Retry ever appeared. "Reset All MHS Data" immediately fixed it —
Units 4 and 5 then downloaded normally, and collection/unit switching worked.

## Root cause

Two defects stacked on top of a browser behavior:

1. **Browser behavior (the trigger).** Chrome persists Background Fetch
   registrations across browser restarts and can leave one **paused**
   (shutdown mid-fetch, low disk, crash recovery); resuming from the tray is
   known-flaky, especially after the registering service worker has since
   been updated (which had just happened — the round-2 deploy). The
   Chromebook was carrying such a fetch from an earlier session.

2. **Reconnect matched by unitId only, ignoring version.**
   `_reconnectActiveDownloads` adopted any in-progress fetch whose unitId
   was in the manifest — including one for an **old version** (likely here,
   given the collection switching done on the MacBook). The monitor then
   polls the *current*-version fetch ID, finds nothing, and the `!fresh`
   backstop clears tracking and fires the cache status ('not_cached').

3. **The auto-download pipeline had no re-entry from 'not_cached'/'partial'.**
   Its only trigger was 'cached'. Compounding it, while the unit was
   (mis)tracked as active, init's `checkAllCacheStatus` skipped it, so
   `cacheStatus[currentUnit]` was undefined and the post-init `runPipeline()`
   did nothing. Net: current-unit card stuck on "Downloading..." at 0%,
   nothing running, no Retry (tracking — and with it the stall watchdog —
   was already cleared), stale paused fetch sitting in the tray.

**Why Reset fixed it:** `purgeAllMHSData` aborts **every** MHS Background
Fetch across all versions, deletes all caches, and reloads — removing the
wedged fetch and letting a clean pipeline run.

(If the stale fetch had instead been for the *current* version, the adopt
path is correct and the stall watchdog fires 'stalled' + Retry after ~2.5
minutes — that leg works as designed.)

## Fix

1. `_parseUnitIdFromFetchId` → `_parseFetchId` returning `{unitId, version}`;
   `_reconnectActiveDownloads` adopts only on a unit **and version** match
   and aborts mismatched (or legacy versionless) in-progress fetches — the
   same treatment as no-manifest-unit fetches. Aborting also unblocks
   pruning of the old-version cache on the same init pass.
2. Units template: the pipeline now also re-runs on 'not_cached'/'partial'
   for the current/next unit (guarded by `isReloading`), so any path that
   clears tracking terminally — tray-canceled fetch, stale-fetch backstop —
   self-heals instead of dead-ending. `doClearAllDownloads`/`doResetAllData`
   set `isReloading` before purging (their status churn must not trigger
   re-downloads mid-purge; the reload restarts clean) and clear it again on
   failure since no reload will come.

## Verification note

Static checks pass (node --check, concatenated SW, template parse, go
build). Device re-test of this specific path requires recreating a stale
fetch: start a unit download on the Chromebook, shut the browser mid-fetch,
switch the account's collection from another device, then reopen the units
page — the stale fetch should be aborted and the current version should
download; the tray entry disappears.
