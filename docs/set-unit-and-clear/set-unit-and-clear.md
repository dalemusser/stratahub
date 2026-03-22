# Set Unit & Clear Downloads

This document describes the "Set to Unit" and "Clear All Downloads" features available in two places: the **MHS Dashboard** (for leaders/admins) and the **Mission HydroSci units page** (for any user with MHS access).

## Overview

"Set to Unit" allows changing a student's current unit in Mission HydroSci. Setting to unit X means:
- `current_unit` becomes the selected unit
- `completed_units` becomes all units before it
- Setting to Unit 1 is equivalent to a full progress reset (empty completed_units)

"Clear All Downloads" removes all cached unit files from the device without affecting progress.

These two actions were previously combined as "Reset All Progress" which both reset progress and cleared downloads. They are now separate concerns.

## Data Model

Progress is tracked in two independent systems:

| System | Collection | Field | Meaning |
|--------|-----------|-------|---------|
| **Mission HydroSci** (stratahub) | `mhs_user_progress` | `current_unit` | The unit the student is authorized to play — set by Mission HydroSci and the Set to Unit feature |
| **MHS Grader** (mhsgrader) | `progress_point_grades` | `currentUnit` | The unit the student last started playing in the game — set automatically when the grader sees a unit start event |

These values can diverge. For example, if a leader sets a student to Unit 4 via the dashboard, the grader's `currentUnit` will still reflect whatever unit the student last played until they launch Unit 4 in the game.

## MHS Dashboard — Set Progress

### Location

Progress tab > hover over a member row > click the settings icon next to the member name.

### UI

A dropdown menu appears (positioned with `position: fixed` to escape the overflow-hidden scroll container) showing "Set to Unit" with buttons for each unit (Unit 1 through Unit 5).

### Authorization

Only leaders, admins, coordinators, and superadmins can access the dashboard. The set-progress handler enforces:
- Role must be leader, admin, coordinator, or superadmin
- Requester must have access to the selected group
- Target user must be a member of the selected group

### Confirmation

Clicking a unit shows a browser `confirm()` dialog: "Set to Unit X for [Member Name]? All prior units will be marked completed."

### Flow

1. User clicks unit button in dropdown
2. `hx-confirm` shows confirmation dialog
3. On confirm, HTMX POSTs to `/mhsdashboard/set-progress` with `user_id`, `unit`, and `group`
4. Server calls `ProgressStore.SetToUnit()` to update `mhs_user_progress`
5. Server responds with `HX-Trigger: refreshGrid`
6. Client receives trigger, closes any open menus, and refreshes the grid via HTMX

### Auto-refresh behavior

The dashboard grid auto-refreshes every 30 seconds. If a set-progress menu is open when the timer fires, the refresh is deferred (countdown resets to 30s) to prevent the menu from being destroyed mid-interaction. Server-triggered refreshes (after a set-progress action) close the menu first and then refresh immediately.

### Files

- **Handler:** `internal/app/features/mhsdashboard/dashboard.go` — `HandleSetProgress`
- **Grid template:** `internal/app/features/mhsdashboard/templates/mhsdashboard_grid.gohtml` — set-progress menu markup
- **View template:** `internal/app/features/mhsdashboard/templates/mhsdashboard_view.gohtml` — `toggleSetProgress` / `closeSetProgressMenus` JS, refresh-skip logic

## MHS Dashboard — Current Unit Indicators

The dashboard progress grid shows two bar indicators on each student's row:

### Green bar (top of cell)
- **Source:** `mhs_user_progress.current_unit` (Mission HydroSci progress store)
- **Meaning:** The unit the student is currently assigned to in Mission HydroSci
- **CSS class:** `mhs-in-mhs-unit-bar`
- **Legend label:** "MHS Unit"

### Red bar (bottom of cell)
- **Source:** `progress_point_grades.currentUnit` (MHS Grader)
- **Meaning:** The unit the student last started playing in the game
- **CSS class:** `mhs-in-unit-bar`
- **Legend label:** "Playing Unit"

Both bars have hover tooltips (implemented via JS with `position: fixed` to avoid z-index clipping issues in the scrollable grid). The green bar tooltip reads "Current unit in Mission HydroSci" and the red bar tooltip reads "Unit last played in game".

### Data loading

`buildProgressRows` loads both data sources:
- Grader data via `loadProgressGrades()` from the mhsgrader database
- MHS progress via `ProgressStore.ListByUserIDs()` from the stratahub database

Each cell in the grid has both `IsInCurrentUnit` (grader) and `IsInMHSUnit` (progress store) boolean flags.

