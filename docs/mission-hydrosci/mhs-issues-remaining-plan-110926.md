# MHS issues of Oct–Nov 2026: remaining work plan (2026-11-09)

**What this is.** Everything still open from the issues record [`mhs-issues-102826.md`](mhs-issues-102826.md) (Issues 1–6 and its plan) after the StrataHub items B.1–B.4 and the play gate shipped on 2026-11-09. Each item says what it is, why, its size (S/M/L), what it waits on, and how we will know it worked. School letters are the record's (the key is local only); no real hosts here.

**How to resume.** Read this page top to bottom, then the record's sections named in each item. Field checks (§1) come first because they decide whether shipped work needs a second pass. When an item moves, update its status here and in the record, commit and push.

**Rule that governs every item (Dale, 2026-11-09).** A student whose game activity and saves do not reach our servers does not play: unrecorded play is not participation in the study, and the game's store of unsent entries fills up. Block only when the evidence points at the school; a device with no internet and an outage on our side still play. See the record's "Play gate".

---

## 0. Done on 2026-11-09 (for reference)

| Item | What | Commits |
|---|---|---|
| B.1 | Probe of the game services in the game's own form, blocked-filter bar, `services_blocked` on records, Activity "no game activity reaching the server" | log service 55a28e0, save service 4a82027, StrataHub 4f61216 |
| B.2 | Download integrity: every file's size and the loader checked, damaged entries removed, damaged loader repaired at launch, failed next-unit downloads go to the units page | 140626a |
| B.3 | Held tail: whole-file restart after two held tail attempts or a partial from an earlier day (≤ 2 a day), 5-minute wait in the tail, "held at the tail since" pill; service worker 1.0.17 | 6aac09e |
| B.4 | One game window per device; discard notice; Activity discard and second-window flags (Activity version 5, summary version 2) | 00e7690, c8b7c67 |
| Gate | Play gate: the game does not start when the school blocks its log or save service; Activity version 6 | c7a0732, fbbcb54, 564219c |

---

## 1. Field checks (no code; read-only queries and the Activity page)

