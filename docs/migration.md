# Database Migrations

This document contains mongosh scripts for one-time database migrations.

---

## Migration 1: Add Workspace Foundation (December 2024)

**Purpose:** Create the initial workspace and set `workspace_id` on all existing records.

**Context:** Phase 1 of multi-tenancy implementation. This migration:
1. Creates a `workspaces` collection with one workspace
2. Adds `workspace_id` to all existing documents in major collections

**Prerequisites:**
- MongoDB/mongosh access to the database
- Application should be stopped or in maintenance mode (optional but recommended)

### Script

```javascript
// Connect to the database
use strata_hub

// 1. Create the workspace
const workspaceId = ObjectId();
db.workspaces.insertOne({
    _id: workspaceId,
    name: "Mission HydroSci",
    name_ci: "mission hydrosci",
    subdomain: "mhs",
    status: "active",
    created_at: new Date(),
    updated_at: new Date()
});

// 2. Update all collections to set workspace_id
db.organizations.updateMany({}, { $set: { workspace_id: workspaceId } });
db.groups.updateMany({}, { $set: { workspace_id: workspaceId } });
db.users.updateMany({}, { $set: { workspace_id: workspaceId } });
db.resources.updateMany({}, { $set: { workspace_id: workspaceId } });
db.materials.updateMany({}, { $set: { workspace_id: workspaceId } });
db.group_resource_assignments.updateMany({}, { $set: { workspace_id: workspaceId } });
db.material_assignments.updateMany({}, { $set: { workspace_id: workspaceId } });
db.group_memberships.updateMany({}, { $set: { workspace_id: workspaceId } });

// 3. Verify
db.workspaces.find();
db.organizations.findOne({}, { workspace_id: 1 });
```

### Verification

After running, verify counts per collection:

```javascript
db.organizations.countDocuments({ workspace_id: { $exists: true } });
db.groups.countDocuments({ workspace_id: { $exists: true } });
db.users.countDocuments({ workspace_id: { $exists: true } });
db.resources.countDocuments({ workspace_id: { $exists: true } });
db.materials.countDocuments({ workspace_id: { $exists: true } });
db.group_resource_assignments.countDocuments({ workspace_id: { $exists: true } });
db.material_assignments.countDocuments({ workspace_id: { $exists: true } });
db.group_memberships.countDocuments({ workspace_id: { $exists: true } });
```

### Notes

- This migration is idempotent for the updateMany calls (running again just overwrites with same value)
- The workspace insert will fail if run twice (duplicate `_id`) - this is expected
- If you need to customize the workspace name/subdomain, modify the `insertOne` call before running

---

*Last updated: December 2024*
