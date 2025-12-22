# Resources - Required Data

This document describes the required and optional fields for creating and editing resources.

## Create Resource

| Field | Required | Validation | Notes |
|-------|----------|------------|-------|
| Title | Yes | max 200 characters | Must be unique |
| Launch URL | Yes | valid absolute HTTP/HTTPS URL | |
| Status | Yes | one of: active, disabled | Defaults to "active" |
| Type | No | must be valid resource type | Defaults to models.DefaultResourceType |
| Subject | No | | |
| Description | No | | |
| Show in Library | No | checkbox | |
| Default Instructions | No | | |

## Edit Resource

| Field | Required | Validation | Notes |
|-------|----------|------------|-------|
| Title | Yes | max 200 characters | Must be unique |
| Launch URL | Yes | valid absolute HTTP/HTTPS URL | |
| Status | Yes | one of: active, disabled | Defaults to "active" |
| Type | No | must be valid resource type | Defaults to models.DefaultResourceType |
| Subject | No | | |
| Description | No | | |
| Show in Library | No | checkbox | |
| Default Instructions | No | | |

## Server-Side Validation

**Create (adminnew.go):**
```go
type createResourceInput struct {
    Title     string `validate:"required,max=200" label:"Title"`
    LaunchURL string `validate:"required,url" label:"Launch URL"`
    Status    string `validate:"required,oneof=active disabled" label:"Status"`
}
```

**Edit (adminedit.go):**
```go
type editResourceInput struct {
    Title     string `validate:"required,max=200" label:"Title"`
    LaunchURL string `validate:"required,url" label:"Launch URL"`
    Status    string `validate:"required,oneof=active disabled" label:"Status"`
}
```

Additional validation:
- **Type**: `inputval.IsValidResourceType(typeValue)` - must be a valid resource type
- **Launch URL**: `urlutil.IsValidAbsHTTPURL(launchURL)` - must be an absolute HTTP/HTTPS URL
- **Title uniqueness**: Duplicate check before insert/update

## Form Template Status

**New Resource (resources_new.gohtml):**

| Field | Has `required` attribute |
|-------|-------------------------|
| Title | Yes |
| Type | No (but select has default value) |
| Subject | N/A (optional) |
| Description | N/A (optional) |
| Status | No (but select has default value) |
| Show in Library | N/A (optional, checkbox) |
| Launch URL | Yes |
| Default Instructions | N/A (optional) |

**Edit Resource (resources_edit.gohtml):**

| Field | Has `required` attribute |
|-------|-------------------------|
| Title | Yes |
| Type | No (but select has default value) |
| Subject | N/A (optional) |
| Description | N/A (optional) |
| Status | No (but select has default value) |
| Show in Library | N/A (optional, checkbox) |
| Launch URL | Yes |
| Default Instructions | N/A (optional) |
