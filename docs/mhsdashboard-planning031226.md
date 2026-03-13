# MHS Dashboard — Planning 2026-03-12

> **Updated** after discussion — reflects decisions on status terminology, "in unit" indicator, and data source architecture.

## Decisions

1. **mhsgrader is the single data source** — The dashboard reads only from `progress_point_grades`. It does not query stratalog directly. All status computation (active, passed, flagged) is done by the grader.
2. **Status terminology** — Dashboard maps these statuses to visual indicators:
   - `pending` (no record) — not started (white/empty cell)
   - `active` — in-progress (pencil icon or animated indicator, not color-dependent)
   - `passed` — completed successfully (theme-dependent color, was "green")
   - `flagged` — completed with concerns (theme-dependent color, was "yellow"; clickable for review modal)
3. **"In unit" bar indicator** — A horizontal bar/line above the progress point cells in a student's row, spanning the unit they are currently in. Subtle, theme-independent, shows at a glance where each student is working.
4. **Reason codes** — Implement detailed reason display for Units 1–3 now. Units 4–5 when available.

## Current State

The MHS Dashboard (`/mhsdashboard`) is a leader/admin-facing progress tracking grid.

### What's Implemented

**Progress Tab:**
- Color-coded grid (currently uses "green"/"yellow" terminology — to be updated)
- Frozen-header synchronized scrolling (student names + unit/point headers)
- Clickable point headers showing point info modal
- Clickable yellow cells showing review reason modal with teacher guidance
- Group selector dropdown (role-scoped: leaders see their groups, admins see all)
- 6 colorblind-friendly theme options
- 30-second auto-refresh with manual refresh button
- Sort toggle (A-Z / Z-A by student name)

**Devices Tab:**
- Per-student device readiness table
- Shows: device type, PWA installed, per-unit cache status, storage bar, last seen
- Stale device detection (>7 days)

**Data Sources:**
- StrataHub DB: users, groups, group_memberships, organizations, mhs_device_status
- MHSGrader DB: progress_point_grades collection (single source for all progress states)

**Access Control:**
- Allowed roles: leader, admin, coordinator, superadmin
- Leaders scoped to their groups; admins/coordinators/superadmins see all groups

### Key Files

| File | Purpose |
|------|---------|
| `mhsdashboard/handler.go` | Handler struct with DB, GradesDB, ErrLog, Log |
| `mhsdashboard/routes.go` | `GET /` (ServeDashboard), `GET /grid` (ServeGrid for HTMX) |
| `mhsdashboard/dashboard.go` | Core logic: group loading, member fetching, grade loading, row building |
| `mhsdashboard/config.go` | Loads `mhs_progress_points.json` with sync.Once |
| `mhsdashboard/types.go` | View models: DashboardData, GridData, MemberRow, CellData, DeviceInfo, etc. |
| `mhsdashboard/templates/mhsdashboard_view.gohtml` | Main page layout, controls, tabs, modals |
| `mhsdashboard/templates/mhsdashboard_grid.gohtml` | Grid content (progress + devices), HTMX-refreshed |
| `internal/app/resources/mhs_progress_points.json` | Unit/point definitions (names, descriptions) |

## Work to Complete

### 1. Update `mhs_progress_points.json`

Fix point counts and update placeholder names/descriptions from mhsgrading/ docs:

- **Unit 4**: 4 → 6 points (add u4p5, u4p6; update all names/descriptions)
- **Unit 5**: 6 → 4 points (remove u5p5, u5p6; update all names/descriptions)
- **Units 1–3**: Verify names/descriptions match mhsgrading/ docs

### 2. Update Status Terminology in Dashboard

Replace "green"/"yellow" references throughout dashboard code:
- `dashboard.go`: Grade loading, row building, cell status mapping
- `types.go`: CellData status field
- Templates: CSS classes, conditional rendering
- Map: `passed` → success color, `flagged` → review color, `active` → icon/animation, `pending` → empty

### 3. Add "Active" (In-Progress) Display

- Render `active` status cells with a pencil icon or animated indicator
- Must work across all 6 colorblind-friendly themes (not color-dependent)
- `active` cells are not clickable (no review modal — student hasn't finished)

### 4. Add "In Unit" Bar Indicator

- Horizontal bar/line above progress point cells for the unit the student is currently in
- Data source: unit-level tracking from mhsgrader (stored per-student)
- Spans the columns for that unit in the student's row
- Subtle accent — works across all themes

### 5. Implement Reason Code Display for Units 1–3

- Use per-point reason codes and teacher guidance from `Reason-codes-and-instructor-messages.md`
- Review modal shows: reason code label, short description, long description, teacher guidance
- Replace current generic mapping with spec-driven content

### 6. Reason Codes for Units 4–5

When available from the analytics team, add to the reason code system. The dashboard infrastructure built for U1–3 will support U4–5 without structural changes.

## Dependencies

- **mhsgrader**: Must produce `active`/`passed`/`flagged` statuses and unit-level tracking before dashboard can display them
- **Grading spec**: Reason codes for Units 4–5 pending from analytics team
- **`mhs_progress_points.json`**: Must be updated before dashboard displays correct U4–5 point names

## Implementation Sequence

1. **Update `mhs_progress_points.json`** — Can do immediately
2. **Update status terminology** — After mhsgrader switches from green/yellow to passed/flagged
3. **Add "active" cell rendering** — After mhsgrader produces `active` status
4. **Add "in unit" bar** — After mhsgrader produces unit-level tracking
5. **Implement U1–3 reason codes** — Can proceed in parallel with grader work
6. **Add U4–5 reason codes** — When specs are available
