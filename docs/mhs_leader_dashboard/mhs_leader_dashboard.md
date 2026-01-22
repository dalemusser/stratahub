# MHS Leader Dashboard - Design Document

## Overview

The **Mission HydroSci (MHS) Leader Dashboard** provides teachers and group leaders with a visual overview of student progress through the MHS curriculum. The dashboard displays a grid-based view showing each student's completion status across all curriculum units and progress points.

---

## Purpose

- Allow leaders to quickly assess group-wide progress at a glance
- Identify students who may need additional support (yellow status)
- Track completion of individual activities across all units
- Switch between different groups to view each group's progress

---

## Data Architecture

### Progress Point Configuration

The curriculum structure (units and progress points) is defined in a configuration file:

**File**: `internal/app/resources/mhs_progress_points.json`

This file contains:
- Unit definitions (ID, title)
- Progress point definitions (ID, short name, description)
- Number of progress points per unit

The configuration file is loaded once at startup and cached for the lifetime of the application.

### Progress Values

Progress values are stored in the database and computed from log data:

| Value | Color | Meaning |
|-------|-------|---------|
| (no value) | White | Not started - student has not attempted the progress point |
| 1 | Yellow | Completed with difficulties |
| 2 | Green | Successfully completed |

### Data Flow

```
┌─────────────────────────────────┐
│  mhs_progress_points.json       │ ← Loaded once, cached
│  (curriculum structure)         │
└─────────────────────────────────┘
              ↓
┌─────────────────────────────────┐
│  Dashboard Handler              │
│  - Loads group members          │
│  - Fetches progress values      │
│  - Combines with curriculum     │
└─────────────────────────────────┘
              ↓
┌─────────────────────────────────┐
│  Dashboard UI                   │
│  - Grid layout from config      │
│  - Cell colors from values      │
└─────────────────────────────────┘
```

---

## Visual Design

### Color Palette

| Name | Hex Code | Usage |
|------|----------|-------|
| Hydro Deep | `#0c2d48` | Header background |
| Hydro Mid | `#145374` | Secondary headers |
| Hydro Light | `#2e8bc0` | Tertiary headers, accents |
| Hydro Pale | `#b1d4e0` | Subtle text, borders |
| Hydro Foam | `#e8f4f8` | Light backgrounds |
| Status Success | `#22c55e` | Completed cells (green) - value 2 |
| Status Warning | `#eab308` | Completed with difficulties (yellow) - value 1 |
| Status Empty | `#ffffff` | Not Started cells (white) - no value |

### Typography

- **Primary Font**: Outfit (sans-serif) - Used for all UI text
- **Monospace Font**: Space Mono - Used for progress point IDs

---

## Layout Structure

### Header
- Application branding: "Mission HydroSci" with water droplet icon
- Subtitle: "Leader Dashboard — Group Progress Overview"
- Blue gradient background (deep → mid → light)

### Group Selection Bar
- **Group selector dropdown** - allows leader to switch between groups
- Student count for selected group
- Last updated timestamp
- Legend showing cell status meanings (green/yellow/white)
- **Refresh button** and **countdown timer** (30 second auto-refresh)

### Data Grid (Main Content)

The grid uses a frozen row/column pattern for navigation:

```
┌─────────────┬────────────────────────────────────────┐
│  CORNER     │  HEADER (scrolls horizontally)         │
│  (frozen)   │  - Unit titles                         │
│             │  - Progress point IDs                  │
│             │  - Short descriptions (vertical)       │
├─────────────┼────────────────────────────────────────┤
│  NAMES      │  DATA AREA                             │
│  (scrolls   │  (scrolls both directions)             │
│  vertically)│                                        │
│             │  [cells showing student progress]      │
│             │                                        │
└─────────────┴────────────────────────────────────────┘
```

#### Corner Cell (Frozen Both Ways)
- "Name / Student" label
- "Progress Points →" indicator

#### Header Area (Frozen Vertically, Scrolls Horizontally)
- **Row 1**: Unit headers spanning their progress points
- **Row 2**: Progress point IDs (e.g., u1p1, u1p2)
- **Row 3**: Short activity descriptions (displayed vertically)

#### Names Column (Frozen Horizontally, Scrolls Vertically)
- Student names in alphabetical order
- Alternating row background colors for readability

#### Data Area (Scrolls Both Ways)
- Grid of status cells (28px × 28px)
- Synchronized scrolling with header and names
- Container fits within page and resizes with window (like /resources)

