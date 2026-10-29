# Mission HydroSci on 4 GB Chromebooks: what happens, why, and what to do today

Written 2026-10-29 for teachers, coordinators and school IT, from the device-state logging that went live on 2026-10-27 and three class days of data (2026-10-27 → 10-29, about 900 student-days at 14 schools). This is the "advice and documentation on the issue as it stands" step; the changes that will make the problem less likely are listed at the end and tracked in [MHS issues under discussion](mhs-issues-102826.md) (Issue 1). School names are replaced by letters; hosts are placeholders.

## The short version

- On **Chromebooks with 4 GB of memory and an ARM (MediaTek) processor**, Chrome regularly **unloads the game while another window or tab is in front of it**. When the student comes back, the game reloads and starts from the last save point. To the student it looks like the game "paused and restarted". No other kind of device we see in schools does this (one event in 1,000 launches elsewhere; one in five launches on these machines).
- Chrome only unloads a page that is **not visible**. The game is never unloaded while it is the window in front. Three quarters of the reloads we can trace followed the game being hidden behind something else, usually for less than a minute.
- **What students should do:** restart the Chromebook before class, sign in, open only the game, and **stay in it**. Do not switch to another tab or window while the game runs, do not open the game a second time, and when the screen freezes, wait half a minute instead of clicking elsewhere. If the game does reload, press **START**; progress since the last save point is lost, everything before it is kept.
- **What school IT can do today:** add the game's host to Chrome's **"Exceptions to tab discarding"** setting in the Google Admin console (Users & browsers → Other settings). Chrome then unloads other tabs first, not the game. Keep the game's browser data between sessions so the unit files are not downloaded at every launch. If 8 GB or Intel Chromebooks exist in the building, use them for this class.
- **What we are changing** (next weeks): one game window per device, a plain message when Chrome has unloaded the game, a flag on our Activity page so these students are seen without a teacher email, and a smaller memory footprint in the game itself.

## What the student and the teacher see

The student is playing. The screen goes blank or grey, or the game simply disappears and the page shows the loading screen again, then the game's START screen. After START the game is at the **last save point**, which in the first minutes of a unit is the beginning of the unit. The teacher who watches it sees "the game paused her and restarted at the beginning of the level". Nothing was lost on our side: every save the game made is on the server, and the reload restores the latest one.

On the play page, under "Status details", the reload shows up as:

> The browser had discarded this page earlier (usually when the device is short of memory) and reloaded it

Often a few lines earlier there is "The page was frozen for 20 s" (the game stopped drawing for a while before the unload) and "The device reports critical CPU pressure".

## Why it happens