| ID | Check | Where to look | Passes when | If it fails |
|---|---|---|---|---|
| FC-1 | Play gate and B.1 at School J and School L | Next class day: launch records (`services-blocked` outcomes, `services_blocked`), the Activity page, the local `filtercheck.js` | Gated students see the message (records `services-blocked`), the schools show amber; or, once a district allows the hosts, `filtercheck.js` says OPEN and the gate lets everyone in | The probe or the evidence rule is wrong on that filter: read the gated records' `services` steps (host answered? server view?) |
| FC-2 | Gate false positives anywhere else | Fleet-wide count of `services-blocked` records per school per day | Only schools with a known block | A school that delivers events and saves yet has gated launches: loosen the evidence rule before the next class |
| FC-3 | B.2 damaged files | Step logs with "Removed … damaged file", crash records "createUnityInstance is not defined" | Removals followed by completed downloads; no repeated ReferenceError loops (School D was the case) | A removal loop on one file: the size guard or the build's file list |
| FC-4 | B.3 held tail | Schools D, K, C (and any new school's first days): "fetching … whole again" lines, the "held at the tail since" and "held on 2+ days" pills | Held students complete after one whole restart; persisted profiles stop re-holding every day | The filter holds whole-file requests too: B.7 and the district note become the only levers |
| FC-5 | B.4 one window and discards | `week.js` second-window counts before/after 2026-11-09; Activity discard pills on 4 GB Chromebook schools (B, C, F, K) | Second game windows drop sharply; discard notices appear where Chrome closes the game | Second windows persist: the unnamed-window edge (record, B.4 step 1) is commoner than thought |

Recheck cadence: after each school's next class day for FC-1 to FC-3, then weekly in the review (`week.js`).

---

## 2. StrataHub code

### B.5 Crash reporter (Issue 5) — S — **done, deployed 2026-11-09 (0efd457)**

- **What.** An unattributable `Script error.` (the browser masks errors from scripts of another origin; on School O's managed Windows laptops most likely a filter or monitoring extension) is reported as a crash and latches `crashReported`, which suppresses every real crash report for that page. Record it as a notice instead, never latch on it, keep crash reporting live; load the Unity scripts with `crossorigin="anonymous"` where they are served cross-origin so real messages arrive.
- **Check.** A test page that throws a cross-origin error, then a real one: the first is a notice, the second a crash record. In the field: School O's "Script error." records turn into notices and real crashes still arrive.
- **As built.** The play page logs a masked "Script error." (no detail) as a browser notice and never latches the crash reporter; the loader and its content-server retry load with `crossOrigin = 'anonymous'` (the content server sends CORS headers for every workspace origin; every cached copy was stored from a CORS request). `mhssteplog.IsCrashNotice` has the same rule and the Activity merge applies it to stored summaries (Activity version 7), so School O's older records become notices when their days are recomputed.
- **Verified on dev after the deploy:** launches from the downloaded files and with the unit's cache deleted both rendered with the CORS-mode loader; two masked errors became one browser notice and a real error after them was still reported as a crash (test data: one crash record and one crash event for the test member). Field check: School O's next days.

### B.6 Logging-health count (Issue 6) — S — **done, deployed 2026-11-09**

- **Correct the record first.** The "failed to count events: context deadline exceeded" lines come from the dashboard's count (`mhsdashboard/debug.go`), not from the heartbeat's logging-health check (which already asks a bounded "any entry since" question). Re-read the journal to confirm which callers time out.
- **What.** Replace that count with a bounded existence query (limit 1 on the `{game, user_id, _id}` index), or cap it; make sure the query it runs is covered by an index on DocumentDB.
- **Check.** No more deadline lines in the journal for a week.
- **Found (2026-11-09, journal 11-02 onward):** all 127 "failed to count events" lines came from one place, the dashboard's staff debug view (`mhsdashboard/debug.go`), on one day: it counted each student's events one by one inside one request, and once the request's time ran out every remaining student failed. The heartbeat's logging-health check logged no failure in that period. So Issue 6's conclusion that `logs_state` is partly noise was wrong; the empty and "quiet" states are real.
- **As built.** The debug view reads the counts in aggregates of 25 students on the `{game, user_id, _id}` index, within a 20-second budget that ends before the request's deadline; students it did not reach show "?" instead of 0, and a failure logs one line for the group. An all-time count is heavy for busy students (200 students with 654,677 events took about 27 s on DocumentDB), which is why it is chunked and budgeted rather than one query. Not verified in a browser (the debug view needs an administrator's session); the field check is the journal.

### B.7 Smaller objects on the content server (Issue 2, solution 2) — L, after FC-4

- **What.** At build upload, store each large unit file as a series of objects of 8 MB or less listed in the manifest; the service worker fetches them in order, keeps them as the parts it already uses and assembles the single cached file the Unity loader expects. No tail to hold; a retry costs at most 8 MB.
- **Waits on.** B.3 a week in the field (FC-4). If B.3 alone clears the holds, B.7 drops to P3.
- **Check.** A school with a scanning filter completes downloads on its first day without a hold.

### Small follow-ups (S each)

- **S-1** ~~The play gate's heading still says "Loading Unit N…"~~ **Done 2026-11-09 (48d5ca0):** while gated it says "<unit> cannot start yet".
- **S-2** The service worker's `sw-cache.js` comment still names `_verifyLargestFile` (now `_fileDamage`); fix with the next service worker change (a comment-only edit would reinstall the worker on every device).
- **S-3** ~~Units page warning~~ **Done 2026-11-09 (48d5ca0):** the play page remembers a gated launch on the device (cleared when the services answer) and the units page shows an amber line naming the hosts before the next Launch. Verified on dev with the simulated block.
- **S-4** ~~Re-scope ACT-4 and DL-4~~ **Done 2026-11-09:** both notes are in [`mhs-remaining-work-plan.md`](mhs-remaining-work-plan.md) (ACT-4 down to P3; DL-4 stays P2 and is the next small code item there).

---

## 3. Game team package (one hand-off, not started) — M

Written as a `03-for-the-game-dev` page in the hand-off structure (Part 1 what to do, Part 2 other actions, Part 3 supporting information), every change listed as required. Coordinate with the save fix hand-off already prepared (`mhs-updates/gamesave-fix-102426`, version 1.3), which also touches START and saves.

| ID | Change | Why |
|---|---|---|
| G-1 | START does not continue without the save service: after a timeout it says plainly that the game cannot start because progress cannot be saved, and asks the student to tell the teacher. No "continue anyway". | Issue 3; the play gate rule. **Already in the game (2026-11-09):** the save fix v1.3 makes START wait with "Can't load your progress yet: no connection to the save service. Retrying in N s" and resume when the service answers; the unit never starts over. The one exception, a device resuming from its own last save while it keeps asking, is the same as being offline, which the rule allows; inside StrataHub the play gate stops a filtered device before the game loads. No change needed |
| G-2 | Save when the page becomes hidden (`visibilitychange`), since nothing runs at a discard. **Analysis part 1 (2026-11-09): not safe as a general save.** A resume restores the savers' data (quest states and variables; the player's position is not saved) and then rebuilds the world by replaying the unit's checkpoint events up to the saved checkpoint. 57 quest responses across the game change the world on success or failure without saving (switch objects, enable colliders, unlock doors, start conversations; 25 of them in Unit 2); a save taken after one of them records the quest progress, but the resume does not redo the change. The safer lever under study: checkpoints the scenes already define (a checkpoint number and a replay event exist) where no save is taken; each has to be verified before it is proposed. **Correction (part 2):** the player's position and many of those objects *are* saved (built-in active/enabled/position savers, seen in real saves), so part 1 overstated the gap; but the savers and the checkpoint replay are applied in an undefined order (the save system iterates a hash set), so wherever a saver and the replay disagree about an object the outcome is arbitrary. At a designed checkpoint they agree; elsewhere they need not. Each candidate is checked by simulating the replay against the state at that point. **Existing checkpoints, checked 2026-11-10** (Unity harness: every real save resumed with the replay first and last, and, for Unit 2, live play from one save to the next compared with resuming the next save): Units 1, 2 and 5 resume the same in either order; Unit 3 (a supply crate, 10 saves) and Unit 4 (a puzzle collider, 13 saves) depend on the order (G-6). Unit 2 resumes differ from live play in ways students can see: the map hint panel is on after a resume at 6, the map fog of war differs at 6, 7, 10 and 12, the map hint text is lost at 4, and five Unit 2 quests (the final quest and four side quests) are listed as Unit 1 quests, so every Unit 2 load resets them. At checkpoint 14 that leaves the final quest unassigned, its waypoint missing and the wrong character talkable; in production 11 of the 15 students who reloaded there still finished the unit. **New save points, Unit 2 (tested 2026-11-11):** five steps already have a checkpoint number and a replay event but do not save (2, 3, 5, 9, 11; they split gaps whose median is 7–12.5 minutes). With saving switched on there and the checkpoint-first fix in place, each one resumes like live play except for differences the existing save points already have (all on the G-6 list), and playing on from a resumed save reaches the next checkpoint in the same state as uninterrupted play. **Package written 2026-11-11:** the five save points are part of `mhs-updates/save-resume-fix-111126` (with G-6), verified together in a local build. Dev-site test build done 2026-11-11: collection `20261111-saveresume-SaveResumeFixTest` (Units 2–4 with the fix, not active; Unit 2 loads and starts with the test member). Next: the hand-off | Issue 1: a discard loses everything since the last save. Some schools can only use 4 GB Chromebooks (2026-11-09), so this and G-3 are the remaining levers for them |
| G-6 | **The existing save code (found 2026-11-09, G-2 analysis):** (1) a save stores only the checkpoint number; what that number replays is the unit's checkpoint replay list and skip list in the build that resumes it, so any change to those lists between releases changes how every existing save resumes, with nothing to catch it. Between the late-September and 2026-10-29 snapshots only one such change happened: Unit 2's skip list went from 3, 4, 5, 8 to 3, 4, 5, 9 (events unchanged), so Unit 2 saves at checkpoints 10–14 resume differently since then (event 8 now always replays, event 9 only as the current checkpoint); it may be a deliberate fix, to be confirmed with the team. (2) The save library applies the savers in an undefined order (a hash set), so the checkpoint replay runs before some savers and after others; wherever an existing checkpoint's replay disagrees with what the savers recorded, the resumed state is arbitrary. The G-2 simulation (2026-11-10) found two: a supply crate in Unit 3 and a puzzle collider in Unit 4. | Asks for the team: treat the replay and skip lists as frozen once saves exist (append-only, never reorder), say what the Unit 2 skip-list change was for, and apply the checkpoint saver before every other saver, so the replay cannot overwrite what the savers recorded (in Unit 3 the checkpoint saver applying after the crate saver is what hid the crate). Tested in a local build 2026-11-10 as a change to MHS code only (the purchased save library is untouched): every Unit 2, 3 and 4 save then resumes the same whatever the order, the crate and collider come back as saved, and Unit 2 resumes are unchanged except one character at checkpoint 8 that now matches live play; make the Unit 3 crate and Unit 4 collider savers agree with the replay; list the five Unit 2 quests under Unit 2; set rather than toggle the map fog of war; keep the map hint panel and text consistent on resume. **Package written and pushed 2026-11-11: `mhs-updates/save-resume-fix-111126`** (checkpoint replay first in MHS code, the five quests under Unit 2, fog flag set, Unit 2 replay events 4–6 for the map hints, and G-2's five save points; every change required). Verified together in a local build: every save of Units 1–5 resumes the same in either saver order and re-saves unchanged; the 16 Unit 2 live-vs-resumed pairs no longer show the map hint, fog, quest-reset or checkpoint 14 differences; no new errors; 19 Editor tests, 19/19 with the fix and 13 failing without it. Left to the team with the reasons (cosmetic, no saved data affected): Jasper visible at the camp after resumes at 1–7, Anderson's model at 1, Toppo's Usable at 14, the map highlight under the fog at 6–7, and scene overrides of replay calls the prefab no longer has (Unity drops them; appending to those events would revive them). Dev-site test build done 2026-11-11: collection `20261111-saveresume-SaveResumeFixTest` (Units 2–4 with the fix, not active; Unit 2 loads and starts with the test member). Next: the hand-off |
| G-3 | Memory: initial WebGL memory, one player voice, occlusion data, Unit 3 as well as Unit 2; the resume that re-activates the quest | Issue 1; the Unit 2 review (dialogue, occlusion, heap growth). Highest-value item for schools that only have 4 GB Chromebooks |
| G-4 | Optional flag from the page after a discard reload: skip the main menu and resume | Issue 1 (the record lists it as a game-side option; required only if Dale decides so) |
| G-5 | Confirm what the 2026-11-02 release contains: the garden-box fix in Unit 4, logger 1.3, the save fix | Issue 4. **Answered from the team's code snapshot of 2026-10-29 (changelist 12527, the release is 12543):** the save fix v1.3 files are there unchanged (with its `GameLogger`, which includes logger 1.3 and the transition-gate fix) and so is the garden-box fix. Only a check of the release build itself remains (the hand-off checks of each package on the dev site) |

