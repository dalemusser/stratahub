# MHS Activity: cross-school activity overview

A staff page (`/mhsactivity`, menu "MHS Activity") that answers "who was active today and how did it go?" for Mission HydroSci across every school in the workspace: who opened the units page, downloaded (complete, failed, held), launched (ok, stalled, failed), played, crashed, and what the devices looked like, per school, group, student and day. It reproduces in-app the analysis that until October 2026 was done by hand with database scripts after each school day.

## 0. Status and how to resume

| Phase | Ships | State |
|---|---|---|
| 0 | `internal/app/system/mhssteplog` (step-log parser, per-record summary, hold and crash-notice classifiers); write-time `summary` on member load records; structured `detail` keys on the client's step-log entries | DONE 2026-10-13, deployed 2026-10-13 01:49 PT |
| 1 | `models.MHSActivityDay`, `internal/app/system/mhsactivity` (`ZonesFor`, `DayBounds`, `ComputeDay`, the merge), the page at `/mhsactivity` computed live for today, yesterday, the last 7 days or a custom range of at most `mhs_activity_live_max_days` (7), cached per zone-day for `mhs_activity_live_cache_ttl` (60 s), policy, routes, menu, config keys | DONE 2026-10-13; verified against a copy of production data for 2026-10-12 and 2026-10-07..09 (see §6) |
| 2 | `internal/app/store/mhsactivitydays` (`mhs_activity_days`, `mhs_activity_marks`), `RollupDay`, the job `mhs-activity-rollup` (every `mhs_activity_rollup_interval`, the last `mhs_activity_catch_up_days` closed days), the backfill (`/mhsactivity/admin`, admin only: a day range, optional stamping of summaries, optional recompute of final days, progress polled every 5 s, stop), the page's stored path (a closed day whose organizations all have a final mark comes from the store; a request may compute at most `mhs_activity_live_max_days` live; windows up to `mhs_activity_max_days`) | DONE 2026-10-13; backfill run on the local copy for 2026-10-05..12 (see §6) |
| 3 | CSV exports (`/mhsactivity/schools.csv`, `/mhsactivity/students.csv`: one row per student-day and download episode), the Device Tests viewer's **Member** filter (`user=`) with a Records link per student for admin/analyst, "School activity →" on the MHS Dashboard's info bar (admin/coordinator), the e2e journeys (admin nav, analyst menu + page, leader refused) | DONE 2026-10-13 |
| 4 | student-day timeline (one student's merged, de-duplicated step log for one day across devices); docs done 2026-10-13 (`ai/context.md`, `docs/docs_index.md`, the remaining-work plan's ACT-1..4, the download risks note, the drops investigation's §0) | timeline OPEN |

**Decisions (2026-10-13, project owner):**
- Hybrid data path. An organization-local day that is still open (its end plus `mhs_activity_close_after`, default 6 h, has not passed) is computed live on each request from the raw records, with a short in-process cache. A closed day is rolled up once by the background job into `mhs_activity_days` and never recomputed unless a backfill is re-run. One compute function serves both paths.
- Audience: superadmin, admin and analyst see the whole workspace; coordinators see their assigned organizations (viewscope, as the Members Report does); leaders and members get 403 and no menu link.
- Day boundaries follow each organization's `time_zone`; "Today" on the page means each school's local today.

**To resume:** read this section, then §2 (definitions) and §3 (data model). After deploying phase 2, open `/mhsactivity/admin` as an administrator and run the backfill from the first day with load records (the form offers `mhs_activity_backfill_start`) to yesterday with "stamp summaries" on; the job keeps the last three days current from then on. Phase 3 starts from `internal/app/features/mhsactivity/report.go` (the rows the CSV writes) and `internal/app/features/viewers/views/devicetests.go` (the `user` filter the student links need). Phase 0's parser is the contract everything else builds on: `mhssteplog.Parse` turns one step-log entry into a typed event (preferring the entry's `detail` map, falling back to the sentence), `mhssteplog.Summarize` folds a record's entries into `models.MHSMemberSummary`, and `templates_pin_test.go` fails when a page sentence the parser depends on changes. Before changing a sentence in `mhs-delivery.js` or the play page, change the parser and the pin test with it.

**Why the measurements rule out live computation for multi-day windows (production, 2026-10-13):** member load records average 18 KB (their step logs); a day has about 260 of them, a week 3000, a month 8800. The game log collection holds about 100 k Mission HydroSci events per school day; a per-student aggregate over 7 days took 8 s and over 30 days 187 s. One day is cheap; a window is not.

## 1. What the page shows

Window selector: Today (default), Yesterday, Last 7 days, Custom (at most 31 days). Organization and group selects narrow the view. `staff=1` reveals staff rows (teachers' and administrators' own launches), which are never counted as students.

