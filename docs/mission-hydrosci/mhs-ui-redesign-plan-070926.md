# MHS Units Page UI Redesign Plan

**Date:** 2026-07-09
**Status:** Implemented 2026-07-09 (uncommitted). Verified locally: build/vet/tests, browser-driven
as admin and as gated member (gate, keyword unlock, banner+countdown, gated action without
re-prompt, Lock now, re-gate, delete confirm + clean not-configured error), light + dark themes.
**Participants:** Dale Musser, Claude
**Reference screenshots (current UI):** `stratahub/feedback/mhs1.png`, `mhs2.png`, `mh3.png`

## Background

The `/missionhydrosci/units` page currently serves two jobs at once: launching the game
(what members need) and managing downloads, progress, versions, and saved data (what
teachers, testers, and developers need). Everything is stacked on one page in collapsible
sections, ending with `▶ Manage version` pressed against the footer where it is barely
visible and opens downward off-screen.

This plan was agreed after the "Delete Saved State" / "Delete Saved Settings" actions were
added (stratasave delete endpoints + stratahub server-side calls, July 2026), which made the
crowding problem worse and prompted the redesign discussion.

## Problems with the current UI

1. **The launcher is buried.** The hero card (progress dots, unit title, Launch) is clear,
   but below it sit five more sections of admin tooling. A student sees a wall of controls
   under their Play button.
2. **Grouping is by widget, not meaning.** "Set to unit" (progress data, stratahub,
   reversible) sits in the same box as Clear All Downloads / Reset All MHS Data (local
   device, recoverable). The online deletes (stratasave, irreversible) are a separate box.
   Three data domains and three risk levels read as one pile of red buttons.
3. **Red overload.** Four large red buttons; the genuinely irreversible actions do not
   stand out from the recoverable ones.
4. **Bottom-anchored disclosure fails mechanically.** `▶ Manage version` hugs the footer;
   opening it requires scrolling to see what appeared. Version info also matters at a
   glance for testers and support — and it is the hardest thing to see.
5. **Help hidden in "?" modals** because the page is too crowded for inline captions.

## Audiences

- **Members (students):** normally need only Launch. Controls matter only when something
  breaks (stuck downloads, stuck in a unit).
- **Teachers/leaders:** fix problems on the student's device, authorizing via the
  workspace's member-auth mode (staffauth / keyword). Support scripts start with
  "what version is displayed?"
- **Testers/developers:** log in as members in the dev workspace (member-auth = `trust`)
  and need frequent access to version switching, clears, and deletes.

## Agreed design

### Design principles

- The units page is a **game launcher plus read-only status** — everything visible,
  nothing actionable, nothing dangerous.
- **One entry point** to all maintenance (a dedicated page), instead of stacked
  collapsible sections.
- Group controls by **data domain and risk**; state the scope on every action.
- Same surface and same gating for all roles; the member/tester split is handled by the
  existing workspace member-auth modes, not by forked layouts.

### Launcher — `/missionhydrosci/units` (redesigned)

Top to bottom, visible to **all users**:

1. Install banner (when applicable), page title + "?" — unchanged.
2. **Hero card** — progress dots, unit title, status, Launch button, live pipeline
   caption ("Unit 3 downloading… 42%") — unchanged.
3. **Version line** — muted single line, **all users, always**:
   `Version: <collection name>` with an `(override)` badge when a per-user override is
   active. Rationale: support's first question is "what version is displayed?"; teachers
   over a shoulder and testers logged in as members verify at a glance. Links staff to
   the Manage page's version card.
4. **Unit list — status only.** All units shown plainly (no collapsible): name, version,
   size, live status (Ready to play / Downloaded / Downloading X% / Not downloaded).
   **No buttons** — per-unit Download/Clear move to the Manage page. Rule: "everything
   visible, nothing actionable" keeps the student page tap-safe.
5. **Device Storage meter** — below the unit list. Existing color thresholds
   (blue → yellow → red) make low storage self-announcing; no separate warning banner.
6. **`⚙ Manage downloads & data`** — discreet link, visible to all roles.

Removed from the launcher: Manage-downloads collapsible, per-unit buttons, Set-to-unit,
Clear All Downloads, Reset All MHS Data, Saved game data box, Manage version collapsible,
their "?" help modals, and the staff-auth modal.

### Manage page — `/missionhydrosci/manage` (new)

