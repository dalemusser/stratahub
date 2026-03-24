# Authorization for Unit Changes and Download Clearing

## Why authorization is required

Changing a student's unit or clearing their downloads in Mission HydroSci are actions that affect the student's experience. To prevent accidental or unauthorized changes, these actions require a staff member — a superadmin, admin, coordinator, or leader — to authorize them.

This applies when a **member** (student) initiates the action from the Mission HydroSci units page. Staff members using the MHS Dashboard or the units page are not subject to additional authorization beyond their standard login.

## What happens to data

**No save data or logging data is deleted** when a unit is changed or a download is cleared.

- **Set to Unit** changes the pointer to which unit the student is currently playing. All prior units are marked as completed. The student's save data, progress point grades, logging data, and scores remain intact. You are simply telling the system which unit the student should be on.
- **Clear All Downloads** removes cached unit files from the student's device to free up storage. It has no effect on the student's progress, grades, or any server-side data.

## Per-workspace configuration

The authorization mode is configured on a per-workspace basis in StrataHub. Each workspace can independently choose what is required when a member performs a unit change or clears downloads. This is set in the workspace's site settings by an administrator.

Three options are available:

### Staff Auth (default)

A leader, coordinator, admin, or superadmin must authenticate directly in the Mission HydroSci interface on the student's behalf. When a member initiates a unit change or download clearing, an authorization dialog appears. The staff member enters their login ID, then verifies their identity using their configured authentication method — password, email verification code, or trust (depending on how their account is set up). Once authenticated, the action proceeds.

This is the default and most secure option. It ensures a staff member is physically present and explicitly approves each action.

### Keyword

A shared keyword is configured for the workspace. When a member initiates an action, they are prompted to enter the keyword. This is a simpler approach that does not require a staff member to be present, but provides a basic gate to prevent accidental changes.

### Trust

No additional authorization is required for members. The action proceeds with a simple confirmation dialog. This option is appropriate for environments where members are trusted to manage their own unit changes.

## How staff authentication works

When the workspace uses staff auth (the default), the authorization flow works as follows:

1. The member clicks "Set to Unit" or "Clear All Downloads" on the Mission HydroSci units page.
2. An authorization dialog appears asking for a staff member's login ID.
3. The system looks up the staff member and determines their authentication method:
   - **Trust:** The staff member is immediately authorized (no additional step).
   - **Password:** The staff member enters their password in the dialog.
   - **Email:** A verification code is sent to the staff member's email. They enter the code in the dialog.
4. On successful authentication, the system issues a one-time authorization token.
5. The token is included with the unit change or download clearing request.
6. The server validates and consumes the token, then performs the action.

The staff member must be a leader, coordinator, admin, or superadmin. They must belong to the same workspace as the student (superadmins, who have no workspace restriction, are also accepted).

## Summary

| Aspect | Detail |
|--------|--------|
| **Who needs authorization** | Members (students) only |
| **Staff roles that can authorize** | Superadmin, admin, coordinator, leader |
| **Authorization modes** | Staff auth (default), keyword, trust |
| **Configured where** | Per workspace, in StrataHub site settings |
| **Data deleted** | None — all save data, logging data, grades, and scores are preserved |
| **What "Set to Unit" does** | Moves the pointer to which unit the student is playing |
| **What "Clear Downloads" does** | Removes cached files from the device; no server-side data is affected |
