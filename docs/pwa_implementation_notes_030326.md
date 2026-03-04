Good questions to work through before planning. Here's my input and questions:                                 

  On the completion signal (#5): We already have a signal. The play page intercepts Unity's attempt to navigate  
  to the next unit's external URL (adroit.games / cloudfront.net). Right now we block it and show the "Unit
  Complete" overlay. That navigation attempt is the game saying "this unit is done, go to the next one." We could
   use that interception point to record the completion and trigger the transition instead of showing the
  overlay.

  Questions on that:
  - Is that navigation-to-next-unit reliable as a completion signal? Does it always fire when a unit is truly
  complete, and only then?
  - Does the destination URL contain the next unit's identity (e.g., does Unit 1's completion navigate to a URL
  containing unit2)?

  On the log data (#7): You mentioned log entries that indicate end of unit. Where do those logs live? Is this
  data the game sends to an API endpoint, or telemetry in a database we already have access to? If we can read
  completion from existing log data, that gives us a second source of truth — useful for students who completed
  units before this system existed.

  On "current unit" (#6): Does "which unit they are in" mean strictly the last completed unit + 1? Or could a
  student be partway through a unit and need to resume at a specific point within it? (Unity WebGL does have
  autoSyncPersistentDataPath enabled, which saves local progress via IndexedDB, so mid-unit save/resume may
  already work — the question is whether we just need to know which unit to launch, not where within it.)

  On the single launch point UI (#1): Should the units page change from showing 5 individual cards with
  Download/Play/Clear buttons to a single "Run" button? Should students still see the individual units (for
  download status) but only have one launch action? Or is this a completely different page?

  On downloading: Currently students download units individually. With a single launch point:
  - Do we pre-download all units at once?
  - Download on-demand (auto-download the next unit when the current one is completed)?
  - Or keep manual per-unit downloads but only allow launching the current unit?

  On transitioning between units (#4): When a student finishes Unit 1, should we:
  - Navigate to a new page load for Unit 2 (cleanest — full teardown of Unit 1's WASM/audio, fresh load of Unit
  2)?
  - Or try to swap units within the same page (faster but riskier with WASM memory)?

  My instinct is full page navigation. We already have the audio cleanup infrastructure, and a fresh page load
  guarantees no state leaks between units.
  
  ---
  The completion signal we already have could work but I don't want to rely on it because what we are doing  
  now is hack. I'd like the Unit app to have something similar but that has an explicit javascript function  
  call with whatever information we need. E.g. I would like to design and develop a real solution rather         
  than rely on us catching the current navigation attempt. I suppose we could use the current navigation     
  attempt for the purposes of getting a working version and doing all of the other work that is needed now       
  while we wait for the dev team to give us what we really want.  Right now those navigation urls are fully  
  qualified urls and they do contain what the next unit is but not exactly the easiest path to parse to find 
   out the unit from the url. Here is what we do know. If we are in unit 1 then any request to navigate away 
   is to the next unit, unit 2.  And if we are in unit 2 then any request to navigate away is to the next    
  unit, unit 3. Etc. Currently unit 5 is the last unit so we shouldn't go anywhere after we are in unit 5.   
  There is a game save/restore service (save.adroit.games) that the Unity app uses to save game state and    
  load game state.  So if we are in Unit 3 and the student leaves before Unit 3 is complete, on the next     
  launch we would load Unit 3 and the unit would use data from save.adroit.games for the user to put them    
  back in the unit where they should be.                                                                     
  Log entries are stored using a service at log.adroit.games (using stratalog).  The stratalog database is   
  accessible to the statahub app. They are on the same cluster and stratalog, stratasave, and stratahub      
  databases are accessible to all three services. We also have mhsgrader that gets data from stratalog and   
  creates the MHS Dashboard data that is used for the dashboard the teachers see to show the student's       
  progress in StrataHub in the MHS Dashboard feature.  So, the log entry that indicates an end of unit has   
  occurred can be accessed by StrataHub to know that a unit has been completed.  That is less straight       
  forward than keeping a collection for the students that indicates which unit they are currently in. As you 
   indicated it could be backup data or data used from existing game play to know where they are. I do not   
  know yet if the dev team has implemented end of unit progress points entries. I have requested it and it   
  may be in the latest build (not the build we are currently using in Mission HydroSci in StrataHub). I am   
  meeting tomorrow to find out. So it is something we can put in the plan to use and need to make sure is    
  implemented or we can have a plan that relies on end of unit notifications from the game to store data     
  ourselves that is used to track which unit the student is in.                                              
  So, yes, a student can be partway through a unit when they leave and then return.  We should put them back 
   into whatever unit they are in and the game save/restore should put them back in the right place in the   
  unit. We implemented save.adroit.games rather than using localstore so the student could move between      
  computers and resume game play and not have to depend on local data that would not be available if they    
  continue play on another computer. For example a student may play on a computer at school and then go home 
   and play on their own computer there.                                                                     
  Rather than individual cards there should be one Launch Point for the game that takes them to the correct  
  unit. With regard to pre-downloading units we have options. 1) we could still have a button for them to    
  download invidual units, 2) we could do it intelligently as they play in that the next unit could be       
  downloading while they are playing a unit, 3) there had been in the original plan a way for a push         
  notification to tell the device to download a unit (although now looking at what we have I am not sure     
  that complexity is needed and it might make the whole thing more fragile), 4) we could direct them when to 
   download one or more units, 5) we could download them all in advance if enough memory is available and we 
   know the amount of memory because it is displayed in Mission HydroSci as a bar, 6) we can do whatever is  
  not fragile and makes the most sense. For devices low on storage we could remove units that we will not    
  return to because they have already been played.  I am open to the most intelligent and stable             
  implementation of downloading units.                                                                       
  I agree that a full teardown to navigate to the next unit makes the most sense. A little bit faster isn't  
  work the risks regarding WASM memory and audio playback.  Stability and reliability are very important. If 
   we get flaky behavior then I will get a ton of pushback on this approach.  
  
  ---
  
  
  On downloading — my recommendation:

  The most stable approach that handles low storage:

  1. Current unit must be downloaded to play. The Launch button checks this and prompts download if needed.
  2. Auto-download the next unit while they play. When Unit 2 launches, start downloading Unit 3 in the
  background. By the time they finish Unit 2 (hours of gameplay), Unit 3 is ready.
  3. Auto-clear completed units they won't return to. After confirming Unit N+1 is fully cached, delete Unit
  N-1's cache. The student never needs to think about storage.
  4. Keep manual override available. A collapsible "Storage" section at the bottom of the page showing what's
  downloaded and letting them manually clear if needed. Not prominent, but accessible.

  This means at most 2 units are cached at any time (current + next), which is ~300-400 MB worst case. That fits
  comfortably even on 16 GB Chromebooks. The student never sees a download screen or makes download decisions —
  it just works.

  On the UI:

  The page becomes much simpler:

  - Unit progress indicator showing where they are (Unit 3 of 5, with completed units marked)
  - One prominent Launch / Continue button
  - Download status if their current unit isn't cached yet (progress bar + "Downloading Unit 3...")
  - Storage bar (existing, maybe collapsed by default)
  - The install PWA banner (existing)

  Questions I have:

  1. Workspace context — Is a student's progress tied to a specific workspace (class/school), or is it global to
  their account? If they're in two workspaces, do they have independent progress?
  2. Admin/teacher controls — Should a teacher or admin be able to reset a student's progress or move them to a
  specific unit? (e.g., "start this student over at Unit 1" or "skip to Unit 4 for testing")
  3. First-time experience — When a student with no progress record hits the page, do we start downloading Unit 1
   automatically, or wait for them to tap Launch?
  4. What happens after Unit 5? — When they complete the final unit, what should the page show? A completion
  message? A summary?