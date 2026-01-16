# CSRF Protection Implementation

This document describes how Cross-Site Request Forgery (CSRF) protection is implemented in StrataHub, derived from the strata base implementation.

---

## Overview

StrataHub inherits CSRF protection from strata using the [gorilla/csrf](https://github.com/gorilla/csrf) package. The implementation includes:

- Server-side middleware that validates CSRF tokens on state-changing requests
- Automatic token injection for HTMX requests via JavaScript
- Hidden form field support for traditional form submissions
- Proper error handling with HTMX-aware redirects

---

## Configuration

### Environment Variable

| Variable | Default | Description |
|----------|---------|-------------|
| `STRATAHUB_CSRF_KEY` | `dev-only-csrf-key-please-change-0123456789` | CSRF token signing key (32+ chars in production) |

**Important:** Always use a strong, unique key in production. Generate one with:

```bash
openssl rand -base64 32
```

### Middleware Settings

The CSRF middleware is configured in `internal/app/bootstrap/routes.go`:

```go
csrfOpts := []csrf.Option{
    csrf.Secure(secure),                    // HTTPS-only cookies in production
    csrf.Path("/"),                         // Cookie valid for all paths
    csrf.CookieName("csrf_token"),          // Cookie name
    csrf.FieldName("csrf_token"),           // Form field name
    csrf.SameSite(csrf.SameSiteLaxMode),    // SameSite cookie attribute
    csrf.ErrorHandler(...),                 // Custom error handling
}

// In dev mode, trust localhost origins
// Note: TrustedOrigins uses HOST only (not full URL)
if !secure {
    csrfOpts = append(csrfOpts, csrf.TrustedOrigins([]string{
        "localhost:8080",
        "localhost:3000",
        "127.0.0.1:8080",
        "127.0.0.1:3000",
    }))
}

csrfMiddleware := csrf.Protect([]byte(appCfg.CSRFKey), csrfOpts...)
```

### Origin Validation

gorilla/csrf validates the `Origin` header on POST requests. In development mode, you must explicitly trust localhost origins via `csrf.TrustedOrigins()`. Without this, form submissions from `localhost` will fail with "origin invalid".

In production (when `secure=true`), the Origin header is validated against the actual request host, which works automatically for same-origin requests.

---

## How It Works

### 1. Token Generation

When a user visits any page, the middleware:
1. Generates a unique CSRF token for the session
2. Stores it in a secure cookie (`csrf_token`)
3. Makes it available to templates via `csrf.Token(r)`

### 2. Token Delivery

The token is delivered to the browser two ways:

**Meta Tag (for JavaScript/HTMX):**
```html
<meta name="csrf-token" content="{{ .CSRFToken }}">
```

**Hidden Form Field (for traditional forms):**
```html
<input type="hidden" name="csrf_token" value="{{ .CSRFToken }}">
```

### 3. Token Validation

On POST, PUT, PATCH, and DELETE requests, the middleware checks for a valid token in:
1. The `X-CSRF-Token` header (used by HTMX/JavaScript)
2. The `csrf_token` form field (used by traditional forms)

If validation fails, the request is rejected with a 403 Forbidden response.

---

## Using CSRF Protection

### BaseVM Integration

The `CSRFToken` field is automatically included in `BaseVM` and populated for every request:

```go
// internal/app/system/viewdata/viewdata.go
type BaseVM struct {
    // ... other fields
    CSRFToken string // CSRF token for forms
}
```

Both `viewdata.New(r)` and `viewdata.NewBaseVM(r, db, title, backURL)` populate this field automatically.

---

### Traditional Forms

For standard HTML forms that submit via POST, include a hidden field:

```html
<form method="POST" action="/some/endpoint">
    <input type="hidden" name="csrf_token" value="{{ .CSRFToken }}">

    <!-- Your form fields -->
    <input type="text" name="title" value="">

    <button type="submit">Submit</button>
</form>
```

**StrataHub Examples:**
- `settings/templates/settings.gohtml` - Site settings form
- `organizations/templates/organization_new.gohtml` - Create organization
- `members/templates/member_edit.gohtml` - Edit member
- `leaders/templates/admin_leader_new.gohtml` - Create leader
- `materials/templates/materials_new.gohtml` - Create material
- `login/templates/login.gohtml` - Login form

**Key points:**
- Field name must be `csrf_token` (matches `csrf.FieldName` setting)
- Include in every form that uses POST, PUT, PATCH, or DELETE
- The token is automatically validated by middleware

---

### HTMX Requests

HTMX requests are handled automatically via JavaScript in `layout.gohtml`. No additional work is needed in your templates.

**How it works:**

The layout includes a meta tag and event listener in the `<head>`:

```html
{{ if .CSRFToken }}<meta name="csrf-token" content="{{ .CSRFToken }}">{{ end }}
<script>
  // CSRF token injection for HTMX requests
  // Use 'document' instead of 'document.body' since body doesn't exist yet in <head>
  document.addEventListener('htmx:configRequest', function(evt) {
    var token = document.querySelector('meta[name="csrf-token"]');
    if (token) {
      evt.detail.headers['X-CSRF-Token'] = token.content;
    }
  });
</script>
```

This automatically adds the `X-CSRF-Token` header to every HTMX request.

**StrataHub HTMX Examples:**

Group member management (`groups/templates/group_manage.gohtml`):
```html
<form hx-post="/groups/{{ $.GroupID }}/manage/add-member"
      hx-target="#members-list-body"
      hx-swap="outerHTML"
      hx-disable-element>
    <select name="memberID">...</select>
    <button type="submit">Add</button>
</form>
```

Resource assignment deletion (`groups/templates/resource_assignment_modal.gohtml`):
```html
<form hx-post="/groups/{{ .GroupID }}/assign_resources/remove"
      hx-target="#content"
      hx-swap="innerHTML"
      hx-confirm="Are you sure you want to delete this resource assignment?">
    <input type="hidden" name="assignmentID" value="{{ .AssignmentID }}">
    <button type="submit">Delete</button>
</form>
```

**Note:** No hidden CSRF field is needed for HTMX forms - the header is injected automatically.

---

### Custom JavaScript/Fetch Requests

For custom JavaScript making POST requests, read the token from the meta tag:

```javascript
fetch('/api/endpoint', {
    method: 'POST',
    headers: {
        'Content-Type': 'application/json',
        'X-CSRF-Token': document.querySelector('meta[name="csrf-token"]').content
    },
    body: JSON.stringify({ key: 'value' })
});
```

**StrataHub Example - Heartbeat API** (`layout.gohtml`):
```javascript
function sendHeartbeatFetch() {
  var headers = {'Content-Type': 'application/json'};
  var csrfToken = document.querySelector('meta[name="csrf-token"]');
  if (csrfToken) {
    headers['X-CSRF-Token'] = csrfToken.content;
  }
  fetch('/api/heartbeat', {
    method: 'POST',
    credentials: 'same-origin',
    headers: headers,
    body: JSON.stringify({page: window.location.pathname})
  });
}
```

---

## Error Handling

### CSRF Validation Failures

When CSRF validation fails, the middleware:

1. **Logs the failure** with details:
   ```
   WARN: CSRF validation failed path=/some/path method=POST reason=token missing
   ```

2. **For HTMX requests:** Returns 403 with `HX-Redirect: /login` header, causing the browser to redirect to the login page.

3. **For regular requests:** Returns 403 with message "CSRF token invalid or missing".

### Common Causes of CSRF Failures

| Cause | Solution |
|-------|----------|
| Missing hidden field in form | Add `<input type="hidden" name="csrf_token" value="{{ .CSRFToken }}">` |
| HTMX request without header | Ensure layout.gohtml has the meta tag and configRequest handler |
| Session expired | User needs to log in again (token is session-bound) |
| Token mismatch | Don't cache pages with CSRF tokens; ensure fresh token on each load |

---

## Template Checklist

When adding new features, ensure CSRF protection for all state-changing operations:

### For Traditional Forms

- [ ] Form has `method="POST"` (or PUT/DELETE)
- [ ] Form includes `<input type="hidden" name="csrf_token" value="{{ .CSRFToken }}">`
- [ ] View model embeds or includes BaseVM (for CSRFToken field)

### For HTMX Forms/Buttons

- [ ] Element uses `hx-post`, `hx-put`, `hx-patch`, or `hx-delete`
- [ ] Template extends layout (which provides the CSRF header injection)
- [ ] View model embeds or includes BaseVM (for meta tag)

### For JavaScript/Fetch

- [ ] Layout includes `<meta name="csrf-token">` tag
- [ ] JavaScript reads token from meta tag
- [ ] Fetch/XHR includes `X-CSRF-Token` header

---

## Files Involved

| File | Purpose |
|------|---------|
| `internal/app/bootstrap/config.go` | Defines `csrf_key` configuration |
| `internal/app/bootstrap/routes.go` | Configures CSRF middleware |
| `internal/app/system/viewdata/viewdata.go` | Populates CSRFToken in BaseVM |
| `internal/app/resources/templates/layout.gohtml` | Meta tag and HTMX header injection |
| Feature templates | Hidden form fields for traditional forms |

### Templates with CSRF Hidden Fields

Traditional form templates that include the hidden CSRF field:

| Feature | Templates |
|---------|-----------|
| Settings | `settings.gohtml` |
| Organizations | `organization_new.gohtml`, `organization_edit.gohtml` |
| Members | `member_new.gohtml`, `member_edit.gohtml` |
| Leaders | `admin_leader_new.gohtml`, `admin_leader_edit.gohtml` |
| Groups | `group_new.gohtml`, `group_edit.gohtml`, `resource_assignment_create.gohtml`, `resource_assignment_edit.gohtml` |
| Materials | `materials_new.gohtml`, `materials_edit.gohtml`, `materials_assign_form.gohtml`, `admin_material_assignment_edit.gohtml` |
| Resources | `resources_new.gohtml`, `resources_edit.gohtml` |
| System Users | `system_user_new.gohtml`, `system_user_edit.gohtml` |
| Workspaces | `workspace_new.gohtml`, `workspace_settings.gohtml`, `workspace_delete.gohtml` |
| Login | `login.gohtml`, `password.gohtml`, `verify_email.gohtml`, `change_password.gohtml` |
| Profile | `profile.gohtml` |
| Pages | `page_edit.gohtml` |
| Upload CSV | `upload_csv.gohtml` |

### Templates with HTMX POST (Header-Based CSRF)

HTMX templates that rely on automatic header injection:

| Feature | Templates |
|---------|-----------|
| Groups | `group_manage.gohtml` (add/remove members and leaders) |
| Groups | `resource_assignment_modal.gohtml` (delete assignment) |

---

## Security Considerations

### SameSite Cookie Attribute

The CSRF cookie uses `SameSite=Lax` mode, which:
- Prevents the cookie from being sent on cross-site POST requests
- Allows the cookie on top-level navigations (clicking links)
- Provides good protection while maintaining usability

### Secure Flag

In production (`env=prod`), the CSRF cookie has the `Secure` flag set, meaning it's only sent over HTTPS connections.

### Token Scope

- Tokens are bound to the user's session
- Each session gets a unique token
- Tokens are validated against the session cookie

---

## Troubleshooting

### "CSRF token invalid or missing" Error

1. Check that the form includes the hidden field with correct name (`csrf_token`)
2. Verify BaseVM is being used and CSRFToken is populated
3. Check browser dev tools for the csrf_token cookie
4. Ensure the session hasn't expired

### HTMX Requests Failing with Login Redirect

1. Verify the meta tag is in the page: `<meta name="csrf-token" content="...">`
2. Check that layout.gohtml has the `htmx:configRequest` event listener
3. Ensure the listener uses `document.addEventListener` (not `document.body.addEventListener`)
4. Look for JavaScript errors in browser console
5. Verify the request headers include `X-CSRF-Token` in browser dev tools Network tab

### Token Not in Template

1. Ensure your view model embeds BaseVM: `viewdata.BaseVM`
2. Use `viewdata.New(r)` or `viewdata.NewBaseVM(r, db, title, backURL)`
3. Verify the template is accessing `.CSRFToken` correctly

---

## Relationship to Strata

StrataHub inherits its CSRF implementation from strata. The key differences:

| Aspect | Strata | StrataHub |
|--------|--------|-----------|
| Env Variable | `STRATA_CSRF_KEY` | `STRATAHUB_CSRF_KEY` |
| HTMX Usage | Minimal | Extensive (groups, materials) |
| Template Count | ~15 forms | ~35 forms |

When strata's CSRF implementation is updated, StrataHub should be reviewed for compatibility.

---

## References

- [gorilla/csrf Documentation](https://github.com/gorilla/csrf)
- [OWASP CSRF Prevention Cheat Sheet](https://cheatsheetseries.owasp.org/cheatsheets/Cross-Site_Request_Forgery_Prevention_Cheat_Sheet.html)
- [HTMX Security Documentation](https://htmx.org/docs/#security)
- [SameSite Cookies Explained](https://web.dev/samesite-cookies-explained/)
- [strata/docs/csrf_implementation.md](../../../strata/docs/csrf_implementation.md) - Base implementation documentation
