# Devices View: plan, rationale, and editor notes

Companion to `devices-tab.md` (the guide-ready text). Written 2026-09-13 against the
current `mhsdashboard` and `missionhydrosci` code; the Devices tab itself was redesigned
the same day and the text describes the final design. Figures live in `images/`. The source guide is
`sources/August 2026 Mission HydroSci Teacher Guide.pdf` (121 pages, Canva).

## 0. Status and how to resume (2026-09-13)

**Done.** `devices-tab.md` is final for the design as built and approved; the three
figures in `images/` and the compressed spliced PDF match it. The Devices tab itself was
redesigned the same day (14 commits; see §4 for the encoding) and the launcher now
reports download trouble, so the tab's amber badge is reliable.

**Handoff.** Give the guide's author the markdown, the figures, and the compressed PDF
(the Devices pages are 119-1 … 119-5; page 119 rebuilt with only its Analytics section).
The inserted pages approximate the Canva typography; final layout is done in Canva.

**Deployment.** In production as of 2026-09-13 (Dale deployed and confirmed), so the
guide can go to teachers as written.

**Open, in priority order.**
1. Watch a live fall class in its first week: with the new derivation and reporting,
   bands should read cleanly and amber should appear only for real download trouble.
2. Product candidates in §5 (stale dots during a play session, storage thresholds, the
   empty PWA column, row height). None block the guide.
3. If the Devices tab changes again: rerun `TestWriteDevicesFixture` and the capture
   steps in §7 for new figures, then `build-guide-pdf.sh` for the PDF (do not commit the
   ~99 MB output; the repo keeps only a compressed copy).

## 1. Where it goes in the guide

The guide's Appendix, *How To Use the Teacher Dashboard* (pp. 118–120), covers the
dashboard's views in order: Progress, Devices, Analytics, Survey. The Devices View
section is one paragraph on p. 119. The new text replaces that paragraph in place.

Two corrections to the existing paragraph are folded into the replacement:

- It lists the device types as "Mac, Windows, iPad". The dashboard reports
  Chromebook, iPad, macOS, Windows, Android, Linux, or Other. Chromebooks are the main
  classroom device and were missing from the list.
- It says a high Storage percentage means "a device is using a high percentage of
  memory". The column is browser storage (downloaded units and local data versus the
  browser's storage allowance for the site), not RAM. The new text explains what the
  number is and, more usefully, how to read the two amounts to tell "device is full"
  from "MHS is holding too many units".

The guide's other sections use short paragraphs, tables, and boxed **Tip** callouts.
The new text follows that pattern so it drops into the Canva layout. The section
runs about three pages at the guide's density; the *Solving problems* table and the
*Quick reference* can be trimmed if space is tight, in that order.

## 2. What the section covers and why

| Part | Purpose for the teacher |
|---|---|
| Opening + columns table | Orientation. Every column named and defined once. |
| How to read a row / Reading the unit cells | The cells are the only non-obvious part of the view: background is the student (green completed, striped purple current), dot is this device. Amber is the one state that means something is wrong; a small gray dot in the purple cell is normal on a device the student is not using. Two caveats matter in practice (the band follows the account, not the device; dots are as of Last Seen). |
| Device details | Rarely needed, but turns "the Chromebook is weird" into a report tech support can act on. |
| Storage | The one column teachers are likely to misread. Explains the 70/90% colors, the auto-download pause at 90%, and how to distinguish "device full" from "MHS full". |
| Two-minute check before class | The highest-value routine: catches downloads, full devices, stale devices, and never-logged-in students before they cost class time. |
| Solving problems | Symptom-first table. Teachers hear the symptom; the table maps it to the column to look at and the action. |
| What it tells you about progress | Sets expectations: unit-level snapshot only; points to Progress and Analytics for detail. |
| Quick reference | One-glance lookup for the Canva sidebar or a boxed panel. |

## 3. Value to teachers (what the view actually buys them)

- **Class time.** Downloads are the most common start-of-class delay. A gray dot on
  today's unit, seen the day before, is a download that can happen while students
  arrive instead of when the teacher says "launch".
