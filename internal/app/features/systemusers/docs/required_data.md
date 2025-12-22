# System Users - Required Data

This document describes the required and optional fields for creating and editing system users.

## Create System User

| Field | Required | Validation | Notes |
|-------|----------|------------|-------|
| Full Name | Yes | max 200 characters | |
| Email | Yes | valid email, max 254 characters | Must be unique across all users |
| Role | Yes | one of: admin, analyst | |
| Auth Method | Yes | valid auth method | one of: internal, google, classlink, clever, microsoft |
| Status | N/A | | Always set to "active" for new system users |

## Edit System User

| Field | Required | Validation | Notes |
|-------|----------|------------|-------|
| Full Name | Yes | max 200 characters | |
| Email | Yes | valid email, max 254 characters | Must be unique across all users |
| Role | Yes | one of: admin, analyst | Disabled for self-edit |
| Auth Method | Yes | valid auth method | one of: internal, google, classlink, clever, microsoft |
| Status | Yes | one of: active, disabled | Disabled for self-edit |

## Server-Side Validation

**Create (new.go):**
```go
type createSystemUserInput struct {
    FullName   string `validate:"required,max=200" label:"Full name"`
    Email      string `validate:"required,email,max=254" label:"Email"`
    Role       string `validate:"required,oneof=admin analyst" label:"Role"`
    AuthMethod string `validate:"required,authmethod" label:"Auth method"`
}
```

**Edit (edit.go):**
```go
type editSystemUserInput struct {
    FullName   string `validate:"required,max=200" label:"Full name"`
    Email      string `validate:"required,email,max=254" label:"Email"`
    Role       string `validate:"required,oneof=admin analyst" label:"Role"`
    AuthMethod string `validate:"required,authmethod" label:"Auth method"`
    Status     string `validate:"required,oneof=active disabled" label:"Status"`
}
```

Additional validation:
- **Email uniqueness**: Duplicate check before insert/update
- **Self-edit restrictions**: Users cannot change their own role or status
- **Last admin protection**: Cannot delete the last active admin

## Form Template Status

**New System User (system_user_new.gohtml):**

| Field | Has `required` attribute |
|-------|-------------------------|
| Full Name | Yes |
| Email | Yes |
| Role | No (select has default value) |
| Auth Method | No (select has default value) |

**Edit System User (system_user_edit.gohtml):**

| Field | Has `required` attribute |
|-------|-------------------------|
| Full Name | Yes |
| Email | Yes |
| Role | No (select has default value; disabled for self) |
| Auth Method | No (select has default value) |
| Status | No (select has default value; disabled for self) |

