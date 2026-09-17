# Game activity not recorded

*Content for the Mission HydroSci Teacher Guide, Troubleshooting. New section. The Devices view section (`devices-tab.md`) refers to it. Plain language for teachers; the technical side is in `docs/mission-hydrosci/mhs-logging-support-checklist.md`.*

---

While a student plays, the game sends a record of what they do (the dialogue they read, the tools they use, where they go) to the Mission HydroSci research log. The research study depends on that record. On rare occasions a device stops sending it, and the game itself gives no sign: it plays normally, progress still saves, and nothing looks wrong to the student.

Mission HydroSci now watches for this and tells you in two places.

## What you will see

**On the student's screen.** A thin amber bar appears along the bottom of the game:

> **!** Game activity is not being recorded on this device. You can keep playing. Please tell your teacher.

The student can keep playing. Nothing they are doing is at risk. The bar has a **What to do** button with the steps below, and an **×** to hide it.

**On the Devices view of the dashboard.** The **Logs** column shows, for each device, whether the game's activity record reached the server the last time the student played on it:

| Logs cell | Meaning |
|---|---|
| **✓ with a date and time** | Recorded. The last confirmation that the record was arriving from this device. |
| **Amber ! Not recorded** | The student played for several minutes on this device and nothing arrived. |
| **Amber ! Cache full** | The game reported that its local store for unsent records is full on this device. |
| **Amber ! Nearly full** | That local store is nearly full: records have been piling up unsent on this device for a long time. |
| **—** | No verdict yet: no play session on this device recently, or the last one was too short to check. |

Point at the cell for the details and the steps. Amber means something is wrong, as everywhere else on the dashboard.

## What to do

1. **Let the student finish.** Progress and settings are saved on the server. The problem is only with the activity record.
2. **When they are done, clear the site data for the Mission HydroSci site on that device.** In Chrome: click the padlock (or the "tune" icon) at the left of the address bar, choose **Site settings**, then **Delete data**. This clears the game's stuck local store. It also removes the downloaded units, which is why the next step downloads them again.
3. **Sign the student in again and open the unit.** The unit downloads again, so do this on good Wi-Fi.
4. **If the notice comes back on the same device**, the school's network is probably blocking the address the game sends its records to. Tell your program contact, name the device, and mention the notice. The game will keep playing meanwhile; only the record is affected.

If you can, before step 2, tell your program contact which device it was and when it happened. They may ask for a file from the device that helps the game's developers find the cause.

## When this happens

It is rare, and it is about the device, not the student's account. It has been seen on a computer that had been used to play the game many times over several months. A device shared by many students collects more over time than a personal one. The other cause is a network that blocks the game's log address while allowing everything else; in that case every device on that network is affected and the fix is on the network side.

## What is not affected

- The student's progress through the units, and their settings. Both are saved on the server as they play.
- The game itself. It plays normally.
- Other devices. Signing in on another device works as usual.