Check: their build passes the hand-off's build checks on the dev site before it is activated for schools.

**How game changes are done (Dale, 2026-11-09).** Measure first. If a change is warranted: (1) a document for the game developers describing the change (a hand-off package in `mhs-updates`), (2) the same change made on our local copy of the Unity project and tested there, (3) a test build on the dev site that others can play, test and comment on. The game team will most likely make the change on their own copy, so the document and the tested build are the deliverables, not the local change alone.

**Measured 2026-11-09 (read-only, production data since 2026-10-27; local queries).**
- *G-2, play lost at a discard.* 255 discards found; in the 209 whose run had game events, the play between the student's last accepted save and their last game event before the window went away was: median 3 min, upper quarter 12 min or more, worst tenth 24 min or more; 63 discards (30 %) lost more than 10 minutes. 146 discards came from runs with no accepted save at all before the window went away (median run 2 min, worst tenth 26 min). **A change is warranted.**
- *G-3, memory.* The game's WebAssembly heap is a fixed 495 MB in almost every run of every unit (594 MB in some), plus 210–300 MB of the page's JavaScript heap: about 750 MB per game window. Discards per 100 launches on 4 GB Chromebooks: Unit 1 27.6, Unit 3 18.8, Unit 2 11.1, Unit 4 4.6, Unit 5 2.3; on devices with more memory at most 1.4. **A change is warranted.** Correction after reading the project (same day): the heap is not a fixed setting. Every release build profile starts it at 32 MB and grows it geometrically (20 % steps, at most 96 MB each) up to 2048 MB, and a WebAssembly heap never shrinks; 495 MB is the step most runs end on, so the game really grows to 400–495 MB. The starting-memory setting is not the lever; the savings have to come from what a unit loads (the Unit 2 review: dialogue audio 92 MB, occlusion data 29 MB, texture formats on ARM GPUs), measured with Unity's memory tools on a local build.