ChromeOS does not use swap space. When the device runs out of memory, Chrome frees memory by **unloading ("discarding") background tabs and windows**, the least recently used first; the tab stays in the tab strip and reloads when it is made visible again (Chromium's [tab discarding design note](https://www.chromium.org/chromium-os/chromiumos-design-docs/tab-discarding-and-reloading/)). A loaded unit of Mission HydroSci is the biggest thing on the machine: the game's own memory grows to about 500 MB when a unit's scene loads, the browser page around it holds another 200 to 300 MB, and the graphics driver has the unit's textures. On a 4 GB Chromebook, with ChromeOS and Chrome itself taking their share, that leaves little room. The moment the game is hidden behind another window or tab, it is the first candidate to be unloaded, and on these machines that happens within seconds.

Two things make it worse:

- **A second copy of the game.** Opening the game again from the units page while a game window is already open doubles the footprint. On the 4 GB ARM machines, 9 of the 12 student-days with a second game window had the game unloaded, against 60 of 294 without one.
- **Downloads running in the background.** While a unit plays, the page downloads the next unit (about 200 MB) so it is ready. On a machine whose stored data is wiped between sessions (see School B below), that download starts over at every launch and runs all period.

Why these machines and not the others: the 4 GB ARM Chromebooks (8 cores, Mali graphics) also run the game at half the frame rate of 4 GB Intel Chromebooks (median 17 vs 30 frames per second), and their graphics memory is shared with the main memory. The 4 GB Intel and AMD Chromebooks in the same schools show the same CPU pressure warnings but practically no unloads.

## What three class days show (2026-10-27 → 10-29, students only)

| Device class | Student-days | Launches | Game unloaded by Chrome | Per 100 launches | Student-days with 2 or more | Freezes ≥ 10 s | Median fps |
|---|---|---|---|---|---|---|---|
| 4 GB ARM Chromebook (8 cores) | 306 | 616 | 138 | 22 | 26 | 32 | 17 |
| 4 GB Intel/AMD Chromebook (2 cores) | 231 | 324 | 1 | 0.3 | 0 | 7 | 30 |
| 8 GB Chromebook (any) | 61 | 138 | 1 | 0.7 | 0 | 4 | 26 to 60 |
| iPad | 287 | 520 | 0 | 0 | 0 | 26 (see note) | 11 |

Note: the iPad "freezes" are the tab being suspended for minutes while another app was in front, not the game stalling; iPads never reload the game this way.

What preceded the unloads at the school where most of them happened (137 unloads over two days):

| What the previous run looked like | Count |
|---|---|
| The game window was hidden at its last heartbeat | 90 of 121 with a heartbeat (the rest were visible at the last beat and hidden after it) |
| The reload came within 60 s of the previous run's last heartbeat | 101 of 136 |
| The previous run had lasted less than 5 minutes | 88 of 136 |
| The unit was fully loaded (game memory ≥ 450 MB) | 68 of 121 |

In words: the student had been playing for a few minutes, something else came in front of the game for less than a minute, Chrome unloaded the game, and the student came back to a reloading page. Some students went through this 7 to 11 times in one period.

Where it happened: 10 of the 14 active schools had no unload at all. School F (ARM 4 GB machines in one classroom) had the student whose report started this work: 8 unloads and 4 freezes in 45 minutes on 2026-10-27. School B, in its first week with 100 ARM 4 GB Chromebooks, had 22 unloads among 15 students on 10-28 and 100 unloads among 47 students on 10-29. School C and School G, which have the same machines in part of their fleet, had 1 to 3 a day.

## What to do: teachers and students

Before class:

1. **Restart the Chromebook** (full shutdown and power on, not just closing the lid). This is the only way to be sure nothing else is using memory.
2. Sign in and **open only the game**. No other tabs, no other windows, no Google Classroom or document open beside it. If the class uses a worksheet, use paper or a second device.
3. **Launch the game once.** If a game window is already open, use it; never open a second copy from the units page.

During class:

4. **Stay in the game window.** Do not switch tabs or windows while the game runs. This includes going back to the units page "to check something".
5. **When the game freezes, wait.** Freezes of 10 to 30 s are the machine catching up after a scene loads. Clicking elsewhere during a freeze is exactly what puts the game in the background and gets it unloaded. Half a minute of patience usually ends with the game continuing.
6. **If the game reloads anyway, press START.** The game continues from the last save point. Nothing before that point is lost.
7. If one machine does it repeatedly while others in the room are fine, **swap the machine**, not the account. The problem follows the device.

At the end of class: close the game window normally (it saves on the way out), then sign out or shut down.

If the room has a mix of Chromebooks, give the Intel or 8 GB machines to this class and keep the ARM 4 GB machines for other work.

## What to do: school IT

1. **Add the game's host to Chrome's tab-discarding exceptions.** Google Admin console → Devices → Chrome → Settings → Users & browsers → section *Other settings* → **Exceptions to tab discarding**. Add the host that serves the game (for StrataHub schools `<stratahub-host>`; the exact host is in the launch link your students use). The underlying Chrome policy is `TabDiscardingExceptions` (ChromeOS and Chrome 108 or later); its definition says the listed pages "will never be discarded by the browser. This applies to memory pressure and high efficiency mode discarding" ([policy definition](https://chromium.googlesource.com/chromium/src/+/HEAD/components/policy/resources/templates/policy_definitions/Miscellaneous/TabDiscardingExceptions.yaml)). With it, Chrome frees memory by unloading other tabs instead of the game. It is a help, not a cure: if the game is the only thing left and memory still runs out, the operating system can still kill it, which then shows as a crash ("Aw, Snap!") rather than a quiet reload.
2. **Keep the game's stored data between sessions.** The game stores each unit's files (130 to 290 MB) in the browser so they download once. On devices where browsing data is wiped between sessions (ephemeral or guest sessions, incognito, clear-on-exit policies, a short browsing-data lifetime), every launch downloads the unit again and starts the next unit's download in the background, which costs memory and bandwidth on exactly the machines that have neither to spare. If your devices are configured that way, allow site data for the game's host, or exempt these devices for the duration of the unit.
3. **Review what else is running in the managed profile.** Force-installed extensions and background apps use memory in every tab. On 4 GB machines, fewer is better.
4. **Prefer 8 GB or Intel Chromebooks** for this class where the choice exists. In our data, 4 GB Intel Chromebooks run the game without unloads; 8 GB machines of either kind do too.

A user-level alternative to item 1 exists (chrome://settings/performance → "Always keep these sites active"), but on managed devices it is usually locked, and on devices that reset their profile it does not survive; the Admin console setting is the one to use.

## What we see on our side, and what to send us

Our Activity page flags a student whose game "froze N times" or who is "stuck relaunching"; Chrome's unloads appear in the per-launch records under "Status details" as the line quoted above, with the number of other game windows open on the device and the device's CPU pressure. A teacher report that names the school, the class period, the date and the student's first name is enough for us to find the records; a photo of the Status details panel helps when the page is still open.

## What we are changing

Tracked in [MHS issues under discussion](mhs-issues-102826.md), Issue 1:

- **One game window per device.** Launching from the units page will reuse the open game window (switching it to the requested unit) instead of opening a second one.
- **A plain message after an unload.** The play page will say that Chrome closed the game to free memory while another window was in front, that the game is loading again, and that START continues from the last save point, so the student knows what happened and the teacher knows what to tell them.
- **A flag on the Activity page** for students whose game was unloaded twice or more in a day, so they are seen without a teacher email.
- **Less memory in the game**: a smaller initial memory reservation, shipping only the chosen player voice, dropping or rebaking the occlusion data, and a resume that does not re-run the quest's activation (which is what makes a resume look like a restart). These are for the game team.
- **No background download of the next unit on 4 GB machines while a unit is playing**, or a much smaller one; under consideration.

## Appendix: how the numbers were made

Member launch records (`mhs_device_tests`, kind `member`) since 2026-10-27 06:41 UTC, students only (users with role member), de-duplicated by student, time and message. "Game unloaded by Chrome" = the step "The browser had discarded this page earlier …" (`document.wasDiscarded` on load). "Freeze" = the step "The page was frozen for N s" (N ≥ 10). Device class from `mhs_device_status.device_details` (`device_memory_gb`, `cpu_cores`, `uad_platform`); 4 GB with 8 cores on ChromeOS is the ARM MediaTek class (the ChromeOS user agent is frozen and does not say). The "what preceded the unload" table pairs each unload with the student's previous record on the same day. Queries and raw output are in the local report folder `mhs-reports-102826-issues/` (outside the repository; it names schools and students).
