# Member Navigation and Landing Pages

This document describes how navigation and landing pages work for members during the impact study. Members fall into two groups based on their group's app assignments in **Groups > Manage > Apps**:

- **Experimental group**: Members assigned Mission HydroSci
- **Control group**: Members not assigned Mission HydroSci

## Menu

The Dashboard has been removed from the member menu. It did not serve any meaningful purpose for members — they don't use the dashboard widgets, and it added an unnecessary step between login and their actual work.

### Experimental group (Mission HydroSci assigned)

1. Mission HydroSci
2. Resources

### Control group (Mission HydroSci not assigned)

1. Resources

Mission HydroSci does not appear in the menu for control group members. They have no visibility into or access to Mission HydroSci.

## Landing Pages

### After login

| Group | Destination |
|-------|-------------|
| Experimental | `/missionhydrosci/units` — the Mission HydroSci units page |
| Control | `/member/resources` — the Resources page |

If a return URL is present (e.g., the member was redirected to login from a specific page), that URL takes precedence.

### Visiting the base URL (`/`) while logged in

The same routing applies when a logged-in member navigates to the base URL of the workspace (e.g., `https://mhs.adroit.games`):

| Group | Destination |
|-------|-------------|
| Experimental | Redirected to `/missionhydrosci/units` |
| Control | Redirected to `/member/resources` |

Non-member roles (admin, leader, coordinator) continue to see the landing page as before.

### Launching the installed Chrome App (PWA)

The PWA manifest sets `start_url` to `/missionhydrosci/units`. When a member launches the installed Chrome App:

- If their session is still active, they land directly on the Mission HydroSci units page.
- If their session has expired, they are redirected to the login page. After logging in, they are sent to `/missionhydrosci/units` (via the return URL preserved by the auth redirect).

The PWA is only relevant to experimental group members. Control group members do not have the Mission HydroSci install banner and are not prompted to install the app.

## Why This Was Done

The impact study requires a clean separation between experimental and control groups. Each group should land directly on the feature that matters to them:

- **Experimental group**: Mission HydroSci is their primary activity. Going straight to the game units page eliminates unnecessary clicks and reduces confusion.
- **Control group**: Resources is where they take tests, complete surveys, and access teacher-provided materials for traditional instruction. It serves a similar role for the control group as Mission HydroSci does for the experimental group.

Removing the Dashboard simplifies the member experience and ensures that both groups start in the right place from the moment they log in.

## Non-member Roles

These changes only affect members. Admin, leader, coordinator, and superadmin roles retain the Dashboard in their menu and continue to land on `/dashboard` after login. They can access all features regardless of app assignments.
