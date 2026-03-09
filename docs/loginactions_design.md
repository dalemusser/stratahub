# Login Actions Registry

## Overview

The Login Actions Registry is a StrataHub-level architectural feature that allows features to register actions that should run after a user successfully logs in. This decouples feature-specific post-login behavior from the login system itself — login doesn't need to know about any specific feature, it just runs registered actions.

This is analogous to startup items in an operating system: the OS doesn't know what each program does, it just launches whatever is registered to run at login.

This design is intended to be portable to the Strata framework so it can be used in other Strata-based applications.

---

## Motivation

Features sometimes need to perform work immediately after a user authenticates. Examples:

- **Mission HydroSci**: Register service worker and begin downloading game units if the game is assigned to the user
- **Future features**: Sync data from external services, prefetch content, initialize client-side state

Without a registry, each feature would require a direct modification to the login handler — hardcoding feature-specific logic into a core system component. The registry pattern keeps login generic and lets features manage their own post-login needs.

---

## Two Types of Actions

### Server-Side Actions

Run during the login request handler, after the session is created but before the response is sent. These are Go functions executed on the server.

**Use cases:**
- Write a record to the database (e.g., "last login" timestamp)
- Fetch data from an external API and cache it
- Set session values or cookies

**Constraints:**
- Must be fast (< 100ms) — login should not feel slow
- Must not block login on failure — fire-and-forget with error logging
- Run sequentially in registration order (but each is independent)

### Client-Side Actions

Emit JavaScript that runs in the browser after the login redirect lands. These are not Go functions — they produce JS code that the server includes in the post-login page response.

**Use cases:**
- Register a service worker and trigger downloads (Mission HydroSci)
- Initialize client-side analytics or tracking
- Set localStorage values

**Constraints:**
- Must be lightweight — the user is landing on their dashboard, not waiting for background tasks
- Must not block page rendering — run asynchronously
- Must handle failures silently — a failed client-side action should log to console, not show an error

---

## Architecture

### Package Location

```
internal/app/loginactions/
  registry.go     // Registry type, action types, registration and execution
```

### Core Types

```go
package loginactions

import (
    "context"
    "log"
    "net/http"
)

// ServerAction runs on the server during login, after session creation.
// It receives the request context (which includes the authenticated user)
// and the HTTP request. It must not write to the response.
type ServerAction struct {
    Name        string
    Description string
    Run         func(ctx context.Context, r *http.Request) error
}

// ClientAction produces JavaScript to run in the browser after login.
// ShouldRun determines whether this action applies to the current user.
// Script returns the JavaScript code to execute (will be wrapped in
// an async IIFE for isolation).
type ClientAction struct {
    Name        string
    Description string
    ShouldRun   func(ctx context.Context, r *http.Request) bool
    Script      func(ctx context.Context, r *http.Request) string
}

// Registry holds all registered login actions.
type Registry struct {
    serverActions []ServerAction
    clientActions []ClientAction
    errLog        *log.Logger
}

// New creates a new login actions registry.
func New(errLog *log.Logger) *Registry {
    return &Registry{
        errLog: errLog,
    }
}

// RegisterServer adds a server-side action to run on login.
func (reg *Registry) RegisterServer(action ServerAction) {
    reg.serverActions = append(reg.serverActions, action)
}

// RegisterClient adds a client-side action to run after login.
func (reg *Registry) RegisterClient(action ClientAction) {
    reg.clientActions = append(reg.clientActions, action)
}
```

### Execution

