# Mission HydroSci — Game logging: everything that still needs fixing or deciding

**Date:** 2026-09-28
**Scope:** the game's logging (the schools' build and the game team's two new logger builds), the research data already in the log service, grading, StrataHub, and test cleanup. One list, with the evidence for each item and who acts on it.
**Background:** the investigation and what is already live are in `mhs-game-logging-silent-failure-plan.md` (§0). The game team's handoff is `mhs-updates/gamelogger-fix-093026/` (the fixed files and `03-for-the-game-dev.md`), which supersedes `mhs-updates/gamelogger-cache-overflow-091626/` (the investigation, especially note `03-builds-12438-12446-test.md`). This document does not repeat the whole story; it lists what is still open.

**Builds referred to below.** The schools' collection (MHS-Release_V1.0.0) runs units v2.8.1 (builds of 2026-09-14). The game team's new logger is in build 20260921-12438 (units v2.8.2; collection 20260921-12438-LoggingTest) and build 20260925-12446 (Unit 2 v2.8.3). The collection 20260925-12446-ClassifierIssuesTesting mixes them: Unit 2 from 12446 and Units 1, 3, 4 and 5 from 12438.

---

## Summary

| # | Item | Who | Priority | State |
|---|---|---|---|---|
| G1 | New logger builds resend every accepted batch and loop on refused ones (`toSend` kept between passes) | Game team | Blocks the new builds | **Fixed in the delivered files** (2026-09-30); awaiting the game team's build |
| G2 | Events' details overwritten by later events (shared `LoggingData` dictionary), in every build since February 2026 | Game team | High (research data) | **Fixed in the delivered files** (2026-09-30); awaiting the game team's build |
| G3 | Hardening so a future slip cannot flood the log service | Game team | High | **Done in the delivered files** (2026-09-30) |
| G4 | First event of each session sent without a user id and refused | Game team | Medium | **Fixed in the delivered files** (2026-09-30): the id is stamped on before sending |
| G5 | Two logger instances, `_instance` never set in `Awake` | Game team | Medium | **Fixed in the delivered files** (2026-09-30) |
| G6 | Game errors seen in the browser console during test runs | Game team | Low | The `JSON must represent an object type` exception is **fixed in the delivered files** (`SettingsSaveManager.cs`, 2026-09-30) and verified gone on the dev site; the once-seen `ArgumentOutOfRangeException` was not investigated and did not recur in the 2026-10-01 runs |
| G7 | Verification the next build must pass | Game team + us | Before any build reaches schools | Run on the delivered fix 2026-09-30 and 2026-10-01, versions 1.0–1.3 (all pass); to be run again on the team's build |
| D1 | About 700,000 duplicate entries in the log data from testing the new builds | Us (data) | High | Decision needed |
| D2 | Overwritten details in the existing data since February 2026 | Us + research partner | High | Decision needed |
| D3 | Recovered backlogs now arrive with mostly overwritten details | Us (StrataHub) | High | Decision needed |
| D4 | The log service has no protection against a client flooding it | Us (log service) | Medium | Proposal |
| R1 | Six grading rules read details that can be overwritten | Us (grader) | Medium | Check needed |
| R2 | Duplicates can inflate the grader's counts for the affected accounts | Us (grader) | Medium, after D1 | After the clean-up |
| S1–S6 | StrataHub follow-ups (incident records, flood detection, PDF, Chromebook look, Manage page wording, later items) | Us (StrataHub) | Mixed | See §4 |
| T1–T4 | Testing and environment clean-up | Us | Low | See §5 |

The decisions only the project lead can make are collected in §6.

---

## 1. The game (game team)

