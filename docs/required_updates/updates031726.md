# Required Update: Game Identity — Shift from `login_id` to User ObjectID

**Date:** March 17, 2026

---

## The Problem

StrataHub currently provides `login_id` (a human-readable string like `jsmith`) as the identity for Mission HydroSci. This value flows through the MHSBridge into the Unity game and is used as the key in save and log records stored by `save.adroit.games` and `log.adroit.games`.

Three issues with using `login_id` as game identity:

1. **Not globally unique.** `login_id` is unique within a workspace but not across workspaces. If the same `login_id` exists in two workspaces, their save/log records collide. This hasn't caused real problems yet because there are no real-user collisions — only test/dev accounts — but it is structurally unsound.

2. **Mutable.** An admin, coordinator, or leader can edit a user's `login_id`. If this happens, the user's save/log history is orphaned under the old ID and a new empty record begins under the new ID. The game has no way to detect or recover from this.

3. **Exposes PII.** `login_id` values are often based on real names (e.g., `jsmith`, `jane.doe`). Save and log data stored at `save.adroit.games` and `log.adroit.games` contains these identifiers in plain text. For student data (COPPA, FERPA), the save/log services should not need to store human-readable identifiers. De-identified data at rest is the safer default.

---

## The Solution

Replace `login_id` with the user's MongoDB ObjectID as the identity passed to the game:

```
user_objectid
```

For example: `65f1e2d3c4b5a6978899aabb`

In StrataHub, user ObjectIDs are globally unique across all workspaces — each user has a single `_id` in MongoDB that never changes. This makes the user ObjectID sufficient as a standalone identity without needing to include the workspace.

This format is:

- **Globally unique** — the user ObjectID is unique across all workspaces in StrataHub
- **Immutable** — MongoDB ObjectIDs never change regardless of `login_id` edits
- **De-identified** — no PII in the identifier; save/log data is opaque at rest

### Workspace Context as a Separate Field

If save/log services need to know which workspace a user belongs to (e.g., for partitioning or per-workspace queries), the workspace ID can be passed as a separate field alongside the identity rather than embedded in it. This keeps the identity simple and avoids coupling structural information into the identifier itself.

```json
{
  "user_id": "65f1e2d3c4b5a6978899aabb",
  "workspace_id": "65a1b2c3d4e5f6a7b8c9d0e1",
  ...
}
```

The workspace ID is available in StrataHub via `workspace.IDFromRequest(r)` and can be passed to save/log services as a separate header or field when needed.

---

## What Changes in StrataHub

### MHSBridge Identity Injection

**File:** `internal/app/features/missionhydrosci/play.go`

The play handler currently extracts `user.LoginID` and passes it to the template. It needs to instead pass the user's ObjectID:

```go
// Current
loginID := user.LoginID

// New
identity := user.ID.Hex()
```

The user ID is already available as `user.ID` (a MongoDB ObjectID).

### MHSBridge Template

**File:** `internal/app/features/missionhydrosci/templates/missionhydrosci_play.gohtml`

The template injects identity into JavaScript and monkey-patches `XMLHttpRequest` and `fetch` to intercept calls to `/api/user`. The intercepted response currently returns `login_id`. It needs to return the user ObjectID instead (the game treats whatever it receives as an opaque string).

### `/api/user` Endpoint

**File:** `internal/app/features/userinfo/handler.go`

This endpoint currently returns `login_id` as a string. If any code path calls this endpoint directly (not through the MHSBridge intercept), it should also return the user ObjectID. Review whether the endpoint is called outside of the bridge intercept; if not, no change is needed here, but it should be updated for consistency.

### Session Cookie

**File:** `internal/app/system/auth/auth.go`

No change needed. The session cookie already stores `user_id` as a MongoDB ObjectID hex string. The cookie is set at `.adroit.games` and contains `user_id` and `is_authenticated`. The ObjectID is already available — it's just not being passed through to the game today.

---

## What Changes in Save/Log Services

### Record Key

Records at `save.adroit.games` and `log.adroit.games` are currently keyed by `login_id`. After the change, they will be keyed by the user ObjectID (e.g., `65f1e2d3c4b5a6978899aabb`).

### Optional: Store Workspace as a Separate Field

If per-workspace queries are needed, save/log services can accept and store the workspace ID as a separate field alongside the user identity:

```json
{
  "user_id": "65f1e2d3c4b5a6978899aabb",
  "workspace_id": "65a1b2c3d4e5f6a7b8c9d0e1",
  ...
}
```

This enables queries like "all records for this workspace" or "all records for this user across workspaces" without needing to call StrataHub.

---

## Data Migration

A one-time migration script to convert existing save/log records from `login_id` keys to user ObjectID keys.

### Steps

1. **Export the user mapping from StrataHub** — for each workspace, produce a mapping of `login_id` → `user_objectid`. This requires querying the `users` collection within each workspace's context.

2. **Identify and handle collisions** — find `login_id` values that appear in more than one workspace. Based on current data, the only collisions are test/dev accounts. These records can be deleted from save/log services.

3. **Transform save/log records** — for each record, look up the `login_id` in the mapping and replace it with the user ObjectID. Records whose `login_id` does not appear in the mapping (deleted users, test accounts) can be archived or deleted.

4. **Verify** — confirm that all migrated records resolve correctly and that the game can load save data for existing users under the new identity format.

### Feasibility