```go
// RunServerActions executes all registered server-side actions.
// Each action runs independently — a failure in one does not affect others.
// Errors are logged but do not propagate (fire-and-forget).
func (reg *Registry) RunServerActions(ctx context.Context, r *http.Request) {
    for _, action := range reg.serverActions {
        if err := action.Run(ctx, r); err != nil {
            reg.errLog.Printf("login action %q failed: %v", action.Name, err)
        }
    }
}

// ClientScripts returns the JavaScript to include in the post-login page.
// Each applicable action's script is wrapped in an async IIFE for isolation.
// Returns empty string if no client actions apply.
func (reg *Registry) ClientScripts(ctx context.Context, r *http.Request) string {
    var scripts []string
    for _, action := range reg.clientActions {
        if action.ShouldRun(ctx, r) {
            js := action.Script(ctx, r)
            if js != "" {
                // Wrap in async IIFE for isolation and error containment
                wrapped := fmt.Sprintf(
                    "/* login action: %s */\n(async function() { try { %s } catch(e) { console.error('Login action %s failed:', e); } })();",
                    action.Name, js, action.Name,
                )
                scripts = append(scripts, wrapped)
            }
        }
    }
    return strings.Join(scripts, "\n")
}
```

---

## Integration with Login

### Where Actions Execute

All authentication methods in StrataHub converge on `createSessionAndRedirect()` in `internal/app/features/login/handler.go`. This is the single point where server-side actions run.

```go
// In createSessionAndRedirect(), after sess.Save() and audit logging:
h.LoginActions.RunServerActions(r.Context(), r)
```

The login handler receives the registry as a dependency:

```go
type Handler struct {
    // ... existing fields ...
    LoginActions *loginactions.Registry
}
```

### How Client Scripts Reach the Browser

Client-side actions need their JavaScript delivered to the browser. There are two approaches, depending on the login flow:

**Redirect flows (password, trust, email code, Google OAuth):**

After login, the user is redirected to their destination (typically `/dashboard`). The client scripts need to be available on that landing page. The approach:

1. After `sess.Save()`, call `reg.ClientScripts(ctx, r)` and store the result in the session as a one-time value:
   ```go
   scripts := h.LoginActions.ClientScripts(r.Context(), r)
   if scripts != "" {
       sess.Values["login_actions_js"] = scripts
       sess.Save(r, w)
   }
   ```

2. The layout template checks for this session value and emits it once:
   ```html
   {{ if .LoginActionsJS }}
   <script>{{ .LoginActionsJS }}</script>
   {{ end }}
   ```

3. The middleware or page handler reads the value, passes it to the template, and clears it from the session so it only runs once:
   ```go
   if js, ok := sess.Values["login_actions_js"].(string); ok && js != "" {
       delete(sess.Values, "login_actions_js")
       sess.Save(r, w)
       // pass js to template data
   }
   ```

**Magic link flow:**

The magic link handler renders a success page directly (no redirect). Client scripts can be emitted directly into that page's template.

### Session Value Cleanup

The `login_actions_js` session value is a one-shot: read once, then deleted. If the user's first page load after login fails (browser crash, network error), the scripts run on the next successful page load. If the session expires before any page loads, the value is lost — which is fine, since the actions will have another chance to run on the next login or when the user visits the relevant feature page.

---

## Registration During Bootstrap

Features register their login actions during app bootstrap, in `internal/app/bootstrap/routes.go`, alongside route registration:

```go
// Create the registry
loginActionsRegistry := loginactions.New(errLog)

// Features register their actions
// (Mission HydroSci example)
loginActionsRegistry.RegisterClient(loginactions.ClientAction{
    Name:        "missionhydrosci-early-download",
    Description: "Register service worker and begin downloading game units",
    ShouldRun: func(ctx context.Context, r *http.Request) bool {
        // Check if game is assigned to this user
        user := auth.GetSessionUser(ctx)
        return isMHSAssigned(user)
    },
    Script: func(ctx context.Context, r *http.Request) string {
        return `
            if ('serviceWorker' in navigator) {
                navigator.serviceWorker.register('/missionhydrosci-sw.js')
                    .then(function(reg) {
                        return fetch('/missionhydrosci/api/progress');
                    })
                    .then(function(resp) { return resp.json(); })
                    .then(function(progress) {
                        // Send download message to SW for current unit
                    });
            }
        `
    },
})

// Pass registry to login handler
loginHandler := loginfeature.NewHandler(
    // ... existing deps ...
    loginActionsRegistry,
)
```

---

## Failure Isolation

### Server-Side

