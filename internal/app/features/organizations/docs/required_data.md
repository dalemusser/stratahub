# Organizations - Required Data

This document describes the required and optional fields for creating and editing organizations.

## Fields

| Field | Required | Validation | Notes |
|-------|----------|------------|-------|
| Name | Yes | max 200 characters | Must be unique (case-insensitive) |
| Time Zone | Yes | must be in curated timezone list | Selected from dropdown |
| City | No | max 100 characters | |
| State | No | max 100 characters | |
| Contact Info | No | max 500 characters | |

## Server-Side Validation

The validation is defined in `new.go` and `edit.go` using struct tags:

```go
type createOrgInput struct {
    Name     string `validate:"required,max=200" label:"Organization name"`
    City     string `validate:"max=100" label:"City"`
    State    string `validate:"max=100" label:"State"`
    Contact  string `validate:"max=500" label:"Contact info"`
    TimeZone string `validate:"required,timezone" label:"Time zone"`
}
```

Additional validation:
- **Timezone validity**: `timezones.Valid(tz)` checks the timezone is in the curated list
- **Unique name**: Duplicate check on `name_ci` (case-insensitive) before insert/update

## Form Template Status

| Field | Has `required` attribute |
|-------|-------------------------|
| Name | Yes |
| Time Zone | Yes |
| City | N/A (optional) |
| State | N/A (optional) |
| Contact Info | N/A (optional) |