**Status 2026-10-01 (version 1.3 of the fix).** G1–G5 are fixed and G3 is done in `mhs-updates/gamelogger-fix-093026/Game-Code/` (`Systems/Logging/GameLogger.cs`, `Systems/Logging/LoggingData.cs`, and `Systems/Save Load System/SettingsSaveManager.cs` for G6), based on the game team's 2026-09-28 project copy. Version 1.1 added the findings of two independent reviews, version 1.3 those of a second pair; version 1.2 added per-entry ids (`session_id`, `seq`, `entry_id`), a `recovered` flag on cache-loaded entries, `sent_at`, and `429`/`Retry-After` handling, so the log service can de-duplicate, date and pace entries later without another game build (the server side of that is listed in the bundle's `02-logging-redesign-if-starting-over.md` §8). Unit 1 builds of each version, made here in Unity 6000.0.74f1 from the release profile, passed the G7 checks on the dev site on 2026-09-30 and 2026-10-01 (`01-changes.md` in that folder has the change-by-change description and the results; six test collections on the dev site hold the builds (unit1 v2.8.3–v2.8.8), nothing is active for students). The game team applies the three files to the mainline (`03-for-the-game-dev.md` in the bundle says how), builds, and uploads through MHS Builds; the same checks run again on their build before it is made active. For G6, the exception `JSON must represent an object type` is not from the logger: `SettingsSaveManager` parses the save service's settings-load response with `JsonUtility.FromJson`, which throws on the body `null` the service returns for a player without saved settings; the load coroutine dies there on every scene load (defaults apply anyway). The guard before the parse is in the delivered `SettingsSaveManager.cs` (third file of the bundle) and was verified on the dev site; details in the fix bundle's `01-changes.md` §8.

### G1. The new logger builds resend every accepted batch, and loop on a refused one