- **Faster triage.** Most in-class complaints resolve to one of: still downloading,
  device full, wrong account, new device. Each has a distinct signature on this view
  (dot color, red bar, name mismatch, second row). The teacher can tell the student
  what to do without leaving the dashboard.
- **Better support requests.** Device details give tech staff and the MHS team the OS,
  browser, memory, and storage in one click, so "it doesn't work" becomes a report.
- **Verifying a fix.** Ask the student to open MHS, refresh, and confirm the dot or bar
  changed. The section explains why the refresh is needed (reports are sent on open
  and on download completion).
- **A unit-level progress scan.** Not the view's main job, but green check circles give a
  fast "who has finished which unit" count without reading the Progress grid.

## 4. Behavior verified in code (for whoever edits or updates the text)

- **Logs column (added 2026-09-17).** From the newest "launch-ok" member launch
  record on that device in the last 30 days (`mhs_device_tests`, kind `member`)
  plus the launcher's last reading of the game's PlayerPrefs store
  (`mhs_device_status.playerprefs_bytes`). Amber "!" with "Not recorded" when the
  heartbeat check found no log entries after about five minutes of play
  (`logs_state: none`), "Cache full" when the page saw the game's
  "Failed to save cached logs" console error (`cache_errors > 0`), "Nearly full"
  when the store is ≥ 90 % of Unity's 1 MB cap; a green ✓ and the `logs_seen_at`
  time when entries were confirmed; a dash otherwise. The tooltip carries the
  remedy. (`mhsdashboard/logging_health.go`, `missionhydrosci/logs_health.go`;
  plan: `docs/mission-hydrosci/mhs-game-logging-silent-failure-plan.md`.)

- **Rows.** One `mhs_device_status` document per (workspace, user, device); a device
  is a random id in the browser's local storage, so a new Chromebook, a new Chrome
  profile, or cleared browser data creates a new row. Rows for one student share the
  name cell and are sorted by `last_seen`, newest first. No device documents → "No
  device data". (`dashboard.go loadDeviceMap`, `mhsdashboard_grid.gohtml`.)
- **Device type.** From the user agent: CrOS → Chromebook; touch + iPad/Macintosh →
  iPad; Macintosh → macOS; Windows; Android; Linux; else Other.
  (`missionhydrosci_units.gohtml detectDeviceType`.)
- **PWA.** `display-mode: standalone` at report time, i.e. the student launched from
  the installed app on that visit. It is a fact about the visit, not a permanent
  install flag.
- **Unit cells.** Background = the student, from grading: `completed` (every progress
  point's latest grade is `passed` or `flagged` — flagged is completed-with-concern on
  the Progress tab and must not stall the band) → solid green; the grader's own
  `CurrentUnit` (not "first unit not completed", so a skipped unit stays untinted)
  → purple with a faint diagonal hatch; a completed unit that is also the grader's
  current unit shows as completed; no grades → no tint. The band spans every row of
  the student's block; tinted cells inside a block drop the row divider so it is
  continuous. Dot = this device's `unit_status`: `cached` → 13px blue disc,
  `downloading` → blue ring, `error`/`stalled`/`retrying` → amber badge with an
  exclamation mark and a dark ring (hover text names which), `partial`/`not_cached`
  → 8px gray dot (hover text distinguishes an interrupted download, which resumes
  when MHS is next opened on that device). Every state has a non-color cue (solid vs
  hatched; disc size, ring, badge) so the tab survives grayscale and red–green
  color blindness; verified with feColorMatrix simulations. Colors: light green-200 /
  purple-200, gray #6b7280; dark #1e4d31 / #503580, gray #64748b; blue #3b82f6 both.
  Rows have no zebra fill: a faint line between a student's devices, a 2px line
  between students drawn from both sides (the dashboard's dark-mode `!important`
  border override repaints row lines, so these carry `!important` too).
  (`dashboard.go` unit progress block and `orderDevices`; `mhsdashboard_grid.gohtml`;
  CSS `.mhs-unit-*`, `.mhs-device-*` in `mhsdashboard_view.gohtml`; tests in
  `devices_test.go`.)