Same route-group middleware as `/units` (RequireSignedIn, five roles,
RequireApp("missionhydrosci")). Header: `← Back to Mission HydroSci`, title
"Manage Mission HydroSci", subtitle "Tools for troubleshooting, testing, and data
management."

Four **always-open cards** (nothing collapsed), in this order:

1. **This device** — storage meter; the per-unit rows **with** Download/Clear buttons;
   `Clear all downloads` and `Reset all MHS data` as **neutral/outline** buttons (not
   red — they are recoverable) with inline captions replacing the "?" modals:
   - Clear all downloads: "Removes downloaded unit files from this device. Files
     re-download when needed. Online progress and saves are not affected."
   - Reset all MHS data: "Also removes cached app pages and local MHS data — use for
     troubleshooting. Requires internet on next launch."
2. **Progress** — "Current: … · Completed: …" summary, then
   `Set progress to [unit ▾] [Apply]` with caption "Marks all earlier units complete.
   Does not affect downloads or saved game data."
3. **Game version** — the existing collection card (name, override badge, description,
   unit/version list), `Change version` and `Use default`; collection-picker modal
   unchanged.
4. **Online saved game — danger zone** (last; amber-bordered card; the **only red
   buttons** on the page) — `Delete saved state` ("the in-progress saved game") and
   `Delete saved settings` ("audio, controls, and preferences"), each captioned
   "Permanent — cannot be undone. Applies to the signed-in account." Confirmation
   dialogs state the consequence explicitly.

### Access & authentication — entry-gated page with a staff unlock session

Current behavior (pre-redesign): every gated member action prompts the full staff-auth
flow individually (login + password/emailed code per action; tokens are one-time-use).
Tension identified: persistent auth risks a student continuing after the teacher walks
away; per-action auth is tedious for multi-step fix-test-fix sessions.

Agreed model:

- **Staff roles, and members in `trust` workspaces:** `/manage` opens directly. No gate,
  no banner. Dev-workspace testing is unchanged.
- **Members in `keyword` / `staffauth` workspaces:** the page itself is **entry-gated** —
  the auth flow renders instead of the page content, which is never sent until auth
  succeeds (enforced server-side, not hidden client-side). Rationale: with all read-only
  status now on the launcher, there is no legitimate "just looking" use of /manage; a
  member should not see the available controls at all.
- **On success, a staff unlock session starts:** a server-side record scoped to the
  member's session (who authorized, expiry). The page shows a persistent banner:
  `🔓 Staff access — authorized by <name> — expires in 9:42 — [Lock now]`.
- **The unlock covers all gated actions on the page, including the irreversible online
  deletes.** No re-prompts while unlocked; confirm dialogs still appear.
- **Duration: sliding expiry (resets on each gated action), configurable per workspace**
  via a new workspace setting stored beside the existing MHS member-auth mode setting
  (relevant when mode ≠ trust). **Default: 10 minutes.**
- **Unlock ends** via: sliding expiry; the **Lock now** button (returns to the launcher);
  or logout (unlock is session-scoped, so it cannot follow the account to another
  device). After it ends, the next visit re-gates. If it expires mid-visit, the next
  request is rejected server-side and the page returns to the gate.
- `checkMemberAuth` is extended to honor an active unlock; every action POST is still
  validated server-side (client unlock state is cosmetic only).
- **Audit:** the unlock records the authorizer, so gated actions during the window can be
  logged with attribution ("authorized by <staff>") — including in keyword mode, which is
  anonymous today.
- Keyword mode uses the same unlock (the teacher types the keyword once per fix session,
  which also reduces shoulder-surfing exposure).

Walk-away risk assessment: a student acting within the unlock window can only affect
their own account/device (skip own progress, delete own save). Bounded by the sliding
expiry, the Lock now button, and session scoping.

### Defaults agreed

- Entry-link label: `⚙ Manage downloads & data`.
- Delete confirmations: consequence-stating confirm dialog (no type-to-confirm).
- Hero card keeps its live pipeline caption.
- Storage meter appears on both pages.
- Unlock duration default 10 minutes; per-workspace configurable.

### Decision history (options considered and rejected)

