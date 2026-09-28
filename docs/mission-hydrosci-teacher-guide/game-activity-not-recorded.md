# Game activity not recorded

*Content for the Mission HydroSci Teacher Guide, Troubleshooting. New section. The Devices view section (`devices-tab.md`) refers to it. Plain language for teachers; the technical side is in `docs/mission-hydrosci/mhs-logging-support-checklist.md`.*

---

While a student plays, the game sends a record of what they do (the dialogue they read, the tools they use, where they go) to the Mission HydroSci research log. The research study depends on that record. Sometimes a device stops sending it for the rest of a play session, usually after its internet connection dropped for a moment, and the game itself gives no sign: it plays normally, progress still saves, and nothing looks wrong to the student. The game keeps the record on the device meanwhile.

Mission HydroSci watches for this, tells you in two places, and fixes it by itself: the next time the game is opened on that device, the page clears what stopped the game sending, and the saved record is sent.

## What you will see

**On the student's screen.** A thin amber bar appears along the bottom of the game:

> **!** Your game activity is not reaching the server right now. It is being saved on this device and will be sent the next time the game is opened. You can keep playing.

The student can keep playing. Nothing they are doing is at risk. The bar has a **What to do** button with the steps below, and an **×** to hide it.

If the bar says instead that **this device cannot save your game activity**, the device is refusing to store the record at all (a private browser window does this, and so does a store that has filled up after a long time). Activity from that point in the session is not kept; ask the student to use a normal browser window next time.

**On the Devices view of the dashboard.** The **Logs** column shows, for each device, whether the game's activity record reached the server the last time the student played on it:

| Logs cell | Meaning |
|---|---|
| **✓ with a date and time** | Recorded. The last confirmation that the record was arriving from this device. |
| **Amber ! Not recorded** | The student played for several minutes on this device and nothing arrived. The record is saved on the device and is sent the next time the game is opened there. |
| **Amber ! Cache full** | The game reported that its local store for unsent records is full on this device. |
| **Amber ! Nearly full** | That local store is nearly full: records have been piling up unsent on this device for a long time. |
| **—** | No verdict yet: no play session on this device recently, or the last one was too short to check. |

Point at the cell for the details and the steps. Amber means something is wrong, as everywhere else on the dashboard.

## What to do

1. **Let the student keep playing.** Progress and settings are saved on the server. The problem is only with the activity record.
2. **Nothing needs clearing on the device.** The next time the game is opened on that device, on good Wi-Fi, the saved record is sent. About five minutes into that session the Devices view shows the device as recorded again.
3. **If the notice comes back in the next session on the same device**, the school's network is probably blocking the address the game sends its records to. Tell your program contact, name the device, and mention the notice. The game will keep playing meanwhile; only the record is affected.

You do not need to clear the site data for the Mission HydroSci site; doing so would delete the record the device has saved.

## If the device goes offline

A different bar appears when the device loses its internet connection while the game is open:

> **!** This device has lost its internet connection. You can keep playing. Your game activity is being saved on this device and will be sent later. Please tell your teacher.

The game keeps playing, but it cannot save the student's progress online while the device is offline. Once the connection has been gone for more than about ten seconds, the game stops sending its activity record for the rest of that session and keeps it on the device instead. The bar disappears by itself when the connection is back; a few minutes later the recording bar above may appear in its place, because the game is no longer sending.

What to do: check the device's Wi-Fi; if the whole class sees the bar, the network is down. Keep the game tab open until the connection is back so progress can save again. Nothing needs clearing afterwards: the next time the game is opened on that device, the saved record is sent. Until then the Devices view shows the device as **Not recorded**.

## When this happens

It is about the device and its connection, not the student's account. The usual cause is a moment without internet during play: a Wi-Fi drop of more than about ten seconds is enough. The other cause is a network that blocks the game's log address while allowing everything else; in that case every device on that network is affected, the notice comes back every session, and the fix is on the network side.

## What is not affected

- The student's progress through the units, and their settings. Both are saved on the server as they play.
- The game itself. It plays normally.
- Other devices. Signing in on another device works as usual.
