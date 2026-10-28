# MHS issues under discussion, 2026-10-28

A working record. Each issue is written down as it is discussed so the details are not lost; once all of them are on the page a plan follows (§ "Plan", to be written), then the work. School, teacher and student names stay out of this document (the repository is public); the local report folders named below hold them.

Status key: **documented** = written down, nothing changed · **planned** = in the plan below · **done** = fixed and deployed, with the commit.

---

## Issue 1 — Chrome discards the game window on low-memory Chromebooks; a second game window makes it worse; the reload looks like "the game restarted the unit"

**Status:** documented (2026-10-28). Nothing changed in StrataHub or the game.

### The report

2026-10-27, a teacher at one school: one student "will be in the middle of playing the game (Level 3) and the game itself pauses her and restarts at the beginning of Level 3. I sat with her and watched it happen. Everyone else has been able to proceed today except her. She was the one who had the most issues on Friday as well." The game build was the version before the garden-box and save fixes (unit3 v2.9.5).

### The same issue, seen before

This is the Unit 2 "lag and exits" report from another school on 2026-10-26, now observable. There, two students on the same ARM 4 GB Chromebook class looped at the Unit 2 opening: 40–58 fps at the menu, the scene load took the heap to 495 MB, the frame rate fell to 0.1–6, the window lost focus and the run ended within 60–110 s, several times with no `pagehide` (the teacher saw the screen go green or white and the game "exit"). They never reached the first save point, so every relaunch really did restart the unit. The hypothesis recorded then was memory exhaustion on 4 GB ARM devices once the heap reaches 495 MB, worsened by leftover game windows; the device-state logging deployed 2026-10-27 01:41 CDT (`5af3f12`: `wasDiscarded`, freezes, other windows, CPU pressure) exists to test it, and the 2026-10-27 student below is its first confirmation. The only difference is that she had save points, so her restarts were apparent rather than real. The tester's note of 2026-10-19 from the week review, "If you pause in the game, it restarts the unit", is the same thing from the student's chair. Related memory/report folder: `mhs-reports-102626-unit2-lag/` (local).

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

- **Play page: a discard notice** (detailed below, discussed 2026-10-28).
- **MHS Activity: count discards and second game windows** (detailed below, discussed 2026-10-28).
- **Game team:** the Unit 2 memory findings apply to Unit 3 (initial WebGL heap 512 MB so the heap does not grow in copy steps at the scene load; lighter assets); a resume that re-activates the quest and replays its hint makes a resume look like a restart.

### Candidate in detail: a discard notice on the play page (discussed 2026-10-28)

**What happens today.** A discard is Chrome unloading a hidden tab or window to free memory while keeping the tab itself (its title, its place, its window name). Nothing happens on screen while the window is in the background. The moment the student brings it back, Chrome reloads the page from scratch. The reloaded play page therefore looks exactly like a fresh launch: the loading overlay ("Loading Unit 3…", the bar, "Initializing…"), 12–17 s of Unity start, the main menu, START, the Transition, the unit opening with its quest re-announced. The only thing the page knows is `document.wasDiscarded`, which Chromium sets on the document loaded after a discard (it stays false after a manual reload or a renderer crash, so it is precise but Chromium-only). The page records it as a `page/warn` step, "The browser had discarded this page earlier (usually when the device is short of memory) and reloaded it". That step lives in the "Status details" panel inside the loading overlay, which is hidden unless a launch step fails, and in the launch record we read in Device Tests. The student sees nothing that explains the reload, and neither does the teacher standing next to them.

**The proposal.** A fourth notice bar. The page already has three amber bars of the same shape (logging, offline, signed-out: text, a "What to do" button that opens a paragraph for the teacher with numbered steps, a dismiss button; `mhs-notice-bar`). The discard bar is shown as soon as the reloaded page opens, when `wasDiscarded` is true, over the loading overlay, so the student reads it during the Unity start; it stays until dismissed, and hides itself some time after "Game is rendering" (a minute, say) so it does not sit over the game.

Draft wording for the student: "Chrome closed the game to free up memory while another window or tab was in front. The game is loading again. Press START to continue from your last save point." For the teacher (the "What to do" paragraph): what a discard is; the game needs about 750 MB of memory in its window and this device has N GB (`navigator.deviceMemory`); if other game windows are open now, say how many (the page knows from the `mhs-play-windows` channel); steps: close other tabs and windows, keep one copy of the game open, when the game freezes wait 30 s instead of switching to another window, and a Chromebook with more memory if one is available. The detail span can carry the device facts the way the offline bar's `mhs-off-detail` does.

