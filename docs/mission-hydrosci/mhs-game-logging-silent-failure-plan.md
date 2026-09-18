# Mission HydroSci — Silent Game-Logging Failure — Detection and Mitigation Plan

**Date:** 2026-09-16 (status updated 2026-09-17)
**Status:** §5.1 to §5.4 built on 2026-09-17 (see §0 for what shipped, what was verified, and what is still pending).
**Scope:** `stratahub`, feature `internal/app/features/missionhydrosci`, the MHS dashboard Devices tab, the teacher guide, and a handoff bundle for the game team (`mhs-updates/gamelogger-cache-overflow-091626/`). The game build is treated as final for the September launch.

**The problem in one paragraph.** On one Windows browser profile, the game stopped sending gameplay logs to the log service. Not partially: for the whole of every session, on every unit build, while the game played normally. Nothing told the player, the teacher, StrataHub, or the log service. It was discovered days later by an analyst who played specifically to generate data and then found none under her id. Clearing the browser's site data fixed it at once. The research study depends on this data, and a shared classroom machine in this state loses every student who uses it.

---

## 0. Status and how to resume

**Decided (2026-09-16).**
- The game is final for launch. Real students start in under a week. No new game build before then unless the trigger in §5.5 fires. Anything that could destabilize loading or play is out.
- Work goes into StrataHub: detect the failure independently of the game, show it to teachers and researchers, and document the remedy. Measurements and alerts are preferred over game changes.
- The evidence from the incident machine is gone (all site data cleared). The exact game-side mechanism stays a hypothesis unless it recurs; the plan must work without knowing it.

**Decided 2026-09-17 (project lead).** Build the whole plan now, secondary signals included. Students see the in-page notice and teachers see the Devices tab, both with the fix. Member launch records stay in the Device Tests viewer behind its Kind filter for now; where and to whom data is shown is a later discussion.

**Built 2026-09-17 (all of §5.1 to §5.4).**
- Server: the heartbeat handlers (member and device test) run the "are logs arriving" check on every 10th beat, then every 20th once seen, against the log service's collection through `store/logdata.HasEntrySince` (one index seek, 3 s budget, errors → unknown); the reply carries `{"logs": "seen"|"none"|"unknown"}`; the record gets `logs_state`, `logs_checked_at`, `logs_seen_at`, `no_logs_flagged_at`, and the page's `cache_errors` / `playerprefs_bytes`. A flagged session is logged as `logging-health: playing with no log entries arriving`. (`missionhydrosci/logs_health.go`, `store/mhsdevicetests`, `models/mhsdevicetest.go`.)
- Play page: console hook on the game's "Failed to save/load cached logs" text; PlayerPrefs store size read from IndexedDB before launch and once a minute (`MHSStepLog.readUnityPrefsBytes`, read-only, never creates the database); one calm notice along the bottom of the game with a "What to do" panel for the teacher; both readings ride on the heartbeat; the reply's verdict shows or clears the notice. Device-test play pages get the same.
- Units page: the store size goes into the device-status report (`playerprefs_bytes`) and the step log's Storage line. Preflight wording now says the game plays but records nothing when a service is blocked.
- Devices tab: a **Logs** column (✓ and time when seen; amber "!" with Not recorded / Cache full / Nearly full; dash when no verdict) with the remedy in the tooltip and the header's info icon, plus a legend entry. (`mhsdashboard/logging_health.go`.)
- Device Tests viewer: game-telemetry lookup by the member's user id for member records; a Logs column, a Logs filter (Problem / None arrived / Seen), and a Logging line in the detail. Kind = Member load record + Logs = Problem is the researcher's "launched but nothing arrived" list.
- Docs: teacher guide section `mission-hydrosci-teacher-guide/game-activity-not-recorded.md` and the Devices view text; support checklist `mhs-logging-support-checklist.md` (evidence before remedy; network case).

