# Mission HydroSci — Game logging failure — Support checklist

**Date:** 2026-09-17 (updated 2026-09-28: the play page repairs the store)
**For:** whoever handles a report that a student's (or tester's) gameplay is not appearing in the log service, or sees the amber **Logs** badge on the Devices tab.
**Background:** `mhs-game-logging-silent-failure-plan.md` (why the game fails silently, what StrataHub now detects). Teacher-facing text: `../mission-hydrosci-teacher-guide/game-activity-not-recorded.md`.

The game gives no sign when its logging fails. It plays normally, progress saves, and the only traces are the ones StrataHub now collects. Since 2026-09-28 the play page also fixes the known cause by itself: before the game starts, it removes the empty entry that stops the schools' build (v2.8.1) from sending, and the saved backlog goes out on that launch (§3). Most flagged devices therefore need nothing done to them.

---

## 1. Confirm it is this problem

Any one of these confirms it:

- **Devices tab (dashboard):** the device's **Logs** cell shows an amber **!** with *Not recorded*, *Cache full* or *Nearly full*. Hover for details. (Column added 2026-09-17; devices that have not played since then show a dash.)
- **Device Tests viewer:** set Kind to *Member load record*, Logs to *Problem*, and the date range. Each row is one play session where the server saw no gameplay entries after about five minutes of play while the game's local store was growing (so the game was producing events it could not send), or where the page saw the game's cache-write error, or where that store was nearly full. A session marked *Quiet* is a game that sat at a menu and produced nothing; it is not a problem. Open the row: the *Logging* line has the state, the times, and the store size. The *Gameplay* line is the log service's count for the member's id (fixed 2026-09-17 to look up by user id).
- **The student's screen:** the amber bar along the bottom of the game, "Your game activity is not reaching the server right now."
- **Server journal** (StrataHub): `logging-health: playing with no log entries arriving` lines, one per flagged session, with the record id and unit.

What does *not* confirm it: the launcher's preflight warning that the log service is unreachable. That is a network finding (see §4), and it says the same thing about every device on that network. The **offline bar** ("This device has lost its internet connection") is the page noticing its own heartbeats failing. It is not itself the recording problem, but it predicts one: an outage of more than about ten seconds during play kills the game's logging on that device for good (verified 2026-09-18), so a device that showed the offline bar will show Not recorded a few minutes after reconnecting, until its next launch sends the saved record (§3). The Device Tests detail shows how long the device was offline.

## 2. Capture evidence (only when asked)

The cause on the game side is reproduced and documented (bundle notes `02-reproduction-and-specimen.md` and `03-builds-12438-12446-test.md`), so a field specimen is needed only for a case that does not fit it (for example a device still flagged after a launch that recorded the repair). The next launch changes the store, so capture before that launch:

1. Open the game's site in the browser, then DevTools (F12, or ⋮ → More tools → Developer tools).
2. **Application** tab → **IndexedDB** → the database named `/idbfs` → object store `FILE_DATA`.
3. Find the entry whose key ends in `/PlayerPrefs`. Its `contents` field is the game's PlayerPrefs store. Right-click the value → *Copy* (or note its byte length if copying fails), and paste it into a file named after the device and date.
4. Also copy the browser **Console** output if it shows `Failed to save cached logs (WebGL): Could not store preference value`.
5. Send both to the project lead with: workspace, the student's or tester's account, device type, the date and time of the affected session, and whether the device is shared.

The page reports the store's size on its own (Devices tab tooltip, Device Tests detail, and the step log's *Storage* line: "Game log cache store: N KB of 1,024 KB"). The size alone is not a specimen; the contents are.

## 3. Remedy on the device

Usually none. The play page repairs the store before every launch (`MHSStepLog.repairUnityLogCache` in `mhs-steplog.js`): it removes the empty and unreadable entries from the `game_logs_cache.json` record in the game's PlayerPrefs file and leaves every other byte as it was. The schools' build then sends the whole saved backlog on that launch. The launch's step log (Device Tests viewer, the member load record) shows one *Storage* line:

- *Unblocked the game's saved activity record: removed 1 empty entry … N saved events will be sent from this launch* — the device was stuck and is not any more;
- *Game's saved activity record checked: nothing blocking* — normal;
- *Could not check the game's saved activity record (…)* — an old browser, a timeout (3 s cap) or a storage error; the game started as usual.

1. Let the student keep playing. Progress and settings live on the server.
2. Watch the Devices tab on the next play session on that device: the Logs cell should turn to a ✓ with a time within about five minutes of play.
3. Only if it does not: §4 (the network), then §2 (evidence), and as a last resort clear the site data (padlock or tune icon in the address bar → **Site settings** → **Delete data**; DevTools → Application → Storage → *Clear site data* does the same). That deletes the saved record, the downloaded units and the service worker. The Manage page's clear buttons do not reach the game's store; they remove only the downloads, the app shell and the launcher's own settings.

What is lost with the repair: nothing the game saved. Caveat for analysis: in a backlog sent after an outage, events from the game's `LoggingData` assets can carry a later event's `data` (the shared-dictionary defect, bundle note 02); the event type, time and student are right.

## 4. If it comes back, or affects several devices at once

Several devices flagged at the same site, or one device flagged again straight after a clear, points at the network rather than the device: a content filter or proxy blocking the log service's host while allowing the save service and the site itself.

- The launcher's step log on the units page shows a *Game services* line: "unreachable" for the log host confirms a block from that network.
- The fix is on the district side: allow-list the game's log service host (and the save service host) for HTTPS. The play page's preflight names both.
- Until then the game plays but produces no research data from that network. Say so to the program lead, since it changes what the study can use.

## 5. What the server records, for later analysis

- `mhs_device_tests` (kind `member`): per launch, `logs_state` (seen / none / unknown), `logs_checked_at`, `logs_seen_at`, `no_logs_flagged_at`, `cache_errors`, `playerprefs_bytes`, and the same two readings on each heartbeat.
- `mhs_device_status`: `playerprefs_bytes` from the units page, per device, before any launch.
- The log service's own data is the ground truth: an entry per gameplay event under the member's user id. The Device Tests detail page shows the count for the session's member.

## 6. Escalation to the game team

The handoff bundle `mhs-updates/gamelogger-cache-overflow-091626/` has the write-up, the two-minute reproduction, a captured store, and a drop-in `GameLogger.cs` with the additions it still needs listed. On the schools' build, any log request that fails twice in a row, ten seconds apart, still ends logging for the rest of that session; the page's repair makes the next launch send the saved record, so the damage is a delay, not a loss. The game fix (the drop-in with `toSend` rebuilt on every pass, note 03) belongs in the next build; the project lead decides the timing.
