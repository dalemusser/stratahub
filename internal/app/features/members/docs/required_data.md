# Members - Required Data

This document describes the required and optional fields for creating and editing members.

## Create Member

| Field | Required | Validation | Notes |
|-------|----------|------------|-------|
| Full Name | Yes | max 200 characters | |
| Email | Yes | valid email, max 254 characters | Must be unique across all users |
| Organization | Yes | valid ObjectID | Leaders are locked to their org; Admins select from dropdown |
| Auth Method | No | one of: internal, google, classlink, clever, microsoft | Defaults to "internal" |
| Status | N/A | | Always set to "active" for new members |

## Edit Member

| Field | Required | Validation | Notes |
|-------|----------|------------|-------|
| Full Name | Yes | max 200 characters | |
| Email | Yes | valid email, max 254 characters | Must be unique across all users |
| Organization | Read-only | | Displayed but not editable |
| Auth Method | No | one of: internal, google, classlink, clever, microsoft | |
| Status | No | one of: active, disabled | Defaults to "active" if not "disabled" |

## Server-Side Validation

The validation is defined in `types.go` and used by both create.go and viewedit.go:

```go
type memberInput struct {
    FullName string `validate:"required,max=200" label:"Full name"`
    Email    string `validate:"required,email,max=254" label:"Email"`
}
```

Additional validation:
- **Organization**: Required for admins (checked separately in create.go); Leaders are automatically locked to their org
- **Status**: Normalized to "active" if value is not "disabled"
- **Email uniqueness**: Duplicate check before insert/update
- **Auth method**: Defaults to "internal" if empty

## Form Template Status

**New Member (member_new.gohtml):**

| Field | Has `required` attribute |
|-------|-------------------------|
| Full Name | Yes |
| Email | Yes |
| Organization | Yes (when not locked) |
| Auth Method | N/A (optional) |

**Edit Member (member_edit.gohtml):**

| Field | Has `required` attribute |
|-------|-------------------------|
| Full Name | Yes |
| Email | Yes |
| Auth Method | N/A (optional) |
| Status | No (but select has no blank option, so always has value) |