**Verified 2026-09-17.** `go build`, `go vet`, and the tests of every touched package; `node --check` on both JS modules; every inline script block of the play and units templates syntax-checked for both template branches; a local server start compiled all pages; the Devices grid rendered from the fixture test with the new column in every row. Deployed to the shared service, then a headless device-test run on the dev workspace (unit 2, run id starting `ffffffffcc85fd01`, form name "Automated check", safe to delete): the play page launched with no page errors, the notice stayed hidden, the store reader returned the real Unity store (536 bytes, database `/idbfs` at Emscripten version 21), heartbeats carried the readings and returned JSON, and at the tenth beat the check answered "none" for a game nobody was playing. That last result was correct as a fact and wrong as a verdict, which produced the "idle is not failure" rule above; it was rebuilt, redeployed and re-run the same night. The offline bar was verified the same way (run id starting `ffffffff6cec46`): cutting the browser's network showed the bar within seconds with a "Connection lost" step entry, restoring it cleared the bar with "Connection back", and the next heartbeat carried the offline time. Not verified: the console-error text on a real game store at the cap, and the Devices tab with real rows.

**Still pending (owner: project lead).**
1. ~~The optional experiment in §5.6~~ Done 2026-09-18, headless, with a stronger result than hoped: the failure reproduces in two minutes and the mechanism is observed (see §2).
2. When to add the second part of the game fix (§5.5: loop guard, bridge reporting) to the handoff bundle.
3. A Chromebook look at the play page (notice hidden in normal play; the Storage line in the step log) and at the Devices tab with a class.

**Resume by** reading §1 for what was verified, §5 for the design, §7 for costs.

---

## 1. What happened (verified from server data, read-only, 2026-09-16)

Sources: the log service's data and request ledger, its service journal, StrataHub's member launch records and heartbeats, device-status rows, and two records the analyst exported from the Device Tests viewer.

**The machine.** Windows 10, Chrome 152, a high-end desktop, used by a member of the production team who plays the game repeatedly across builds. The same browser profile had been used on the game's dev workspace since at least early September and on the production workspace since March.

**Timeline, 2026-09-15 into 09-16 (UTC).**

| Time | Verified |
|---|---|
| 21:02 | Production workspace, her February account: units page downloaded two units. The page's own reachability probe reached the log service and the save service. No launch there. |
| 21:20, 21:21 | Dev workspace, a test account: two host-page crash reports ("Failed to load Unity loader script", an older unit build) were **delivered to the log service** from this browser. The path was open. |
| 21:38, 23:41, 00:23 | She signed in to the log service console, presumably looking for her data. |
| 23:39 to 00:20 | Dev workspace, the account in her screenshot: **four launches, 46 heartbeats, about 22 minutes of visible play, normal frame rate. Zero log entries. Zero rejections. Zero failed requests from her address.** The browser console showed `Failed to save cached logs (WebGL): Could not store preference value` on every event. |
| about 00:21 | Site data cleared (new device id, units re-downloaded). |
| 00:22:30 on | First launch after the clear: the known first-event rejection, then accepted entries 50 ms later. 12,318 entries that day across all five units, with the first-event rejection at each of seven launches. |

**History.** The same machine logged normally on the dev workspace on Sep 4 to 6 (about 30,000 entries under two earlier test accounts). Nothing from it between Sep 6 and Sep 15 except one crash report on Sep 12. The profile broke somewhere in that gap. Her production account has zero log entries ever.

**Ruled out.**
- Network blocking: crash reports and probes reached the log host from that browser the same evening; the request ledger has no failed request from her address in four days.
- Server rejection: none from her address before the clear.
- Partial loss: it was total, every session.
- Account-specific: it followed the browser origin across accounts.
- Gameplay impact: frame rate and heap identical before and after. Nothing a player would notice.
- The service worker: it intercepts same-origin requests only and never touches requests to the log host.

**Not knowable now.** Whether the game never made the requests or made them and they failed inside the browser. From the server those are identical, and the cache that would have told us was deleted with the clear.

---

## 2. Why the game fails silently

From the game's `GameLogger.cs` (the copy in the 2026-05-26 handoff bundle). Every failure is absorbed at one of three levels, none of which reaches the player, the host page, or the server.

