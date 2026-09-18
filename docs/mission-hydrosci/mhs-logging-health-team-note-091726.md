# Team note: Mission HydroSci now watches its own game logging

*Draft for email or Discord, 2026-09-17. Paste as is or trim; the headers work in both.*

---

**Subject: Mission HydroSci now tells us when a student's gameplay isn't being recorded**

Hi all,

Short version: we found a way the game can silently stop sending its gameplay logs, StrataHub now detects it and tells the student and the teacher what to do, and the change is live on every workspace as of September 17.

**What happened**

On one heavily used Windows browser profile, the game stopped sending its gameplay logs to the log service. The game played normally, progress saved, and nothing looked wrong. Nothing was recorded for the whole of every session, on every unit build, until the browser's site data was cleared, which fixed it at once. It was discovered days later only because someone played specifically to generate data and then found none under their id. The game gives no sign when its logging fails, so a shared classroom machine in that state would lose every student who used it, and the research study would not know until much later.

**What's live now**

- **On the student's screen.** If the game runs for about five minutes with none of its logs reaching the server, a thin amber bar appears along the bottom of the game: "Game activity is not being recorded on this device. You can keep playing. Please tell your teacher." A **What to do** button holds the steps for the teacher. A separate bar appears if the device loses its internet connection; it clears when the connection returns. Note that an outage of more than about ten seconds during play kills the game's logging on that device for good (see below), so the recording bar follows a few minutes later.
- **Devices tab on the teacher dashboard.** A new **Logs** column: a check mark and time when the record is arriving, an amber **!** (Not recorded, Cache full, or Nearly full) when it is not, with the fix in the tooltip.
- **Device Tests viewer (admins and analysts).** A Logs column and a Logs filter. Kind = Member load record with Logs = Problem is the list of play sessions where logging failed. Member records now also show the game's own log count, which they did not before.
- **Units page.** The game's local log store size is reported with the device status and shown in the status details, so a device that has been piling up unsent logs can be spotted before a launch.

**How it works, briefly**

The game itself never reports a logging failure, so StrataHub checks from the outside. While a unit runs, the page sends a heartbeat every 30 seconds; every tenth beat the server asks the log service whether anything has arrived from that launch. Nothing arriving only counts as a failure when the game's local store shows it is producing events it cannot send (the store grows, or is nearly full, or the game printed its cache error), so a student sitting at a menu screen is not flagged.

**If you get a report**

There is a support checklist in the repo (`docs/mission-hydrosci/mhs-logging-support-checklist.md`). The order matters: capture the evidence first, then clear. The fix on the device is to clear the site data for the game's site (padlock in the address bar, Site settings, Delete data) and sign in again; progress and settings are on the server and are not affected; the unit downloads again. If the same device comes back flagged, or several devices at one school flag at once, the school network is probably blocking the log service host, and that is a district-side fix.

**What this does not do**

It does not change the game. A handoff bundle with a fix for the game's logger is written and waiting for the game team. The cause is now reproduced and observed, not hypothesized: when a log request fails twice in a row, ten seconds apart, for any reason (a blocked host, a Wi-Fi drop, being offline), the game's sender stops for good on that browser profile and every later session is silent until the site data is cleared. The captured store and the two-minute reproduction are in the bundle. Given that, the game fix belongs in the next build rather than waiting for a trigger.

**Asks**

- If you see the amber bar during your own testing, tell me which device and when.
- Teachers' guide text for this is written; the PDF needs rebuilding before it goes out.

More detail: `docs/mission-hydrosci/mhs-game-logging-silent-failure-plan.md` (the plan and what was verified) and `docs/mission-hydrosci-teacher-guide/game-activity-not-recorded.md` (the teacher-facing text).
