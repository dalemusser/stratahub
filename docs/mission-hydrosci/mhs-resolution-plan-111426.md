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
| 1 | FC-1, FC-2 | Play gate at Schools J and L; gate false positives anywhere else | **Done 2026-11-14; a false positive found and fixed (a856be5, deployed).** J: the gate stopped its 2 launches on 11-11 ("Failed to fetch", a real block); on 11-12 two students got through on 4 GB Chromebooks, one of which was blocked on 11-04/05, so J's filter may have changed (recheck after the next class days). L: no launches since 11-05. **School S (new) was a false positive:** 34 gate stops and about 90 in-play "blocked" marks over 11-11..13 were "no answer within 8 s", and in most of them the same student's game events reached the log service within 3 minutes (a slow network, not a filter). Fix: a block is confirmed with a probe that waits 45 s, at the gate and in play; the Activity day ignores an in-play mark when that day's events arrived (Activity version 8). Real blocks show as "Failed to fetch" |
| 2 | FC-3 | B.2 damaged-file removals: removals followed by completed downloads, no loops | **Done 2026-11-14; a gap found and fixed (35418b8, deployed).** Loader repairs worked (removal, then the launch from the content server). At School D, 7 devices then crashed on every launch with "unityFramework is not defined" (50 records, 11-10 and 11-11): the framework file had a correct stored content-length over a broken body, which the cache check accepted without reading it. Now the check reads the framework (bytes, zero-fill, the name in a plain framework) and the play page removes a damaged framework and reloads once (outcome `launch-repair`). Field check: School D's next class days |
| 3 | FC-4 | B.3 held tail: whole-file restarts complete; decides B.7 | **Done 2026-11-14: fails at School T, so B.7 is needed.** Student-units restarted whole / completed afterwards since 11-09: T 36 / 5, O 6 / 3, S 5 / 4, U 1 / 1. At T the filter holds the whole-file fetch at the same point as the tail-only one (128 of 130 MB, every time), and after the second restart the download failed with "Quota exceeded" (I-1 on a fresh profile). B.3 helps where a stale partial was the cause (S, U); it cannot help where the filter holds every large object. **B.7 built and deployed the same day (2899271, service worker 1.0.18):** files of 32 MB or more are also stored as 8 MB pieces (25 builds of the 5 collections in use, done by the job at start); the service worker downloads and assembles them on both download paths. Verified on dev: Unit 3's data file fetched as its 18 pieces, Unit 2 (213 MB) assembled to the exact size, Unit 1 launched from an assembled file. Kill switch: `mhs_download_pieces_off = true`. Field check: School T's next class days (held restarts should stop; any piece that fails shows as a dropped connection) |
| 4 | FC-5 | B.4 one window: second game windows before and after 2026-11-09; discard notices | **Passes 2026-11-14.** Launches that saw another game window: 129 of 2437 (5.3 %) on 11-02..06, 70 of 4312 (1.6 %) on 11-09..13. Discards per day are unchanged (they depend on the device), and the discard notice and Activity counts are recorded |
| 5 | I-2 | The Daily Digest read against real data (does it mark the days the checks above find?) | **Data side done 2026-11-14:** the Digest reads the stored days; after the Activity v8 recompute, the student-days flagged "filter blocking" on 11-11/12/13 dropped from 43/61/75 to 8/4/4 (the launches the gate actually stopped), so School S's false marks go away as the recompute finishes. A look in a browser needs a staff sign-in |

Any check that fails becomes the next item, ahead of everything below.

### Phase 2: hand-offs for the game team (prepared here, sent by Dale)

| # | Item | What | Status |
|---|---|---|---|
| 6 | Save fixes | Hand-off to the game team | **Ready for Dale to send (2026-11-14).** The save fix v1.3 needs no hand-off: it is already in the team's 2026-10-29 code and in the 2026-11-02 release (G-5). What remains is `mhs-updates/save-resume-fix-111126` (G-2 and G-6; pushed, verified, dev-site test build `20261111-saveresume-SaveResumeFixTest`): send the link to its `03-for-the-game-dev.md`, with the two questions in G-6 (what the Unit 2 skip-list change 8 → 9 was for; keep the replay and skip lists append-only once saves exist) |
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
| 19 | DOC-1, DOC-2 stale feature and issues READMEs | P2 | |
| 20 | OPS-1 alert on download errors (the Daily Digest may already be the answer: decide) | P2 | **Closed 2026-11-14:** the Daily Digest is the alert (Dale's decision 2026-11-09, emails later); the runbook is the support checklist and the school IT one-pager |
| 21 | SEC-3, SEC-4, SEC-5 | P3 | **Done 2026-11-14** |
| 22 | UX-2, UX-3, UX-6, UX-7, UX-8 | P3 | **Done 2026-11-14** |
| 23 | DL-5, DL-6, DL-8, ARCH-3 | P3 | **Done 2026-11-14** |
| 24 | ACT-2, ACT-3, ACT-4 | P3 | ACT-2 done, ACT-4 closed (superseded by B.7) 2026-11-14; ACT-3 (a rollup for windows over 31 days) waits until someone needs it |
| 25 | S-2 service worker comment (with the next service worker change, never alone) | P3 | **Done 2026-11-14** with B.7 |
| 26 | ARCH-1, ARCH-2, TEST-1 (large: refactor or freeze; a JS test harness) | P2/P3 | |

### Phase 4: housekeeping

| # | Item | Status |
|---|---|---|
| 27 | H-1 database-backed tests for B.4 step 2 and the play gate | **Done 2026-11-14:** the local mongod (32 days up, 100 % CPU from deferred drops) restarted with `launchctl kickstart -k gui/<uid>/homebrew.mxcl.mongodb-community` (Homebrew refuses its untrusted tap); the handler tests pass |
| 28 | H-2 dev test data from 2026-11-09 and the save/resume tests: keep, labelled as test data | Kept. Added 2026-11-14 (test member on dev, not field data): launches at 23:15, 23:33, 23:40 and 23:55 UTC and the Unit 1/2/3 downloads of the B.7 check; one probe log entry and one probe save under game `probe-enforce-111426`, user `0123456789abcdef01234567` |
| 29 | mhsgrader's uncommitted rebuilt binary; three untracked zip bundles in mhs-updates | |

### Waiting on others (checks ready, nothing to do until then)

| Item | Waits on | Check when it arrives |
|---|---|---|
| D-1 Schools J and L filter | the districts | `filtercheck.js` after each class day (part of FC-1) |
| ~~Restricted-key enforcement in the log and save services~~ | — | **Done 2026-11-14:** in 14 days of journals the old key was used outside the developer id once (2026-11-03, an admin account playing from the Editor, now allowed too); enforcement on in both services; old key with any other id refused (403), current keys accepted (201) |
| B.7 smaller content objects | FC-4 | decided in item 3 |
| Member Status API task 7 | Abt | already receiving events; tick the task in its plan |
| Repeat students across semesters | Dale's decision | the enrollment-id design is written |
| I-1 quota error at the tail, I-3 real-device discard, RDY-1 iPad pass, RDY-6 guest profiles | a device | the steps are in the source plans |
| Device Test in production | real-device verification | loading-status plan §0 |
| mhs-web design decisions | Dale | `mhs-web/docs/unit2-design.md` §11 |