| Decision | Chosen | Rejected alternatives |
|---|---|---|
| Maintenance surface | Dedicated page `/missionhydrosci/manage` | Tabbed modal (hides 3 of 4 groups, modal-on-modal stacking, poor on Chromebooks); slide-over drawer (too cramped for version card / unit rows) |
| Launcher entry point | Discreet link, all roles | Staff-only visibility (breaks authorize-on-student-device flow); role-dependent labels (two labels to document) |
| Version visibility | All users, always, on launcher | Staff-only / staff + override-only (breaks the "what version is displayed?" support script) |
| Unit list on launcher | Status-only rows, no buttons | Rows with Download button; full rows with Download + Clear (tap-safety for students) |
| Group order on Manage | Device, Progress, Version, Deletes | Progress-first; Version-first (danger zone belongs last) |
| Member access to /manage | Entry-gated (page never renders without auth) | Auth on first action (would let members browse the available controls) |
| Auth persistence | Sliding unlock session, per-workspace duration, default 10 min | Per-action auth (tedious: full login per action); fixed-duration window (mid-work re-auth) |
| Deletes during unlock | Covered by the unlock | Sudo-style fresh auth per delete (inconsistent, "why did it ask again?") |

## Implementation scope (when implemented)

- New: `manage.go` (handler/VM) + `missionhydrosci_manage.gohtml`; route in `routes.go`.
- New: staff-unlock store (TTL-cleaned collection), gate/lock/status endpoints; extend
  `checkMemberAuth` to honor an active unlock.
- New: per-workspace unlock-duration setting + field in the workspace settings UI beside
  the MHS member-auth mode.
- Launcher restructure of `missionhydrosci_units.gohtml`; JS split — `mhs-delivery.js`
  loads on both pages; pipeline/launch JS stays on units; unit-row actions, clear/reset,
  set-unit, version, deletes, and the auth modal move to manage.
- Unchanged: all existing action endpoints, the stratasave delete API, the staff-auth
  and keyword verification flows themselves.
- Tailwind rebuild; verify light + dark themes and a Chromebook-sized viewport
  (~1366×768).

## Addendum 2026-07-09: content-area layout alignment (implemented)

After the initial implementation, both pages were reworked to follow the StrataHub
content-area design parameter (surfaces extend to the content-area edges with a
left/right/bottom margin and fill the available height — see
`docs/ui-design-content-area.md`; canonical docs in `stratasave/docs/ui_design.md` /
`ui-design-patterns.md`; reference pages: Organizations, Resources).

- **Launcher:** two full-width surfaces — the hero surface (launch content centered and
  width-constrained inside; the version line is its bottom caption) and a status surface
  with `flex-1` holding the full-width unit rows, storage meter, and the ⚙ Manage link.
- **Manage page:** full-width surfaces; **This device** takes `flex-1` (the analog of the
  Organizations table panel) and now contains a proper table — Unit | Version | Size |
  Downloaded | Progress | Actions. A per-row **Set current** button replaces the
  Progress card's select+Apply (the Progress card was removed; its summary line moved
  above the table). Completed units now show live download state and keep Download/Clear
  (with Set current, a completed unit can become playable again). Game version and the
  danger zone follow as compact full-width surfaces; the gate view uses a filling surface.
- **Clear all downloads / Reset all MHS data are now solid red** (user decision),
  matching the delete buttons. The recoverable-vs-permanent distinction is carried by the
  danger zone's amber border and "cannot be undone" captions instead of color.
- **Manage page header uses the standard StrataHub back pattern**: a bordered `← Back`
  button inline with the page title at the top-left of the content area (matching e.g.
  the Add Group page), replacing the earlier centered text-link header.

### Download-pipeline fixes (2026-07-10, verified on dev.adroit.games)