- No real-user `login_id` collisions exist across workspaces — only test/dev accounts
- Test/dev collision records can be deleted without impact
- The mapping from `login_id` to user ObjectIDs is deterministic (query StrataHub's database)
- The migration can be done while the system is live — old records are read-only during migration, new records use the new format immediately

---

## Developer Tooling

With opaque ObjectIDs replacing human-readable `login_id` values, developers and researchers lose the ability to quickly identify whose data they're looking at in save/log records.

### Needed: Lookup Tool

A utility (CLI script, admin endpoint, or dashboard feature) that maps between:

- Human-readable name / `login_id` → user ObjectID
- User ObjectID → human-readable name / `login_id` / workspace name

This could be:

- A simple Go CLI tool that queries StrataHub's MongoDB directly
- An admin-only API endpoint in StrataHub (e.g., `/admin/api/identity-lookup`)
- A column in an existing admin view

The tool should be restricted to admin/coordinator roles — the point of de-identification is that save/log services don't need access to this mapping in normal operation.

---

## Privacy and Compliance Benefit

After this change, save and log data stored at `save.adroit.games` and `log.adroit.games` contains no PII. The records are keyed by opaque ObjectIDs that are meaningless without access to StrataHub's user database.

This provides:

- **COPPA compliance** — student identifiers in game telemetry and save data are de-identified
- **FERPA alignment** — educational records in external services do not contain directly identifiable information
- **Data breach resilience** — if save/log data is exposed, it reveals nothing about the students' identities
- **Simpler data sharing** — game telemetry can be shared with researchers without a de-identification step

---

## Game Development Team Impacts

The Unity game itself treats the identity value as an opaque string — it does not interpret or validate the format. However, several changes are required in the Unity project and the game dev team's workflow.

### 1. Understanding the Identity Change

The identity values that appear in save/log records will change from human-readable strings (e.g., `jsmith`) to opaque MongoDB ObjectIDs (e.g., `65f1e2d3c4b5a6978899aabb`). The game dev team needs to understand that these are the same users — just identified differently. Historical data will be migrated so existing save data is preserved under the new keys.

### 2. Identity Lookup Tool

A lookup tool will be provided to resolve ObjectIDs back to human-readable names, `login_id` values, and workspace names. The game dev team will need to use this tool when investigating player issues, reviewing save/log data, or debugging. The tool replaces the previous workflow of reading `login_id` values directly from the data.

### 3. New `.jslib` and C# Files

The JSON structure returned by MHSBridge and `/api/user` is being cleaned up. The current response has a legacy issue: the field named `email` actually contains the `login_id`, a holdover from when identity was email-based. Not everyone has an email address, so identity moved to `login_id`, but the `email` field name was kept for backward compatibility with existing game builds.

The new response adds a properly named `user_id` field:

```json
{
  "user_id": "65f1e2d3c4b5a6978899aabb",
  "login_id": "jsmith",
  "email": "jsmith"
}
```

The game dev team needs to provide a new `.jslib` and C# file for the Unity project that reads `user_id` instead of `email` or `login_id`. This replaces the previous `.jslib` and C# file that handled identity.

### 4. Transition Period — Old and New Builds Coexist

During the transition, the MHSBridge response will include all three fields (`user_id`, `login_id`, and `email`) so that old game builds continue to work. Old builds read `email` or `login_id` and are unaffected. New builds read `user_id` and send it to save/log services.

Once all old builds are flushed out (PWA cache updates, no more cached old versions in the field), the legacy `login_id` and `email` fields can be removed from the response.

### 5. Audit Other Identity References in the Unity Project

Beyond the `.jslib` and C# file that handles `/api/user`, there may be other places in the Unity project that reference `email` or `login_id` — logging calls, analytics events, error reporting, or UI display. These should be audited and updated to use `user_id` where appropriate.

### 6. Build Coordination

The rollout must be sequenced:

1. **StrataHub deploys first** — MHSBridge begins serving all three fields (`user_id`, `login_id`, `email`). Old builds keep working.
2. **New Unity build is released** — reads `user_id` from the response and sends it to save/log services.
3. **Save/log migration runs** — existing records are rekeyed from `login_id` to user ObjectID.
4. **Old builds flush out** — once no cached old builds remain in the field, legacy fields can be removed.

The game dev team needs to understand this sequence and coordinate their build release timing with the StrataHub deployment.

### 7. Testing Environment

A dev/staging StrataHub environment will be available that emits the new response format (with all three fields) so the game dev team can test the new `.jslib` and C# files before the production rollout. Testing should verify:

- The new build correctly reads `user_id` from the response
- Save data is written and read using the new identity
- Existing save data (migrated from `login_id`) loads correctly under the new identity

---

## Summary of Changes

| Component | Change |
|---|---|
| `play.go` (MHSBridge) | Pass `user.ID.Hex()` instead of `user.LoginID` |
| Play template (MHSBridge JS) | Serve `user_id`, `login_id`, and `email` in response during transition |
| `/api/user` endpoint | Return user ObjectID (for consistency) |
| Save/log services | Key records by user ObjectID; optionally store workspace as separate field |
| Existing save/log data | One-time migration script to remap `login_id` → user ObjectID |
| Developer tooling | Build lookup utility for ObjectID ↔ human-readable name |
| Game (Unity) | New `.jslib` and C# file to read `user_id`; audit other identity references |
| Session cookie | No changes (already stores ObjectID) |

---

## Implementation Order

1. **Build the lookup tool first** — developers need this before the migration
2. **Update StrataHub** — MHSBridge and `/api/user` serve all three fields (`user_id`, `login_id`, `email`)
3. **Game dev team releases new Unity build** — new `.jslib` and C# file read `user_id`
4. **Update save/log services** — accept and index the new format; optionally accept workspace as separate field
5. **Run migration** — transform existing records
6. **Verify** — confirm existing users can load their save data, new records use the new format
7. **Clean up** — remove legacy `login_id` and `email` fields from MHSBridge response; remove `login_id` references from save/log service code
