# MHS Collection Resolution — Implementation Plan

**Date:** 2026-04-05

## Context

MHS Builds allows creating collections of unit versions. We need a flexible system for determining which collection a player sees. Different groups may need different versions (e.g., freezing a study group on a known-good build while newer groups get updated builds). Testers and developers need to select any collection for testing, including when logged in as members so their play data flows through the full pipeline (game → log → grader → dashboard).

This replaces the current session-based "Use for Testing" approach with a database-backed, per-user override system and adds per-group collection pinning.

---

## 1. Collection Resolution Hierarchy

When a user plays Mission HydroSci, the system resolves which collection to use in this order (most specific wins):

1. **Per-user override** — user has explicitly chosen a collection via the picker in Mission HydroSci (stored in `mhs_user_progress`)
2. **Per-group pin** — the user's group has been pinned to a specific collection (stored in `group_app_settings`)
3. **Workspace active** — the default collection for all members in this workspace (stored in `site_settings.mhs_active_collection_id`)
4. **None** — no collection configured; Mission HydroSci shows "not currently available"

---

## 2. Per-User Override

### What it does

Any user — member, admin, coordinator — can have a specific collection selected for them. When they play Mission HydroSci, they get that collection's units instead of whatever the group or workspace default is.

### How it's set

A **collection picker** appears in Mission HydroSci (on the units page) for users who have access:

- **Admins and coordinators**: see the picker directly, no extra auth needed
- **Members**: the picker is hidden by default. Access is granted via the same staff-auth method configured in the workspace settings (trust / keyword / staffauth). This is the same pattern used for clearing downloads and setting the unit.

The picker shows:
- "Default" — removes the override, returns to group/workspace collection
- A list of all available collections with name and version summary

### How it's stored

Add a field to the `mhs_user_progress` document:

```
CollectionOverrideID *primitive.ObjectID `bson:"collection_override_id,omitempty"`
```

This is per-workspace, per-user. Setting it to nil means "use default" (group pin or workspace active).

### How it's cleared

The user (or an admin acting on their behalf) selects "Default" in the picker. This clears the override. The same auth method is required to change or clear.

### Store changes

Add to `mhsuserprogress.Store`:
- `SetCollectionOverride(ctx, workspaceID, userID, collectionID *ObjectID) error`

---

## 3. Per-Group Collection Pin

### What it does

A group can be pinned to a specific collection. All members of that group play that collection instead of the workspace active one. This is useful for:
- Freezing a study group on a known-good build while the game evolves
- Updating just one unit (e.g., fix unit 5) and pinning the group to the new derivative collection
- Running different groups on different versions for A/B comparison

### How it's set

In **Groups > Manage > Apps** where Mission HydroSci is enabled for a group. Below the enable/disable toggle, add:

- **Collection**: dropdown showing "Use workspace active (default)" plus all available collections
- Selecting a collection pins the group; selecting "Use workspace active" removes the pin

### How it's stored

Add a field to `group_app_settings`:

```
MHSCollectionID *primitive.ObjectID `bson:"mhs_collection_id,omitempty"`
```

When nil or absent, the group follows the workspace active collection. When set, the group is pinned.

### Where it's managed

- **Groups > Manage > Apps** — the primary place to set/change the pin
- Not in the MHS Dashboard (dashboard is read-only for this; it shows what's in use but doesn't change it)

---

## 4. Changes to Manifest Resolution

### Current flow (`api_manifest.go`)

```
resolveManifest(r):
  1. Check session for staff override → use that
  2. Check workspace active collection → use that
  3. Return empty
```

### New flow

```
resolveManifest(r):
  1. Check per-user override (mhs_user_progress.collection_override_id) → use that
  2. Check per-group pin (group_app_settings.mhs_collection_id for user's MHS group) → use that
  3. Check workspace active collection (site_settings.mhs_active_collection_id) → use that
  4. Return empty (MHS not available)
```

The old session-based override (`mhs_collection_override` session key) is removed. The `HandleSetCollectionOverride` and `HandleClearCollectionOverride` endpoints are replaced by new endpoints that write to `mhs_user_progress`.

### Group lookup

The user may belong to multiple groups. For MHS, we need the group that has `missionhydrosci` enabled. Use `groupapps.Store` to find the user's MHS-enabled group and check its `mhs_collection_id`.

---

## 5. Collection Picker in Mission HydroSci

### Location

On the units page (`missionhydrosci_units.gohtml`), accessible to:
- Admins and coordinators: always visible (small icon/button)
- Members: hidden by default, revealed after staff-auth (same pattern as existing member action authorization)

### UI

A button/icon that opens a modal or dropdown listing:
- **Default** (use group/workspace collection) — shown first, selected when no override is set
- Each available collection: name, unit version summary (e.g., "unit1-4:v2.2.2, unit5:v2.2.3")
- The currently effective collection is highlighted
- Selecting one triggers the auth flow (for members) then saves the override

