# PWA Architecture: Path Comparison

## Context

StrataHub hosts Mission HydroSci as a game delivery feature. The game requires PWA capabilities — offline play, background downloads, content caching, and installability. As we plan for additional games and long-term maintainability, we have three architectural paths for how games relate to StrataHub as a PWA.

This document compares the three options to inform a team decision.

---

## Option A: Platform PWA (Games Inside StrataHub)

**StrataHub is the PWA.** Games are features that run inside StrataHub's service worker, manifest, and install flow. A feature registry lets games declare their caching needs without the SW knowing about specific games.

*Detailed design: [pwa_plan.md](pwa_plan.md)*

### How It Works

- One manifest (`/manifest.json`) and one service worker (`/sw.js`) for the entire site
- Games register with a feature registry, declaring their content prefixes, page URLs, cache names, and broadcast channels
- The SW routes requests based on the registry — cache-first for game content, network-first for pages, pass-through for everything else
- A shared delivery manager JS handles downloads, background fetch, storage checks, and error reporting
- Workspace settings control the PWA name, icon, and branding
- Users install one app per workspace — it contains StrataHub and all assigned games

### Advantages

- **Single deployment** — one codebase, one server, one deploy pipeline
- **Shared infrastructure** — auth, sessions, cookies, user identity are already available (same origin)
- **No auth handoff needed** — the game runs in the same session as StrataHub
- **Unified user experience** — users install one app, navigate between platform and games seamlessly
- **Lower operational cost** — one domain, one TLS cert, one monitoring setup
- **Progress data is local** — the game writes directly to the same database StrataHub reads from
- **Works today** — the current architecture is a simpler version of this; the plan extends it

### Disadvantages

- **PWA complexity in StrataHub** — the service worker, caching, background fetch, and delivery manager all live in StrataHub's codebase
- **Scope conflicts risk** — all games share one SW scope (`/`); a bug in the SW can break the entire site
- **Coupled deploys** — a game update requires a StrataHub deploy (or at minimum, restarting the server)
- **Codebase growth** — each new game adds feature code, routes, templates, and static assets to StrataHub
- **Games are not portable** — Mission HydroSci cannot be used outside of StrataHub without extracting it
- **Content delivery shares resources** — large Unity builds (hundreds of MB per unit) are served through the same infrastructure as StrataHub's lightweight web pages

---

## Option B: Standalone Game PWAs (Games Outside StrataHub)

**Each game is its own PWA at its own origin.** StrataHub provides authorization and identity. Games run independently with their own manifest, service worker, and content delivery.

*Detailed design: [standalone_game_implementation.md](standalone_game_implementation.md)*

### How It Works

- Each game lives at its own domain (e.g., `mhs.play.adroit.games`)
- StrataHub acts as an OAuth 2.0 provider — the game redirects to StrataHub for login and receives an access token
- The game has its own backend, service worker, manifest, and install flow
- The game manages its own progress data using its own services (e.g., `log.adroit.games`, `save.adroit.games`); the teacher dashboard can live with the game
- StrataHub's UI links to the game and manages app assignment (Groups > Manage > Apps)
- Users install the game as a separate app from their device's perspective

### Advantages

- **Clean separation** — game code is entirely separate from StrataHub's codebase
- **No PWA conflicts** — each game owns its entire origin; no shared SW scope, no feature registry, no routing complexity
- **Games are portable** — Mission HydroSci can be used by other platforms, not just StrataHub
- **Independent deploys** — game updates don't touch StrataHub; StrataHub updates don't touch the game
- **Independent scaling** — game content delivery (large files) scales separately from StrataHub (lightweight pages)
- **Focused codebases** — StrataHub stays a platform; the game stays a game
- **Simpler StrataHub** — no service worker infrastructure, no delivery manager, no background fetch handling in StrataHub at all
- **Natural multi-game scaling** — adding a second game means creating a new project, not adding complexity to StrataHub

### Disadvantages

