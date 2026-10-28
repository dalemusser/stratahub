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
- **Game team: memory, and the resume that looks like a restart** (detailed below, discussed 2026-10-28).

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

### Candidate in detail: items for the game team (discussed 2026-10-28)

**A. Memory.** The Unit 2 review of 2026-10-27 (local folder `mhs-reports-102726-unit2-review/`) measured all five builds from the build reports in the project's `Assets/BuildReports/` (2026-10-27 builds). Unit 3 against the others, MB in the packed build:

| content | Unit 1 | Unit 2 | Unit 3 | Unit 4 | Unit 5 |
|---|---|---|---|---|---|
| total | 222 | 284 | 228 | 204 | 207 |
| dialogue voice audio | 20 | 79 | 46 | 40 | 34 |
| occlusion culling data | 1.4 | 29 | 21 (16 overworld + 4.6 dungeon) | 2.7 | 10 |
| textures | 125 | 97 | 102 | 95 | 96 |
| sandbox folders shipped | 35 | 37 | 11 | 21 | 29 |

Field data (students, 2026-10-05 → 10-26): runs that froze on ARM 4 GB devices, Unit 1 2.5 %, Unit 2 3.2 %, **Unit 3 0.3 %**, Unit 4 0.3 %, Unit 5 0.5 %; JS heap median Unit 3 228 MB against Unit 2's 289 MB. Unit 3 is a middle-weight unit. What tipped the 2026-10-27 student was the device's state (a second game window, switching away during freezes), not Unit 3's content. The general suggestions still apply to every unit, and two Unit-3-specific ones exist:

1. **Start the WebGL heap at 512 MB** in each Release profile (today 32 MB, growing 20 % at a time to the 495 MB every unit ends at; each step allocates and copies). The 20–24 s freezes logged yesterday sat exactly at the scene load, where that growth happens. A settings change; test on one ARM Chromebook first.
2. **Texture format on Mali.** The builds carry DXT textures only; if the Mali GPUs lack `WEBGL_compressed_texture_s3tc`, Unity unpacks every texture at load. Unverified. Cheap StrataHub step to answer it for the whole fleet: the play page logs the WebGL extension list (or just that one flag) in the device details once per device. If missing, an ASTC/ETC2 texture variant chosen by the page at launch.
3. **Unit 3's occlusion bake** (16 MB overworld, same 0.2 m smallest hole as Unit 2's): rebake coarser or turn occlusion off for open terrain.
4. **Dialogue audio**: Unit 3 ships 1,311 clips, six player voices of ~151 lines each; a student hears one voice, so ~750 clips never play for them. Ship the chosen voice only (Addressables or one bundle per voice); consider 22 kHz for speech.
5. **Strip sandbox folders** from release builds (11 MB in Unit 3, up to 37 MB elsewhere).
6. Build size is memory on a 4 GB device: every MB of the data file stays in the page for the session.

**B. The resume that looks like a restart.** Code path in the project copy (the shipped v2.9.x builds):

- `CheckpointSaver.ApplyData` (Save Load System / CustomSavers) restores the scene checkpoint, invokes the checkpoint events `events[0..id]`, then at end of frame `ApplyWaypoints()` raises the quest-entry event (`QuestEntryData … Active`) for every active entry of the tracked quest.
- `QuestEventValueListener.ProcessValueResponses` (Quest Listener) receives it. `response.QuestActivated` is a plain field, false in the freshly loaded scene and not part of the save, so the restored quest is treated as **newly activated**: the quest's `OnQuestActivated` events fire, `questActiveEvent` is written to the game log, waypoints are set again, and when `SaveOnActivation` is on, `DoAutoSave` sets the scene checkpoint variable to `AutoCheckpointOnActivation` and saves (that is the checkpoint rewind on load noted 2026-10-15 in the save-fix findings §8). Then each active entry's `OnQuestEntryActivated` events run again.
- Supply Run (`Prefabs/Mini-games/TeraCrate/Quest1-Supply Run.prefab`): the quest-level `OnQuestActivated` list is empty and `SaveOnActivation` is off, so for this quest the visible effects are the entry activations: colliders re-enabled and variables set. Among the re-enabled colliders are four "boundary" triggers (East/South/North/West Boundary, Dialogue System triggers on enter) that start the conversation `U3/Tera` at entries 207 ("Head upstream of our location while I look for the exact crate locations."), 205 ("Nice. You're headed in the right direction…") and 206 ("Whoa there, TK, I said the crates were upstream…"). After a resume the boundaries are armed again, so Tera's directional guidance plays again as the student moves; the 2026-10-27 resumed runs show entry 207 then 205 within a minute of the scene load. With the quest tracker showing the quest again, this is "starting Level 3 over" from the student's chair, though position and variables are restored.
- Data side effect: every resume logs a `questActiveEvent`, so activation counts are inflated (Supply Run activations per student: median 2, 90th percentile 4, maximum 42). Check with the grading rules whether anything counts activations.