1. Summary strip: students opened / played / blocked; downloads complete / failed / held / held then done; launches ok / stalled / failed; crash records real vs browser notices; schools active / with problems; freshness ("computed 2 min ago (live)" or "stored 06:12", partial banners).
2. Schools table, one row per active organization, linking to the school view and to the MHS Dashboard.
3. School view: groups table and students table with per-unit cells ("U1 done 132 MB", "U2 held at 202/214 MB", "U3 failed network"), launches, crashes, devices (count, new), current unit, last seen; links to the dashboard group and, for admin/analyst, to the Device Tests viewer filtered to the student.
4. Per-day table (organization × day) when the window spans more than one day.
5. CSV export of schools and student-days.
6. Student-day timeline (phase 4): the merged, de-duplicated step log of one student on one day across devices.

Colour rule: amber only for real problems (failed or held downloads, launch failures, blocked students, device churn); green for played / complete; gray for zero counts and "held then done"; muted text for browser notices; no red. Every state has a text cue.

## 2. Definitions

All queries filter `workspace_id`. Students are `users.role == "member"`. Teachers (`leader`) are attached to their groups with membership role `leader`, so rosters filter both the membership role and the user role; non-members get staff rows. A record without `organization_id` takes the user's current organization; users without one are skipped.

| Signal | Source | Rule |
|---|---|---|
| Opened | `activity_events` page views of `/missionhydrosci/units`, any member record, `mhs_device_status.last_seen` in the day, or any game log event | union |
| Episode | one (user, device, unit, local day) | the unit of download counting; records are merged per episode (counters by max, booleans by or), never counted raw, because `download-failed` is re-posted at most every 10 min while retries continue and records from one tab overlap |
| Download complete / failed | outcomes `download-complete` / `download-failed` / `download-switched`; the summary's per-unit facts | state per episode: `cached`, `complete`, `held_done`, `held`, `failed`, `in_progress` |
| Held | last progress within `HoldTailMB` (16) of the total, then resumes at that same point (2+), stalls or retries, and no completion; a later completion the same day makes it held then done with the hold duration | `mhssteplog.Summarize` (the October 2026 holds all stopped exactly 12 MB before the end of each file) |
| Launched | `launch-ok` once per "Game is rendering"; a watchdog warning followed by rendering within 10 min is a recovered stall; `launch-failed` and its paired `crash` count once | |
| Played | game log events for the student in the day (user ids chunked, `_id` range) | fallback when the log service is unavailable: launch records with logs seen or 4+ heartbeats, flagged partial |
| Crash | "Crash (type, phase): message" entries | notice = `unhandled_rejection` whose message mentions pointer lock, a permissions check, fullscreen or AudioContext (`mhssteplog.IsCrashNotice`); everything else real |
| Blocked | opened, did not play, and had a failed or held download or a launch failure | the headline problem count |
| Device churn | `mhs_device_status.created_at` within the day = a new browser profile | organization flag when 80 % or more of student-days used a new device over 5+ student-days |

Day attribution: every step carries its own time; events land on the local day they happened, so a record stored after midnight that carries the previous evening's entries counts there. Records are read for the day plus `close_after`.

De-duplication: events keyed (device, time, step, message); the client collapses consecutive identical entries and moves their time, so same (device, step, state, message) within 90 s also collapse for warn/fail entries.

## 3. Data model

Phase 0 (done):
- `models.MHSDeviceTest.Summary *MHSMemberSummary` (`summary`), written by `HandleMemberStepLog` for every new member record: per-unit download facts (`units[]`: total, furthest progress, drops, stalls, retries, fails, last error class, complete with time / size / duration / rate, cached, held with the point and the hold duration, background-fetch refused / frozen), launch events (`launch[]`: loader ok/fail, Unity ok/fail, watchdog warn/fail, rendering, each with its time), crashes (`crashes[]` with the notice flag), the latest storage line, and the page's log context (device id, service worker, delivery and download-mode versions). `MHSMemberSummaryVersion` is bumped when the reading changes so a backfill can re-stamp.
- Client `detail` keys (message text unchanged): download requesting `{unit, version, total_mb}`, progress `{unit, mb, total_mb, pct, mbps}`, resumed `{unit, resumed_mb, drops}`, stall `{unit, quiet_s}`, waiting `{unit, waiting_s}`, retry `{unit, attempt, in_s, error_class, path}`, fail `{path, unit, version, error_class, reason}`, complete `{unit, version, mb, seconds, mbps}`, cached `{unit, cached}`, background fetch refused `{unit, kind, reason}` and frozen `{kind, unit, quiet_s, mb, …}`, storage `{used_mb, quota_mb, persisted}`; play page loader `{source, unit}`, Unity started `{unity_s, unit}`, watchdog `{idle_s, phase, unit}` (+ `fallback`), failures `{reason, unit}`, crash `{type, phase, unit}`, rendering `{unit}`.