The original manage-page design deliberately ran no auto-download pipeline ("manual
control only"). That decision was wrong in practice: with the pipeline living only on
the launcher, downloads stopped advancing whenever the user was on /manage — after
Reset/Clear nothing re-downloaded, the next unit never started, and mid-download page
switches read as frozen. Fixes:

- **/manage now runs the same auto-download pipeline as the launcher** (current unit,
  then next, then cache trim), with the same purge-churn guards. Downloads behave
  identically regardless of which MHS page is open.
- ~~Clear all downloads / Reset all MHS data on /manage navigate to the launcher~~
  Revised same day (user preference + root-cause finding): **these actions now stay on
  /manage with no reload**; re-downloads run in the same page and show live progress in
  the units table.
- Clearing the current unit's download on /manage re-downloads it immediately, as the
  launcher always did.

**Root cause of the frozen download percentages (discovered by instrumenting real
Chrome against the CDN through a throttling proxy):** Chrome streams Background Fetch
byte-level progress only to the browsing context that STARTED the fetch. Any page
loaded afterward — a reload or a navigation — polls a frozen snapshot (the sum of
completed files) no matter how it re-obtains the registration. The download itself is
unaffected; only the display freezes. Consequences and fixes:

- **Never reload/navigate when a download may be in flight.** "Set current" now updates
  the page in place (badges, summary, buttons, pipeline vars) instead of reloading;
  Clear-all/Reset-all stay on /manage without reloading.
- **`mhs-delivery.js` poller floor:** `_pollDownloadOnce` additionally sums the manifest
  sizes of completed Background Fetch records (visible from any context) and uses that
  as a floor — a page that genuinely navigated mid-download now advances at file
  boundaries instead of freezing at the reconnect-time percentage.
- **Background Fetches are now created BY the service worker** (page posts the existing
  `download` message instead of calling `backgroundFetch.fetch()` itself). The SW is the
  one context shared by every page: its progress listener broadcasts live byte-level
  percentages over the BroadcastChannel, so navigating between /units and /manage
  mid-download keeps the percentage climbing on whichever page is open. Pages send a
  periodic `attachProgress` keepalive while a download is active — it wakes/extends the
  SW so the listeners keep firing, and re-attaches them after an SW restart. The
  file-boundary poller floor remains as the backstop. (SW_VERSION 1.0.8; deploy-safe in
  both directions: old SW ignores nothing it doesn't know, old pages keep the old path.)
- The brief "0%" at a download's start is Chrome's download-service warm-up (byte
  counts stay 0 for the first several seconds even in the creator context).

All verified on dev.adroit.games through a ~1.2 MB/s throttling proxy: live climbing
percentages after Clear-all while staying on /manage; Set current mid-download left the
active download's percentage climbing (previously froze); navigate-away-and-back
stepped 86→93→96% at file boundaries and reached Downloaded.

### Chromebook paused-download fixes (2026-07-15, verified on dev)

Field report from an ACER C933: Chrome **paused** every Background Fetch (ChromeOS
showed its own "Paused Downloading…" notifications) — downloads sat at 0%, our
watchdog reported "stalled", and Retry started another Background Fetch that Chrome
paused again. A paused fetch on an already-completed unit persisted across sessions
and was faithfully re-adopted on every page load ("Downloading 0%" on a Completed
unit). Chrome pauses background fetches under device conditions (metered connection,
battery saver), which is why only some devices see this. Fixes (all in
`mhs-delivery.js` + template pipelines; no SW changes):

- **Zero-byte auto-switch:** a Background Fetch that has delivered no bytes after 90s
  is aborted (nothing to lose) and the download switches to the SW sequential
  fallback — regular fetches, immune to download-service pausing, live byte-progress
  broadcasts. Fully automatic; no user interaction.
- **Retry uses the fallback path** instead of starting another (pausable) Background
  Fetch.
- **`mhs-prefer-fallback-until` (localStorage, 24h):** once pausing is detected, all
  downloads on that device go straight to the fallback for a day; self-corrects when
  the TTL expires.
- **Stray-download cleanup:** `autoCleanup` also aborts in-flight fetches for units
  outside current+next+manual, and both pipelines run it on every pass — kills
  persisted zombie fetches (the "Downloading on a Completed unit" case).
- Stall message now says "stalled or paused" and points at the browser's Paused
  notification (Resume) as well as Retry.
- Bonus fix: device-status upsert duplicate-key race (concurrent first reports from
  two pages) now retried — was a recurring 500 in telemetry.

**Resolution note:** the affected ACER later displayed a ChromeOS "Profile error
occurred" dialog — a corrupted local Chrome profile, which holds the download-service
job database, SW registrations, and Cache Storage. That corruption is the likely root
cause of the paused/zombie fetches on that one device (remedied by removing and
recreating the ChromeOS account). The fixes above remain as defense-in-depth: any
device whose download service misbehaves now self-heals via the fallback path.

## Related documents

- `docs/set-unit-and-clear/` — original set-unit/clear design and auth approach
- `docs/mission-hydrosci/mhs-collection-resolution-plan.md` — collection/version model
- `docs/reset-all-progress.md` — Reset All MHS Data background
- `internal/app/features/missionhydrosci/README.md` — feature overview; where state
  lives and what is safe to delete
