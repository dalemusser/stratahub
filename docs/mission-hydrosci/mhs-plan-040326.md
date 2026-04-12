# Mission HydroSci Build Management Plan

**Date:** 2026-04-03

## Context

Currently, updating MHS unit builds requires manually uploading files to S3, editing an embedded JSON manifest (`mhs_content_manifest.json`) with filenames/sizes, rebuilding the Go binary, and redeploying. This is slow and error-prone. Developers provide builds as zip files (e.g., `20260401-10923.zip`) but there's no traceability from version numbers back to CI/CD build identifiers.

This plan adds:
1. Browser-based zip upload that pushes files to S3
2. Database-driven "collections" replacing the static manifest
3. Per-workspace production collection selection
4. Session-based staff override for testing alternate collections

### Current State

- 5 units (unit1–unit5) stored in S3 bucket `adroit-cdn` under `mhs/` prefix
- Each unit has versioned folders: `mhs/unit1/v2.2.2/Build/...`, `mhs/unit1/v2.2.2/StreamingAssets/...`
- Current versions: unit1–unit4 at v2.2.2, unit5 at v2.2.3
- Static embedded JSON manifest loaded once at startup via `sync.Once`
- Service worker caches by version: `mhs-unit-unit1-v2.2.2`

### Clean Break — No Legacy Migration

We are starting fresh rather than migrating existing data. The current static manifest, embedded JSON, and `sync.Once` loading will be **removed entirely**. The existing unit files remain in S3 but are not tracked by the new system. The first collection will be created when the first build zip is uploaded through the new UI. This eliminates seed migration logic, fallback code paths, and backward-compatibility complexity.

---

## 1. New Domain Models

### `mhs_builds` Collection — Individual Unit Version Records

**File:** `internal/domain/models/mhs_build.go`

| Field | Type | Description |
|-------|------|-------------|
| ID | ObjectID | |
| UnitID | string | "unit1"–"unit6+" |
| Version | string | "2.2.3" |
| BuildIdentifier | string | "CICDTesting/20260401-10923" — links to CI/CD build |
| Files | []MHSBuildFile | {Path, Size} for each file uploaded |
| TotalSize | int64 | Sum of file sizes |
| DataFile | string | e.g., "unit1.data.unityweb" |
| FrameworkFile | string | e.g., "unit1.framework.js.unityweb" |
| CodeFile | string | e.g., "unit1.wasm.unityweb" |
| CreatedAt | time.Time | |
| CreatedByID | ObjectID | |
| CreatedByName | string | |

**Indexes:** `{unit_id: 1, version: 1}` (unique), `{created_at: -1}`

### `mhs_collections` Collection — Named Sets of Unit Versions

Replaces the static `mhs_content_manifest.json`. Collections are **immutable** — create new ones, never edit.

**File:** `internal/domain/models/mhs_collection.go`

| Field | Type | Description |
|-------|------|-------------|
| ID | ObjectID | |
| Name | string | "Build 20260401-10923 — 2026-04-01" |
| Description | string | Optional notes |
| Units | []MHSCollectionUnit | All units with version info (always all units) |
| CreatedAt | time.Time | |
| CreatedByID | ObjectID | |
| CreatedByName | string | |

Each `MHSCollectionUnit` contains: UnitID, Title, Version, BuildIdentifier, DataFile, FrameworkFile, CodeFile, Files ([]MHSBuildFile), TotalSize.

A collection always contains ALL units (currently 5, extensible to 6+). When only some units are updated, unchanged units inherit their versions from the previous collection.

**Indexes:** `{created_at: -1}`

### Addition to `SiteSettings`

**File:** `internal/domain/models/sitesettings.go`

Add field: `MHSActiveCollectionID *primitive.ObjectID` — per-workspace selection of which collection members see.

---

## 2. New Stores

### `internal/app/store/mhsbuilds/store.go`

- `Create(ctx, build)` — insert new build record
- `GetByUnitVersion(ctx, unitID, version)` — lookup specific build
- `ListByUnit(ctx, unitID)` — all versions for a unit, newest first
- `LatestByUnit(ctx, unitID)` — for suggesting next version number
- `EnsureIndexes(ctx)`

### `internal/app/store/mhscollections/store.go`

- `Create(ctx, collection)` — insert new collection
- `GetByID(ctx, id)` — fetch single collection
- `List(ctx, limit)` — all collections, newest first
- `Latest(ctx)` — most recent collection (for inheriting unit versions when creating new ones)
- `EnsureIndexes(ctx)`

---

## 3. Configuration Changes

### New Config Keys

**File:** `internal/app/bootstrap/config.go`

