# Standalone Game Architecture: Mission HydroSci as Its Own PWA

## Concept

Instead of Mission HydroSci living as a feature inside StrataHub, it would be a standalone PWA at its own origin (e.g., `game.missionhydrosci.org`). StrataHub would serve as the authorization and identity provider but would not be the context in which the game operates. The game lives in its own silo with just what it needs to support play.

The game has two needs from the outside:
1. **Authorization** — am I allowed to be accessed?
2. **Identity** — who is this person playing me?

StrataHub provides both, but the game runs independently.

## What It Would Look Like

- Mission HydroSci lives at its own domain/origin (e.g., `game.missionhydrosci.org`)
- It is a standalone PWA with its own manifest, service worker, install flow, and caching
- StrataHub is the auth/identity provider — the game redirects to StrataHub for login (OAuth-style), gets back a token
- The game talks to its own backend for progress, device status, and content delivery
- StrataHub links to the game but does not host it
- Each game (Mission HydroSci, future games) would follow this same pattern

## Advantages

- **Clean separation** — the game is truly its own product, not tangled in StrataHub's routing, middleware, and deployment
- **No PWA conflicts** — the game owns its entire origin. No competing manifests, no scope conflicts, no feature registry needed
- **Reusable by other platforms** — another LMS or system could integrate Mission HydroSci without needing StrataHub
- **Simpler for each project** — StrataHub stays a platform (users, groups, workspaces, dashboards), the game stays a game (play, progress, content delivery)
- **Independent deploy cycles** — game updates don't require a StrataHub deploy and vice versa
- **Independent scaling** — the game's content delivery (large Unity builds) doesn't share infrastructure with StrataHub's lightweight web pages
- **Cleaner codebase** — each project has a focused scope rather than one codebase doing everything

## Considerations and Challenges

### Auth Handoff

The game can't share cookies/session with StrataHub because they're on different origins. Needs a token-based auth mechanism:

- **Option A: OAuth 2.0 / OIDC** — StrataHub acts as an OAuth provider. The game redirects to StrataHub for login, gets an access token. Standard, well-understood, supports refresh tokens.
- **Option B: Signed redirect tokens** — StrataHub generates a short-lived signed JWT when the user clicks "Play Mission HydroSci." The game validates the signature and establishes its own session. Simpler but less standard.
- **Option C: Shared token service** — a lightweight auth service that both StrataHub and the game trust. More infrastructure to build.

OAuth 2.0 (Option A) is likely the right choice — it's standard, battle-tested, and libraries exist for Go.

### Progress Data Flow

The MHS Dashboard in StrataHub shows student progress, and the grading system reads progress data. If the game lives on its own, progress data needs to flow back to StrataHub somehow:

- **Option A: Game calls StrataHub API** — the game's backend POSTs progress updates to a StrataHub webhook/API endpoint. StrataHub stores them. Simple but creates a runtime dependency (if StrataHub is down, progress recording might fail).
- **Option B: Shared database** — both read/write the same progress collection. Avoids network calls but couples the data layer. Migration and schema changes become coordination problems.
- **Option C: Event-based** — the game publishes progress events to a queue (e.g., Redis, SQS). StrataHub consumes them. Decoupled but adds infrastructure.

Option A (game calls StrataHub API) is the simplest starting point.

### Content Delivery

The Unity game builds are served from a CDN. This doesn't change — the CDN is already external to StrataHub. The game's service worker and caching logic would move from StrataHub's codebase into the game's codebase. The delivery manager JS, background fetch handling, and cache management all move with the game.

### Two Things to Operate

Instead of one deployment (StrataHub), there are now two (StrataHub + the game). Each needs its own:
- Server / hosting
- Domain and TLS certificate
- Deployment pipeline
- Monitoring
- Database (or shared database — see above)

This is manageable but not free. For a single game it may feel like overhead. For multiple games it pays for itself.

### What Stays in StrataHub

- User accounts, login, authentication
- Groups, workspaces, membership management
- MHS Dashboard (reads progress data from wherever it's stored)
- Grading system
- Links to the game in menus / UI
- App assignment (Groups > Manage > Apps)

### What Moves to the Game

- Game play pages (Unity loader, play template)
- Units page (download management, install banner)
- Content manifest and CDN fallback
- Service worker and caching (delivery manager, background fetch, offline support)
- Progress tracking API (the game writes progress, StrataHub reads it)
- Device status reporting

## Compatibility with Current Work

The work being done now (single manifest, single SW, clean install flow inside StrataHub) is compatible with both futures:

- **If we stay with the current architecture** (game as a StrataHub feature): the PWA plan proceeds as designed. The game is a plugin in the platform SW via the feature registry.
- **If we move to standalone**: we extract the game code from StrataHub into its own project. The manifest, SW, and delivery manager become the game's own PWA infrastructure instead of being part of StrataHub's platform PWA. StrataHub removes the game feature and replaces it with links + auth handoff.

No work done now needs to be thrown away in either direction.

## When This Makes Sense

This architecture makes more sense as:
- More games are added (each gets its own origin, no conflicts)
- Mission HydroSci needs to be used outside of StrataHub contexts
- The game team and platform team want independent release cycles
- The complexity of hosting games inside StrataHub exceeds the complexity of operating them separately

For the immediate term (impact study, single game, small team), the current architecture works. This document captures the long-term direction for future discussion.