1. **Cache write or read fails.** Caught, printed to the browser console as an error, execution continues. This is the message in the screenshot. The game persists its **entire** unsent-log queue as one PlayerPrefs string, and Unity caps WebGL PlayerPrefs at 1 MB for the whole store, so once the queue is over the cap every write throws. Continuing is correct for gameplay, but this is the only trace anywhere, and only with DevTools open.
2. **A log POST fails.** Printed to the game's in-game debug console only (and only if debug logging is on in that build), then the same entry is retried every ten seconds forever. Nothing in the browser console, nothing to the server. One entry that can never succeed stalls everything behind it, and the cache grows to the cap.
3. **An exception escapes the send loop.** Unity ends the coroutine. A static "sending" flag stays set, so no new sender is ever started. Every later event is queued and cached, none sent. On WebGL with the usual setting of catching only explicitly thrown exceptions, a null reference in that loop may print nothing at all.

**Mechanism, observed 2026-09-18 (supersedes the hypothesis below).** Reproduced on the dev workspace by blocking the log service host in the browser at launch: the game POSTs its first event, the retry ten seconds later, and then never sends again for the session, blocked or not, while writing every event to its store; a reload does not recover. The captured store shows why. The cached queue's first entry is an empty object, `{}`, and the send loop reads an empty head entry as "queue empty" and exits, every time, forever. Two timed runs on 2026-09-18 pinned the sequence: an entry is sent, the same entry is retried once ten seconds later, and after that second failure the entry is emptied in place and no further attempt is ever made; the identity of the entry does not matter (a normal event with a proper user id breaks the same way). The logger source we have has no code that empties an entry and would retry forever, so the shipped build carries a newer send loop with an attempt limit that clears the entry's dictionary instead of removing it from the queue. Normally a send is answered within a fraction of a second and the entry is dequeued; it takes two network-level failures of one entry, ten seconds apart, at any point in a session. One brief outage, and the profile never logs again until its site data is cleared. Two further findings from the same specimen: queued entries share the `LoggingData` assets' reusable data dictionary, so a backlog sent after an outage carries wrong payloads; and the logger component lives on the console-manager object in both core-systems prefabs with static state split across instances. Details, the reproduction recipe, and the specimen are in the handoff bundle (`mhs-updates/gamelogger-cache-overflow-091626/02-reproduction-and-specimen.md`). The fix needs three things on top of the bundle's drop-in: copy the dictionary on enqueue, skip empty entries when loading the cache, and diff the shipped logger against the drop-in so the attempt-limit code is not carried forward.

**Leading hypothesis for the incident (written 2026-09-16, before the reproduction).** A full cache alone does not explain a sender that never even tries: with the host reachable, the code would drain a full cache one entry at a time. The consistent explanation is level 3: the persisted cache, once loaded at startup, kills the send loop on its first pass (for example an entry that deserializes to null), after the flag is set, so nothing sends all session, every session, and the cache grows until the cap, which is when the console error finally appears. The cap error is a late symptom. The cache format has been unchanged since January, so a format change is not the trigger.

**Two facts that shape everything below.**
- The PlayerPrefs store is one per site origin and per company/product name. Every unit build on a site shares it, and every account that plays in that browser profile shares it. A shared lab machine accumulates from everyone.
- The failure is invisible from inside the game's own reporting. Any detector that depends on the game noticing something will miss it. The detector has to be external.

---

## 3. Constraints

- Real students start in under a week. Nothing that has been working may be destabilized. Loading and play paths are not to be touched by anything speculative.
- The game build is final. A new build needs discussion, the game team, and a build cycle; it happens only on the trigger in §5.5.
- Everything must fail safe: a detector that cannot check reports "unknown", never "failure". Students must never see an error they did not cause.
- Repo is public: no real hostnames, addresses, names, or ids in this document.

---

## 4. What can be known from outside the game

