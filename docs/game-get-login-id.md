# How Unity WebGL Games Should Get User Identity

This document defines the recommended approach for Unity WebGL games to retrieve the logged-in user's identity (login_id, name) from StrataHub, independent of which workspace or domain the game is launched from.

## Background

Unity WebGL games use a C# → JavaScript bridge (`.jslib`) to call a server endpoint and retrieve the current user's identity. Historically, the jslib was hardcoded to call `https://adroit.games/api/user`. This broke when:

1. The game is launched from a workspace subdomain (e.g., `mhs.adroit.games`) — the cookie is valid but a workspace mismatch on the apex `/api/` endpoint prevents authentication.
2. The game is loaded from a CDN (e.g., `cdn.adroit.games`) — cross-origin, different session scope entirely.

See `docs/cookies-and-identities.md` for the full investigation.

## The `/api/user` Endpoint

**Route:** `GET /api/user` (all StrataHub instances)

**Response format:**
```json
{
  "isAuthenticated": true,
  "name": "Aiden Blaizen",
  "email": "blaizen",
  "login_id": "blaizen"
}
```

| Field | Type | Description |
|-------|------|-------------|
| `isAuthenticated` | boolean | Whether a valid session exists |
| `name` | string | User's full name |
| `email` | string | User's login_id (legacy field name for backwards compatibility) |
| `login_id` | string | User's login identifier (preferred) |

When not authenticated:
```json
{
  "isAuthenticated": false,
  "name": "",
  "email": "",
  "login_id": ""
}
```

## Recommended Approach: Same-Origin `/api/user`

The simplest and most robust approach is for the game's jslib to call `/api/user` as a **relative URL** (same-origin), not an absolute cross-origin URL.

### Why This Works

When the game is launched from `mhs.adroit.games/mhs/play/unit1`:
- The page origin is `https://mhs.adroit.games`
- A relative fetch to `/api/user` hits `https://mhs.adroit.games/api/user`
- The session cookie (domain `.adroit.games`) is sent automatically
- The workspace middleware matches the user's workspace — authenticated
- No CORS needed (same-origin)

### jslib Implementation

**Current (broken):**
```csharp
// Hardcoded cross-origin URL — DO NOT USE
Application.ExternalCall("GetUserFromServer", "https://adroit.games/api/user");
```

**Recommended:**
```javascript
// In the .jslib file
GetUserIdentity: function(callbackObjectName, callbackMethodName) {
    var objName = UTF8ToString(callbackObjectName);
    var methodName = UTF8ToString(callbackMethodName);

    var xhr = new XMLHttpRequest();
    xhr.open('GET', '/api/user');  // Relative URL — same origin
    xhr.withCredentials = true;
    xhr.onload = function() {
        if (xhr.status === 200) {
            // Send JSON string back to Unity
            SendMessage(objName, methodName, xhr.responseText);
        } else {
            SendMessage(objName, methodName, '{"isAuthenticated":false}');
        }
    };
    xhr.onerror = function() {
        SendMessage(objName, methodName, '{"isAuthenticated":false}');
    };
    xhr.send();
}
```

Key points:
- Use `/api/user` (relative), never `https://adroit.games/api/user` (absolute)
- Include `withCredentials = true` (sends cookies for same-origin requests automatically, but explicit is safer)
- Handle errors gracefully — return unauthenticated rather than crashing

## Interim: Identity Bridge (Current Implementation)

Until game builds are updated with the new jslib, the StrataHub launcher intercepts identity requests client-side.

### How It Works

1. The Go handler (`play.go`) extracts user identity from the session server-side
2. Identity is injected into the page as JavaScript variables
3. Before Unity loads, `XMLHttpRequest` and `fetch` are monkey-patched
4. Any request to `/api/user` (on any domain) is intercepted and returns the injected identity
5. The real network request is never made

### StrataHub Launcher (`mhs_play.gohtml`)

```
Go session → PlayData.UserName, PlayData.UserLoginID → template → JavaScript variables
                                                                        ↓
Unity jslib calls /api/user → intercepted by XHR/fetch patch → returns injected identity
```

The bridge handles both `XMLHttpRequest` and `fetch` APIs, matching any URL with pathname `/api/user` regardless of domain.

### CDN-Hosted Builds (`mhs-index-template.html`)

For game builds hosted directly on CloudFront (not through StrataHub's service worker):

```
StrataHub link → https://cdn.adroit.games/mhs/unit1/index.html?name=John&login_id=jdoe
                                                                        ↓
index.html reads query params → same XHR/fetch interception → returns identity from params
```

## Migration Path

### Phase 1: Identity Bridge (Current)
- StrataHub launcher injects identity server-side
- CDN builds receive identity via URL query parameters
- Old game builds with hardcoded `adroit.games/api/user` work through the bridge
- No game-side code changes required

### Phase 2: Update jslib to Use Relative URLs
- Update the Unity jslib to use `/api/user` (relative) instead of `https://adroit.games/api/user`
- Rebuild and redeploy game builds to CDN
- Test that each unit correctly retrieves identity from the hosting origin

### Phase 3: Remove Legacy Hacks
- Remove the `/api/*` special case in `workspace.go` (lines 84-102) that routes apex API requests to the first workspace
- Let apex `/api/*` requests be true apex requests (skip workspace validation)
- Remove the identity bridge from `mhs_play.gohtml` (the jslib now calls same-origin)
- Keep the CDN bridge for standalone CDN-hosted builds if needed

### Phase 4: Optional Enhancements
- Consider passing identity via Unity's `createUnityInstance` config rather than HTTP interception
- Consider a dedicated `/mhs/api/identity` endpoint that returns identity scoped to the MHS feature
- Add workspace-aware identity: include workspace name/ID in the response for multi-workspace games

## Security Considerations

- **Query parameter identity (CDN builds):** The `name` and `login_id` in the URL are visible in browser history, server logs, and referrer headers. This is acceptable for game identity (non-sensitive) but should not be extended to include passwords, tokens, or session IDs.
- **HttpOnly cookies:** The session cookie has `HttpOnly=true`, so JavaScript cannot read it. Identity must come from a server endpoint or server-side injection — JavaScript cannot extract it from the cookie directly.
- **Same-origin policy:** Using relative URLs for `/api/user` avoids all cross-origin issues (CORS, SameSite, cookie scoping) by design.

## Key Files

| File | Purpose |
|------|---------|
| `internal/app/features/userinfo/handler.go` | `/api/user` endpoint |
| `internal/app/features/mhsdelivery/play.go` | Injects identity into PlayData |
| `internal/app/features/mhsdelivery/templates/mhs_play.gohtml` | Identity bridge (XHR/fetch interception) |
| `host-test/mhs-index-template.html` | CDN identity bridge template |
| `internal/app/system/workspace/workspace.go` | Workspace middleware (contains `/api/*` special case to remove) |
