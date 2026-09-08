# Mission HydroSci Device Test — Admin Guide

**Audience:** StrataHub admins and MHS staff who set up the test for a school and read the results.
**Companion:** `tester-guide.md` / `tester-guide.html` is what you give the person at the school.
**Design background:** `../mission-hydrosci/mhs-loading-status-and-unit2-device-test-plan.md`.

## 1. What the device test is

A public web page, one per workspace, that lets a school check whether a
device will run Mission HydroSci before the study starts. It downloads and
launches **Unit 2**, the largest and historically most troublesome unit, the
same way a student's launcher does: same service worker and cache, same
content server, same game log and save services, same play page. If a device
gets through Unit 2 it will handle the rest of the game.

The tester needs **no account and no login**. They open the link, fill in a
short form, and the page does the rest. Every step of the run is recorded and
shown to you in the **Device Tests** view.

## 2. Prerequisites

- A Unit 2 build has been uploaded through **MHS Builds** (it must be on the
  CDN).
- Either the workspace has an **active collection** that includes Unit 2, or
  you will pick a specific Unit 2 build in the settings below.
- Admin access to the workspace's Site Settings.

## 3. Turning it on

1. Sign in to the workspace as an admin and open **Site Settings**.
2. Find the section **Mission HydroSci — Device Test**.
3. Tick **Enable the device test**.
4. Choose the **Unit 2 build to run**:
   - **Use the active collection's Unit 2** (the default) runs whatever
     Unit 2 the workspace's active collection uses, which is what students
     get. This is the right choice for a readiness check.
   - Or pick a specific build from the list. Only builds already uploaded to
     the CDN are listed, each with its version, build identifier, size and
     upload date.
5. Save the settings.

The **Link to give a school** field in the same section shows the URL:

```
https://<workspace-host>/missionhydrosci/devicetest
```

The workspace host is the address the workspace is served from; the exact
link is shown ready to copy in Site Settings. The address is fixed per
workspace; there is nothing to create per school.

While the test is disabled the page says "The device test is not enabled for
this site right now." Turn it off again after the school's testing window;
the recorded runs stay.

## 4. Giving the link to a school

Send the school the link and the tester guide. Ask them to:

- test on the **actual devices and network** students will use (a Chromebook
  in the classroom on the school Wi-Fi, not a laptop at home);
- keep the tab open while the download runs;
- use a keyboard and a trackpad or mouse (the game needs them, and the
  opening part has to be played through before the Unit 2 content loads);
- keep the sound on and report any audio problem, since whether sound works
  is one of the things we need to learn;
- play at least until the Unit 2 content has loaded and they are moving
  around in it, and finish the unit if they can;
- quote the six-character **test code** shown at the end (or on the run
  page) when they report anything.

If the school has a content filter, it helps to tell their IT staff in
advance which hosts the game needs: the StrataHub workspace host, the content
server, and the game log and save service hosts. Their names are in the
deployment configuration, and the run page's Status details panel shows each
one as it is checked; a blocked host is detected within seconds and named,
so a first run that fails on this is still informative.

## 5. What the tester sees

1. **Landing page** — what the test does, what is recorded, and the form:
   school or district and name (required); role, email, device type,
   school-managed or not, network type, notes (optional).
2. **Run page** — one Unit 2 card. The download starts by itself. The
   **Status details** panel is open and shows every step with timings:
   service worker, game list, storage, content server and game-service
   checks, download method, download progress with rate and time left,
   waiting countdowns, automatic retries, file verification. Downloads never
   give up: every failure schedules the next attempt with a visible
   countdown; "Retry now" only skips the wait. A **Send report** box lets the
   tester add a note. When Unit 2 is already on the device from an earlier
   run (same device, same build), the download row says so, the files are
   verified and the content server and game services are probed anyway, so
   no row is left blank.
3. **Launch** — the play page opens in a new tab (in place when installed
   as an app), and the run page stays open behind it with the questionnaire
   revealed. The game tab relays its Launch and Game entries back to the run
   page (marked "game tab" in the panel and in Copy report), so the panel
   there keeps following the loader, Unity start and rendering. The current
   step also shows under the game tab's own loading bar; if a launch step
   fails that panel opens with the log and a Copy report button.
4. **Test complete** — when the game reports the unit finished, the page
   shows the test code and a link back to the run page.
