# Mission HydroSci — Game logging failure — Support checklist

**Date:** 2026-09-17
**For:** whoever handles a report that a student's (or tester's) gameplay is not appearing in the log service, or sees the amber **Logs** badge on the Devices tab.
**Background:** `mhs-game-logging-silent-failure-plan.md` (why the game fails silently, what StrataHub now detects). Teacher-facing text: `../mission-hydrosci-teacher-guide/game-activity-not-recorded.md`.

The game gives no sign when its logging fails. It plays normally, progress saves, and the only traces are the ones StrataHub now collects. Follow the order below: **evidence first, then the remedy.** The remedy destroys the evidence.

---

## 1. Confirm it is this problem

Any one of these confirms it:

- **Devices tab (dashboard):** the device's **Logs** cell shows an amber **!** with *Not recorded*, *Cache full* or *Nearly full*. Hover for details. (Column added 2026-09-17; devices that have not played since then show a dash.)
- **Device Tests viewer:** set Kind to *Member load record*, Logs to *Problem*, and the date range. Each row is one play session where the server saw no gameplay entries after about five minutes of play while the game's local store was growing (so the game was producing events it could not send), or where the page saw the game's cache-write error, or where that store was nearly full. A session marked *Quiet* is a game that sat at a menu and produced nothing; it is not a problem. Open the row: the *Logging* line has the state, the times, and the store size. The *Gameplay* line is the log service's count for the member's id (fixed 2026-09-17 to look up by user id).
- **The student's screen:** the amber bar along the bottom of the game, "Game activity is not being recorded on this device."
- **Server journal** (StrataHub): `logging-health: playing with no log entries arriving` lines, one per flagged session, with the record id and unit.

What does *not* confirm it: the launcher's preflight warning that the log service is unreachable. That is a network finding (see §4), and it says the same thing about every device on that network.

## 2. Capture evidence before anything is cleared

The cause on the game side is still a hypothesis (plan §2). One captured store would settle it. On the affected device, before any clearing:

1. Open the game's site in the browser, then DevTools (F12, or ⋮ → More tools → Developer tools).
2. **Application** tab → **IndexedDB** → the database named `/idbfs` → object store `FILE_DATA`.
3. Find the entry whose key ends in `/PlayerPrefs`. Its `contents` field is the game's PlayerPrefs store. Right-click the value → *Copy* (or note its byte length if copying fails), and paste it into a file named after the device and date.
4. Also copy the browser **Console** output if it shows `Failed to save cached logs (WebGL): Could not store preference value`.
5. Send both to the project lead with: workspace, the student's or tester's account, device type, the date and time of the affected session, and whether the device is shared.

The page reports the store's size on its own (Devices tab tooltip, Device Tests detail, and the step log's *Storage* line: "Game log cache store: N KB of 1,024 KB"). The size alone is not a specimen; the contents are.

## 3. Remedy on the device

1. Let the student finish. Progress and settings live on the server.
2. Clear the site data for the game's site in that browser profile: padlock or tune icon in the address bar → **Site settings** → **Delete data**. (DevTools → Application → Storage → *Clear site data* does the same.) This deletes the game's PlayerPrefs store, the downloaded units, and the service worker; the next visit rebuilds them.
3. Sign in again and open the unit. It downloads again; use good Wi-Fi.
4. Watch the Devices tab on the next play session: the Logs cell should turn to a ✓ with a time within about five minutes of play.

What is lost: the unsent gameplay records on that device (they were not going anywhere). What is not lost: progress, settings, the account.

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

The handoff bundle `mhs-updates/gamelogger-cache-overflow-091626/` has the write-up and a drop-in `GameLogger.cs`. It is deferred until after the September launch unless the plan's trigger fires (more than a handful of student devices flagged in the first week, or a research cohort that needs complete data). A captured store (§2) goes with it.