### Files

- **Data loading:** `internal/app/features/mhsdashboard/dashboard.go` — `buildProgressRows`
- **Cell type:** `internal/app/features/mhsdashboard/types.go` — `CellData.IsInMHSUnit`
- **Store method:** `internal/app/store/mhsuserprogress/store.go` — `ListByUserIDs`
- **Bar CSS:** `internal/app/features/mhsdashboard/templates/mhsdashboard_view.gohtml`
- **Bar HTML:** `internal/app/features/mhsdashboard/templates/mhsdashboard_grid.gohtml`

## Mission HydroSci — Set to Unit

### Location

Units page > expand "Manage downloads" > bordered box at the bottom with "Set to [select] Go".

### UI

A `<select>` dropdown populated from the unit data (showing full titles like "Unit 2: Water Quality"), a "Set to" label, and a "Go" button. The select and button are centered in a bordered container.

### Authorization

Any authenticated user with MHS access can set their own progress. The server does not restrict by role — the keyword gate provides the authorization check for members.

- **Members:** Must enter the keyword `hydroreset` in a modal dialog before the action proceeds
- **Leaders, admins, coordinators, superadmins:** Shown a browser `confirm()` dialog

### Flow

1. User selects a unit and clicks "Go"
2. `mhsSetToUnit()` checks `isMember`
   - If member: shows keyword modal (shared modal used by both Set to Unit and Clear All Downloads)
   - If non-member: shows `confirm()` dialog
3. On confirmation, `doSetToUnit(unit)` POSTs JSON `{"unit": "unit3"}` to `/missionhydrosci/api/progress/set-unit`
4. Server validates unit (must be unit1-unit5), calls `ProgressStore.SetToUnit()`
5. Returns `{"ok": true, "unit": "unit3"}`
6. Client reloads the page

### Files

- **Handler:** `internal/app/features/missionhydrosci/progress.go` — `HandleSetToUnit`
- **Route:** `internal/app/features/missionhydrosci/routes.go` — `POST /api/progress/set-unit`
- **Template:** `internal/app/features/missionhydrosci/templates/missionhydrosci_units.gohtml` — select/button UI, keyword modal, JS functions

## Mission HydroSci — Clear All Downloads

### Location

Units page > expand "Manage downloads" > bordered box at the bottom, below the Set to Unit controls.

### UI

A full-width solid red button labeled "Clear All Downloads".

### Authorization

Same keyword/confirmation pattern as Set to Unit:
- **Members:** Must enter `hydroreset` keyword
- **Non-members:** Browser `confirm()` dialog

### Flow

1. User clicks "Clear All Downloads"
2. `mhsClearAllDownloads()` checks `isMember` and requests confirmation
3. On confirmation, `doClearAllDownloads()`:
   - Calls `manager.deleteAllUnits()` to remove all cached unit files from the service worker cache
   - Removes `mhs-manual-downloads` from localStorage
   - Reloads the page

This action does **not** affect progress — the student's current unit and completed units remain unchanged. Individual unit downloads can also be cleared using the per-unit "Clear" buttons on each unit card.

### Files

- **Template:** `internal/app/features/missionhydrosci/templates/missionhydrosci_units.gohtml` — button markup, JS functions

## Shared Keyword Modal

Both Set to Unit and Clear All Downloads on the Mission HydroSci units page share a single keyword confirmation modal (`#keyword-modal`). The modal is configured dynamically via `showKeywordModal(title, desc, callback)`:

- `title` — displayed as the modal heading
- `desc` — displayed as the description text
- `callback` — function called on successful keyword entry

The keyword is `hydroreset` (case-insensitive). On incorrect entry, an "Incorrect keyword" error is shown. The modal can be dismissed with the Cancel button or the Escape key.

## Store Methods

### `SetToUnit(ctx, workspaceID, userID, targetUnit)`

Located in `internal/app/store/mhsuserprogress/store.go`. Updates `mhs_user_progress`:
- Validates unit format (unit1-unit5)
- Sets `current_unit` to the target
- Sets `completed_units` to all units before the target (e.g., unit3 gets `["unit1", "unit2"]`)
- Updates `updated_at` timestamp

Used by both the dashboard handler and the Mission HydroSci handler.

### `ListByUserIDs(ctx, workspaceID, userIDs)`

Located in `internal/app/store/mhsuserprogress/store.go`. Batch query for the dashboard:
- Fetches all `mhs_user_progress` documents matching the workspace and user IDs
- Returns a map keyed by user ID hex string
- Used by `buildProgressRows` to populate the green "MHS Unit" bar