### Endpoints

| Method | Path | Description |
|--------|------|-------------|
| GET | /api/collections | List available collections for the picker |
| POST | /api/collection-override | Set per-user override (body: `collection_id` or empty to clear) |

The POST endpoint checks the workspace's MHS member auth setting for members. Admins/coordinators bypass auth.

---

## 6. MHS Dashboard Changes

### Show collection info

On the dashboard grid, for each member show:
- **Effective collection**: which collection they're actually using
- **Override indicator**: if they have a per-user override, show it distinctly from the group/workspace default

### Show group pin

In the group selector or group header area, show:
- "Collection: [name]" if the group is pinned
- "Collection: Workspace active" if using the default

### Data tracking

The game log data should already include the unit version (from the manifest). Verify that the version information flows through to the grader and dashboard display. If not, the version from the manifest can be included in the progress tracking.

---

## 7. Remove "Use for Testing" Button

The "Use for Testing" button on the MHS Builds collection detail page is replaced by the collection picker in Mission HydroSci. Remove:
- The `setTestOverride` JavaScript function and button from `mhsbuilds_collection_detail.gohtml`
- The session-based override endpoints (`HandleSetCollectionOverride`, `HandleClearCollectionOverride`)
- The `getCollectionOverride` helper that reads from session
- The `collectionOverrideKey` constant

---

## 8. Model Changes Summary

### `mhs_user_progress` — add field
```go
CollectionOverrideID *primitive.ObjectID `bson:"collection_override_id,omitempty"`
```

### `group_app_settings` — add field
```go
MHSCollectionID *primitive.ObjectID `bson:"mhs_collection_id,omitempty"`
```

### No new collections needed

---

## 9. Files to Modify

| File | Change |
|------|--------|
| `internal/domain/models/mhs_user_progress.go` | ADD `CollectionOverrideID` field |
| `internal/domain/models/groupappsetting.go` | ADD `MHSCollectionID` field |
| `internal/app/store/mhsuserprogress/store.go` | ADD `SetCollectionOverride` method |
| `internal/app/store/groupapps/groupappstore.go` | ADD `GetMHSCollectionID` method, update Enable/save to include collection field |
| `internal/app/features/missionhydrosci/api_manifest.go` | REWRITE `resolveManifest` for new hierarchy, replace session endpoints with DB endpoints |
| `internal/app/features/missionhydrosci/routes.go` | UPDATE endpoints for collection picker |
| `internal/app/features/missionhydrosci/units.go` | ADD collection picker UI (button, modal/dropdown) |
| `internal/app/features/missionhydrosci/templates/missionhydrosci_units.gohtml` | ADD collection picker markup |
| `internal/app/features/groups/apps.go` | ADD collection pin selector to group apps management |
| `internal/app/features/groups/templates/` (apps template) | ADD collection dropdown to manage apps page |
| `internal/app/features/mhsdashboard/types.go` | ADD collection info to member row |
| `internal/app/features/mhsdashboard/dashboard.go` | LOAD collection info for display |
| `internal/app/features/mhsdashboard/templates/` | SHOW collection info in grid |
| `internal/app/features/mhsbuilds/templates/mhsbuilds_collection_detail.gohtml` | REMOVE "Use for Testing" button |

---

## 10. Implementation Phases

### Phase 1: Data layer
- Add `CollectionOverrideID` to `mhs_user_progress` model and store
- Add `MHSCollectionID` to `group_app_settings` model and store
- No migration needed (new optional fields)

### Phase 2: Collection resolution
- Rewrite `resolveManifest` with the new 4-tier hierarchy
- Remove session-based override code
- Add new endpoints for per-user override (with staff-auth for members)

### Phase 3: Collection picker in Mission HydroSci
- Add list collections endpoint
- Add picker UI to units page (admin/coordinator: visible; member: after auth)
- Wire auth flow for members

### Phase 4: Group collection pin
- Add collection selector to Groups > Manage > Apps
- Wire to group_app_settings store

### Phase 5: Dashboard updates
- Show effective collection per member
- Show group pin status
- Verify version tracking in game data pipeline

### Phase 6: Cleanup
- Remove "Use for Testing" button from MHS Builds
- Remove session-based override code
- Update the override banner in units template to read from DB instead of session

---

## 11. Verification

1. **No override, no group pin**: member sees workspace active collection
2. **Group pinned**: member in pinned group sees pinned collection; member in unpinned group sees workspace active
3. **User override**: member with override sees override collection regardless of group pin
4. **Override cleared**: member returns to group pin or workspace active
5. **Admin in picker**: can select any collection without auth
6. **Member in picker**: must pass staff-auth before seeing picker
7. **Dashboard**: shows effective collection and override indicator per member
8. **No active collection**: Mission HydroSci shows "not available" message