5. **Post-play questions** — once Launch is pressed the run page shows a
   short questionnaire (it is also there on any later visit to the run page,
   which the game's back arrow and the Test complete link lead to): did the sound play, did the keyboard and pointer
   work, did the picture look right, how did it run, how far they got, and
   notes. Answers can be updated later. These cover what the page cannot
   observe by itself; the sound answer is required.

## 6. Reading the results

Open **Device Tests** from the menu (admins and analysts), or go to
`/views/device-tests`.

**The table.** One row per run:

| Column | Meaning |
|---|---|
| Started | When the run began (UTC) |
| School | From the form (or the organization, for member records) |
| Tester | Name and role from the form |
| Device | Detected device type, platform and browser |
| Network | Detected connection type, speed and latency |
| Path | **Background** (Chrome's background download) or **Direct** (inside the service worker; the tab must stay open). A ↻ means the page had to switch from background to direct |
| Download | Size and time, with speed and stall/retry counts in the tooltip |
| Stage | How far the run got (below) |
| Sound | The tester's answer on whether sound played (Worked / No sound / Problems), or — if not answered |
| Last problem | The most recent failing step and its message |
| Duration | From start to the last activity seen |

**Stages**, in order: Started → Downloading → Downloaded → Launching →
Gameplay → Completed. **Failed** means the most recent step is a failure.
Because the page keeps retrying, a run can move out of Failed again; the
"Last problem" column keeps the latest failure either way.

**Filters:** date range, stage, kind, device, school (prefix), sound
answer, test id.
**Chips** above the table: runs, reached gameplay, completed, failed now,
last run.

**The detail.** Click a row. It shows:

- the form answers, the detected device and network, device id, IP address
  and user agent;
- the download summary (path, size, time, speed, stalls, retries, whether it
  switched) and the launch timings (loader, Unity start, first frame);
- **Game telemetry for this run**: the number of log events the game sent,
  first and last event times, scenes seen, the grader's progress points and
  current unit;
- the tester's reports;
- the full step timeline; and the diagnostics snapshot (browser
  capabilities, WebGL renderer, storage, cache inventory, client hints,
  battery, page timing).
- **Tester's answers** and notes from the post-play questionnaire.
- **Ended** and **Last heartbeat**: while the game runs, the page sends a
  heartbeat every 30 seconds (game memory, JavaScript heap, frame rate, tab
  visibility) and a final one when the tester leaves. A run whose beats stop
  without that final one is shown as **Page stopped responding at …** in the
  Last problem column, with the last memory figures; that is what a crashed
  tab ("Aw, Snap") looks like from the server, since a crashed page can
  report nothing itself. The detail lists the beats, newest last, so a
  memory climb before a crash is visible.
- **Download this run as JSON** for the complete record.

**Exports.** *Export CSV* gives the table for the current filter. *Export
JSON* gives every matching run in full, for analysis.

### What counts as a pass

- **Reached gameplay** (stage Gameplay or later): the device downloaded the
  unit, started Unity, and rendered the game. This is the check that
  matters most.
- **Completed**: the tester finished Unit 2. This confirms the device holds
  up through a full unit.
- A run stuck at Downloading with many retries, or Failed with a content
  server or storage problem, points at the network or the device rather
  than the game; the Last problem column and the step timeline say which.

### Member load records

The same view holds a second kind of record, under the **Kind** filter:
**Member load record**. A signed-in student's launcher stores its own step
log when a download completes or fails (at most every ten minutes while
retries continue), when a launch fails, succeeds, or crashes while loading,
and when the student presses **Send report** in the Status details panel.
The row shows the member's name, organization and outcome; the detail has
the same step timeline. This is how a field report like "connection error in
room 12" can be read step by step without asking the student for anything.

## 7. How runs are identified

Each run gets a 24-character id that is also the `user_id` the game sends to
the log and save services. It starts with `ffffffff` followed by 16 random
hex characters. A real user id is a MongoDB ObjectID whose first eight
characters are its creation time; `ffffffff` decodes to the year 2106, so no
real id can ever start with it. Anything in the game services' data with such
an id is device-test data, and the `mhs_device_tests` collection says which
run. In code: `models.IsMHSDeviceTestUserID`.

No account is created and nothing is written to users, organizations,
groups or progress. The MHS dashboard therefore does not list test runs; the
Device Tests view is the place to look.

## 8. Data, privacy and retention

- Run records hold the form, the detected device, browser, storage and
  network details, the step log (newest 300 entries, each stored once: a
  batch resent by a closing tab is dropped), the download and
  launch summaries, the heartbeats from the game page (newest 240, about
  two hours), the tester's post-play answers and notes, the tester's Send
  report notes, and timestamps. They are kept indefinitely.
- Tester name, email, IP address and the questionnaire notes are visible
  only in the admin/analyst view. Nothing about the tester reaches the game
  services; the game only ever sees the run id.
- Steps, diagnostics, summaries and reports are accepted until the unit is
  completed or for 24 hours after the start; heartbeats and the
  questionnaire are accepted for the full 24 hours, so a tester who keeps
  playing after finishing, or answers the questions later, is still
  recorded. After that the run's page still works but nothing more is
  stored.
- Members' load records (kind "member") hold the same kinds of data minus
  the form and questionnaire, plus the member and organization ids.
- The full list of what is recorded is in the tester guide, so the school
  sees the same description you do.

## 9. Limits and settings

- **Start quota.** Starting runs is limited per client IP address:
  `mhs_device_test_start_limit` runs per `mhs_device_test_start_window` in
  `config.toml` (defaults 10 per 10 minutes; a change needs a service
  restart). Only pressing "Start the Unit 2 test" counts; downloads,
  retries, launches and reporting never do. A whole school behind one
  address shares the budget, so raise it before a large group tests at once.
- **Download timing** values the pages use are also in `config.toml`
  (`mhs_frozen_switch_ms`, `mhs_fallback_stall_ms`, `mhs_keepalive_ms`).
- **Game-service keys.** The play page renders the log and save service
  keys as it does for students; anyone with the URL can read them from the
  page source, which was already true of every student browser. Keep the
  test disabled when not in use.
- **Maintenance mode** does not block the device test or the content route.

## 10. Questions that come up

**A logged-in user opened the link. Does that matter?** No. The run is
anonymous either way; the person's account, progress and saves are untouched,
and the pages look the same for everyone: a slim site header instead of the
usual menu, and no session activity tracking, so a long test cannot end in a
sign-out. The run page never removes or interrupts other units on the device,
so a teacher testing on a student's Chromebook does no harm.

**Two testers on the same network started at once. Are they separate?**
Yes; each run has its own id and record. They share the start quota.

**I changed the build while someone was mid-test.** Their run keeps the
version it started with. The next run uses the new choice.

**The landing page says "not enabled" but the switch is on.** The workspace
has no active collection with a Unit 2 and no build was chosen, or the chosen
build no longer exists. The server log gives the exact reason.

**Where do I see what the game itself logged?** In the run's detail under
"Game telemetry for this run", pulled live from the log service and the
grader by the run's id.