- **Two deployments per game** — each game needs its own server, domain, TLS, deploy pipeline, and monitoring
- **Auth handoff required** — cross-origin means no shared cookies/sessions; must implement OAuth 2.0 or signed tokens between StrataHub and each game
- **Dashboard moves with the game** — the teacher dashboard for a game would live with the game rather than in StrataHub, since the game owns its own progress data. Alternatively, the game would need to implement a service/API that StrataHub could use to retrieve progress data for display
- **Two apps to install** — students may need to install both StrataHub (for assignments, dashboard) and the game (for play), though in practice students primarily use only the game
- **More infrastructure** — additional domains, certificates, hosting costs, and monitoring per game
- **Implementation effort** — requires building OAuth provider in StrataHub, token validation in the game, and a progress API; more upfront work than Option A
- **Navigation boundary** — moving from StrataHub to the game is a cross-origin navigation (redirect), not an in-app page transition

---

## Option C: Hybrid (Support Both)

**StrataHub supports both integrated and standalone games.** Some games run inside StrataHub as features (Option A). Others run as standalone PWAs at their own origins (Option B). StrataHub provides the infrastructure for both patterns.

### How It Works

- StrataHub implements the platform PWA (Option A) with the feature registry for integrated games
- StrataHub also implements an OAuth 2.0 provider and progress API for standalone games
- Each game chooses which model fits: lightweight games or tightly-coupled games run inside StrataHub; large, independently-developed, or multi-platform games run standalone
- The dashboard, grading, and app assignment work the same regardless — they read progress data from the database, whether written by an integrated feature or received via API

### Advantages

- **Flexibility** — the right architecture for each game, not a one-size-fits-all decision
- **Incremental migration** — Mission HydroSci can start as an integrated feature (already is) and move to standalone later without a hard cutover
- **Handles diverse games** — a simple embedded quiz can be a StrataHub feature; a large Unity game with its own team can be standalone
- **OAuth benefits other integrations** — building an OAuth provider in StrataHub enables future integrations beyond games (external tools, partner platforms, LTI)
- **No throwaway work** — everything built for Option A (feature registry, delivery manager) and Option B (OAuth, progress API) has value

### Disadvantages

- **Most implementation work** — requires building both the platform PWA infrastructure and the OAuth/progress API
- **Two patterns to maintain** — developers need to understand both the integrated and standalone models
- **Testing surface** — more code paths, more configurations, more edge cases
- **Decision overhead** — each new game requires an architecture decision (integrated vs. standalone)

---

## Comparison Summary

| Dimension | A: Platform PWA | B: Standalone Games | C: Hybrid |
|---|---|---|---|
| **StrataHub complexity** | Higher (SW, caching, delivery) | Lower (links + OAuth + API) | Highest (both) |
| **Game portability** | Not portable | Fully portable | Per-game choice |
| **Deploy independence** | Coupled | Independent | Per-game choice |
| **Auth mechanism** | None needed (same origin) | OAuth 2.0 / signed tokens | Both |
| **Operational cost** | One deployment | One per game + StrataHub | Varies |
| **Multi-game scaling** | Adds complexity to StrataHub | Each game is isolated | Best of both |
| **User experience** | Single app, seamless | Separate apps per game | Mixed |
| **Upfront implementation** | Moderate | Moderate-high | High |
| **Risk of SW bugs** | Affects entire site | Isolated to each game | Integrated games share risk |
| **Progress data** | Direct DB writes | Game owns its own services | Both paths |

---

## Factors for the Decision

### If only Mission HydroSci for the foreseeable future
Option A is simpler — one codebase, no OAuth, no cross-origin concerns. The platform PWA plan is well-defined and extends the current architecture.

### If multiple games are planned
Option B scales more naturally — each game is isolated, independently deployable, and doesn't add complexity to StrataHub. The upfront cost of OAuth and the progress API is paid once and reused.

### If games may be used outside StrataHub
Option B is necessary — a game that needs to work with other platforms cannot be embedded inside StrataHub.

### If the team is small and wants to minimize operations
Option A has fewer moving parts — one server, one deploy, one thing to monitor.

### If long-term flexibility is valued over short-term simplicity
Option C provides the most flexibility but at the cost of maintaining two integration patterns.

---

## Current State

Mission HydroSci currently runs as an integrated feature in StrataHub with its own service worker and manifest (the simpler predecessor of Option A). The platform PWA plan (Option A) would generalize this into a feature registry pattern. The standalone plan (Option B) would extract the game entirely.

Work done under either option is not wasted — the delivery manager, caching logic, and background fetch handling are needed regardless of where they run. The OAuth provider built for Option B would have value beyond games. The feature registry built for Option A is straightforward to implement.
