# Mission HydroSci — Device Test: Tester Guide

Thank you for helping us check that Mission HydroSci will run on your school's
devices. The whole test takes about ten to twenty minutes on a normal school
network, most of it waiting for a download.

## What this test does

Mission HydroSci is a science adventure game that runs in the web browser. The
test downloads the game's largest chapter, **Unit 2**, and starts it, exactly
the way it will happen for students. If Unit 2 runs on a device, the rest of
the game will too.

You do not need an account, and nothing is installed except the game's own
files in the browser.

## Before you start

- **Use the real thing.** Test on the same kind of device students will use,
  on the school's own network. A Chromebook on the classroom Wi-Fi tells us
  more than a laptop at home.
- **Give it space.** The download is a few hundred megabytes. If the device
  is nearly full, the page will tell you how much room it needs.
- **Keep the tab open.** On some devices the download only runs while the
  page is open. The page will say if that applies to you.
- **You need a keyboard and a trackpad or mouse.** The game is controlled
  with them, and you have to get through the opening part before the Unit 2
  content itself loads and runs.
- **Turn the sound on.** We need to know whether audio works on the device,
  so play with the volume up and tell us about any problem with it.

## Step 1 — Open the link

Open the link you were given in the device's normal browser (Chrome on a
Chromebook or Windows or Mac; Safari on an iPad). It looks like:

```
https://<school-site>/missionhydrosci/devicetest
```

## Step 2 — Fill in the form

Tell us your school or district and your name (both required), and, if you
like, your role, an email address in case we need to follow up, the device
and network you are using, and any notes: a content filter, a shared device,
how many students will use this kind of device. Then press **Start the
Unit 2 test**.

## Step 3 — Let it download

The next page shows one card for Unit 2 and starts the download on its own.

- The **progress bar** shows how far along it is, how much has arrived, and
  how long it has taken.
- **Status details** below it lists every step the page takes, with the time
  each took. This is how we see exactly where something went wrong, so leave
  it open.
- If nothing arrives for a while, the page says so and counts down to its
  next attempt. **You do not need to do anything.** The download keeps trying
  by itself until it succeeds; the **Retry now** button only skips the wait.
- If the page says **"Keep this tab open"**, do that: on this device the
  download pauses if the tab is closed or hidden. Otherwise you can switch
  tabs while it downloads.

## Step 4 — Launch

When the download finishes, the card says **Ready to play** and a green
**Launch Unit 2** button appears. Press it. The game loads for a short while
(a loading bar with the current step under it), then the first scene appears.

## Step 5 — Play

Use the keyboard and the trackpad or mouse to get through the opening part
and into Unit 2 itself; play at least until the unit's content has loaded
and you are moving around in it. That is the important check. If you have
time, play through Unit 2 to the end; that confirms the device holds up for
a whole chapter.

Listen as you play. If there is no sound, or it stutters, cuts out, or is out
of step with what is on screen, note when it happened and include it in your
report; audio problems are one of the things this test is for.

When you finish the unit, the page shows **Test complete** and a six-character
**test code**. Please note it down. If you stop earlier, the same code is at
the top of the run page.

## If something goes wrong

- **Send report.** On the run page, under Status details, type what you saw
  and press **Send report**. Your note and the full step list are sent to us.
- **Copy report.** Also under Status details. It copies the step list as
  text so you can paste it into an email or message.
- **Tell us the test code.** It lets us find your run straight away.

### Messages you might see

| Message | What it means | What to do |
|---|---|---|
| *Cannot reach the game content server (…)* | The school network or a content filter is blocking the server that holds the game files. | Ask your IT staff to allow the host named in the message. The page keeps checking and continues on its own once it is allowed. |
| *Not enough free space on this device* | The device does not have room for the unit. | Free up space or clear other downloads; the download resumes by itself. |
| *No data received for N s … switching to the direct download method* | The browser's background download stopped. The page is switching to a different method. | Nothing; keep the tab open. |
| *Retrying automatically in N s (attempt N)* | The last attempt failed (usually the network). | Nothing; wait, or press Retry now. |
| *Keep this tab open* | On this device the download runs only while the page is open. | Leave the tab open and visible until the download finishes. |
| *Too many test runs have been started from this network* | The school's address reached the limit for new runs. | Wait a few minutes and start again, or spread starts out. |
| *The game loader failed to load* | The game files could not be read. | Go back to the run page; it will re-download what is missing. |

## Questions

**Can I close the tab and come back?** If the page said the download runs
in the background, yes; otherwise the download pauses until you return. Your
run page link works for 24 hours.

**Do I need to install anything?** No. The game runs in the browser.

**Will this use up the device's storage?** The unit's files stay in the
browser's storage after the test, the same as they would for a student. They
can be removed by clearing the browser's site data.

**Does the test collect personal information?** It records what you type in
the form, details about the device, browser, storage and network, every step
of the download and launch with timings and any errors, and the game's own
progress data for this run. No account is created. Your name and email are
seen only by the Mission HydroSci team.
