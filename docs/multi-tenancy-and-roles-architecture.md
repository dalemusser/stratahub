# Multi-Tenancy and Roles Architecture

This document captures architectural discussions and decisions regarding multi-tenancy (workspaces), multi-role users, and the coordinator role for StrataHub.

> **Naming Decision (Dec 2024):** The top-level tenant entity is called **Workspace**. See `docs/naming-and-vision.md` for the decision rationale.

## Current State

StrataHub currently uses a simple role-based model:
- Each user has exactly ONE role: `admin`, `analyst`, `leader`, or `member`
- Role determines menu, accessible routes, and capabilities
- Leaders and members belong to one organization
- Simple and clean, but inflexible for complex scenarios

## The Coordinator Role

### Requirements

A new role called **Coordinator** that has admin-like capabilities scoped to assigned organizations.

**Coordinators CAN:**
- View Resources/Materials library (read-only, to know what's available)
- Edit organization details (name, timezone) for their assigned orgs
- Create/manage groups in their assigned orgs
- Add/manage leaders in their assigned orgs
- Add/manage members in their assigned orgs
- Assign resources to groups in their assigned orgs
- Assign materials to orgs/leaders in their assigned orgs
- View member reports for their assigned orgs

**Coordinators CANNOT:**
- Create or delete organizations
- Create/edit/delete resources in the library
- Create/edit/delete materials in the library
- Access organizations they aren't assigned to
- Manage site settings
- Manage system users

### Data Model

- Store as `role: "coordinator"` in users collection
- Add `organization_ids: [ObjectID]` array field for assigned organizations
- Manage in System Users with organization picker (multi-select)

### Navigation

Coordinator menu items:
- Dashboard (stats for assigned organizations)
- Organizations (list assigned orgs only, edit but no create/delete)
- Groups (filtered to their orgs, full CRUD)
- Leaders (filtered to their orgs, full CRUD)
- Members (filtered to their orgs, full CRUD)
- Resources (library view, read-only, can assign to their groups)
- Materials (library view, read-only, can assign to their orgs/leaders)
- Reports (filtered to their orgs)
- Common links (About, Contact, Terms, Privacy)
- Logout

---

## Multi-Role Architecture

### The Problem

Real users often have multiple roles:
- A teacher who is also a student in another course
- A coordinator who is also a member of a group
- An admin who wants to experience the member view

The current single-role model doesn't accommodate this.

### Approaches Considered

| Approach | Description | Pros | Cons |
|----------|-------------|------|------|
| Role Switching | User picks which "hat" to wear | Clean separation | Context switching is annoying |
| Role Merging | Union of all permissions | Seamless | Confusing, potential for unintended access |
| Contextual Roles | Role depends on what you're viewing | Very flexible | Most complex to implement |
| Primary Role + Additions | One role drives UI, but additional access granted | Simpler hybrid | Still some complexity |

### Granularity Insight

Role assignment should be at the **group level**, not just org level:
- Organization = University
- Groups = Courses
- A person could be a leader (teacher) in Course A and member (student) in Course B

This means:
```
user.group_roles = [
  { group_id: "BIO101", role: "leader" },    // teaching
  { group_id: "CHEM202", role: "member" }    // taking
]
```

Coordinator is different - it's org-scoped, not group-scoped. So there are two layers:
- **Org-level roles:** coordinator (manages the org)
- **Group-level roles:** leader, member (participates in specific groups)
- **System-level:** admin, analyst (platform-wide)

### Recommended Model

```
user.contexts = [
  { workspace: "HydroSci", org: "Lincoln High", role: "coordinator" },
  { workspace: "HydroSci", org: "Lincoln High", role: "member", group: "Water Study 101" },
  { workspace: "EcoLearn", org: "Lincoln High", role: "leader", group: "Eco Club" }
]
```

**On login:**
- If one context → auto-select it
- If multiple → present a chooser

**During session:**
- Header shows current context
- Dropdown to switch contexts
- UI/permissions based on current context

---

## Multi-Tenancy (Workspaces)

### The Problem

- Current deployment serves one project (Mission HydroSci)
- Other projects want to use the platform
- Running separate instances is operationally expensive
- Want one deployment serving multiple projects/workspaces

### Approaches Considered

| Approach | Description | Pros | Cons |
|----------|-------------|------|------|
| Separate Instances | Each project gets own deployment | Clean isolation | Operational overhead |
| Subdomain-Based | `mhs.adroit.games`, `projectx.adroit.games` | Feels like separate sites | OAuth complexity |
| Single Domain + Switcher | One URL, user switches workspaces | Simpler auth | Less branded experience |
| Shared DB with Workspace ID | All records have `workspace_id` | Standard multi-tenancy | Must filter all queries |

### Subdomain Approach (Preferred for UX)

**Benefits:**
- Users don't need to "switch" workspaces - they go to different URLs
- Natural isolation - feels like completely separate sites
- Branding potential - each workspace has own logo, colors, name
- Simple mental model - "I'm on the HydroSci site"
- Same credentials work everywhere - shared user identity
- Monetization-ready - each subdomain could be a paying tenant

**Implementation:**
1. Wildcard DNS: `*.adroit.games → server IP` (one-time setup)
2. Wildcard SSL cert via Let's Encrypt with DNS validation
3. Middleware extracts subdomain → looks up workspace → sets context
4. All queries filter by `ctx.WorkspaceID`
5. User authentication works across subdomains
6. User's access to each workspace tracked separately

**Reserved subdomains:** `www`, `api`, `admin`, `app`, `auth`, `mail`, `smtp`, `ftp`

### Single Domain + Switcher (Simpler Auth)

**Benefits:**
- Simpler OAuth setup (one domain, one set of redirect URIs)
- No wildcard SSL complexity
- Workspace/role switching unified in one UI

**Trade-offs:**
- Less "separate site" feel
- All workspaces share visual branding (or need in-app theming)

### The Cross-Cutting Case

What if Lincoln High School participates in both HydroSci and EcoLearn?

**Options:**
1. **Duplicate entities:** Lincoln High exists twice (data duplication issues)
2. **Shared entities, scoped participation:** Lincoln High exists once, participation tracked per workspace

**Recommended:** Shared entities with workspace-scoped participation:
```
org_workspace_memberships = [
  { org_id: "Lincoln High", workspace_id: "HydroSci" },
  { org_id: "Lincoln High", workspace_id: "EcoLearn" }
]
```

---

## OAuth and Authentication

### The Problem

OAuth providers require explicit redirect URIs:
- `https://mhs.adroit.games/auth/google/callback`

Most don't allow wildcards. Each new subdomain would need configuration changes, and for Clever/ClassLink with COPPA, potentially new approval processes.

### Solution: Central Auth Domain

Use `auth.adroit.games` for all OAuth callbacks:

1. Register `auth.adroit.games` with all OAuth providers (one-time)
2. All OAuth flows route through this domain
3. Origin workspace stored in OAuth `state` parameter
4. After auth success, redirect to originating subdomain with signed token
5. Subdomain verifies token, creates session

**Flow:**
```
User on mhs.adroit.games clicks "Login with Google"
  → Redirect to Google with callback auth.adroit.games
  → Google redirects to auth.adroit.games
  → Verify auth, create signed token
  → Redirect to mhs.adroit.games/auth/complete?token=xyz
  → Verify token, create session
```

**Result:**
- One set of redirect URIs per provider
- One approval process per provider
- New workspaces work automatically

### COPPA Considerations

COPPA approval (Clever, ClassLink) is often per-school-district, not per-domain:
- Data sharing agreements with districts
- Privacy policy reviews
- Operator agreements

This work exists regardless of subdomain architecture.

---

## SSL/TLS with Let's Encrypt

### Wildcard Certificates

Let's Encrypt supports wildcard certs but requires DNS validation:

```bash
certbot certonly --dns-route53 -d "adroit.games" -d "*.adroit.games"
```

With Route53, this can be automated. Tools:
- `certbot` with Route53 plugin
- `lego`
- `acme.sh`

Renewal can be automated via cron. One-time setup, then hands-off.

---

## Phased Implementation Plan

### Phase 1: Add Workspace Foundation ✓ (Completed Dec 2024)

1. ✓ Create `workspaces` collection
2. ✓ Add `workspace_id` field to all major collections:
   - organizations
   - groups
   - users
   - resources
   - materials
   - assignments (group_resource_assignments, material_assignments)
   - group_memberships
3. Set all existing records to the single workspace ID (via mongosh migration)
4. Queries don't need to filter yet (only one workspace)
5. Build coordinator with workspace awareness baked in

### Phase 2: Coordinator Role

1. Add "coordinator" to valid roles
2. Add `organization_ids` array to User model
3. Update System Users UI for coordinator org assignment
4. Create authorization helpers: `CanAccessOrganization(user, orgID)`
5. Create `menu_coordinator` template
6. Filter views for coordinators (orgs, groups, leaders, members)
7. Resources/Materials: show library, hide create/edit/delete for coordinators
8. Reports: filter to coordinator's orgs

### Phase 3: Subdomain Routing (When Ready)

1. Set up wildcard DNS: `*.adroit.games`
2. Set up wildcard SSL cert
3. Add subdomain → workspace lookup middleware
4. Add workspace context to all request handling
5. Filter all queries by workspace
6. User workspace access checking
7. Workspace management UI for admins

### Phase 4: Central Auth Domain (With Phase 3)

1. Set up `auth.adroit.games` subdomain
2. Register with OAuth providers
3. Implement cross-subdomain auth flow
4. Signed token verification

### Phase 5: Multi-Role Context Switching (When Needed)

1. Move role from user-level to context-level
2. Build role/context switcher UI
3. Per-group role assignments
4. Session stores current context
5. Context-aware authorization throughout

---

## Data Model Evolution

### Current Model

```
users:
  - id, email, role, organization_id, ...

organizations:
  - id, name, ...

groups:
  - id, organization_id, name, ...

group_memberships:
  - user_id, group_id, role (leader/member)
```

### Future Model (Fully Evolved)

```
workspaces:
  - id, subdomain, name, logo_url, theme, settings

users:
  - id, email, password_hash, full_name
  - is_super_admin (platform-wide)

user_workspace_access:
  - user_id, workspace_id
  - is_admin, is_analyst, is_coordinator

coordinator_orgs:
  - user_id, workspace_id, org_id

organizations:
  - id, workspace_id, name, ...

org_workspace_memberships:
  - org_id, workspace_id (for orgs in multiple workspaces)

groups:
  - id, workspace_id, org_id, name, ...

group_memberships:
  - user_id, group_id, role (leader/member)

resources:
  - id, workspace_id, ...

materials:
  - id, workspace_id, ...
```

---

## Key Architectural Decisions

1. **Roles remain primary model** - simple, auditable, handles 90% of cases
2. **Context switching over role merging** - clearer user mental model
3. **Workspace determined by subdomain** - natural isolation, branded experience
4. **Central auth domain** - solves OAuth redirect URI problem
5. **Shared entities with scoped participation** - orgs can exist in multiple workspaces
6. **Phase workspace_id first** - every new feature naturally supports multi-tenancy

---

## Monetization Considerations

The subdomain model naturally supports SaaS:
- Each workspace could be a paying tenant
- Pricing per workspace, per user count, or feature tiers
- Free tier for open source/education, paid for commercial
- Open source self-hosted (free) vs managed service (paid)

---

## Open Questions

1. **Workspace branding:** How much customization per workspace? (logo, colors, name only? or full theming?)
2. **Cross-workspace users:** How to handle user profile that spans workspaces?
3. **Super-admin:** Platform-wide admin vs workspace admin distinction?
4. **Workspace deletion:** Soft delete? What happens to data?
5. **Workspace cloning:** Template workspaces for quick setup?

---

*Document created: December 2024*
*Last updated: December 2024*