| Quantity | Measured today? | How | Cap |
|---|---|---|---|
| Browser storage for the site (units, caches, IndexedDB) | Yes: step log, device-status report, Devices tab | `navigator.storage.estimate()`, `persisted()` | Browser quota (GBs) |
| **Unity PlayerPrefs store** (holds the unsent-log cache) | **No** | Read the IndexedDB database Unity mounts, file entry whose path ends in `/PlayerPrefs`, take its byte length. Read-only, same origin, milliseconds. | **1 MB, a Unity constant** |
| WebAssembly and JavaScript heap | Yes: heartbeat every 30 s | `performance.memory`, the Unity module's memory buffer | n/a |
| **Whether log entries are arriving** | **No** | StrataHub already reads the log service's collection on the same cluster (`store/logdata`) | n/a |

Not knowable from outside: the size of the game's in-memory queue and whether its sender is alive. Only a game change reports those.

Cautions for the PlayerPrefs read: check that the database exists before opening it (opening creates an empty one), and expect the file to lag the game's writes by seconds.

---

## 5. The plan

### 5.1 Detect "playing, but nothing is arriving" on the server (about one day)

The one detector that works whatever the cause.

**Mechanism.** The play page already sends a heartbeat every 30 s to its launch record (`POST /missionhydrosci/api/steplog/{id}/heartbeat`). The handler adds: on the 10th beat and every 10th after (every five minutes), one indexed read on the log collection: any entry for this user with an id newer than the launch start (ids are time-ordered; the index on game, user id and id makes this a single seek with limit one). The reply, empty today, returns `{ "logs": "seen" | "none" | "unknown" }` and, when seen, the handler stamps `logs_seen_at` on the launch record. Once seen, keep checking every ten minutes to catch a sender that dies mid-session.

**Page behaviour.** After five minutes of visible play with `none`, show one calm notice and stamp `no_logs_flagged_at` on the record. Draft wording: "Game activity is not being recorded on this device. You can keep playing. Please tell your teacher." The remedy goes to the teacher, not the student. `unknown` never shows anything.

**Safety rules.** Short timeout on the read; any error is `unknown`. The known first-event rejection does not interfere (the second event of a session arrives seconds later). The heartbeat stays fire-and-forget on the client.

**Offline is a different condition (added 2026-09-17 after a no-network test).** With the network gone, no heartbeat reaches the server, so no verdict comes back, and the page's own signals fire only at the cap. The game's sender is not broken in this case: it stops trying while the device is unreachable, writes every event to its store, and sends the backlog itself when the connection returns, up to the cap (about 40 to 60 minutes of active play). The save service fails too, which is the larger risk for the student. The page therefore detects offline itself: two failed heartbeats in a row (about a minute), or the browser's offline event, show a separate offline bar ("lost its internet connection, keep this tab open, the game will save your progress and send your activity when the connection returns"), and the first answered beat clears it. The offline bar replaces the logging bar while it shows. A unit launched from its cached files with the network already down is the corner case: the launch record cannot be created, so no heartbeat would ever run. The page therefore trusts the browser's own offline flag at load, keeps retrying the launch record every 30 seconds (each failure counts as a failed beat), and starts the heartbeats the moment it succeeds; the browser's online event triggers the retry at once. Cumulative offline time rides on the next successful beat (`offline_ms`) and appears in the Device Tests detail.

**The game can fullscreen its own canvas (found 2026-09-17 in the blocked-host run).** When the game, rather than the page's button, requests fullscreen, the canvas alone enters the browser's top layer and covers every page element, the notice bars included. The bars are therefore shown through the Popover API where the browser has it (Chrome, recent Safari), which puts them in the top layer above the canvas, pinned to the viewport's bottom edge, and re-raised on every fullscreen change; older browsers get the plain in-page bar, which is visible unless the canvas itself is fullscreen.

**Idle is not failure (added 2026-09-17 after the first live run).** A game sitting at its opening screen or in a background tab produces no events, so nothing arrives, and "none" alone would raise the notice on every screen in a class told to launch and wait. The discriminator is the game's own store: it writes every event it cannot send to its PlayerPrefs store, so with the sender broken the store grows with play, while an idle game leaves it still. The heartbeat carries the store size; the record keeps the first readable size as the baseline; "none" is reported only when the store has grown by at least 4 KB since launch, is at or past 90 % of the cap, or the page saw the game's cache-write error. Otherwise the state is `quiet`: no notice, no badge, and the check keeps running. A browser that cannot read the store reports 0 and is treated as idle, so it never raises a false alarm (the cache-error hook still covers it at the cap).