- **When a device reports.** Only from the Mission HydroSci launcher page: once after
  the initial cache check of all units, again whenever a unit download completes on
  that page, and whenever a download hits `error` or `stalled` (first report at once,
  then at most one per unit per minute while the failure loop continues; recovery is
  not reported separately, the eventual `cached` report clears it). The play page does not report, and the next-unit download that runs at
  unit completion inside the play page is not reported until the launcher is next
  opened. `last_seen` is the server time of the latest report. Stale = more than 7 days.
- **Storage.** `navigator.storage.estimate()` usage and quota. Dashboard bar: orange
  above 70%, red above 90%. Launcher: bar yellow above 60%, red above 80%; auto-download
  of the next unit is suppressed at 90% or more and the "Low storage" notice shows.
  The launcher keeps current + next + manually downloaded units and auto-cleans the
  rest.
- **Manage downloads & data** (`/missionhydrosci/manage`) is entry-gated for members
  in `keyword` and `staffauth` workspaces; staff unlock lasts a sliding 10 minutes by
  default. Per-unit Download and Clear live there, not on the launcher.
- **Grading latency.** mhsgrader scans every 5 seconds, so green dots follow gameplay
  closely; the dashboard itself refreshes every 30 seconds.

## 5. Things the text works around (candidates for product changes)

1. **Dots can be stale for a whole play session.** Reporting from the play page at
   unit completion (or a lightweight heartbeat) would make Last Seen mean "last
   played" and keep the download dots current. The text explains the refresh step
   instead.
2. **Dashboard and launcher storage thresholds differ** (70/90 versus 60/80/90). Not a
   problem for teachers, but the numbers in the guide are the dashboard's; keep them
   in sync if either changes.
3. **The PWA column is a column of dashes** for most classes. It could hide itself
   when no device in the group has the app installed.
4. **Row height** is set by the two-line Storage cell; one line would fit more
   students on a screen.

## 6. The sample screenshot, resolved

The April class that puzzled us (Unit 5 downloaded on nearly every device while
Unit 2 showed as current) turned out to be a class that skipped Unit 3 and finished
on Unit 5. The old derivation marked Unit 2 "current" forever because one of its
points was flagged rather than passed. Counting flagged as finished and taking the
current unit from the grader fixed it, and the bands now read: 1, 2, 4, 5 completed,
3 untouched.

## 7. Editor checklist

- Replace the *The Devices View* paragraph on p. 119 with `devices-tab.md`.
- Figures: `images/devices-tab-light.png` (the whole view), `images/devices-table-light.png`
  (the unit cells, referenced in the text by the fictional names), and
  `images/devices-tab-dark.png` (dark theme, optional). All are rendered from the real
  template with fictional students, so there is no student data in them. To
  regenerate after a change:

  ```
  MHS_DEVICES_FIXTURE_OUT=/tmp/devices.html go test ./internal/app/features/mhsdashboard/ -run TestWriteDevicesFixture
  ```
  then serve `/tmp` with a copy of `internal/app/resources/assets` beside the file,
  open it in a browser, run `switchTab('devices')`, and capture `#mhs-dashboard` and
  `#mhs-tab-devices` (add the `dark` class to `<html>` for the dark figure). The
  students and states are defined in `devices_fixture_test.go`.
- An updated PDF to hand back alongside the markdown: `August 2026 Mission HydroSci
  Teacher Guide (Devices View update) compressed.pdf` (12 MB, Git LFS) is the copy kept
  in the repo; the full-size build output (~99 MB) lives in cloud storage and is not
  committed. It is the Canva export with the Devices pages spliced in after page 118,
  numbered 119-1 … 119-5 so the existing page numbers and table of contents stay valid,
  and page 119 rebuilt with only its Analytics section. `build-guide-pdf.sh` regenerates
  the full-size file from `devices-tab.md` and `images/` (needs pandoc, playwright-cli,
  qpdf, poppler). The inserted pages approximate the guide's typography; the Canva
  layout is still the place to finish them.
- Keep the guide's boxed **Tip** style for the two tips.
- The section does not repeat the dashboard URL; the appendix intro already gives it.
- If items in section 5 are implemented, update caveat 2 under *Reading the unit
  cells* (item 1) and the Storage thresholds (item 2).