| Key | Default | Description |
|-----|---------|-------------|
| `mhs_s3_region` | "" | AWS region for MHS CDN S3 bucket |
| `mhs_s3_bucket` | "" | S3 bucket name (e.g., "adroit-cdn") |
| `mhs_s3_prefix` | "mhs/" | Key prefix for MHS builds |
| `mhs_s3_acl` | "public-read" | Default ACL for uploaded objects |
| `mhs_s3_access_key_id` | "" | AWS access key (optional, uses default chain if empty) |
| `mhs_s3_secret_access_key` | "" | AWS secret key (optional) |

Separate from the existing materials storage config because the CDN bucket may be a different S3 bucket with different credentials.

### New AppConfig Fields

**File:** `internal/app/bootstrap/appconfig.go`

```go
MHSS3Region          string
MHSS3Bucket          string
MHSS3Prefix          string
MHSS3ACL             string
MHSS3AccessKeyID     string
MHSS3SecretAccessKey string
```

### New DBDeps Field

**File:** `internal/app/bootstrap/dbdeps.go`

```go
MHSStorage storage.Store  // S3 client for CDN bucket
```

Initialize in `ConnectDB` (`db.go`) when `MHSS3Bucket` is set, using `storage.NewS3()` from waffle's storage package.

---

## 4. New Feature: Build Management Admin UI

### File Structure

```
internal/app/features/mhsbuilds/
├── handler.go           — Handler struct (DB, MHSStorage, stores, logger)
├── routes.go            — admin-only routes
├── types.go             — view models
├── upload.go            — zip analysis + S3 upload logic
├── manual.go            — create collection from existing S3 files (no upload)
├── collections.go       — collection list, detail, activation
├── templates.go         — embed directive
└── templates/
    ├── mhsbuilds_upload.gohtml            — zip upload form
    ├── mhsbuilds_upload_review.gohtml     — review detected units, set versions
    ├── mhsbuilds_manual.gohtml            — manual collection creation form
    ├── mhsbuilds_collections.gohtml       — collection list
    └── mhsbuilds_collection_detail.gohtml — single collection view + activate/test buttons
```

### Routes (mounted at `/mhsbuilds`, admin-only)

| Method | Path | Handler | Description |
|--------|------|---------|-------------|
| GET | /upload | ServeUpload | Upload form |
| POST | /upload | HandleUpload | Analyze zip, render review |
| POST | /upload/confirm | HandleUploadConfirm | Execute S3 upload + create collection |
| GET | /manual | ServeManual | Manual collection creation form |
| POST | /manual | HandleManual | Verify S3 files + create collection |
| GET | /collections | ServeCollections | List all collections |
| GET | /collections/{id} | ServeCollectionDetail | Single collection view |
| POST | /collections/{id}/activate | HandleActivateCollection | Set as workspace active |

Mount in `bootstrap/routes.go` in the workspace-scoped admin group.

---

## 5. Zip Upload Flow (Two-Step)

### Step 1: Analyze (POST /upload)

1. Accept multipart form: zip file + build identifier (pre-filled from filename via JS)
2. Save zip to temp file (`os.CreateTemp`) — needed because `archive/zip` requires `io.ReaderAt` for random access
3. **Detect units** by walking zip entries for paths matching `*/Build/*` and `*/StreamingAssets/*`
4. Handle variable zip structure: the zip may have a wrapper folder or unit folders at the root. Detection finds the `unit\d+` folder that is the parent of `Build/` or `StreamingAssets/`
5. For each detected unit, look up latest version in `mhs_builds` store to suggest next (auto-increment patch: 2.2.2 → 2.2.3)
6. Store temp file path + analysis results in session
7. Render review page showing: detected units, file counts, sizes, editable version inputs, editable build identifier, auto-generated collection name

### Step 2: Confirm (POST /upload/confirm)

1. Re-read temp zip file from session-stored path
2. For each unit at its confirmed version:
   - Extract files from zip
   - Upload to S3 at `{unitID}/v{version}/{relative_path}` (e.g., `unit1/v2.2.3/Build/unit1.data.unityweb`)
   - Detect key files by extension: `.data.unityweb`, `.framework.js.unityweb`, `.wasm.unityweb`
   - Record file paths and sizes
3. Create `MHSBuild` records in MongoDB for each uploaded unit
4. **Create new `MHSCollection`:**
   - Inherit all units from latest existing collection
   - Override units that were just uploaded with new versions
   - Name: "Build {identifier} — {date}"
5. Clean up temp file
6. Redirect to collection detail page

### Size and Timeout Handling

