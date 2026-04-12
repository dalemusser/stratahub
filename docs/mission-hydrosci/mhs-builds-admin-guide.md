# MHS Builds — Admin Guide

## Overview

MHS Builds manages the game unit versions that students play in Mission HydroSci. The system lets you:

- Upload new game builds (zip files from the development team)
- Create collections of unit versions (manually or from uploads)
- Control which collection each workspace, group, or individual user plays
- Test new builds while logged in as a member so play data flows through the full pipeline

---

## Key Concepts

### Units

Mission HydroSci has 5 units (unit1 through unit5), each covering a different water science topic. Each unit is a separate WebGL game build with its own version number (e.g., v2.2.2). Units are stored in S3 and served to students via CDN.

### Builds

A build is a specific version of one unit. When the development team provides a new version of the game, it arrives as a zip file (e.g., `20260401-10923.zip`) containing one or more units. Each unit in the zip gets assigned a version number and uploaded to S3.

The **build identifier** (e.g., `CICDTesting/20260401-10923`) links a version number back to the CI/CD build that produced it, so developers and testers can trace issues to specific builds.

### Collections

A collection is a named set of all units at specific versions. For example:

| Unit | Version | Build |
|------|---------|-------|
| unit1 | 2.2.2 | CICDTesting/20260313-10763 |
| unit2 | 2.2.2 | CICDTesting/20260313-10763 |
| unit3 | 2.2.2 | CICDTesting/20260313-10763 |
| unit4 | 2.2.2 | CICDTesting/20260313-10763 |
| unit5 | 2.2.3 | CICDTesting/20260326-10855 |

Collections are **immutable** — once created, they don't change. To update units, create a new collection. This ensures students don't experience unexpected changes mid-session.

A collection always contains all units. When only some units are updated, the unchanged units carry forward from the previous collection.

---

## Which Collection Does a Student See?

The system checks in this order (first match wins):

1. **Per-user override** — if someone has explicitly chosen a collection for this user (via the collection picker in Mission HydroSci)
2. **Per-group pin** — if the user's group has been pinned to a specific collection (set in Groups > Manage > Apps)
3. **Workspace active** — the default collection set for the workspace (set in MHS Builds > Activate for Workspace)
4. **None** — if nothing is configured, Mission HydroSci shows a "not currently available" message

### Example Scenario

- Workspace active collection: "Build March 2026" (units at v2.2.2, unit5 at v2.2.3)
- Group "Mrs. Smith's Class" is pinned to "Build March 2026" (they started their study and shouldn't change)
- Group "Mr. Johnson's Class" has no pin (they get whatever the workspace active is)
- You update the workspace active to "Build April 2026" with newer unit versions
- Mrs. Smith's class continues with the March build; Mr. Johnson's class gets the April build
- A tester logs in as a member in Mr. Johnson's class but selects "Build April 2026 (beta)" via the picker — they see the beta build while everyone else in the class sees the stable one

---

## Creating Collections

### Upload a Build (from developer zip)

1. Go to **MHS Builds** in the admin menu
2. Click **Upload Build**
3. Select the zip file from the development team
4. Optionally enter a build identifier (defaults to the zip filename)
5. Click **Analyze Build** — the system detects which units are in the zip
6. Review detected units, adjust version numbers if needed
7. Click **Confirm & Upload to CDN** — files are uploaded to S3 and a new collection is created

The new collection inherits unchanged units from the most recent existing collection and overrides the ones that were just uploaded.

### Create Manually (from existing S3 files)

Use this when the unit files already exist in S3 and you just need to create a collection record pointing to them. This is useful for:

- Initial setup (pointing to units already in S3 from before MHS Builds existed)
- Creating a new collection that remixes existing unit versions (e.g., rolling back one unit)

1. Go to **MHS Builds** > **Create Manually**
2. Enter a collection name and optional description
3. For each unit, enter the version number and build identifier
4. Click **Verify & Create Collection** — the system verifies files exist in S3

---

## Activating a Collection for a Workspace

This sets the default collection that all members in the workspace see (unless overridden by a group pin or user override).

1. Go to **MHS Builds** > click on a collection name
2. Click **Activate for Workspace**

Only one collection can be active per workspace at a time. Activating a new one replaces the previous one.

To deactivate (make no collection available), you would remove the active collection — Mission HydroSci would then show "not currently available" to members.

---

## Pinning a Collection to a Group

Pin a group to a specific collection so they stay on that version even when the workspace active changes.

1. Go to **Groups** > find the group > **Manage**
2. Go to the **Apps** tab
3. Under Mission HydroSci, select a collection from the dropdown
4. Choose **"Use workspace active"** to unpin (the group follows whatever is active for the workspace)

### When to pin a group

- A study group is mid-research and shouldn't experience version changes
- You want to test a new build with one group before rolling it out to everyone
- Different groups need different versions for comparison

---

## Per-User Collection Override

Any user can be given a specific collection to play, overriding both the group pin and workspace active.

### For admins and coordinators

The collection picker is visible in Mission HydroSci on the units page. Click it, select a collection, and you're playing that version. Select "Default" to return to the normal collection.

### For members (students)

The collection picker is hidden by default. To reveal it:

1. Use the same authorization method configured in workspace settings (trust / keyword / staff authorization) — the same method used for clearing downloads and setting the unit
2. Once authorized, the picker appears and the member can select a collection
3. The same authorization is required to change or clear the selection

This allows testers to:
- Log in as a member
- Select a test build via the picker
- Play the game — their data flows through the full pipeline (game → log → grader → dashboard)
- Observe results on the MHS Dashboard

---

## MHS Dashboard and Collection Visibility

The MHS Dashboard shows which collection each student is using:

- **Effective collection**: the collection actually being used by that student
- **Override indicator**: if the student has a per-user override, it's shown distinctly so you know they're not on the group/workspace default
- **Group pin**: the group header shows whether the group is pinned to a collection or following the workspace active

This gives full visibility into who is playing what version.

---

## Maintenance Mode

When deploying updates or testing on the server, use **Maintenance Mode** to block non-admin access:

1. Log in as superadmin at the apex domain (adroit.games)
2. Go to **Workspaces** > **Maintenance**
3. Enable maintenance mode and add an optional message
4. Members, leaders, coordinators, and analysts see an offline page
5. Admins and superadmins can work normally (orange banner reminder)
6. Disable maintenance mode when done

---

## Typical Workflow: New Build from Developers

1. **Enable maintenance mode** (if needed for deployment)
2. Developers provide a zip file (e.g., `20260401-10923.zip`)
3. Go to **MHS Builds** > **Upload Build**
4. Upload the zip, review detected units, confirm
5. A new collection is created automatically
6. **Test the collection**: select it via the picker in Mission HydroSci, verify it works
7. **Activate for workspace** when ready for students
8. If specific groups should stay on the old version, **pin those groups** before activating the new collection
9. **Disable maintenance mode**

---

## Typical Workflow: Fixing One Unit

1. Developers provide a zip with just the fixed unit (e.g., only unit5)
2. Upload the zip — the new collection inherits unit1-4 from the previous collection and uses the new unit5
3. Test the new collection
4. Activate it — groups that are pinned stay on their version; unpinned groups get the fix
