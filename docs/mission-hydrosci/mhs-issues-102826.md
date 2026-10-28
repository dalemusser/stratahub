# MHS issues under discussion, 2026-10-28

A working record. Each issue is written down as it is discussed so the details are not lost; once all of them are on the page a plan follows (§ "Plan", to be written), then the work. School, teacher and student names stay out of this document (the repository is public); the local report folders named below hold them.

Status key: **documented** = written down, nothing changed · **planned** = in the plan below · **done** = fixed and deployed, with the commit.

---

## Issue 1 — Chrome discards the game window on low-memory Chromebooks; a second game window makes it worse; the reload looks like "the game restarted the unit"

**Status:** documented (2026-10-28). Nothing changed in StrataHub or the game.

### The report

2026-10-27, a teacher at one school: one student "will be in the middle of playing the game (Level 3) and the game itself pauses her and restarts at the beginning of Level 3. I sat with her and watched it happen. Everyone else has been able to proceed today except her. She was the one who had the most issues on Friday as well." The game build was the version before the garden-box and save fixes (unit3 v2.9.5).

### What the records show (read-only, 2026-10-27, Central time)

Device: one Chromebook all week, ARM (MediaTek, Mali-G72 GPU), 4 GB RAM, Chrome 154, the game running in the installed PWA window. The user agent says `x86_64` because Chrome freezes that string; the game's own device report (`device.gdName`) is the reliable source. Same hardware class as the freeze-loop machines at another school on 2026-10-26.

| | count |
|---|---|
| launches of the unit | 12 |
| reloads carrying `document.wasDiscarded` ("The browser had discarded this page earlier (usually when the device is short of memory) and reloaded it") | 8 |
| "The page was frozen for 20–24 s", each right after the unit's scene loaded (wasm heap 115 → 495 MB) | 4 |
| "Another game window is open on this device (1 besides this one)" | 1 |
| pointer-lock browser notices | 3 |
| connection losses | 2 |
| minutes in the game | 45 |
| quests finished | 0 |
| frame rate, median / worst tenth | 14 / 5.5 fps |
| memory per game window (wasm heap + JS heap) | ≈ 495 + 240 MB |

Before every discard the last heartbeat shows `visibility: hidden`: another window or tab was in front when Chrome took the game window's memory. The longest uninterrupted run was 11 minutes at 8–25 fps. The unit's first quest takes students 9 minutes median (p25 7, p75 15; 189 of 200 students since 2026-10-05 finished it); this student had 9 activations of it since Friday and no finish.

**Saves worked.** The save service keeps the last 5 saves per student (1,060 students hold exactly 5). Hers that day: one after a resume (0 supplies), one at 13:46:33 with the quest's three supplies recovered and a new position, then two resume autosaves whose content is identical to the 13:46 save, which proves the restore applied position and variables. The save ledger has no refused request of hers.

**Why this student.** Of the 10 classmates active that day, 3 used the same ARM 4 GB model and 7 used Intel Chromebooks. The Intel group ran at 20–33 fps with 0 discards and 0 freezes. One classmate on the same ARM model had 3 launches, 0 discards and finished 3 quests at 8 fps; another had 15 launches, 2 discards, 4 freezes and 2 "second game window" warnings. Fleet-wide that day, discards occurred at two schools only (10 at this one, both on these two students; 1 at the other ARM-fleet school). The device-state logging that made this visible (`wasDiscarded`, freezes, other windows, CPU pressure) went live 2026-10-27 01:41 CDT (stratahub `5af3f12`), so Friday's reloads cannot be classified; Friday showed the same frame-rate pattern plus a 34-minute network loss while the game kept running.

### What "pauses and restarts" is

1. **The pause** is the page freezing for 20–24 s right after the scene loads, or the frame rate collapsing to 0.1–5 fps.
2. **The restart** is Chrome discarding the hidden game window to free memory and reloading it when the student returns: Unity restarts (12–17 s), main menu, START, Transition, the unit opens with its current quest re-announced and the companion's hint replayed. Position and variables are restored from the last save, but to the student and the teacher this is "the beginning of Level 3".
3. **The cost** per reload is about a minute to get back in plus whatever happened since the last save point, never the whole unit.