- Set `http.MaxBytesReader` to ~1.5GB on upload routes
- Use generous context timeout (10+ minutes) for the confirm step
- Waffle's `storage.Put` handles S3 multipart uploads automatically for large files
- Initial version uses a simple "uploading..." spinner; progress polling is a future enhancement

---

## 6. Manual Collection Creation (No Upload)

Creates a collection from unit versions that already exist in S3, without uploading any files. Useful for:
- **Initial setup** — creating the first collection pointing at the existing v2.2.2/v2.2.3 files already in S3, so students with cached units don't need to re-download
- **Remixing** — assembling a collection from previously uploaded unit versions (e.g., roll back one unit to an older version)

### Flow (GET/POST /manual)

1. Form displays a row per unit (unit1–unit5+) with fields: version, build identifier
2. If a latest collection exists, pre-fill versions from it (makes it easy to change just one unit)
3. On submit, for each unit+version pair:
   - Call `storage.List(ctx, "{unitID}/v{version}/")` on the MHS S3 store to discover all files at that path
   - If no files found, return an error ("No files found in S3 for unit1 v2.2.3")
   - Call `storage.Head(ctx, path)` on each file to get its size
   - Detect key files by extension (`.data.unityweb`, `.framework.js.unityweb`, `.wasm.unityweb`)
4. Create `MHSBuild` records for any unit+version combos not already in the database
5. Create new `MHSCollection` with all units at the specified versions
6. Redirect to collection detail page

### Template (`mhsbuilds_manual.gohtml`)

- Collection name field (auto-generated, editable)
- Description field (optional)
- Table with one row per unit: Unit ID, Version (text input), Build Identifier (text input)
- Pre-filled from latest collection if one exists
- "Verify & Create" button
- Note: "Files must already exist in S3 at the expected paths"

---

## 7. Dynamic Manifest Serving

### Modifications to `internal/app/features/missionhydrosci/`

**handler.go** — Add fields:
- `CollectionStore *mhscollections.Store`
- `SessionMgr *auth.SessionManager` (for reading override from session)

**New shared method: `resolveManifest(r *http.Request) (ContentManifest, error)`**

Two-tier resolution:
1. Check session for staff override collection ID → use that collection
2. Else check workspace `MHSActiveCollectionID` from site settings → use that collection
3. If neither is set, return empty manifest / error (no collection configured)

Converts `MHSCollection` → `ContentManifest` struct (same shape the service worker expects).

**Files updated:**
- `api_manifest.go` — `ServeContentManifest` uses `resolveManifest(r)`. Remove embedded JSON, `sync.Once`, `loadContentManifest()`, and `mhs_content_manifest.json`
- `units.go` — `ServeUnits` uses `resolveManifest(r)`. Show "no collection configured" state when none is active
- `play.go` — `ServePlay` uses `resolveManifest(r)`

**Files removed:**
- `mhs_content_manifest.json` — no longer needed

---

## 8. Staff Override for Testing

### Mechanism

Session-based: store override collection ID in gorilla session as `mhs_collection_override` (ObjectID hex string). Only readable for non-member roles (admin, coordinator, leader).

### New Endpoints (in missionhydrosci routes)

| Method | Path | Description |
|--------|------|-------------|
| POST | /api/collection-override | Set override (body: `collection_id`) |
| POST | /api/collection-override/clear | Clear override |

Access controlled via staff-auth pattern (same as existing set-unit and clear-cache flows — a staff member authenticates while logged in as a member).

### UI Indicator

When override is active, display a yellow banner in `missionhydrosci_units.gohtml` and `missionhydrosci_play.gohtml`:

> **Testing collection: Build 20260401-10923 — 2026-04-01** [Clear]

Override is automatically cleared on logout (session destroyed).

---

## 9. Per-Workspace Active Collection

- Admin selects active collection via `/mhsbuilds/collections/{id}/activate`
- Stores `MHSActiveCollectionID` in workspace site settings
- The manifest endpoint reads this when no staff override is present
- Can also be managed from the settings UI (add MHS section showing current active collection)

---

## 10. Clean Break — No Legacy Migration

No seed migration or backward compatibility logic. The transition is:

1. **Remove** the embedded `mhs_content_manifest.json` file
2. **Remove** the `sync.Once` loading, `loadContentManifest()`, `cachedManifest`, and `staticFS` embed from `api_manifest.go`
3. **Replace** with database-only manifest resolution via `resolveManifest(r)`
4. If no active collection is set for a workspace, the units page shows a "no collection configured" message and the manifest API returns an empty response
5. The first collection is created when an admin uploads the first build zip through the new UI
6. Existing files in S3 (`mhs/unit1/v2.2.2/...` etc.) remain untouched but are not tracked by the new system

