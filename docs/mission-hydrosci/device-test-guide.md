# Mission HydroSci — Unit 2 Device Test guide

**Audience:** MHS staff who set up the test for a school and read the results.
**Design and rationale:** `mhs-loading-status-and-unit2-device-test-plan.md` §4.

## What it is

One public URL per workspace that lets a school check whether a device can
run Mission HydroSci, with no account and no login:

```
https://<workspace-host>/missionhydrosci/devicetest
```

The page downloads and launches Unit 2 (the largest unit) exactly as a
student's launcher would: same service worker and cache, same CDN, same log
and save services, same play page. Every step is recorded and shown in the
**Device Tests** view.

## Turning it on

Site Settings (admin) → **Mission HydroSci — Device Test**:

1. Tick **Enable the device test**. While it is off, the URL shows "The
   device test is not enabled for this site right now."
2. Choose the **Unit 2 build to run**. The list holds the Unit 2 builds
   already uploaded to the CDN (version, build identifier, size, upload
   date). Leave it at "Use the active collection's Unit 2" to run whatever
   the workspace's active collection uses, which is what students get.
3. Copy the **Link to give a school** field.

Turn it off again after the school's testing window; the run records stay.

## What the school sees

1. **Landing page** — what the test does, what is recorded, and a short
   form: school or district, name (both required), role, email (optional),
   device, whether it is school-managed, network, notes.
2. **Run page** — one Unit 2 card. The download starts by itself. The
   "Status details" panel is open and shows every step: service worker,
   game list, storage, content server and game-service checks, download
   method, download progress with rate and time left, waiting countdowns,
   automatic retries, verification. Downloads never give up: every failure
   schedules the next attempt with a visible countdown, and "Retry now" only
   skips the wait. A **Send report** box lets the tester add a note.
3. **Launch** — the play page. The current step shows under the loading
   bar; if a launch step fails, the panel opens with the log and a Copy
   report button.
4. **Test complete** — when the game reports the unit finished, the page
   shows a six-character **test code** (the last characters of the run id)
   and a link back to the run page. Ask testers to quote the code when they
   report anything.

Getting into the game is the important check; finishing the unit is the
full one.

## Reading the results

**Device Tests** view (`/views/device-tests`; admin and analyst; menu entry
under Mission HydroSci):

- One row per run: started, school, tester, device (type · platform ·
  browser), network, download path (Background or Direct, with ↻ when the
  page had to switch), download size and time, stage reached, last problem,
  duration.
- Stages: Started → Downloading → Downloaded → Launching → Gameplay →
  Completed. **Failed** means the most recent step is a failure; because
  the page keeps retrying, a run can move out of Failed again.
- Filters: date range, stage, kind (device test, or a member's stored load
  record), device, school, test id. Chips: runs, reached gameplay, completed,
  failed now, last run.
- **Member load records** (Kind filter): a signed-in student's page stores
  its own step log when a download completes or fails (at most every ten
  minutes while retries continue) and when a launch fails, succeeds, or
  crashes while loading. The row shows the member's name, organization and
  the outcome; the detail has the same step timeline. This is how a field
  report like "connection error in room 12" is read without asking the
  student for anything.
- Click a row for the detail: the form, detected device and network, the
  download and launch summaries, the game telemetry for the run (events,
  first and last event, scenes seen, progress points from the grader), the
  tester's reports, the full step timeline, and the diagnostics snapshot.
  "Download this run as JSON" gives the whole record.
- **Export CSV** gives the table; **Export JSON** gives every matching run
  in full for analysis.

## How runs are identified

Each run gets a 24-character id that is also the `user_id` the game sends to
stratalog and stratasave. It starts with `ffffffff` followed by 16 random
hex characters. A real user id is a MongoDB ObjectID whose first eight
characters are its creation time; `ffffffff` decodes to the year 2106, so no
real id can ever start with it. Anything in `logdata`, `player_states`,
`player_settings` or `progress_point_grades` with such an id is device-test
data, and the `mhs_device_tests` collection says which run. In code:
`models.IsMHSDeviceTestUserID`.

No account is created and nothing is written to users, organizations,
groups or progress. The MHS dashboard therefore does not list test runs;
the Device Tests view is the place to look.

## Data and retention

Run records (`mhs_device_tests`) hold the form, the detected device details,
the step log (last 300 entries), summaries, tester reports and timestamps.
They are kept indefinitely. Tester name, email and IP are visible only in
the admin/analyst view. A run accepts writes for 24 hours after it starts
or until the unit is completed.

## Limits

- Starting runs is limited to 10 per 10 minutes per client IP. A whole
  school behind one address shares that budget; tell a large group to
  stagger starts or ask us to raise it (`deviceTestStartLimit` in
  `internal/app/features/missionhydrosci/devicetest.go`).
- The play page renders the game-service keys as it does for students;
  anyone with the URL can read them from the page source, which was already
  true of every student browser. Keep the test disabled when not in use.