---

## 4. Schools and districts

| ID | What | Status | Next step |
|---|---|---|---|
| D-1 | School J and School L: allow the log and save hosts (all methods, no body inspection), then the public Device Test | Sent 2026-11-09; not acted on as of the 11-04/05 class days | Recheck with `filtercheck.js` after each class day (FC-1); follow up if the next class day is still blocked |
| D-2 | One page for IT before a school's first day (content, log and save hosts; no tail scanning; keep site data; tab-discarding exception; 8 GB or Intel devices where possible) | **Done:** made available to the schools (reported by Dale 2026-11-09; real hosts, so the page lives outside this repository) | Point new schools to it before their first day |
| D-3 | The 4 GB Chromebook advice and the Admin console tab-discarding setting to Schools B, F, C, K (record A.4) | **Done:** provided to the schools (reported by Dale 2026-11-09). Some schools have no devices other than their 4 GB Chromebooks | None for the schools; what helps those classes now is on our side and the game team's: B.4 (one window, discard notice) shipped, G-2 (save on hide) and G-3 (memory) in §3 |
| D-4 | School D's damaged loader (record A.3) | Handled by B.2 (repaired on the next launch) | Close after FC-3 |
| D-5 | Schools C, D and E (tail hold since late October; School E never answered) | Other people deal with the schools; Dale passes them the findings (2026-11-09). No school contacts come to us | When a school has a problem, write a short summary for the people who deal with the schools (what the data shows, what the school's IT should change, the one-pager) and give it to Dale to pass on |
| D-6 | School B: why its browsers keep no site data between sessions (ephemeral or guest profiles?) | Unanswered; goes through the same people as D-5 | Include it in School B's summary if it comes up again; the data (new device ids every launch) is the evidence |
| D-7 | Which districts changed their filters for the tail hold, and what they changed (vendor, setting) | **Closed, unknown:** asked, no reply (2026-11-09) | None. The one-pager stays generic (no vendor setting to quote); B.3 and B.7 must work without that knowledge |

---

## 5. Open investigations

- **I-1 Quota error at the tail (School O, School B).** "Quota exceeded" at 88–94 % with gigabytes of reported quota free. The final write briefly needs twice the file's size, which explains it only if those (not persisted, possibly in-memory) profiles have a real limit far below the reported one. Needs a device that reproduces it; then decide whether to drop parts while they stream (halves the peak, loses progress if that write fails).
- **I-2 Nobody sees a problem day in time.** The Activity page surfaced Issue 1's student and Schools J/L days before anyone wrote, but nobody opens it during the school day. **Decided (Dale, 2026-11-09):** a daily digest as a StrataHub feature: a list of days with a visual mark on the days that have problems worth looking at; selecting a day shows that day's digest. Emails later, not now. **Built and deployed 2026-11-09 (8b94ca7):** "MHS Daily Digest" (menu, and a link on MHS Activity) at `/mhsactivity/digest`: the last 30 days (not before the stored history), newest first; a day with a school that has a problem is marked "Look at this" with the day's problem pills, a quiet day "No problems", an empty day "No activity". A day's page lists the schools to look at (pills and counts) and the quiet ones, each linking into MHS Activity for that school and day. Same access and scope as MHS Activity; it reads the same stored and live days. Covered by unit and render tests; not yet looked at with real data in a browser (needs a staff sign-in).
- **I-3 Issue 1, real-device confirmation.** A real discard on a 4 GB Chromebook with the notice showing, and the one-window change on a Chromebook, a frozen window and the installed app (only desktop Chrome tested).

---

## 6. Housekeeping

- **H-1** Run the database-backed tests for B.4 step 2 and the play gate (`mhsactivity` handler and CSV tests, `missionhydrosci` handler tests) once the local database answers normally again (it took ~30 s per index on 2026-11-09).
- **H-2** Dev test data from 2026-11-09 on the test member (gated launches, simulated discards, "Chrome closed the game 2 times" on the dev Activity day, one test crash record and crash event from the B.5 check): leave or delete, but do not read it as field data.
- **H-3** Related work elsewhere that touches the same code paths: the restricted-key enforcement flip in the log and save services (waits on the game team's build with the new keys), and the save fix hand-off (G-package coordination).

---

## Suggested order

1. FC-1 and FC-2 after the next class day at any school (the gate is the riskiest change: a false positive stops a class).
2. ~~B.5~~ done 2026-11-09.
3. The game team package (G-1 to G-5), together with the save fix hand-off.
4. Per-school summaries for the people who deal with the schools, as problems come up (D-5, D-6). D-2 and D-3 are done; D-7 is closed (unknown).
5. ~~B.6, S-1, S-3, S-4~~ done 2026-11-09; S-2 with the next service worker change.
6. FC-3 to FC-5 in the weekly review; then decide B.7.
7. I-1 to I-3 as devices and time allow.
