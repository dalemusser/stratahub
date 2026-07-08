# MHS-018 — Play-session heartbeat dies under hidden-tab timer suspension

**Priority:** P1 (the MHS-013 protection fails on the primary platform)
**Status:** **Fixed 2026-07-06.** Protocol centralized as
`MHSDeliveryManager.startPlayHeartbeat(unitId, version)` (returns a stop
function) with `MHSDeliveryManager.ACTIVE_PLAY_KEY` shared with the units
template's purge list; the play template collapsed to one call + a pagehide
stop. Beats on the interval, on visibilitychange→visible, and on pageshow.
TTL raised to 30 minutes; expired entries pruned on every map write.
`pruneStaleCaches` reads the same constants.
**Area:** `internal/app/features/missionhydrosci/templates/missionhydrosci_play.gohtml`
(heartbeat block), `internal/app/resources/assets/js/mhs-delivery.js`
(`pruneStaleCaches` expiry)

CONFIRMED. iOS/iPadOS Safari fully suspends a hidden tab's `setInterval`
(Chrome's intensive throttling/tab-freeze can too), and tab switching fires
no `pagehide`. So a student who switches away from the game tab for more
than 3 minutes lets the `mhs-active-play` entry expire while the game is
still alive and resumable. A units-page load after a version-bumping deploy
then prunes the suspended game's cache — exactly the scenario MHS-013's
heartbeat was added to prevent. Online, the SW's cache-miss fallback
degrades to CDN fetches; offline — the feature's core scenario — the
resumed game breaks.

A second candidate half (bfcache: `pagehide` clears the timer and entry with
no `pageshow` re-arm) was **effectively refuted**: the same `pagehide` runs
`cleanupUnity()`, which quits Unity — a bfcache-restored play page is
already a dead game, so there is nothing for the heartbeat to protect. Any
fix should target visibility, not bfcache.

## Fix

1. Refresh the heartbeat on `visibilitychange` → visible (and on
   `pageshow`), so returning to the game tab immediately re-protects it.
2. Lengthen the expiry substantially (e.g. 30 minutes — roughly a class
   period). The cost of a stale entry is only that pruning of ONE
   unit-version cache is delayed until a later visit; the cost of premature
   expiry is deleting a live game's cache. Asymmetric — err long.
3. While in the file: prune expired entries from the map on write
   (`mhsPlayHeartbeat` already does parse-modify-write), so crashed-tab
   entries don't accumulate in localStorage forever.

## Fold-in cleanup (do with this fix)

The heartbeat protocol currently lives as a magic string in three files
(play template write/delete, `pruneStaleCaches` read, units template purge
key list). The play page already loads mhs-delivery.js — move the protocol
into it (e.g. `MHSDeliveryManager.startPlayHeartbeat(unitId, version)`
returning a stop function, plus a shared ACTIVE_PLAY_KEY constant) so the
key and map shape have one owner and the template collapses to one call.