---

## 11. Implementation Phases

### Phase 1: Foundation
- Models: `mhs_build.go`, `mhs_collection.go`
- Stores: `mhsbuilds/`, `mhscollections/`
- Config: MHS S3 keys, AppConfig fields
- DBDeps: `MHSStorage` initialization
- SiteSettings: `MHSActiveCollectionID` field
- Index registration in `EnsureSchema`

### Phase 2: Dynamic Manifest (Remove Legacy)
- Remove `mhs_content_manifest.json`, embedded loading, `sync.Once`, `cachedManifest`
- Add `CollectionStore` + `SessionMgr` to missionhydrosci Handler
- Implement `resolveManifest(r)` (session override → workspace active → no collection)
- Update `ServeContentManifest`, `ServeUnits`, `ServePlay`
- Handle "no collection configured" state in units page

### Phase 3: Build Upload
- Create `mhsbuilds` feature (handler, routes, templates, types)
- Zip analysis logic (detect units, variable structure handling)
- S3 upload logic (extract → upload per file)
- Collection creation (inherit from latest + override uploaded units)
- Upload UI (form → review → confirm)
- Manual collection creation (verify existing S3 files, create collection without upload)
- Mount routes

### Phase 4: Collection Management
- Collection list and detail views
- Workspace activation endpoint + UI
- Add to admin menu

### Phase 5: Staff Override
- Session-based override endpoints
- Override reading in `resolveManifest`
- Override banner in MHS templates
- "Use for Testing" button in collection detail

---

## 12. Key Files to Modify

| File | Change |
|------|--------|
| `internal/domain/models/mhs_build.go` | NEW — build model |
| `internal/domain/models/mhs_collection.go` | NEW — collection model |
| `internal/domain/models/sitesettings.go` | ADD `MHSActiveCollectionID` field |
| `internal/app/store/mhsbuilds/store.go` | NEW — build store |
| `internal/app/store/mhscollections/store.go` | NEW — collection store |
| `internal/app/bootstrap/config.go` | ADD MHS S3 config keys |
| `internal/app/bootstrap/appconfig.go` | ADD MHS S3 AppConfig fields |
| `internal/app/bootstrap/dbdeps.go` | ADD `MHSStorage` field |
| `internal/app/bootstrap/db.go` | INIT MHSStorage S3 client |
| `internal/app/features/missionhydrosci/mhs_content_manifest.json` | REMOVE — replaced by database |
| `internal/app/bootstrap/routes.go` | MOUNT mhsbuilds routes, wire handler |
| `internal/app/system/indexes/indexes.go` | ADD mhs_builds + mhs_collections indexes |
| `internal/app/features/missionhydrosci/handler.go` | ADD `CollectionStore`, `SessionMgr` fields |
| `internal/app/features/missionhydrosci/api_manifest.go` | REWRITE to use `resolveManifest` |
| `internal/app/features/missionhydrosci/units.go` | USE `resolveManifest` |
| `internal/app/features/missionhydrosci/play.go` | USE `resolveManifest` |
| `internal/app/features/missionhydrosci/routes.go` | ADD override endpoints |
| `internal/app/features/missionhydrosci/types.go` | ADD override fields to view models |
| `internal/app/features/mhsbuilds/*` | NEW — entire feature |
| `internal/app/resources/templates/menu.gohtml` | ADD MHS Builds link for admin |
| `internal/app/features/missionhydrosci/templates/*.gohtml` | ADD override banner |

---

## 13. Verification

1. **No collection state**: Start app with empty `mhs_collections` — verify units page shows "no collection configured" and manifest API returns empty response
2. **Manual collection (initial setup)**: Create a manual collection pointing at existing S3 files (unit1–4 v2.2.2, unit5 v2.2.3) → verify S3 files are discovered, collection is created, and manifest matches the old static manifest exactly — students with cached units should not need to re-download
3. **Upload flow**: Upload a test zip with 1-2 units → verify files appear in S3 at correct paths, build records created, new collection created inheriting unchanged units from the previous collection
4. **Manual remix**: Create a manual collection combining versions from different previous uploads → verify it assembles correctly
5. **Collection activation**: Activate new collection for a workspace → verify manifest endpoint returns updated unit versions
6. **Staff override**: Set override to a different collection → verify manifest returns override collection; clear override → returns workspace active
7. **Service worker**: Verify the SW sees version changes and creates new cache entries
8. **Play/Units pages**: Verify unit selector and game launcher load correct unit versions from the active collection
