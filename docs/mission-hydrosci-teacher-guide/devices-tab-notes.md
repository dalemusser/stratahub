# Devices View: plan, rationale, and editor notes

Companion to `devices-tab.md` (the guide-ready text). Written 2026-09-13 against the
current `mhsdashboard` and `missionhydrosci` code. The source guide is
`sources/August 2026 Mission HydroSci Teacher Guide.pdf` (121 pages, Canva).

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
| Reading the unit dots | The cells are the only non-obvious part of the view. Two caveats matter in practice (ring and check follow the account, not the device; dots are as of Last Seen). |
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
- **A unit-level progress scan.** Not the view's main job, but green check marks give a
  fast "who has finished which unit" count without reading the Progress grid.

## 4. Behavior verified in code (for whoever edits or updates the text)

- **Rows.** One `mhs_device_status` document per (workspace, user, device); a device
  is a random id in the browser's local storage, so a new Chromebook, a new Chrome
  profile, or cleared browser data creates a new row. Rows for one student share the
  name cell. No device documents → "No device data". (`dashboard.go loadDeviceMap`,
  `mhsdashboard_grid.gohtml`.)
- **Device type.** From the user agent: CrOS → Chromebook; touch + iPad/Macintosh →
  iPad; Macintosh → macOS; Windows; Android; Linux; else Other.
  (`missionhydrosci_units.gohtml detectDeviceType`.)
- **PWA.** `display-mode: standalone` at report time, i.e. the student launched from
  the installed app on that visit. It is a fact about the visit, not a permanent
  install flag.
- **Unit cells.** Two independent signals drawn together. The dot's fill is the
  device's `unit_status`: `cached` → solid blue, `downloading` → blue outline,
  anything else (`not_cached`, `partial`, `error`, `retrying`, `stalled`) → gray. The
  wrapper carries the grade-derived progress: `current` (first unit not completed) →
  green ring, `completed` (every progress point's latest grade is `passed`) → green
  check at the top right. Neither hides the other, so "current unit, not downloaded
  here" is a gray dot in a green ring. (`dashboard.go` unit progress block;
  `mhsdashboard_grid.gohtml`; CSS `.mhs-device-cell*` in `mhsdashboard_view.gohtml`.)
- **When a device reports.** Only from the Mission HydroSci launcher page: once after
  the initial cache check of all units, and again whenever a unit download completes
  on that page. The play page does not report, and the next-unit download that runs at
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

1. **Completed requires every point `passed`.** A `flagged` point keeps a unit from
   ever showing the Completed check on this view, so a student two units ahead can
   still show the earlier unit's ring. The Progress view treats flagged as
   completed-with-concern. Either count `flagged` as complete for the unit-level
   mark, or use the grader's `currentUnit` (already loaded for the Progress view)
   instead of deriving it. The text has one sentence explaining the current
   behavior; drop it if this changes.
2. **Dots can be stale for a whole play session.** Reporting from the play page at
   unit completion (or a lightweight heartbeat) would make Last Seen mean "last
   played" and keep the download dots current. The text explains the refresh step
   instead.
3. **Dashboard and launcher storage thresholds differ** (70/90 versus 60/80/90). Not a
   problem for teachers, but the numbers in the guide are the dashboard's; keep them
   in sync if either changes.

## 6. Questions about the sample screenshot

In the screenshot used for this work, nearly every device shows Unit 5 as Downloaded
while Units 3 and 4 are Not Downloaded, for students whose current unit is 1 or 2.
The auto-download pipeline keeps only current + next + manual downloads, so that
pattern is unexpected. Possible causes: a collection whose unit ids differ from the
dashboard's headers, a Set current to Unit 5 during earlier testing, or manual
downloads. Worth a look before the screenshot (or one like it) goes into the guide, so
the figure does not contradict the text.

## 7. Editor checklist

- Replace the *The Devices View* paragraph on p. 119 with `devices-tab.md`.
- Add a screenshot from a workspace with test names; the working screenshot shows
  real first names and should not be used.
- Keep the guide's boxed **Tip** style for the two tips.
- The section does not repeat the dashboard URL; the appendix intro already gives it.
- If items in section 5 are implemented, update: the last paragraph of *What the
  Devices view tells you about progress* (item 1) and caveat 2 under *Reading the
  unit dots* (item 2).
