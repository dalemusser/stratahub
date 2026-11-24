# Mission HydroSci: plan for resolving every open item

**Written:** 2026-11-14. **Sources:** [`mhs-issues-remaining-plan-110926.md`](mhs-issues-remaining-plan-110926.md)
(issues record, field checks, game team package), [`mhs-remaining-work-plan.md`](mhs-remaining-work-plan.md)
(the older backlog: SEC, DL, ACT, UX, RDY, ARCH, DOC, OPS, TEST), and the hand-off packages in `mhs-updates`.
Those documents keep the detail; this page is the order of work and who acts on each item. Update the status
column here as items close, and the item's row in its source document too.

## 0. How to resume

Find the first row in §2 whose status is not done and is not waiting on someone else, and do it. Each item is
committed and pushed when finished; StrataHub changes are deployed with `stratahub_update/aws_update.sh` and checked
on dev. Field-check notes live outside the repository (`~/Desktop/mhs-reports-<date>/`) because they name schools.

## 1. Who acts

Every open item falls in one of three groups. The order in §2 follows from them.

- **We can finish it here:** StrataHub code, read-only checks of production data, documentation, housekeeping.
- **Dale's step:** sending a prepared hand-off to the game team, passing a school summary on, a decision.
  Everything around that step (the package, the summary, the options) is prepared here first, so Dale's part
  is the send or the choice.
- **Waiting on others:** the game team's builds, a district's filter change, Abt, a real device. These have a
  check ready to run the moment the thing arrives.

## 2. Order of work

### Phase 1: field checks of the 2026-11-09 changes (now: the first full class week has passed)

The play gate is the riskiest change in production (a false positive stops a class), so it is checked first.