**What it can and cannot do.** It cannot prevent the discard (Chrome decides, the page is not running at that moment), cannot press START for the student, does not know how long the window was gone, and cannot know whether the last save reached the service: on the current build a save in flight at the moment of the discard is lost (the save fix keeps an unsent save on the device and sends it at the next launch). So the wording says "last save point", never "where you were". It is information for two readers: the student, who otherwise thinks the game threw them back to the start of the unit, and the teacher, who gets the device advice on the spot instead of by email. Amber fits: a discard is a real problem, not a normal state.

**Game-side option, not StrataHub.** With a flag from the page (the bridge already carries the identity and the service configuration), the game could skip the main menu after a discard reload and resume directly. That is a game change; noted for the game team, not part of this item.

### Candidate in detail: discards and second game windows on the MHS Activity page (discussed 2026-10-28)

**How freezes are counted today** (`3f346ff`, 2026-10-26, activity version 3). A run "froze" when its heartbeats show a beat under 5 fps after the game's memory passed 400 MB (a unit scene had loaded) and the run ended within five minutes (`models.MHSRunFroze`); the day's count is `Launch.Froze`; three or more in a day set `Flags.FreezeLoop`, which makes the day a problem (`HasProblem`). Pills: student row "game froze N times" (amber with the flag, gray without), school row "N students stuck relaunching"; CSV columns `launches_froze`, `students_freeze_loop` / `freeze_loop`.

**It already surfaced this student.** Her stored day for 2026-10-27 (computed 06:12 CDT on 10-28, final, version 3): launches ok 11, `froze: 6`, `freeze_loop: true`, crash notices 3, blocked false. So her row read "game froze 6 times" in amber and the school row "1 student stuck relaunching" while the class was still running; the classmate on the same device model shows `froze: 2`, no flag. The teacher's email arrived anyway because nobody opens the page during the school day. Two conclusions: the symptom detection works; what is missing is the *cause* on the page, and someone's attention.

**What the discard count adds.**
- *Cause, not symptom.* "Froze" says the game stopped moving; "Chrome closed the game" says why the student is at the menu again, and "2 game windows open" names the thing to fix. Those are the words a coordinator can act on without reading heartbeats.
- *Runs the freeze rule misses.* Two of her eight discards followed 11-minute runs; a run longer than five minutes is never "froze", so a student who is discarded once per long run never reaches the flag.
- *The second game window* is a direct cause and a direct instruction ("close the other one"), today only visible in the step log.

**Data path.** The three device-state steps (`5af3f12`) are plain `page/warn` and `game/warn` steps with structured detail: "The browser had discarded this page earlier …" (`unit`), "Another game window is open on this device (N besides this one)" (`other_windows`), "The page was frozen for N s" (`stall_s`). The parser (`mhssteplog`) does not know them yet; it would gain three kinds (discarded, other window, page frozen), `Summarize` three fields (discards, most other windows seen, page freezes), and `MHSMemberSummaryVersion` goes to 2 (records with a version-1 summary are re-parsed from their steps at compute time, which is the existing path). The day doc gains `Launch.Discarded`, `Launch.PageFrozen`, `Flags.SecondGameWindow` and a problem flag at two or more discards in a day (one discard is ordinary: Chrome's memory saver unloads a tab left in the background over lunch); `HasProblem` includes it; `MHSActivityVersion` goes to 4 so the job recomputes the stored days gradually, as it did for version 3. Pills: student row "Chrome closed the game N times" (amber at 2+, gray at 1) and "2 game windows open" (amber); school row "N students had the game closed by Chrome"; CSV columns for each. The message prefixes join the pin test that guards the parser against wording changes in the page. The data exists only from 2026-10-27 01:41 CDT, so earlier days show nothing; the pill title should say so.

**The attention gap is a separate item.** A page nobody opens surfaces nothing. If the goal is "we hear about it before the teacher writes", the page needs a push: a daily digest of problem days (students with a problem flag, by school) sent to staff, or a morning check as routine. Noted here, not designed.

### Where the evidence is

Dale's local report folder for 2026-10-27 (`mhs-reports-102726-<school>-restart/`) (outside the repository; it names people): the launch-by-launch table, the class comparison, the reply draft, the mongosh queries (`find.js`, `class.js`, `savecheck.js`) and raw output. The same investigation method and a related case: the 2026-10-26 Unit 2 lag / freeze-loop report folder `mhs-reports-102626-unit2-lag/`.

---

## Plan

To be written once all issues are on the page.