Proposed change for the game team: make the restore path distinguishable from a real activation. `CheckpointSaver.ApplyWaypoints` raises the entry event with a *restored* flag (or calls a dedicated method on the listener); on a restored event the listener sets `QuestActivated = true`, re-applies the waypoints, and skips `OnQuestActivated`, the `questActiveEvent` log entry and `DoAutoSave`. Entry activation events run only where they restore unsaved scene state (colliders, variables) — a per-quest review of the prefabs; hint boundaries could carry a "said once" variable so a line does not replay after a resume. Two scripts plus a review of the quest prefabs.

**C. Skip the main menu after a discard reload.** The page knows `document.wasDiscarded`; the bridge already passes identity and service configuration into the game. One more flag could let the game go straight to START's resume path instead of showing the menu. Game change; must fit the save fix's START logic (which waits for the save service's answer). A decision for the team, not a request yet.

**D. Hand-over form.** If these go to the team, an `mhs-updates` package in the usual shape (`03-for-the-game-dev.md`: Part 1 what to do, Part 2 other developer actions, Part 3 supporting information; anything the game needs is listed as required, never optional). A is settings and asset work with build checks; B is a code change with an editor repro (place a save mid-quest, reload, watch the quest tracker and Tera's lines); C is a design question.

### Where the evidence is

Dale's local report folder for 2026-10-27 (`mhs-reports-102726-<school>-restart/`) (outside the repository; it names people): the launch-by-launch table, the class comparison, the reply draft, the mongosh queries (`find.js`, `class.js`, `savecheck.js`) and raw output. The same investigation method and a related case: the 2026-10-26 Unit 2 lag / freeze-loop report folder `mhs-reports-102626-unit2-lag/`.

---

## Issue 2 — Unit downloads still stop at 91 % with 12 MB to go; it is the same few students at a school; what else can be done

**Status:** documented (2026-10-28). Nothing changed.

### The report

Dale, 2026-10-28: students' downloads still stop at 91 %, 12 MB short. Some districts have changed their filter settings and the downloads then work. Where a school has the problem it is always a handful of students who have it persistently. A series of smaller files instead of one large one was discussed earlier; other solutions should be explored.

### What we already knew

- The signature (first seen 2026-10-07 to 09 at School A, record in `mhs-download-drops-investigation-100726.md`): a unit file arrives at full speed until exactly `total − 12 MB` (132 → 120, 213 → 201, 156 → 144), then nothing. A web filter on the managed Chromebooks scans large downloads and withholds the last part until it has finished; the district technician's hotspot test on 2026-10-12 showed the filter travels with the managed device, not the school network, so the fix lives in the filtering vendor's console or the Google Admin console. School A's district made that change on 2026-10-12 and School A has had one hold in the two weeks since (176 episodes, 168 complete).
- Our side as of service worker 1.0.16: downloads resume from 8 MB parts with a `Range` request; "never give up" retries; the MHS Activity page shows held / held-then-done / failed per school, student and day.
- The structural idea on record: serve the large `.data` files as ≤ 8 MB objects so there is no tail to hold (week review 2026-10-23, item 6).

### What the last two weeks show (stored Activity days, 2026-10-14 → 10-27, students only)

| school | class days | students | download episodes | complete | held | held then done | students with a hold | note |
|---|---|---|---|---|---|---|---|---|
| School B | 1 (first day) | 50 | 66 | 3 | 25 | 0 | 20 | plus 37 "quota" failures, see below |
| School C | 8 | 21 | 102 | 76 | 17 | 2 (67 min) | 8 | 5 students hold on 2–4 days |
| School D | 2 | 32 | 61 | 36 | 14 | 2 (41 min) | 11 | 2 repeat students |
| School E | 1 | 14 | 23 | 15 | 5 | 2 (5 min) | 5 | never contacted |
| Schools F, G, A | — | — | 390 | 372 | 5 | 2 | 5 | one hold each, no repeats |
| eight other schools | — | — | 459 | 405 | 0 | 0 | 0 | |

Every hold sits at `total − 12 MB` (median and 90th percentile both 12). Two patterns:

**Pattern 1, whole school on its first days.** School B's first day: 20 of 43 students who downloaded were held, 3 completed; classmates in the same period were 0 complete / 6 held, 1 / 5, 1 / 6. School A looked like this on 2026-10-07 to 09. This is the filter as shipped, before the district changes anything.

**Pattern 2, the persistent handful.** At School C, 5 of 21 students in one class held on 2, 2, 3, 3 and 4 different days while 13, 4, 6 and 8 classmates completed on those same days. The per-episode detail of those five says what "persistent" is:

- It is **not the student's account**: the same student completed one unit and was held on the next on the same device within minutes, and completed on a second device the same afternoon while the first device was held.
- It is **not the hardware class**: the held devices (2-core 8 GB Chrome 154/156, 2-core 4 GB Chrome 149/154) are the same models that completed around them.
- It **is a device profile carrying a stuck partial**. The held devices are the persisted profiles created on the first day and used every day since (the rest of the class gets a fresh profile each period). One of them held Unit 3 at 144 of 156 MB on 10-22, 10-26 and 10-27: the partial sat there for five days and every day's attempt asked the content server for the same last 12 MB. Another held Unit 1 at 120 of 132 MB on 10-20 and 10-21 and completed it on a different, fresh profile the next day.

**Why a stuck partial stays stuck: our resume policy meets the filter.** The service worker saves the body in 8 MB parts; when the stream breaks or the page's stall watchdog fires (`FALLBACK_STALL_MS` = 45 s of no progress), the loop is restarted and the next attempt sends `Range: bytes=<saved>-`, a request for only the last 12 MB. A filter that scans whole objects before releasing their tail is now asked for a tail by itself, and holds it. Six consecutive attempts that save nothing (`MAX_FUTILE_ATTEMPTS`) end the run as `network`; the page schedules another run a minute later, which asks for the same tail. The partial is never discarded, so on a persisted profile the hold is re-created every day. A fresh profile starts from zero, the filter sees the whole object, and 76 of 102 episodes at School C completed. The 25–67 minute "held then done" cases are the filter eventually releasing a tail-only request. (What we cannot see from the client: whether the first break at `total − 12 MB` is the filter closing the connection or only pausing it until the scan finishes; the step logs record both "picked up after a dropped connection" and "no data received for N s, restarting", and both lead to the same tail request.)

**School B's other failure, found while gathering (a separate item).** 37 of its 66 episodes failed with `quota` at 61–94 %: "Not enough free space on this device … Quota exceeded", while the browser reported a quota of 10.2–10.6 GB with at most 316 MB in use. Chrome raises that error when the device's disk itself is nearly full, whatever the per-origin estimate says; the school's 100 devices are all 8-core 4 GB Chromebooks with storage not persisted. The message to the student is right; the advice to the school (free disk space on the Chromebooks, old user profiles are the usual cause) is not on the page. To be its own issue.

### Solutions to explore, in the order I would try them

1. **Stop asking for the tail by itself.** When a run is in the last 16 MB and the stream goes quiet, do not cut it at 45 s: hold the connection open for several minutes and tell the student the download is being checked by the school's filter and will finish by itself ("checking the file … 2:40"). If it still breaks, and once a file has had N tail-only attempts (or its partial is older than today), discard the partial and restart the file from zero, so the filter scans a whole object again. Client-only, no district action, directly aimed at Pattern 2 and at the daily re-creation of the hold. It makes a slow scan slower to fail, so the cap matters.
2. **Smaller objects on the content server.** At build upload, store each large unit file as a sequence of ≤ 8 MB objects and list them in the manifest; the service worker fetches them in order (or two at a time), stores them as the parts it already uses, and assembles the single cached response the Unity loader expects. Every object is scanned whole in seconds; there is no tail; a retry costs at most 8 MB; the 8 MB part flush becomes the natural unit. Server side a split step and a manifest field; client side the fetch loop; nothing for the game team. This is the "series of smaller files" idea, made concrete. Whether the filter treats an 8 MB `Range` response (206) the same as a whole 8 MB object is unknown, so separate objects are the safer form; the Range form could be tested first at School E or School D without a server change.
3. **A one-page note for districts, sent before the first class day.** The filter is on the managed Chromebook, so the change is made where the device policy lives, not on the firewall; name the content host; ask what School A's technician changed so the note can quote the vendor's setting. School B's first day shows every new school starts in Pattern 1.
4. **Make the hold visible as a hold, with its age.** The Activity page already shows held episodes; a per-student "partial stuck at the tail since <day>" flag would have named School C's five students on day two. Folds into item 1's rule.
5. **The disk-full failure at School B** as its own item: the page's teacher paragraph gets the "free disk space on the Chromebook" advice, and the Activity page shows `quota` failures distinctly (they already carry the error class).

What I would not do: anything that disguises the files from the filter. The scan is the school's policy; the fix is to let it finish (item 1) or to give it objects it can finish quickly (item 2), and to tell IT what to exempt (item 3).

### Open questions for Dale

- Which districts made the filter change, and do we know what they changed (vendor, setting)? School A's technician is the one person who has done it.
- School E has never been contacted; School D and School C have the problem now. Who is the contact at each?

### Where the evidence is

Local folder `mhs-reports-102826-issues/` on Dale's desktop (outside the repository): `school-key.md` maps the letters above to the schools, `queries/holds.js`, `holds2.js`, `fortsmith.js`, raw output in `data/raw/`. Earlier: `mhs-reports-101126/` and `-101226/` (School A, October 7–12), the week review of 2026-10-23.

---

## Plan

To be written once all issues are on the page.