### Contributing causes

- A 4 GB ARM Chromebook with ≈ 750 MB per game window; the heap jump at the scene load freezes the page for 20+ s.
- **A second copy of the game.** In a browser tab the units page's Play opens the play page with `window.open(href, '_blank')`, so every click creates a new tab (a student who thinks the game froze clicks Play again). In the PWA, Play navigates the current window, so a second copy there needs a second units-page window.
- Switching away from a frozen game window: a hidden window is the discard candidate.
- On resume the game re-activates the quest and replays its intro/hint, so a resume looks like a restart.

### Advice given to the teacher (reply draft in the local folder)

An Intel Chromebook if one is free (the same switch fixed the same symptom at the other school); otherwise restart the Chromebook before class, keep only the game open, never a second copy of the game, and when the game freezes wait 30 s without switching windows or tabs.

### Design discussed for the second game window (2026-10-28)

Goal (Dale): never two game windows on one device, whatever the units involved. Question asked: unit A is open and the student clicks Play for unit B — what do they end up with? Answer: one window showing unit B; the unit A game is unloaded (its memory freed, its record closed by `pagehide`), never a second copy.

Two layers:

1. **Named window target.** Play opens the play page with a fixed window name instead of `_blank`. When a window of that name exists, the browser navigates it to the requested URL and brings it to the front. This works even when the page in that window is frozen or discarded, because navigation does not need the page's JavaScript.
2. **The units page listens on the game windows' channel.** The play pages already announce themselves on the BroadcastChannel `mhs-play-windows` (`hello` / `here` with the unit id / `bye`, a `here` reply to every `hello`, see the play template). The units page sends `hello` before opening and waits ≈ 300 ms for answers.

| existing game window | student clicks Play for | result |
|---|---|---|
| none | unit B | a new game window with unit B, as today |
| unit B, alive | unit B | that window is focused (`window.open('', name)` + `focus()`), not reloaded; the game keeps running |
| unit A, alive | unit B | that window is switched to unit B; the units page tells the student ("Your game window switched from Unit A to Unit B") |
| any unit, frozen or discarded | any | the browser navigates the named window anyway; the student lands in the requested unit |
| game in the PWA window, units page in a browser tab | unit B | window names do not cross between the two, the channel does: the units page posts a `switch` message, the game window navigates itself to unit B, the tab opens nothing; focus is the student's click on the app window |

Trade-off: switching the window away from a running unit A drops what happened since unit A's last save point, exactly as closing the tab would. Decision direction: do it and say so in the units-page message, do not ask a yes/no question first (the student who needs this most is looking at a frozen window and will click through a dialog). Edge left open: a game window opened by typing its URL has no window name and, if frozen, cannot answer the channel, so a second copy can still appear there; the play page's existing "Another game window" warning shows it to us.

### Other candidate actions (not decided)

- **Play page:** after a discard, show the student a plain message ("Chrome closed the game to free memory while another window was in front. You are back at your last save.") rather than only a step-log line; the step panel already records it.
- **MHS Activity:** count discards per student the way freezes are counted (`Launch.Froze`, `Flags.FreezeLoop`, stratahub `3f346ff`): a "game window discarded N times" pill and CSV column, so these students surface without a teacher's email.
- **Game team:** the Unit 2 memory findings apply to Unit 3 (initial WebGL heap 512 MB so the heap does not grow in copy steps at the scene load; lighter assets); a resume that re-activates the quest and replays its hint makes a resume look like a restart.

### Where the evidence is

Dale's local report folder for 2026-10-27 (`mhs-reports-102726-<school>-restart/`) (outside the repository; it names people): the launch-by-launch table, the class comparison, the reply draft, the mongosh queries (`find.js`, `class.js`, `savecheck.js`) and raw output. The same investigation method and a related case: the 2026-10-26 Unit 2 lag / freeze-loop report folder `mhs-reports-102626-unit2-lag/`.

---

## Plan

To be written once all issues are on the page.
