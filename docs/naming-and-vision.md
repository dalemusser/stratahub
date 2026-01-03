# StrataHub Naming Decisions and Product Vision

This document captures discussions about naming conventions and big-picture product direction for StrataHub.

---

## Part 1: Naming the Top-Level Entity

### Context

StrataHub needs a term for the top-level container that holds a collection of organizations, groups, leaders, members, resources, and materials. This entity:

- Is isolated from other instances
- Has its own identity/branding
- Could have its own subdomain
- Contains organizations → groups → people
- Has its own admins, resources, materials

Use cases include: research projects, corporate training programs, school district deployments, professional association platforms, nonprofit programs.

### Decision: Workspace ✓

**Decided: December 2024**

The term **Workspace** was chosen for the top-level entity. It is:
- Most universally understood from modern SaaS (Slack, Notion, Asana)
- Professional and neutral
- Scales from small team to large enterprise

**How it sounds in context:**
- "Join the HydroSci workspace"
- "Switch to your other workspace"
- "Your organization's training workspace"

### Candidates Considered

| Term | Pros | Cons |
|------|------|------|
| **Workspace** ✓ | Well-understood (Slack, Notion), professional, neutral | Corporate feel, less warm |
| **Community** | Warm, people-centric, implies belonging; works across contexts | Could be confused with forum-style community; somewhat vague |
| **Space** | Short, neutral, flexible | Very generic, bland |
| **Hub** | Fits "StrataHub" branding, implies center of activity | Conflicts with app name itself |
| **Network** | Implies connected entities | Could mean social network |
| **Program** | Fits educational context (training program) | Could mean software program |
| **Site** | Maps to subdomain concept | Too technical, implies website |
| **Realm** | Clear isolation, used by Keycloak | Fantasy connotations |
| **Platform** | Implies foundation for activities | Conflicts with the app itself |
| **Environment** | Accurate for isolated context | Very technical, dev-focused |
| **Tenant** | Standard multi-tenancy term | Cold, implies renting |
| **Campus** | Educational context, contained | Too education-specific |
| **Portal** | Entry point concept | Dated, feels 2000s |
| **Instance** | Technically accurate | No warmth, very technical |
| **Domain** | Maps to subdomain | Confused with internet domain |
| **Zone** | Clear boundary | Generic, clinical |
| **Collective** | Shared purpose | Unusual, political connotations |

### Industry Precedents

| Product | Term Used |
|---------|-----------|
| Slack | Workspace |
| Discord | Server |
| Notion | Workspace |
| Microsoft Teams | Team (more like a group) |
| Salesforce | Org |
| Keycloak | Realm |
| Quora | Space |

---

## Part 2: Product Vision and Missing Features

### Current State

StrataHub is:
- A platform for organizing people (orgs → groups → members)
- A way to deliver resources/materials to those people
- Role-based access (admin, analyst, leader, member)
- Multi-organization support within a single deployment

### Communication & Engagement (Currently Missing)

| Feature | Description | Impact |
|---------|-------------|--------|
| Announcements | Org-wide or group-wide messages from leaders | Keeps users informed |
| Notifications | Email/in-app when new resources assigned, deadlines approaching | Pulls users back to platform |
| Messaging | Leader-to-member, or discussion threads per group | Enables dialogue |
| Activity feed | "What's new in your groups" | Creates engagement |

*Without communication, StrataHub is a content delivery system. With it, it becomes a community platform.*

### Progress & Analytics

**Current state:** Assign resources, run member reports.

**Missing features:**

| Feature | Description |
|---------|-------------|
| Access tracking | Did the member open the resource? When? |
| Completion tracking | Did they finish it? (for videos, quizzes, etc.) |
| Progress dashboards | Visual progress for leaders |
| Engagement analytics | Who's active, who's disengaged |
| Time-on-task | How long did they spend? |

*This transforms StrataHub from "delivery" to "learning management."*

### Assessment & Credentials

| Feature | Description |
|---------|-------------|
| Quizzes/assessments | Embedded or linked evaluations |
| Surveys | Collect feedback from users |
| Certificates | Completion certificates (PDF generation) |
| Badges/achievements | Gamification elements |

### Content Creation vs. Content Linking

**Current model:** Resources/Materials are links or uploaded files.

**Possible evolution:**

| Feature | Description |
|---------|-------------|
| Rich content authoring | Create content in-app (like Notion pages) |
| Content versioning | Track changes, roll back |
| Content templates | Reusable structures |
| SCORM/xAPI support | For e-learning content |

*Note: The current "link to external content" model is simpler and may be the right choice. This is a significant scope expansion.*

### Learning Pathways

**Current model:** Resources are assigned individually to groups.

**Possible evolution:**

| Feature | Description |
|---------|-------------|
| Sequences | Do Resource A, then B, then C |
| Prerequisites | Can't access B until A is complete |
| Branching paths | Different content based on choices/results |
| Self-paced vs. cohort-based | Different timing models |

### Onboarding & Self-Service

**Current model:** Admins/leaders create all accounts.

