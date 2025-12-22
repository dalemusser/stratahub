# Groups - Required Data

This document describes the required and optional fields for creating and editing groups.

## Create Group

| Field | Required | Validation | Notes |
|-------|----------|------------|-------|
| Name | Yes | max 200 characters | Must be unique within organization |
| Organization | Yes | valid ObjectID | Leaders are locked to their org; Admins select from dropdown |
| Leaders | No | valid ObjectIDs | Admins can optionally select; Leaders are auto-assigned |
| Description | No | | |

## Edit Group

| Field | Required | Validation | Notes |
|-------|----------|------------|-------|
| Name | Yes | max 200 characters | Must be unique within organization |
| Organization | Read-only | | Displayed but not editable |
| Description | No | | |

## Server-Side Validation

**Create (groupnew.go):**
```go
type createGroupInput struct {
    Name string `validate:"required,max=200" label:"Name"`
}
```

**Edit (groupedit.go):**
```go
type editGroupInput struct {
    Name string `validate:"required,max=200" label:"Name"`
}
```

Additional validation:
- **Organization**: Required for admins (checked separately); Leaders are automatically locked to their org
- **Name uniqueness**: Duplicate check on `name_ci` within the same organization
- **Leaders**: For leaders creating groups, they are automatically assigned as a leader of the new group

## Form Template Status

**New Group (group_new.gohtml):**

| Field | Has `required` attribute |
|-------|-------------------------|
| Name | Yes |
| Organization | Yes (admin view only) |
| Leaders | N/A (optional) |
| Description | N/A (optional) |

**Edit Group (group_edit.gohtml):**

| Field | Has `required` attribute |
|-------|-------------------------|
| Name | Yes |
| Description | N/A (optional) |