**Would it have caught the incident?** Yes, at about 23:44 on Sep 15, five minutes into the first session, instead of days later.

### 5.2 Show teachers and researchers (about half a day)

- **Devices tab:** an amber badge on a device whose newest launch record is flagged, and a "last log received" time per student read from `logs_seen_at` on StrataHub's own records. No query of the log collection while a teacher looks at the page. Amber is right here: this is a real problem, per the dashboard color rule.
- **Device Tests viewer:** the detail page's game-telemetry lookup uses the record's own id, so it reports zero events for every member record. Use the member's user id. Small, and it gives researchers a "logs received" check per session today.
- **Researcher report:** members who launched a unit on a day and have zero log entries that day. A simple list or CSV. Turns "found out weeks later" into "known that afternoon".

### 5.3 Document the remedy, evidence first (about half a day)

- **Teacher guide** troubleshooting entry: what the badge and the notice mean; the remedy (clear site data for the game's site on that device; progress and settings are on the server and are safe; the units download again, so do it on good Wi-Fi); when it tends to happen (shared devices, heavy long-term use, a network that blocks the log host).
- **Support checklist,** in this order: (1) export the PlayerPrefs file from DevTools (Application, IndexedDB, the Unity database, the entry ending in `/PlayerPrefs`) and send it to us; (2) then clear site data; (3) sign in again. One captured file turns §2's hypothesis into a confirmed or discarded bug.
- **Ops note** for districts: the log host must be allow-listed. Today the launcher's preflight warning says the game "may report a connection error". It reports nothing. Reword.
- **Handoff bundle:** add the shared-store finding and the level-3 wedge for the game team to check (when cleared, see §0 item 5).

### 5.4 Secondary signals, as time allows (about half a day)

- **Console hook** on the play page, installed before the Unity loader runs: match the exact "Failed to save cached logs" text, record a step-log entry, flag the launch record, and include it in the crash-report path. Definitive when it fires, but it fires only in the late phase.
- **PlayerPrefs store size:** read on the units page and sent with the device-status report, so the Devices tab can show "unsent game logs: N KB" for a device before it is launched; read once a minute on the play page for the step log. This is the one indicator that persists between sessions: growth over time on a device means its sender is not keeping up. On the incident machine it would have shown a store near the cap on the units page at 21:02, before any launch.

### 5.5 Game change: deferred, with a trigger

The handoff bundle `mhs-updates/gamelogger-cache-overflow-091626/` contains the write-up and a drop-in `GameLogger.cs`: bounded cache with the oldest entries dropped first and an overflow event reporting the gap, entries dropped only on 400 or 413, capped backoff, device block attached at send time, coalesced cache writes, batch sends, and a loader that reads the old cache format. It compiles against Unity API stubs; it has not been built in the Unity project.

**To add when cleared:** never let the send loop die (catch per iteration, drop entries that will not load, reset the flag), and report a logging-health state to the host page through the bridge so the page can show and forward it.

**Trigger for pulling a build forward:** the detector in §5.1 flags more than a handful of student devices in the first week, or the research partner needs complete data for a named cohort. If a build happens for another reason, the minimal variant (a fixed entry cap plus the loop guard) is a few lines and far lower risk than the full drop-in.

### 5.6 Optional experiment: manufacture a specimen

A tester on a dev test account blocks the log host in DevTools, plays for about an hour so the cache fills, exports the PlayerPrefs file, then unblocks and reloads. If logging recovers, a full cache alone is not the killer and the level-3 theory gains weight. If it does not, we have a reproducible case and a file for the game team. Needs one person and an hour; risks nothing.

---

## 6. What not to do

- **No automatic clearing of the game's storage from the host page.** A full cache drains by itself on the next session if the sender works. If it does not, clearing gains nothing and destroys the evidence. Measure and tell; do not touch.
- **No wrapping of fetch or XHR to watch the game's requests.** It would work, but it sits under the Unity loader days before launch, and §5.1 gives the same answer from the server with no risk to loading.
- **No new game build before students start**, absent the trigger. Fresh student devices begin with an empty store; the failure needs time or a wedged entry first, and §5.1 will show within a day whether it is happening.

---

## 7. Costs

**Heartbeats as deployed today** (one POST every 30 s while the game runs; one lookup and one array append per beat, array capped at 240):

| Quantity | Size |
|---|---|
| One stored beat | about 140 bytes |
| A 50-minute period of beats | about 14 KB |
| A whole play launch record (context, steps, beats) | about 20 KB |
| Per 1,000 students, one period a day, a 90-day semester | about 1.8 GB |

The game's own log stream for the same students is about 100 times larger (about 35 entries a minute at about 1 KB each). Heartbeats are a rounding error next to it. Later improvements, not for launch week: members do not need the 30 s cadence device tests use, so 60 s halves everything; and launch records have no expiry, so a retention job that drops beat arrays after 30 days and keeps the summary belongs in before the collection is a year old.

**The §5.1 check:**

| Cadence | Reads added per student per period | Reads per second at 1,000 concurrent students |
|---|---|---|
| Every beat | 100 | 33 |
| Every 10th beat | 10 | 3 |
| Every 10th beat, then every 10 min once seen (chosen) | 1 to 3 | under 1 |

A few percent on top of the two operations every beat already performs; cents a month on per-operation pricing. Client: no new requests, a 40-byte reply, one comparison.

---

## 8. Sequencing and estimates

| Step | Effort | Depends on |
|---|---|---|
| 5.1 server check, page notice, record fields | 1 day | decision 1 and 2 |
| 5.2 Devices tab badge and last-log time; viewer lookup fix; researcher list | 0.5 day | 5.1 fields |
| 5.3 teacher guide, support checklist, ops note, preflight wording | 0.5 day | wording from 5.1 |
| 5.4 console hook, PlayerPrefs size on units and play pages | 0.5 day | none |
| Chromebook check of the play page and units page after 5.1 to 5.4 | 0.5 day | all above |

About three days, leaving time before students start for the device check. Deploy is by the usual update script; nothing here changes the game, the service worker, or the download path.

---

## 9. Files expected

- `internal/app/features/missionhydrosci/steplog_member.go`: heartbeat reply with the logs state; the periodic read via `store/logdata`; `logs_seen_at`, `no_logs_flagged_at` on the record.
- `internal/domain/models/mhsdevicetest.go`: the two fields.
- `internal/app/features/missionhydrosci/templates/missionhydrosci_play.gohtml`: act on the reply; the notice; the console hook; the periodic PlayerPrefs read.
- `internal/app/resources/assets/js/mhs-delivery.js` and `missionhydrosci_units.gohtml`: PlayerPrefs size in the device-status report; preflight wording.
- `internal/domain/models/mhs_device_status.go`, `store/mhsdevicestatus`: the new field.
- `internal/app/features/mhsdashboard/`: badge and last-log column.
- `internal/app/features/viewers/views/devicetests.go`: lookup by user id.
- `docs/mission-hydrosci-teacher-guide/`: troubleshooting entry.
- `docs/mission-hydrosci/`: support checklist (new).

---

## 10. Related

- Handoff bundle for the game team: `mhs-updates/gamelogger-cache-overflow-091626/` (README, write-up, drop-in `GameLogger.cs`).
- `docs/mission-hydrosci/game-first-log-event-rejected.md`: the other known silent loss (first event of each session lacks the user id).
- `docs/mission-hydrosci/mhs-loading-status-and-unit2-device-test-plan.md`: where the step log, launch records and heartbeats come from (§A1, §A2).
- `docs/mission-hydrosci-teacher-guide/devices-tab-notes.md`: Devices tab encoding and color rule.
- `docs/mission-hydrosci/mhs-remaining-work-plan.md`: launch-readiness items this sits beside.