---

## Interactive Features

### Group Selection
- Dropdown menu at top of UI listing all groups the leader has access to
- Selecting a group refreshes the display with that group's members and progress data
- Members are displayed alphabetically in the Names column

### Auto-Refresh
Following the pattern established in `/activity` (Activity Dashboard):
- **30-second auto-refresh** using HTMX `hx-trigger="every 30s"`
- **Countdown display** showing seconds until next refresh
- **Manual Refresh button** to trigger immediate refresh
- Scroll position is preserved across refreshes

### Scrolling Behavior
- Main data area scrolls both horizontally and vertically
- Header row stays fixed at top, scrolls horizontally with data
- Names column stays fixed at left, scrolls vertically with data
- Container resizes with window (like /resources feature)

### Tooltips
Tooltips on hover for:
- Unit headers → show unit title
- Progress point headers → show full description

Note: No tooltips on individual cells - colors alone indicate status.

---

## Responsive Considerations

- Container fits within page and resizes with window
- Minimum height: 400px for the table container
- Names column width: 140px minimum
- Data cells: 28px × 28px fixed size
- Header row heights: 40px (units), 28px (IDs), 80px (descriptions)
- Both vertical and horizontal scrolling supported

---

## Feature Decisions Summary

| Feature | Decision |
|---------|----------|
| **Data Source** | Progress values from database (computed from logs); curriculum structure from JSON config file |
| **Auto-refresh** | Yes - 30 second interval with countdown and manual refresh button |
| **Filtering** | Not needed - groups are small enough |
| **Sorting** | Not needed - alphabetical by name is sufficient |
| **Export** | Future iteration - not in initial implementation |
| **Multiple Groups** | Yes - dropdown selector to switch groups |
| **Drill-down** | No - cell colors only, no additional detail on click |
| **Mobile** | Follow standard stratahub patterns |

---

## Configuration File Format

### mhs_progress_points.json

```json
{
  "units": [
    {
      "id": "u1",
      "title": "Unit 1 Title",
      "progress_points": [
        {
          "id": "u1p1",
          "short_name": "Point 1",
          "description": "Description of progress point u1p1"
        },
        {
          "id": "u1p2",
          "short_name": "Point 2",
          "description": "Description of progress point u1p2"
        }
      ]
    }
  ]
}
```

---

## Example Curriculum Structure

> **Note**: The following is placeholder/example data for development purposes. The actual MHS curriculum titles and descriptions will be provided separately.

### Units and Progress Points (Example)

| Unit | Title | Progress Points | Total |
|------|-------|-----------------|-------|
| Unit 1 | Unit 1 Title | u1p1 - u1p5 | 5 |
| Unit 2 | Unit 2 Title | u2p1 - u2p6 | 6 |
| Unit 3 | Unit 3 Title | u3p1 - u3p5 | 5 |
| Unit 4 | Unit 4 Title | u4p1 - u4p4 | 4 |
| Unit 5 | Unit 5 Title | u5p1 - u5p6 | 6 |
| **Total** | | | **26** |

---

## Database Schema

### Progress Data (TBD)

Progress values are stored in the database. The exact schema will depend on how log data is structured and computed into progress values.

Expected query pattern:
- Given a group ID, retrieve all members
- For each member, retrieve their progress values for all progress points
- Progress value is: null (not started), 1 (completed with difficulties), or 2 (completed successfully)

---

## Implementation Notes

### Initial Implementation
- Progress data is not yet available in the database
- **Mock data will be generated** for display to demonstrate the UI
- Mock data should simulate realistic distribution of green/yellow/white values

### File Locations
- Config file: `internal/app/resources/mhs_progress_points.json`
- Feature code: `internal/app/features/mhsdashboard/` (TBD)
- Templates: `internal/app/features/mhsdashboard/templates/` (TBD)

### Caching
- The `mhs_progress_points.json` file should be loaded once at application startup
- Cached in memory for the lifetime of the application
- No need to reload on each request

---

## Future Enhancements (Not in Initial Implementation)

- **Export to CSV/PDF** - Allow leaders to export progress data
- **Progress summary statistics** - Show completion percentages per unit
- **Historical view** - Show progress over time

---

## Revision History

| Date | Author | Changes |
|------|--------|---------|
| 2026-01-21 | Initial | Created document based on dashboard1.html and dashboard1.png mockups |
| 2026-01-21 | Update | Added data architecture, group selection, auto-refresh details, feature decisions; marked curriculum as example data |
