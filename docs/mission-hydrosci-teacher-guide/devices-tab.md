# The Devices View

*Content for the Mission HydroSci Teacher Guide, Appendix: How To Use the Teacher Dashboard. Replaces the current "The Devices View" section. See `devices-tab-notes.md` for the plan, rationale, and editor notes.*

---

The Devices view answers a practical question: **is each student's device ready to play Mission HydroSci today?**

The Progress view shows what students have accomplished in the game. The Devices view shows the technical side: what kind of device each student is using, which units are downloaded on it, how much of the browser's storage allowance MHS is using, and when the device last opened MHS.

Use it before class to catch problems early, and during class to sort out whether a problem is with the student's device, their account, or the game itself.

Select your class from the drop-down menu at the top of the page, then click the **Devices** tab. The view refreshes itself every 30 seconds; click **Refresh** to update it right away.

> **Figure placeholder:** screenshot of the Devices view for a class (use a workspace with test names, not real students).

## What each column shows

| Column | What it shows |
|---|---|
| **Student** | The student's name. A student who has used more than one device has one row per device, grouped under their name with the most recently used device first. |
| **Device** | The kind of device: Chromebook, iPad, macOS, Windows, Android, Linux, or Other. Click the name to see details about that device (see *Device details* below). |
| **PWA** | ✓ means the student opened MHS from the installed app the last time they used this device. — means they opened it in a regular browser tab. Both work. The installed app hides the browser's address bar and tabs, which gives the game more screen space. |
| **Unit 1 – Unit 5** | One mark per unit. A dot shows whether the unit is downloaded on this device. On the student's first row, a green ring marks their current unit and a green check circle marks each unit they have completed. An amber dot means the device reported a problem downloading that unit. See *Reading the unit dots* below. |
| **Storage** | How much space MHS is using on this device compared with the space the browser allows it, shown as a bar, a percentage, and the amounts (for example, 729.7 MB / 10.7 GB). |
| **Last Seen** | The date this device last opened MHS. Hover over the date to see the exact time. A date shown in amber is more than a week old. |

If a student has never opened MHS on any device, their row says **No device data**.

## Reading the unit dots

Each unit cell answers two questions: what is on this device, and where is the student. The legend above the table puts it in short form: *Dot: on this device. Ring: current unit. Check: completed.*

| Dot | Meaning |
|---|---|
| **Solid blue** | **Downloaded.** The unit's files are on this device and it is ready to launch. |
| **Blue outline** | **Downloading.** A download of this unit was in progress the last time the device reported in. |
| **Gray** | **Not Downloaded.** The unit is not on this device. This is normal for a unit the student has not reached, and for a device the student is not using at the moment. |
| **Amber** | **Download Problem.** The device reported trouble with this unit's download the last time it reported in: it failed, stalled, or was stuck retrying. Hover over the dot to see which. |

| Mark | Meaning |
|---|---|
| **Green ring** around a dot | **Current unit.** The unit the student is working in. The dot inside shows that unit's state on this device. |
| **Green check circle** | **Completed.** The student has passed every progress point in this unit. Which device it was finished on does not matter, so no dot is shown. |

The ring and the check circles appear on the student's first row only, the device they used most recently. If the student has older devices, those rows show just the dots: what is on that device.

**Amber is the only color that means something is wrong.** Blue and green are confirmations. Gray is neutral: a gray dot inside a green ring means the unit the student is playing is not on this device, which is what you will see for a device they used earlier or have not opened MHS on yet. The download starts on its own when they open MHS there.

Two things to keep in mind when reading the cells:

**1. The ring and check come from gameplay, not from the device.** They show the student's progress, which is saved online with their account. When a student moves to a new device, its row moves to the top and takes the ring, with a gray dot inside until the unit downloads there. The student's own screen says **Ready to play** once it has. The older device drops to a plain row below; to see whether it still has the current unit, look at the same column one row down.

**2. The dots show the device as it was at "Last Seen."** A device reports to the dashboard from the Mission HydroSci page when the student opens it, when a download finishes, and when a download runs into trouble. If a unit was downloaded or cleared after that, the dots do not change until the student opens MHS again. To get a fresh picture, ask the student to open MHS on their device and then click **Refresh**.

> **Tip**
> Green check circles are a quick way to count completed units for the whole class. For where a student is *within* a unit, and for flagged tasks, use the Progress view.

## Device details

Click a device name to open a panel with details about that device: operating system and version, browser and version, screen size, number of CPU cores, device memory, connection type and speed, and the storage MHS is using and has available.

You will not need this often, but it is exactly what technical support asks for when a device misbehaves. "It is a Chromebook with 4 GB of memory, on Wi-Fi, with 9 GB of storage available" is a report someone can act on.

## Storage: what the numbers mean

The Storage column compares the space MHS is using on the device with the space the browser allows MHS to use. That allowance is set by the browser, and it depends on how much free space the device has. It is usually far larger than MHS needs.

