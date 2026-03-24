# Set Unit & Clear Downloads

This document describes the "Set to Unit" and "Clear All Downloads" features available in two places: the **MHS Dashboard** (for leaders/admins) and the **Mission HydroSci units page** (for any user with MHS access).

## Overview

"Set to Unit" allows changing a student's current unit in Mission HydroSci. Setting to unit X means:
- `current_unit` becomes the selected unit
- `completed_units` becomes all units before it
- Setting to Unit 1 is equivalent to a full progress reset (empty completed_units)

**No data is deleted.** Setting a unit only changes the pointer to which unit the student is playing. All save data, logging data, grades, and scores are preserved.

"Clear All Downloads" removes all cached unit files from the device without affecting progress.

These two actions were previously combined as "Reset All Progress" which both reset progress and cleared downloads. They are now separate concerns.

## Data Model

Progress is tracked in two independent systems:

| System | Collection | Field | Meaning |
|--------|-----------|-------|---------|
| **Mission HydroSci** (stratahub) | `mhs_user_progress` | `current_unit` | The unit the student is authorized to play — set by Mission HydroSci and the Set to Unit feature |
| **MHS Grader** (mhsgrader) | `progress_point_grades` | `currentUnit` | The unit the student last started playing in the game — set automatically when the grader sees a unit start event |

These values can diverge. For example, if a leader sets a student to Unit 4 via the dashboard, the grader's `currentUnit` will still reflect whatever unit the student last played until they launch Unit 4 in the game.

## Member Authorization

When a **member** (student) uses Set to Unit or Clear All Downloads on the Mission HydroSci units page, they must pass an authorization check. The authorization mode is configured per workspace via the `MHSMemberAuth` site setting. Three modes are supported:

### Staff Auth (default)

When `MHSMemberAuth` is `"staffauth"` (or empty/unset), a member must have a leader, coordinator, admin, or superadmin authenticate on their behalf. The flow:

1. Member clicks Set to Unit or Clear All Downloads
2. An authorization modal appears asking for the staff member's login ID
3. The staff member's authentication method determines the next step:
   - **Trust:** Immediate authorization (token returned directly)
   - **Password:** Staff member enters their password
   - **Email:** A verification code is sent to the staff member's email; they enter it in the modal
4. On success, the modal returns an `auth_token` which is included with the action request
5. The server validates and consumes the token before allowing the action

The staff auth verifier enforces that the authenticating user is a leader, coordinator, admin, or superadmin, and that they belong to the same workspace (or are a superadmin with no workspace restriction).

### Keyword

When `MHSMemberAuth` is `"keyword"`, a member must enter the workspace's configured keyword (`MHSMemberAuthKeyword`). The comparison is case-insensitive.

### Trust

When `MHSMemberAuth` is `"trust"`, no additional authorization is required for members. The action proceeds with a simple confirmation.

### Non-member roles

Leaders, admins, coordinators, and superadmins are **not** subject to member authorization. They see a standard browser `confirm()` dialog and proceed directly.

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

No additional member authorization is needed — the dashboard is a staff-only interface.

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

Any authenticated user with MHS access can set their own progress. The authorization check depends on role:

- **Members:** Must pass the workspace's configured authorization check (staff auth, keyword, or trust — see Member Authorization above)
- **Leaders, admins, coordinators, superadmins:** Shown a browser `confirm()` dialog — no additional authorization needed

### Flow

1. User selects a unit and clicks "Go"
2. `mhsSetToUnit()` checks `isMember` and `memberAuthMode`
   - If member and mode is not "trust": shows authorization modal (staff auth or keyword depending on mode)
   - If non-member or trust mode: shows `confirm()` dialog
3. On confirmation, `doSetToUnit(unit, authData)` POSTs JSON to `/missionhydrosci/api/progress/set-unit`
   - Includes `auth_token` (staff auth mode) or `keyword` (keyword mode) if applicable
4. Server validates authorization, then calls `ProgressStore.SetToUnit()`
5. Returns `{"ok": true, "unit": "unit3"}`
6. Client reloads the page

### Files

- **Handler:** `internal/app/features/missionhydrosci/progress.go` — `HandleSetToUnit`
- **Route:** `internal/app/features/missionhydrosci/routes.go` — `POST /api/progress/set-unit`
- **Template:** `internal/app/features/missionhydrosci/templates/missionhydrosci_units.gohtml` — select/button UI, authorization modal, JS functions

## Mission HydroSci — Clear All Downloads

### Location

Units page > expand "Manage downloads" > bordered box at the bottom, below the Set to Unit controls.

### UI

A full-width solid red button labeled "Clear All Downloads".

### Authorization

Same authorization pattern as Set to Unit:
- **Members:** Must pass the workspace's configured authorization check
- **Non-members:** Browser `confirm()` dialog

### Flow

1. User clicks "Clear All Downloads"
2. `mhsClearAllDownloads()` checks `isMember` and `memberAuthMode`, requests authorization
3. On confirmation, `doClearAllDownloads()`:
   - Calls `manager.deleteAllUnits()` to remove all cached unit files from the service worker cache
   - Removes `mhs-manual-downloads` from localStorage
   - Reloads the page

This action does **not** affect progress — the student's current unit and completed units remain unchanged. Individual unit downloads can also be cleared using the per-unit "Clear" buttons on each unit card.

### Files

- **Template:** `internal/app/features/missionhydrosci/templates/missionhydrosci_units.gohtml` — button markup, JS functions

## Authorization Modal

Both Set to Unit and Clear All Downloads on the Mission HydroSci units page share a single authorization modal. The modal adapts its UI based on the workspace's `MHSMemberAuth` setting:

### Staff Auth mode
The modal presents a multi-step flow:
1. **Login ID entry:** Member enters the staff member's login ID
2. **Challenge step:** Based on the staff member's auth method:
   - Password: shows a password field
   - Email: shows a verification code field with a "Resend code" option
   - Trust: skips directly to success
3. **Success:** Returns an `auth_token` used to authorize the action

### Keyword mode
The modal shows a single keyword input field. The keyword is validated against the workspace setting (case-insensitive).

### Staff Auth API endpoints

| Endpoint | Handler | Purpose |
|----------|---------|---------|
| `POST /api/auth/start` | `HandleStaffAuthStart` | Initiates auth challenge |
| `POST /api/auth/verify` | `HandleStaffAuthVerify` | Validates credential |
| `POST /api/auth/resend` | `HandleStaffAuthResend` | Resends email code |
| `POST /api/auth/keyword` | `HandleKeywordVerify` | Validates keyword |

## Workspace Settings

Member authorization is configured per workspace in `site_settings`:

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `mhs_member_auth` | string | `"staffauth"` | Authorization mode: `"staffauth"`, `"keyword"`, or `"trust"` |
| `mhs_member_auth_keyword` | string | (empty) | The keyword for keyword mode |

The `GetMHSMemberAuth()` method on `SiteSettings` returns the configured mode, defaulting to `"staffauth"` when empty.

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