Each server action is wrapped in its own error handling. A panic in one action is recovered and logged without affecting others:

```go
func (reg *Registry) RunServerActions(ctx context.Context, r *http.Request) {
    for _, action := range reg.serverActions {
        func() {
            defer func() {
                if r := recover(); r != nil {
                    reg.errLog.Printf("login action %q panicked: %v", action.Name, r)
                }
            }()
            if err := action.Run(ctx, r); err != nil {
                reg.errLog.Printf("login action %q failed: %v", action.Name, err)
            }
        }()
    }
}
```

### Client-Side

Each client action's script is wrapped in an async IIFE with try/catch. A failure in one script does not affect others or the page:

```javascript
/* login action: missionhydrosci-early-download */
(async function() {
    try {
        // action script here
    } catch(e) {
        console.error('Login action missionhydrosci-early-download failed:', e);
    }
})();
```

---

## Design Principles

1. **Login is generic.** It does not import or reference any feature package. It only knows about `loginactions.Registry`.

2. **Features own their actions.** Each feature decides what to do on login and registers its own action. The feature package contains the action logic.

3. **Actions are optional.** If no actions are registered, login behaves exactly as it does today. The registry is a no-op with zero overhead.

4. **Server actions are fast.** If a feature needs to do something slow (API call, large DB query), it should use a goroutine or queue the work rather than blocking the login response.

5. **Client actions are invisible.** The user should not see loading spinners, banners, or errors from login actions. They happen silently in the background.

6. **One-shot delivery.** Client scripts run once on the first page load after login. They are not re-run on subsequent page loads (though features should also handle their concerns on their own pages as a safety net).

7. **Portable to Strata.** The `loginactions` package has no StrataHub-specific dependencies. It uses standard Go interfaces (`context.Context`, `*http.Request`, `*log.Logger`). It can be moved to the Strata framework unchanged.

---

## Example: Mission HydroSci Early Download

This is the first consumer of the Login Actions Registry. See `stratahub/docs/single_launch_design.md` for the full design.

**What it does:** After login, if the Mission HydroSci game is assigned to the user, register the service worker and begin downloading the user's current unit in the background.

**Type:** Client-side action (service worker and Cache API are browser features)

**ShouldRun logic:**
- User role is non-member (always assigned) OR
- User is a member and the game is in their group's assigned apps

**Script logic:**
1. Register `/missionhydrosci-sw.js`
2. Fetch `/missionhydrosci/api/progress` to learn current unit
3. Send download message to service worker for current unit
4. Report device status to `/missionhydrosci/api/device-status`

**Safety net:** The Mission HydroSci page also checks and triggers downloads on load, so if the login action fails or the user was already logged in when the game was assigned, downloads still start when they visit the page.

---

## Future Considerations

### Ordering

Currently actions run in registration order, which is determined by the order of `Register*` calls in bootstrap. If ordering ever matters between features, a `Priority` field could be added. For now, since actions are independent, ordering is not a concern.

### Deregistration

There is no mechanism to deregister actions at runtime. Actions are registered once during bootstrap and persist for the app's lifetime. If conditional execution is needed, use the `ShouldRun` function on client actions or conditional logic within server actions.

### Admin Visibility

A future admin feature could list all registered login actions (name, description, type) for debugging and transparency. The registry already stores this metadata.

### Migration to Strata

When moving this to the Strata framework:
1. Move `internal/app/loginactions/` to `waffle/pantry/loginactions/` (or equivalent shared package location)
2. No code changes needed — the package has no StrataHub-specific imports
3. Update StrataHub's import path to use the Strata package
4. Other Strata-based apps can then use the same registry pattern

---

## Files

| File | Purpose |
|---|---|
| `internal/app/loginactions/registry.go` | Registry type, action types, registration and execution |
| `internal/app/bootstrap/routes.go` | Create registry, register feature actions, pass to login handler |
| `internal/app/features/login/handler.go` | Accept registry as dependency, call `RunServerActions` and `ClientScripts` |
| `internal/app/resources/templates/layout.gohtml` | Emit one-shot client scripts from session |