MHS manages its own space. It keeps the student's current unit and the next unit downloaded and removes older units automatically, so a healthy device normally shows a low percentage. A unit is a few hundred megabytes.

The bar turns **orange above 70%** and **red above 90%**. At 90% or more, MHS stops downloading the next unit ahead of time, and the student's Mission HydroSci page shows a **Low storage** notice. The student can still play the current unit, but when they finish it the next unit has to download before it starts, and that download can fail if the device is nearly full.

A high percentage almost always means the *device* is nearly full rather than that MHS is using a lot. Look at the amounts:

- **MHS is using a small amount (under about 1 GB) but the percentage is high.** The browser's allowance has shrunk because the device is low on space in general. Free up space on the device itself (the Downloads folder, other apps, other websites' data). Clearing MHS units will not help much.
- **MHS is using several gigabytes.** Units that should have been removed are still on the device. Open **Manage downloads & data** from the bottom of the student's Mission HydroSci page and clear the units the student has finished. This page is locked; a teacher, coordinator, or admin unlocks it on the student's device, and the unlock lasts about ten minutes. Clearing a unit only removes the downloaded files. Progress is saved online and is not affected.

## A two-minute check before class

1. Select the class and open the **Devices** view.
2. Scan for **amber** dots. Each one is a download the device reported trouble with. Then look at the dot inside each student's green ring. Gray means the unit is not on the device they used most recently; the download starts on its own when they open MHS, and takes a few minutes on school Wi-Fi.
3. Scan **Storage** for orange or red bars.
4. Scan **Last Seen** for amber dates. The student has not opened MHS on that device in over a week. They may have been absent, or they may have moved to another device (look for a second row under their name). If a device has gone quiet and the student has no newer row, ask which device they are using.
5. Look for **No device data**. That student has never opened MHS. Check that they can log in and find Mission HydroSci in the menu.

> **Tip**
> If several students show gray inside the ring, have them open Mission HydroSci as they arrive so the downloads run while you start class, instead of everyone downloading at the same moment when you say "launch."

## Solving problems with the Devices view

| The student says… | Look at… | What it usually means | What to do |
|---|---|---|---|
| "It's stuck on the loading screen" or "It won't launch." | The dot inside their green ring (if they are on an older device, the same column on that device's row), and Storage. | Amber: the download had a problem (hover for which). Blue outline: still downloading. Gray: not on this device yet. | Have the student stay on the Mission HydroSci page until it says **Ready to play**. MHS retries a stopped download on its own; the page shows what it is doing and when it will retry. If the bar is red, free up space first. |
| "It says the download failed" or "It's paused." | Storage and, in device details, the connection. | Not enough space on the device, or Wi-Fi dropped. | Red bar: free up space on the device. Otherwise check Wi-Fi and click **Retry**, or simply leave the page open; it keeps retrying. |
| "My progress is gone." | The green dots. | Progress is saved online and tied to the student's account, not the device. If the green dots show their progress, it is safe. | Most often the student logged in with a different account. Compare the name on their screen with the dashboard. If the account is right, the game restarted from the last save point in the unit, which is normal. |
| "I'm on a different Chromebook today." | A new top row under their name, with a gray dot inside the ring until the unit downloads there. | Nothing is wrong. The new device downloads its own copy of the unit; progress follows the account. | Have them open MHS a few minutes early so the download finishes before you start. |
| "It's slow" or "It keeps freezing." | Device details: memory and CPU cores; Storage; PWA. | An older or low-memory device, a nearly full device, or many other tabs open. | Close other tabs and apps. Run MHS from the installed app or in fullscreen. If the device is consistently slow, note the details for your technology staff. |
| "It says Mission HydroSci isn't in my menu." | Whether the student appears in the list at all. | The student is logged in to the wrong account, or is not in your class group. | Check the login. If they are missing from the dashboard, contact the MHS team to add them to the group. |

## What the Devices view tells you about progress

The Devices view gives a unit-level snapshot: green check circles are units the student has completed, and the green ring marks the unit they are working in. That is enough for a quick "who is on which unit" scan of the class.

For anything finer, use the other views:

- **Progress view:** which tasks in the unit are done, which are in progress, and which are flagged for a closer look.
- **Analytics view:** how long each task took, which can point to a student who hit a technical problem.

A unit is marked Completed only when every progress point in it has been passed. If a student has moved into the next unit but a progress point behind them was not passed, the earlier unit stays marked Current on this view. The Progress view shows which point it was.

## Quick reference

| I want to know… | Look at… |
|---|---|
| Whether a student's device is ready for the unit they are playing | The dot inside the green ring: blue is ready, gray is not on this device yet, amber had a download problem |
| Whether a device is short on space | Storage bar: orange above 70%, red above 90% |
| When a student last opened MHS on a device | Last Seen (amber means more than a week ago) |
| Which units a student has completed | Green check circles |
| Which unit a student is working in | Green ring |
| What kind of device a student has, and its specs | Click the device name |
| Whether the student uses the installed app | PWA column (✓ = installed app, — = browser tab) |