**What happens.** Both new builds contain the handoff drop-in `GameLogger.cs` (its methods, fields, messages and constants are all in the builds' IL2CPP metadata), with one change: the per-pass list of entries to send, `toSend`, is a field of `GameLogger` instead of a new list on every pass of the send loop, and it is never emptied. So:

- **Defect A.** After the server accepts a batch, the same list goes out again, forever, with anything newly queued added to it: about 8 requests a second, up to 350 duplicate entries a second, from the second request of every session until the page closes. No outage is needed.
- **Defect B.** After a refused batch, the loop switches to single sends but adds the head entry to the still-full list, so a batch one entry longer goes out and is refused again, about every 100 ms, forever. Past 100 entries every request is also refused for size. Nothing is recorded. A store left by the schools' build with its empty `{}` entry triggers this on every launch, and so does any entry without a user id.

**Evidence.** Measured on the dev site on 2026-09-28 (both builds), in the log data from dev testing on 2026-09-25, and reproduced exactly by a port of the loop (`specimen/send-loop-simulation.py`).

**Fix.** Rebuild `toSend` at the top of every pass (a new list, or `toSend.Clear()`), as the drop-in does on its line 350. Any other per-send state kept in a field needs the same care.

**Until then.** Neither build may go to schools. Everyone playing either collection floods the log service while the game is open (see T1).

### G2. Events' details are overwritten by later events from the same component

**What happens.** Many events are logged through the reusable components in `LoggingData.cs`. Each component keeps one dictionary: on every event it clears it, refills it, and hands the logger that same object. The logger converts an entry to text only when it sends it (and when it saves its queue), and the schools' build sends one entry at a time. So when events from one component come faster than they are sent, the waiting entries end up with the latest event's details. The event type, the student, the entry's timestamp and its `eventKey` are fixed when the event happens and stay right; the details (`data`) do not. The "variable" components also put the game's shared variable objects into the dictionary rather than their values, so a late-sent entry shows the variable's value at sending time.

**Events affected:** DialogueEvent (including dialogue-node events), questEvent, argumentationEvent, argumentationNodeEvent, argumentationToolEvent, argumentationAnswerEvent, InputEvent, PuzzlePieceVisibleEvent, ObjectInterEvent, Soil Key Puzzle, soilMachine, WaterChamberEvent, TerasGardenBox, EndOfUnit, DEBUGMenu, and PlayerPositionEvent (since August 2026 `LogPlayerPositionEvent` reuses two fields, so queued position events show the latest position; the April source built them fresh, which is why an earlier version of this note called them safe). Topographic-map events and the dialogue and quest wrapper functions build fresh dictionaries and are not affected.

**How often** (schools' build v2.8.1, all accounts, log data since 2026-09-14; measurable where an entry carries its own key or timestamp):

- Dialogue-node events: 2.5% of those sent within about 5 seconds (1,357 entries, 123 accounts); 52% of those 5 seconds to 2 minutes late; 99.9% of those more than 2 minutes late.
- Quest events: 2.9% within about 5 seconds (101 entries); 9% at 5 seconds to 2 minutes; 98% over 2 minutes.
- PuzzlePieceVisibleEvent: 43% within about 5 seconds (86,872 entries, 110 accounts); 94% at 5 seconds to 2 minutes; 96% over 2 minutes.

"Within about 5 seconds" is normal play on a working connection; "over 2 minutes late" is a backlog saved on the device and sent later. Every build since 20260209 shows about 1–5% of dialogue-node events overwritten in normal play (the spring study's main build, 20260313-10763: 3.1%, 10,026 entries). The new logger builds do not fix it (build 12446: 85.8%).

**Fix.** Build a new dictionary for every event (in `LoggingData.ExecuteLogData` and `LogPlayerPositionEvent`), have the variable components put the variables' current values, not the variable objects, into it, and as a backstop copy `data` when an entry is queued. It belongs in the same build as G1. The true details of past events cannot be recovered. **Confirmed in the shipped source on 2026-09-30** (the game team's project copies): the schools' logger empties the queued entry with `logData.Clear()` at the top of each pass (`logData` is a class field), and the new logger keeps `toSend` as a class field that is never cleared. The fix is delivered: `mhs-updates/gamelogger-fix-093026/01-changes.md` §3.

### G3. Hardening, so a future slip cannot flood the log service

From note 03, to go in with G1 and G2:

1. When loading the saved queue, and again before sending, drop entries that have no fields or no `user_id`, and never attach the device block to such an entry.
2. Pause about a second after a 400 or 413 before the next request. Today those paths continue at once, which is correct only while every pass makes progress.
3. Remove sent entries by identity, not by count: dequeue while the head is one of the entries just sent.
4. Keep batches at 50 or fewer (the server's limit is 100). The drop-in already does.

### G4. The first event of each session goes out without a user id

The session's first event (the debug-menu state change) is logged before identity is set, goes out with `user_id: null`, and is refused (400). Each session loses it, on every device; 739 refusals were logged in the week to 2026-09-08. With the new logger (G1 defect B) such an entry also starts the refusal loop. **Fix:** set identity before the first event, or hold events until it is set. Details: `game-first-log-event-rejected.md`.

### G5. Two logger instances

The `GameLogger` component sits on the console-manager object in both core-systems prefabs, so a second instance wakes with the gameplay scene. `Awake` never assigns `_instance` (only the getter does), and its duplicate check destroys a game object that another singleton shares. With the sending flag, the lock and the user id static but the queue per instance, a path that leaves two loggers alive, or destroys the one holding the running send loop, strands the flag. None of the failures above needed this, but it should go: assign `_instance` in `Awake` and keep one logger. (Note 02.)

### G6. Game errors seen in the browser console

In headless test runs of v2.8.1 (with and without StrataHub's latest page) the game printed `ArgumentException: JSON must represent an object type.` (several times a session) and once `ArgumentOutOfRangeException: Index was out of range.` Their cause was not investigated at the time; they did not stop play. Status 2026-10-01: the first is the settings load's `null` body, fixed in the delivered `SettingsSaveManager.cs`; the second has not been seen again.

### G7. What the next build must pass before it reaches schools

From note 03 (the verification in note 01 checks only for `201` replies and would pass a build with G1):

1. Normal play for a minute: every event reaches the server once, and an idle game makes no requests. For the test account, entries equal distinct (event type, timestamp) pairs.
2. Log host blocked for two minutes, then unblocked: the backlog arrives once, the store empties, the request rate returns to normal.
3. A store wedged by v2.8.1 loaded before launch (specimens in the bundle): the first launch drains it with no refusal loop.
4. A batch with one entry lacking `user_id`: that entry is dropped once, the rest are recorded.
5. For G2: dialogue-node and quest events' details match their `eventKey`, and PuzzlePieceVisibleEvent's `data.timestamp` matches the entry's timestamp, in a burst and in a backlog.

### Meanwhile on the schools' build (v2.8.1)

A log request that fails twice, ten seconds apart, still stops the game sending for the rest of that session. Since 2026-09-28 StrataHub's play page removes the blocking empty entry before every launch, so the next launch sends the saved backlog and nothing needs clearing (verified live). G2 and G4 apply as described.

---

## 2. The research data (log service)

### D1. About 700,000 duplicate entries from testing the new builds

Defect A wrote the same events over and over:

- On 2026-09-25, from three dev accounts: the game team's standalone test build (329,248 entries of 57 distinct events), a tester on a Chromebook (272,811 entries of 60), and a second tester on a Chromebook (55,306 of 37, plus 22,337 re-sent from the old build's backlog, 50 distinct).
- On 2026-09-28, from the dev test member used for these tests (about 26,700).

All are on the dev site, none from students. **Proposal:** keep one copy of each distinct event (same account, event type, timestamp and details) and delete the rest, after a backup of the affected entries. The log service's ledger also holds about 2,400 rows of refused requests from these runs (1,568 from the standalone build, about 880 from 2026-09-28), which can go at the same time.

### D2. Overwritten details in the data already collected

G2 has affected every build since February 2026, including the spring study. Nothing can restore the true details. What is reliable in every entry: `eventType`, `user_id`, the entry's `timestamp`, `sceneName`, `version`, and `eventKey` where present. What is not reliable for the events listed under G2: `data`, most of all in bursts and in anything sent late.

How an analyst can recognise an overwritten entry:

- dialogue-node events: `eventKey` (`DialogueNodeEvent:<conversation>:<node>`) disagrees with `data.conversationId` / `data.nodeId`;
- quest events: `eventKey` (`<questEventType>:<questId>`) disagrees with `data.questEventType` / `data.questID`;
- PuzzlePieceVisibleEvent: `data.timestamp` differs from the entry's `timestamp`;
- any of the affected types: a run of consecutive entries from one account with identical details but different timestamps, or an entry that arrived long after its timestamp.

**Needed:** a data-quality note for the research team and the research partner with the above and the measured rates (§1 G2). Optionally, a one-time pass over the log data that marks entries matching the first three signatures, so analyses can filter on a field instead of re-deriving it.

### D3. Recovered backlogs arrive with mostly overwritten details

Before 2026-09-28 a wedged device's backlog was never sent. StrataHub's repair now lets it through, with the right type, time, student and key, but for the G2 event types almost every detail belongs to a later event (the saved copy was written after the later events had overwritten it). **Proposal:** the repair adds a marker (for example `"recovered": true`) to each entry of a repaired backlog. The log service keeps extra top-level fields, so analysts and the grader could tell exactly which entries to distrust. A small change in `repairUnityLogCache`; the marker is inserted into each entry's JSON text, which otherwise stays byte for byte.

### D4. The log service accepts a flood from one client

Defects A and B each drove one browser to 8–11 requests a second, with bodies up to 144 KB, and every refused request wrote a ledger row. A few classrooms on such a build would put real load on the log service and the database. **Proposal:** a per-account rate limit on the submit endpoint, and/or skipping an entry identical to one already stored for that account (same event type, timestamp and details) at insert time. Either needs design and a check against the batch path.

---

## 3. Grading

### R1. Six rules read details that can be overwritten

The grader matches most things by `eventKey`, which is never overwritten, and does not read dialogue or quest details. These rules do read details of G2 event types:

- u4p1: `Soil Key Puzzle` status Started / Finished;
- u4p3 and u4p4: `soilMachine` counts by floor and machine;
- u5p2: `WaterChamberEvent` counts;
- u4p6: `TerasGardenBox`, latest by details;
- u3p3: the `argumentationToolEvent` bonus.

In v2.8.1 data since 2026-09-14 these events show few suspect entries (consecutive entries of one account with identical details and different times): 42 for Soil Key Puzzle, 38 for argumentationToolEvent, 3 for soilMachine, none for the other two. **Needed:** a rule-by-rule check of whether any grade changed, with the students concerned.

### R2. Duplicates can inflate counts

Rules that count events (u4p3, u4p4, u5p2) would count the duplicates of D1. Only dev accounts are affected. After D1, regrade those accounts.

---

## 4. StrataHub

Done on 2026-09-28, for reference: the play page repairs a wedged store before every launch; the notices, Devices tab, teacher guide section and support checklist say nothing needs clearing; step-log detail accepts any value (commits b77c82e, e4c3c90, a2a397c).

- **S1. Incident records, admin visibility, notification** (plan §5.7, planned before this work): audit-log entries at detection and recovery, an incidents viewer with open / resolved and a note, email on new incidents plus a daily digest. Needs the project lead's go and the recipients.
- **S2. Detect a resend flood.** The logging check sees defect A as healthy (entries do arrive). The heartbeat check could compare an account's entry count with its distinct events over the session, or its arrival rate, and flag a flood. Worth doing only if a build with A could reach schools; D4 is the stronger guard.
- **S3. Rebuild the teacher guide PDF** with the "Game activity not recorded" section and the Devices view text (`docs/mission-hydrosci-teacher-guide/build-guide-pdf.sh`).
- **S4. A look on a real Chromebook:** normal play shows no bar; a relaunch after a Wi-Fi drop shows the repair line; the Devices tab with a real class.
- **S5. The Manage page's "Clear all downloads" and reset** say they remove local MHS data, but never touch the game's own store. Decide whether to say so, or to let the reset remove the game's store too (it would delete a saved backlog).
- **S6. Later:** members' heartbeats at 60 s instead of 30 s; a retention job for launch-record heartbeat arrays; where member launch records live and who sees what.

---

## 5. Testing and environment

- **T1. Pause play on the new-logger collections.** Every session on ClassifierIssuesTesting or LoggingTest floods the log service while it is open (G1). Three dev accounts still have a personal override to one of them (two on ClassifierIssuesTesting, one on LoggingTest). Reset those overrides, or ask their users not to play until a fixed build exists.
- **T2. Delete test runs on the dev site:** the seven "Automated check" device-test runs listed in the plan (§0, open item 3) and the run of 2026-09-28 whose id begins `fffffffff54f`.
- **T3. The project lead's no-network test machine** on the dev site (plan §0, open item 4) no longer needs clearing: its next launch repairs the store. Launching there once confirms it.
- **T4. The handoff bundle.** Superseded 2026-09-30: `mhs-updates/gamelogger-fix-093026/` carries the fixed files with G2 and G3 in, its `03-for-the-game-dev.md` is the page to send and `01-changes.md` the detail; the September bundle stays as the record of the investigation.

---

## 6. Decisions for the project lead

1. **Send G1, G2, G3 (and G4, G5) to the game team for the next build,** with the G7 verification as the bar for it to reach schools. Recommended: yes, as one build. (Files delivered 2026-10-01 as version 1.3; the hand-over itself is still to do.)
2. **Keep the schools on v2.8.1 until then.** Recommended: yes; StrataHub's repair limits a network drop to one session's delay.
3. **D1:** delete the duplicates (keep one copy of each event), after a backup. Recommended: yes.
4. **D2:** the data-quality note for the research team and the research partner; and whether to mark overwritten entries in the existing data.
5. **D3:** mark recovered-backlog entries from the repair. Recommended: yes.
6. **D4 / S2:** protect the log service against floods (rate limit or duplicate skip), and whether StrataHub should detect one.
7. **R1 / R2:** the rule-by-rule grading check, and a regrade of the dev accounts after D1.
8. **S1:** build the incident records and notification, and who receives them.
9. **T1:** reset the three overrides to the new-logger collections.
10. **T4:** done 2026-09-30, superseded by `mhs-updates/gamelogger-fix-093026/`.

---

## References

- Plan and status: `mhs-game-logging-silent-failure-plan.md` §0; launch readiness `mhs-remaining-work-plan.md` RDY-7.
- Support and teachers: `mhs-logging-support-checklist.md`; `../mission-hydrosci-teacher-guide/game-activity-not-recorded.md`.
- Game-team handoff: `mhs-updates/gamelogger-cache-overflow-091626/` — `01` (original write-up and drop-in), `02` (the two-failure wedge, shared-dictionary and two-instance findings, specimen), `03` (the new builds: defects A and B, the cause, hardening, verification), `specimen/` (captured stores, `send-loop-simulation.py`).
- First-event rejection: `game-first-log-event-rejected.md`.
- StrataHub code: `internal/app/resources/assets/js/mhs-steplog.js` (`repairUnityLogCache`, `repairUnityPrefsBytes`), `internal/app/features/missionhydrosci/templates/missionhydrosci_play.gohtml`, tests `tests/js/mhs-steplog-repair.test.mjs` (`make test-js`).
- Game code referred to: `Assets/Scripts/Systems/Logging/GameLogger.cs` and `LoggingData.cs` (the game project).
