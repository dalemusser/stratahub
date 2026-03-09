# localStorage & sessionStorage Usage

## Current Usage

### localStorage (persistent across sessions)

| Key | Feature | Purpose |
|-----|---------|---------|
| `theme` | Layout | Dark/light mode choice (browser copy; canonical value is in the database via Profile preferences) |
| `sidebar-collapsed` | Layout | Sidebar expanded/collapsed state |
| `dismissed-announcements-{loginId}` | Layout + Announcements | Which announcements a user has dismissed (JSON array of ObjectID hex strings) |
| `activity_history_timezone` | Activity | Timezone preference for activity timestamps |
| `audit_log_timezone` | Audit Log | Timezone preference for audit log timestamps |
| `groups_hide_orgs` | Groups | Organization sidebar visibility toggle |
| `members_hide_orgs` | Members | Organization sidebar visibility toggle |
| `leaders_hide_orgs` | Leaders | Organization sidebar visibility toggle |

### sessionStorage (cleared on browser close)

| Key | Feature | Purpose |
|-----|---------|---------|
| `mhs-install-dismissed` | MHS Units | PWA install banner dismissed for this session |
| `missionhydrosci-install-dismissed` | Mission HydroSci | PWA install banner dismissed for this session |
| `missionhydrosci-ios-install-dismissed` | Mission HydroSci | iOS install banner dismissed for this session |
| `missionhydroscix-install-dismissed` | Mission XydroSci | PWA install banner dismissed for this session |
| `missionhydroscix-ios-install-dismissed` | Mission XydroSci | iOS install banner dismissed for this session |

## Known Issue: Announcement Dismissal List Growth

The `dismissed-announcements-{loginId}` list grows unbounded. When an admin deletes an old announcement from the database, its ID remains in the localStorage array forever. Each ID is a 24-char hex string, so growth is slow (~2.5 KB per 100 announcements), but there is no cleanup mechanism.

**Potential fix:** Prune the stored list against announcements actually present on the page — only retain IDs that match a `[data-announcement-id]` element. This would naturally clean up stale entries on each page load.

## Future: Clear Local Data UI

### Concept

Add a UI (likely on the Profile page) that lets users selectively clear browser-stored state. Individual items can be cleared independently, with a "Clear All" option.

### Clearable Items

- Dismissed announcements
- Theme (browser copy — database preference still applies on next load)
- Sidebar state
- Timezone (Activity)
- Timezone (Audit Log)
- Organization sidebar (Groups/Members/Leaders — could group as one item)

### Design Considerations

- **Role filtering:** Only show items relevant to the user's role. A member wouldn't see "Audit Log Timezone" or org sidebar toggles.
- **Labeling:** Avoid "Reset Preferences" — that implies it would reset database-backed preferences (like the theme setting on Profile). This is browser-local state, not account preferences.
- **Confirmation:** A brief confirmation or immediate feedback (not a modal) since clearing individual items is low-risk.
- **All items reset gracefully:** Theme falls back to system preference, sidebar expands, announcements reappear, timezones fall back to browser default, org sidebars show.
