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

### B.6 Logging-health count (Issue 6) — S

- **Correct the record first.** The "failed to count events: context deadline exceeded" lines come from the dashboard's count (`mhsdashboard/debug.go`), not from the heartbeat's logging-health check (which already asks a bounded "any entry since" question). Re-read the journal to confirm which callers time out.
- **What.** Replace that count with a bounded existence query (limit 1 on the `{game, user_id, _id}` index), or cap it; make sure the query it runs is covered by an index on DocumentDB.
- **Check.** No more deadline lines in the journal for a week.

### B.7 Smaller objects on the content server (Issue 2, solution 2) — L, after FC-4

- **What.** At build upload, store each large unit file as a series of objects of 8 MB or less listed in the manifest; the service worker fetches them in order, keeps them as the parts it already uses and assembles the single cached file the Unity loader expects. No tail to hold; a retry costs at most 8 MB.
- **Waits on.** B.3 a week in the field (FC-4). If B.3 alone clears the holds, B.7 drops to P3.
- **Check.** A school with a scanning filter completes downloads on its first day without a hold.

### Small follow-ups (S each)

- **S-1** The play gate's heading still says "Loading Unit N…" above "The game cannot start on this device yet": change the heading while gated.
- **S-2** The service worker's `sw-cache.js` comment still names `_verifyLargestFile` (now `_fileDamage`); fix with the next service worker change (a comment-only edit would reinstall the worker on every device).
- **S-3** Units page: when the last launch on this device was gated, say so on the units page too (before the student presses Launch), from the same server view.
- **S-4** Backlog item ACT-4 (`download-held` outcome) in [`mhs-remaining-work-plan.md`](mhs-remaining-work-plan.md): re-scope now that B.3 logs every hold wait and restart; DL-4 (heartbeat re-arm on `pageshow`) matters more with discards (B.4).

---

## 3. Game team package (one hand-off, not started) — M

Written as a `03-for-the-game-dev` page in the hand-off structure (Part 1 what to do, Part 2 other actions, Part 3 supporting information), every change listed as required. Coordinate with the save fix hand-off already prepared (`mhs-updates/gamesave-fix-102426`, version 1.3), which also touches START and saves.

| ID | Change | Why |
|---|---|---|
| G-1 | START does not continue without the save service: after a timeout it says plainly that the game cannot start because progress cannot be saved, and asks the student to tell the teacher. No "continue anyway". | Issue 3; the play gate rule. **Already in the game (2026-11-09):** the save fix v1.3 makes START wait with "Can't load your progress yet: no connection to the save service. Retrying in N s" and resume when the service answers; the unit never starts over. The one exception, a device resuming from its own last save while it keeps asking, is the same as being offline, which the rule allows; inside StrataHub the play gate stops a filtered device before the game loads. No change needed |
| G-2 | Save when the page becomes hidden (`visibilitychange`), since nothing runs at a discard | Issue 1: a discard loses everything since the last save. Some schools can only use 4 GB Chromebooks (2026-11-09), so this and G-3 are the remaining levers for them |
| G-3 | Memory: initial WebGL memory, one player voice, occlusion data, Unit 3 as well as Unit 2; the resume that re-activates the quest | Issue 1; the Unit 2 review (dialogue, occlusion, heap growth). Highest-value item for schools that only have 4 GB Chromebooks |
| G-4 | Optional flag from the page after a discard reload: skip the main menu and resume | Issue 1 (the record lists it as a game-side option; required only if Dale decides so) |
| G-5 | Confirm what the 2026-11-02 release contains: the garden-box fix in Unit 4, logger 1.3, the save fix | Issue 4. **Answered from the team's code snapshot of 2026-10-29 (changelist 12527, the release is 12543):** the save fix v1.3 files are there unchanged (with its `GameLogger`, which includes logger 1.3 and the transition-gate fix) and so is the garden-box fix. Only a check of the release build itself remains (the hand-off checks of each package on the dev site) |

Check: their build passes the hand-off's build checks on the dev site before it is activated for schools.

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
- **I-2 Nobody sees a problem day in time.** The Activity page surfaced Issue 1's student and Schools J/L days before anyone wrote, but nobody opens it during the school day. Design a daily digest of problem days (students with a problem flag, by school) to staff, or a fixed morning check.
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
5. B.6, S-1 to S-3.
6. FC-3 to FC-5 in the weekly review; then decide B.7.
7. I-1 to I-3 as devices and time allow.