**Possible features:**

| Feature | Description |
|---------|-------------|
| Self-registration | Users sign up, request to join |
| Enrollment codes | "Enter code XYZ to join Group A" |
| Magic links | Email-based passwordless join |
| Approval workflows | User requests, leader approves |
| Waitlists | Group is full, join waitlist |

*This reduces admin burden and scales better.*

### Roles - Current and Potential

**Current roles:** admin, analyst, leader, member, (planned) coordinator

**Potential additional roles:**

| Role | Purpose |
|------|---------|
| Super Admin | Platform-wide, across all workspaces |
| Workspace Admin | Full admin within a workspace |
| Content Curator | Can create/edit resources/materials, but not manage users |
| Observer/Auditor | View-only for compliance/review |
| Support | Can view user info, help with issues, limited edit |
| Parent/Guardian | For COPPA - view child's progress |
| Mentor | Peer-to-peer guidance, member with extra visibility |
| Teaching Assistant | Between leader and member |

### Integration Points

| Integration | Purpose |
|-------------|---------|
| LTI | Embed StrataHub content in LMS (Canvas, Moodle) |
| API | External systems can read/write data |
| Webhooks | Push notifications when events happen |
| SSO/SAML | Enterprise single sign-on |
| SCIM | Automated user provisioning/deprovisioning |

### Mobile & Accessibility

| Feature | Description |
|---------|-------------|
| PWA | Installable, works offline |
| Native apps | iOS/Android (significant investment) |
| Accessibility | WCAG compliance |
| Multi-language | i18n support |

### Monetization Infrastructure

For offering StrataHub as a service:

| Feature | Description |
|---------|-------------|
| Subscription tiers | Free, basic, pro, enterprise |
| Feature flags | Enable/disable features per tier |
| Usage limits | X users, Y storage, Z resources |
| Billing integration | Stripe, etc. |
| Trial periods | 14-day free trial |

---

## Part 3: Architectural Reconsiderations

### Resources vs. Materials Distinction

**Current model:**
- Resources → Members (via groups)
- Materials → Leaders (via orgs)

**Alternative:** One unified content library with flexible assignment targets (groups, orgs, individuals, roles).

**Question:** Is the distinction valuable, or does it add confusion?

### Group Structure

**Current model:** Flat (org has groups)

**Alternatives:**
- Hierarchical groups (sub-groups)
- Group types (course, cohort, team, etc.)
- Group templates

### Assignment Model

**Current model:** Resource → Group (all members see it)

**Alternatives:**
- Individual assignments
- Role-based assignments
- Conditional assignments (based on completion, role, etc.)

### User Identity

**Current model:** Email-based login

**Alternatives:**
- Username option
- Phone number login
- Anonymous/guest access
- Impersonation for support

---

## Part 4: Product Direction Options

Three possible directions for StrataHub's evolution:

### A. Lean Content Delivery Platform

**Focus:** Simplicity

**Features:**
- Assign content, track access, report
- Minimal features, minimal complexity

**Good for:** Quick deployments, simple use cases

### B. Learning Management System (LMS) Lite

**Focus:** Learning outcomes

**Features:**
- Progress tracking, assessments, certificates
- Learning pathways, prerequisites
- Completion reporting

**Good for:** Training programs, courses

### C. Community Learning Platform

**Focus:** Connection and collaboration

**Features:**
- Communication, collaboration
- Discussion, messaging, announcements
- Peer interaction, cohort experiences

**Good for:** Ongoing communities, membership organizations

*These directions aren't mutually exclusive, but knowing the primary direction helps prioritize features.*

---

## Part 5: Current Strengths and Gaps

### Strengths

- Clean, simple architecture
- Works well for the MHS use case
- Solid foundation (Go, MongoDB, HTMX, Tailwind)
- FS Embed means single binary deployment
- Role-based access is straightforward
- Minimal configuration philosophy

### Gaps

| Gap | Impact |
|-----|--------|
| No communication/notification system | Users must proactively check for updates |
| Limited visibility into engagement | Can't tell if resources are being used |
| No self-service onboarding | Admin creates everything manually |
| Single-role limitation | Users can't have multiple roles (being addressed) |

### Highest-Impact Additions

In order of recommended priority:

1. **Workspace model** - Enables multi-tenancy, scales the platform *(Phase 1 complete: Dec 2024)*
2. **Progress/access tracking** - Transforms from delivery to management
3. **Email notifications** - Pulls users back to the platform
4. **Self-registration with codes** - Scales onboarding

---

## Part 6: Open Questions

1. ~~Which term for top-level entity?~~ **Decided: Workspace** (Dec 2024)
2. Should Resources and Materials be unified?
3. What's the primary product direction? (Delivery vs LMS vs Community)
4. Which missing features are highest priority for MHS contract?
5. Which missing features are highest priority for multi-tenant SaaS?
6. What's the timeline for mobile/PWA support?
7. Is internationalization needed?

---

*Document created: December 2024*
*Last updated: December 2024*
