# Leaders - Required Data

This document describes the required and optional fields for creating and editing leaders.

## Create Leader

| Field | Required | Validation | Notes |
|-------|----------|------------|-------|
| Full Name | Yes | max 200 characters | |
| Email | Yes | valid email, max 254 characters | Must be unique across all users |
| Organization | Yes | valid ObjectID | Selected from dropdown |
| Auth Method | No | one of: internal, google, classlink, clever, microsoft | Defaults to "internal" |
| Status | N/A | | Always set to "active" for new leaders |

## Edit Leader

| Field | Required | Validation | Notes |
|-------|----------|------------|-------|
| Full Name | Yes | max 200 characters | |
| Email | Yes | valid email, max 254 characters | Must be unique across all users |
| Organization | Read-only | | Displayed but not editable |
| Auth Method | No | one of: internal, google, classlink, clever, microsoft | |
| Status | Yes | one of: active, disabled | Defaults to "active" if empty |

## Server-Side Validation

**Create (new.go):**
```go
type createLeaderInput struct {
    FullName string `validate:"required,max=200" label:"Full name"`
    Email    string `validate:"required,email,max=254" label:"Email"`
    OrgID    string `validate:"required,objectid" label:"Organization"`
}
```

**Edit (edit.go):**
```go
type editLeaderInput struct {
    FullName string `validate:"required,max=200" label:"Full name"`
    Email    string `validate:"required,email,max=254" label:"Email"`
    Status   string `validate:"required,oneof=active disabled" label:"Status"`
}
```

Additional validation:
- **Email uniqueness**: Duplicate check before insert/update
- **Auth method**: Defaults to "internal" if empty (create), no strict validation on edit
- **Status**: Defaults to "active" if empty on edit

## Form Template Status

**New Leader (admin_leader_new.gohtml):**

| Field | Has `required` attribute |
|-------|-------------------------|
| Full Name | Yes |
| Email | Yes |
| Organization | Yes |
| Auth Method | N/A (optional) |

**Edit Leader (admin_leader_edit.gohtml):**

| Field | Has `required` attribute |
|-------|-------------------------|
| Full Name | Yes |
| Email | Yes |
| Auth Method | N/A (optional) |
| Status | No (but select has no blank option, so always has value) |