| # | Item | What | Status |
|---|---|---|---|
| 1 | FC-1, FC-2 | Play gate at Schools J and L; gate false positives anywhere else | **Done 2026-11-14; a false positive found and fixed (a856be5, deployed).** J: the gate stopped its 2 launches on 11-11 ("Failed to fetch", a real block); on 11-12 two students got through on 4 GB Chromebooks, one of which was blocked on 11-04/05, so J's filter may have changed (recheck after the next class days). L: no launches since 11-05. **School S (new) was a false positive:** 34 gate stops and about 90 in-play "blocked" marks over 11-11..13 were "no answer within 8 s", and in most of them the same student's game events reached the log service within 3 minutes (a slow network, not a filter). Fix: a block is confirmed with a probe that waits 45 s, at the gate and in play; the Activity day ignores an in-play mark when that day's events arrived (Activity version 8), and since version 10 (2026-11-19) a mark that names the save service is checked against that day's saves in the save service (it keeps only the newest 5 saves per student, so for older days it may not know: then a log,save mark goes by the log events and a save-only mark counts). On 11-12..18 every marked launch whose saves could still be checked (32, 7 of them save-only) had saves arriving. Real blocks show as "Failed to fetch" |
| 2 | FC-3 | B.2 damaged-file removals: removals followed by completed downloads, no loops | **Done 2026-11-14; a gap found and fixed (35418b8, deployed).** Loader repairs worked (removal, then the launch from the content server). At School D, 7 devices then crashed on every launch with "unityFramework is not defined" (50 records, 11-10 and 11-11): the framework file had a correct stored content-length over a broken body, which the cache check accepted without reading it. Now the check reads the framework (bytes, zero-fill, the name in a plain framework) and the play page removes a damaged framework and reloads once (outcome `launch-repair`). Field check: School D's next class days |
| 3 | FC-4 | B.3 held tail: whole-file restarts complete; decides B.7 | **Done 2026-11-14: fails at School T, so B.7 is needed.** Student-units restarted whole / completed afterwards since 11-09: T 36 / 5, O 6 / 3, S 5 / 4, U 1 / 1. At T the filter holds the whole-file fetch at the same point as the tail-only one (128 of 130 MB, every time), and after the second restart the download failed with "Quota exceeded" (I-1 on a fresh profile). B.3 helps where a stale partial was the cause (S, U); it cannot help where the filter holds every large object. **B.7 built and deployed the same day (2899271, service worker 1.0.18):** files of 32 MB or more are also stored as 8 MB pieces (25 builds of the 5 collections in use, done by the job at start); the service worker downloads and assembles them on both download paths. Verified on dev: Unit 3's data file fetched as its 18 pieces, Unit 2 (213 MB) assembled to the exact size, Unit 1 launched from an assembled file. Kill switch: `mhs_download_pieces_off = true`. Field check: School T's next class days (held restarts should stop; any piece that fails shows as a dropped connection) |
| 4 | FC-5 | B.4 one window: second game windows before and after 2026-11-09; discard notices | **Passes 2026-11-14.** Launches that saw another game window: 129 of 2437 (5.3 %) on 11-02..06, 70 of 4312 (1.6 %) on 11-09..13. Discards per day are unchanged (they depend on the device), and the discard notice and Activity counts are recorded |
| 5 | I-2 | The Daily Digest read against real data (does it mark the days the checks above find?) | **Data side done 2026-11-14:** the Digest reads the stored days; after the Activity v8 recompute, the student-days flagged "filter blocking" on 11-11/12/13 dropped from 43/61/75 to 8/4/4 (the launches the gate actually stopped), so School S's false marks go away as the recompute finishes. A look in a browser needs a staff sign-in |
| 5a | Saves on slow networks | Played days without saves (found while adding the save check to Activity v10) | **Fixed 2026-11-19 (configuration).** Of the played days whose saves can still be checked, about 3 % have none: mostly short Unit 1 sessions (18 of 119 Unit 1 days, 1 to 2 % in Units 2 to 5; a session that ends before Unit 1's first save point), and School S. At School S the log and save services cut requests off at their 15 s read timeout: since 11-12, 59 state saves and 181 log submissions from School S's address arrived with an empty body and were refused as "Invalid JSON payload" (400), which the game treats as final and drops. `read_timeout` raised from 15s to 120s in the stratasave and stratalog configurations (`read_header_timeout` stays 10s), both restarted about 22:05 UTC, checked with a request whose body came 20 s late. Field check: the save service's refused saves (`ledger_entries`, 400 on /api/state/save) from School S after 11-19. Unit 1 measured 2026-11-19 (item 6a) |

**Recheck 2026-11-19** (class days 11-16..19 against 11-09..13; queries outside the repository):

- **Gate (items 1, School S):** no launch stopped by the gate at School S since the fix (25 before, all on days whose events arrived); 15 in-play marks, of which the Activity day flags 3. 182 to 200 School S students play each class day.
- **School J:** the filter no longer blocks: 16 to 20 students launched on each of 11-16, 11-18 and 11-19, none stopped, game events seen. School L: no launches.
- **Framework repair (item 2, School D):** no "unityFramework is not defined" anywhere since the fix, but School D has not played since (one download attempt on 11-18), so the repair itself is not yet seen in the field.
- **Pieces (item 3, School T):** the held downloads are gone (no whole-file restarts; Unit 3's 156 MB in 31 s), and every School T student who opened the game played (about 55 a day, 88 to 110 launches). What remains is a storage limit: School T's Chromebooks start a fresh profile every day, and a profile holds about one unit, so downloading the next units fails with "Quota exceeded" (631 download-failed records, one device retrying 336 times). The 9 students whose first unit failed on one device played that day on another. Follow-up: item 3a.
- **Activity v10 (item 5):** recomputing oldest day first; 1,660 organization-days done, 2,099 (10-02..11-15) left, at the job's pace. Flagged student-days 11-16..18: 4.
- **Save timeout (item 5a):** no refused saves since the change (after school hours so far); recheck after 11-20.

| # | Item | What | Status |
|---|---|---|---|
| 3a | Next-unit downloads on wiped profiles | On a profile that holds about one unit (School T), the page keeps downloading the next units and each fails with "Quota exceeded": noise in the records and the Devices tab, and wasted bandwidth. Options: after a quota failure on a unit the student is not playing, stop downloading further units on that device (no retry loop, not reported as a failure); and assemble pieces without holding the pieces and the whole file at once (now the peak is twice the file) | **Done 2026-11-19 (service worker 1.0.20).** The pages ask for the next unit as an ahead download; one that runs out of space is put off (status `deferred`: no failure record, no retry, its partial data removed, remembered for 12 hours on the device) and downloads when it becomes the current unit; a plain request (Download on the manage page) lifts it. Assembly that runs out of space is made again deleting each piece as it is read, so the file needs only its own size. Field check: School T's download-failed records and quota retries after 11-20 |

Any check that fails becomes the next item, ahead of everything below.

### Phase 2: hand-offs for the game team (prepared here, sent by Dale)

| # | Item | What | Status |
|---|---|---|---|
| 6 | Save fixes | Hand-off to the game team | **Ready for Dale to send (2026-11-14).** The save fix v1.3 needs no hand-off: it is already in the team's 2026-10-29 code and in the 2026-11-02 release (G-5). What remains is `mhs-updates/save-resume-fix-111126` (G-2 and G-6, and since 2026-11-24 version 1.1 with item 6a; pushed, verified, dev-site test build `20261111-saveresume-SaveResumeFixTest` for Units 2 to 4): send the link to its `03-for-the-game-dev.md`, with the two questions in G-6 (what the Unit 2 skip-list change 8 → 9 was for; keep the replay and skip lists append-only once saves exist) |
| 6a | Unit 1 save points | Unit 1's quest checkpoint moves only at 0 and 3 | **Done 2026-11-24: in the save/resume package, version 1.1 (`mhs-updates` a46434a).** Of 1,219 Unit 1 saves kept in production since 10-01 (428 students), 715 were made during *Gear Up*, 296 at checkpoint 3 and 202 at the end of the unit; 6 fall in between, so a student who stops after *Gear Up* and before 3 (median 18.5 minutes, up to 35) goes back to *Gear Up*. Saving turned on at the start of *TK, Reporting In!* (1), *Info and Intros* entry 4 (2) and *What Was That?* entry 1 (4), which already had their numbers and replay events: Step 5 of `03-for-the-game-dev.md`, three scene lines. Checked in the local copy: each step saves once at its number; resumed, 1 and 2 match the live world, 4 differs only in Jasper's animator (the same with the old number, listed as found and not changed); with the old number, a save at 1 leaves Toppo's walk-in trigger off. Editor tests T20 to T25 added: 25 of 25 pass; with the team's Unit 1 scene T20 to T22 fail. Unit 1 built from the copy (release profile, 2.4 min); the dev-site upload is held for Dale (item 6b) |
| 6b | Partial uploads inherit test builds | MHS Builds fills the units an upload leaves out from the newest collection, whatever it is | **Done 2026-11-24 (Dale chose to fix the inheritance).** The team's 11-19 collection `20261119-12563-Logging Bug Fixes` on dev holds our save/resume test builds of Unit 2 (v3.0.8) and Unit 3 (v3.0.4), because our test collection was the newest when they uploaded Unit 4 alone. Inheriting from the workspace's active collection was ruled out: dev's active collection is an October release, so the team's partial uploads would fill from old builds. Instead a collection can be marked **Test collection** (upload review, manual and edit forms; a Test label in the list and on the page), and uploads fill missing units from the newest collection that is not a test collection (`LatestBase`; the manual form pre-fills from it too). Store test `TestLatestBase_SkipsTestCollections`; admin guide updated. The team should hear that "Logging Bug Fixes" holds our Units 2 and 3 |
| 7 | G-5 | The 2026-11-02 release | **Garden box confirmed in the field (2026-11-14):** Unit 4 v3.0.2 has 436 launch records and no "memory access out of bounds" crash since 11-03, against 7 in 400 (v2.8.1), 6 in 478 (v2.8.4) and 7 in 237 (v2.8.9) before. The save fix and logger 1.3 are in the same code (snapshot 12527); nothing more to check on the release |
| 8 | G-3 | Memory: measure a local Unit 2 and Unit 1 build with Unity's memory tools (dialogue audio, occlusion data, texture formats on ARM), then a package, the same change in the local project and a dev-site test build | |
| 9 | G-4 | Skip the main menu after a discard reload: required only if Dale decides so | Dale's decision |

### Phase 3: StrataHub backlog, by priority

P1 first, then P2, then P3; inside a priority, small items first. Each row's detail is in the source plan.

| # | Item | Pri | Status |
|---|---|---|---|
| 10 | RDY-4 dev-sentinel user seeded into the default workspace | P1 | **Done 2026-11-14:** the row stays (joins from dev log and save data need it) but is created disabled, and an existing one is disabled at start, so it can never be signed into |
| 11 | DL-4 play heartbeat not re-armed on `pageshow` | P2 | **Done 2026-11-14** |
| 12 | DL-2 play overlay cleanup wipes manual pre-downloads | P2 | **Done 2026-11-14** |
| 13 | UX-4 `handleUnitComplete` has no `resp.ok` check | P3 (data loss, so early) | **Done 2026-11-14** |
| 14 | RDY-5 `totalUnits = 5` fallback | P2 | **Done 2026-11-14** |
| 15 | DL-7 `groupappstore` correlated `$lookup` on DocumentDB | P2 | **Fix ready, held for Dale (branch `dl7-group-pins`).** Production DocumentDB refuses the old pipeline ("$lookup argument 'let' is not supported yet") and the error was swallowed: no group pin has ever applied. The 6 pinned groups are test and team groups with no activity in 14 days, pinned to old test builds; merging makes those pins apply. Options: merge as is, or clear the stale pins first |
| 16 | DL-1 service worker download-creation race | P2 | **Done 2026-11-14** (service worker 1.0.19) |
| 17 | DL-3 post-login early-download driver | P2 | **Done 2026-11-14** (sign-in registers the service worker only) |
| 18 | UX-1 `aria-live` announces every percent | P2 | **Done 2026-11-14** |
| 19 | DOC-1, DOC-2 stale feature and issues READMEs | P2 | **Done 2026-11-14** |
| 20 | OPS-1 alert on download errors (the Daily Digest may already be the answer: decide) | P2 | **Closed 2026-11-14:** the Daily Digest is the alert (Dale's decision 2026-11-09, emails later); the runbook is the support checklist and the school IT one-pager |
| 21 | SEC-3, SEC-4, SEC-5 | P3 | **Done 2026-11-14** |
| 22 | UX-2, UX-3, UX-6, UX-7, UX-8 | P3 | **Done 2026-11-14** |
| 23 | DL-5, DL-6, DL-8, ARCH-3 | P3 | **Done 2026-11-14** |
| 24 | ACT-2, ACT-3, ACT-4 | P3 | ACT-2 done, ACT-4 closed (superseded by B.7) 2026-11-14; ACT-3 (a rollup for windows over 31 days) waits until someone needs it |
| 25 | S-2 service worker comment (with the next service worker change, never alone) | P3 | **Done 2026-11-14** with B.7 |
| 26 | ARCH-1, ARCH-2, TEST-1 (large: refactor or freeze; a JS test harness) | P2/P3 | ARCH-2 done 2026-11-14; TEST-1: the harness exists (60 JS tests); ARCH-1 recommendation: freeze during the school year, refactor in the summer (Dale decides) |

### Phase 4: housekeeping

| # | Item | Status |
|---|---|---|
| 27 | H-1 database-backed tests for B.4 step 2 and the play gate | **Done 2026-11-14:** the local mongod (32 days up, 100 % CPU from deferred drops) restarted with `launchctl kickstart -k gui/<uid>/homebrew.mxcl.mongodb-community` (Homebrew refuses its untrusted tap); the handler tests pass |
| 28 | H-2 dev test data from 2026-11-09 and the save/resume tests: keep, labelled as test data | Kept. Added 2026-11-14 (test member on dev, not field data): launches at 23:15, 23:33, 23:40 and 23:55 UTC and the Unit 1/2/3 downloads of the B.7 check; one probe log entry and one probe save under game `probe-enforce-111426`, user `0123456789abcdef01234567`; 2026-11-19 23:05–23:08 UTC: the 3a check (test member, Units 1 and 2 downloaded under a 600 MB storage limit, one download-failed record caused by the test) |
| 29 | mhsgrader's uncommitted rebuilt binary; three untracked zip bundles in mhs-updates | Binary committed 2026-11-14 (the build of 293a5fb, the deployed code). The zips (`end-of-game-ceremony-092426`, `gamelogger-cache-overflow-091626`, `gamelogger-fix-093026`) are hand-off copies of folders already in the repository; left in place for Dale to keep or delete |

### Waiting on others (checks ready, nothing to do until then)

| Item | Waits on | Check when it arrives |
|---|---|---|
| D-1 Schools J and L filter (J passes since 11-16) | the districts (L only) | `filtercheck.js` after each class day (part of FC-1) |
| ~~Restricted-key enforcement in the log and save services~~ | — | **Done 2026-11-14:** in 14 days of journals the old key was used outside the developer id once (2026-11-03, an admin account playing from the Editor, now allowed too); enforcement on in both services; old key with any other id refused (403), current keys accepted (201) |
| B.7 smaller content objects | FC-4 | decided in item 3 |
| Member Status API task 7 | Abt | already receiving events; tick the task in its plan |
| Repeat students across semesters | Dale's decision | the enrollment-id design is written |
| I-1 quota error at the tail, I-3 real-device discard, RDY-1 iPad pass, RDY-6 guest profiles | a device | the steps are in the source plans |
| Device Test in production | real-device verification | loading-status plan §0 |
| mhs-web design decisions | Dale | `mhs-web/docs/unit2-design.md` §11 |