Phases 1–2 (done): `mhs_activity_days` (one document per workspace, user and local day: opened / launched / played / blocked, units, devices, launch, crashes, log, flags, current unit, partial / missing / final / computed_at / version) with indexes `{workspace_id, user_id, day}` unique, `{workspace_id, day, organization_id}`, `{workspace_id, organization_id, day}`; `mhs_activity_marks` (one per workspace, organization and day: final, partial, missing, attempts, students, computed_at, version) with a unique `{workspace_id, organization_id, day}` index. A day computed with a source unavailable is stored but its marks stay non-final (the job retries it up to 12 times, then stores it final with the partial flag, which the page shows). Organization and group totals are summed in Go from student-day documents; windows are capped at 31 days.

## 4. Computation (phases 1–2, done)

`ComputeDay(ctx, deps, workspace, day, zone)` builds the day documents for every student and staff member of the organizations sharing one time zone: people and groups, member records (summary when present, otherwise parsed), device status, page views, game log activity, current unit. Optional sources never fail the day; they set `partial` and `missing`. The page computes open days live (cache 60 s, keyed by workspace, zone and day; scope is applied afterwards in memory) and reads closed days from the store once every organization of the zone has a final mark. `RollupDay` is the one place that stores a zone-day and its marks; the job `mhs-activity-rollup` calls it for the last few closed days of every workspace and zone, and the admin backfill calls it for a range (oldest first, one at a time, at most one backfill at a time), optionally stamping summaries onto the range's load records first so later computations never read steps.

## 5. Configuration

`mhs_activity_enabled` (true), `mhs_activity_close_after` (6h, never below the client's 6 h log retention), `mhs_activity_live_cache_ttl` (60s), `mhs_activity_live_max_days` (7, how many days one request may compute live), `mhs_activity_max_days` (31, the longest window), `mhs_activity_rollup_interval` (15m), `mhs_activity_catch_up_days` (3), `mhs_activity_backfill_start` ("2026-09-08", the first day with load records, offered by the backfill form). Defaults are safe; the deployment configuration needs no change.

## 6. Verification

Phase 0: `go test ./internal/app/system/mhssteplog/` (sentence and detail parsing, hold classification including held then done and the far-from-the-end stop, crash notices, the pin test against the page sources), `go test ./internal/app/features/missionhydrosci/`, `make test-js`.

Phase 1: `go test ./internal/app/system/mhsactivity/ ./internal/app/features/mhsactivity/` (day bounds across the November 2026 daylight-saving change, the merge of overlapping records, stall recovery, day attribution, a database-backed `ComputeDay` with a student, a teacher and another school, the pure report aggregation, role gates for admin, analyst, leader, member and coordinator, the coordinator's organization scope, and the rendered page). Then a comparison against a local copy of production data (the collections the computation reads, exported with `mongosh` as Extended JSON and imported with `mongoimport`; the game log reduced to id, user, time and event type) for the days the October 2026 hand reports covered. The page agreed with the hand counts where the definitions match: for the Monday the schools' students, groups, downloads, fresh browser profiles and the one recovered slow start were identical; for the three days before, the students who never got in differed by one per school because the hand count included a student without any download signal, and the hold classification found 44 of 62 unit downloads held at the school with the scanning filter against the hand count of 54, with the same completion counts. After the first comparison the hold classifier was changed to judge the hold at the point where the connection keeps picking up again, not at the final progress line, and "blocked" over a window was redefined as a problem day with no play at all in the window.

Phase 2: `go test -p 1 ./internal/app/store/mhsactivitydays/ ./internal/app/system/mhsactivity/ ./internal/app/features/mhsactivity/` (the store's idempotent replace and stale-row removal, marks, the job with a fake clock storing a closed day and leaving the open one, a partial day retried and not final, force, the backfill over a range with stamping, the page serving a stored day and refusing a window that needs too many live days, the admin page and a backfill started from it). The packages create every index per test; run them one package at a time on a laptop, or the local database times out. Then the backfill on the local copy for 2026-10-05..12 and a 7-day window read back entirely from the store (§6 numbers unchanged).

Phase 3: `TestCSVExports` (both files, the admin's record links, a leader refused) and `TestDeviceTestsMemberFilter` in the viewers package; the e2e journeys (`tests/e2e/test_admin_journey.py`, `test_analyst_journey.py`, `test_leader_journey.py`) cover the menu link, the page opening for an analyst and the refusal for a leader.

Later: the timeline.

## 7. Risks

- Sentence parsing is brittle by nature; the pin test and the `detail` keys are the guard.
- A held download has no record of its own until a later outcome from the same tab; the classifier reads it from that later record. A `download-held` outcome posted after some minutes in the tail is a follow-up.
- Two tabs on one device share a device id with separate logs; merging by max undercounts (rare in class).
- The 300-entry cap can drop the "Requesting" line; the total is also read from progress lines.
- "Played" uses the log service's receive time; events buffered offline count on the day they arrive.
