# Resource URL Identification — Plan

_Last updated: 2026-05-28 · Status: design, pre-implementation_

Companion to `vocabulary.md` (the permanent parameter contract). This document
covers what we're building, why, and how we'll roll it out. It defines the
per-resource **identification mode** that selects which vocabulary parameters a
resource emits.

---

## Background

When a member opens a URL-based resource, StrataHub appends identity to the
resource's launch URL. Before this feature, that happened for **every** URL
resource, in a fixed human-readable form, with no per-resource control over what
(if anything) to include.

We want two things instead:

- **Per-resource control** over what identity, if any, travels in the URL —
  defaulting to nothing.
- **De-identified-by-default** identity (opaque hex IDs) for anything that does
  need it, since that is the privacy-correct and analytically-stable choice.

The original external-consumer integration predates workspaces and expected
`id`=login_id plus `org` and `group` names, all human-readable. The `legacy`
mode (below) reproduces that exactly, so that consumer keeps working while it
migrates to hex.

## Goals

1. **Opt-in, not blanket.** Default to emitting no identity. A resource declares
   what it needs.
2. **Coherent, valid combinations only.** Offer named modes rather than a grid of
   checkboxes that can produce ambiguous URLs.
3. **Steer toward de-identified (hex)** for the research consumer, which is both
   the FERPA/COPPA/IRB-correct choice and the analytically-correct one (immutable
   keys survive renames).
4. **Provide a time-boxed `legacy` bridge** so the existing consumer is not
   broken during transition, then retire it.
5. **Consolidate** URL building into one function so the logic lives in a single
   tested place and cannot diverge across call sites.

---

## The identification modes

A new per-resource setting selects one mode. Each mode is a fixed parameter set
(no per-field sub-choices — maximum flexibility for the hex case comes from
always sending all four hex IDs, not from UI toggles).

| Mode | Parameters emitted | PII | Intended use |
|------|--------------------|-----|--------------|
| `none` *(default)* | *(none)* | None | Most resources. The default. |
| `hex` | `ws_id`, `org_id`, `group_id`, `user_id` | None | De-identified research, surveys, tests. **Recommended** for any consumer that stores data. |
| `human` | `ws`, `org`, `group`, `user`, `login_id` | **High** | Human-readable display/debugging only. Never production research. |
| `both` | `ws`, `ws_id`, `org`, `org_id`, `group`, `group_id`, `user`, `user_id`, `login_id` | **High** | Pre-research testing / debugging, when you want to eyeball readable values next to the hex. |
| `legacy` | `org`, `group`, `id`(=**login_id**) | **High** | Frozen reproduction of the pre-2026 contract for the existing consumer. Deprecated on arrival; time-boxed. |

### Notes per mode

- **`hex`** sends all four hierarchy/user IDs even when a consumer only needs two.
  This costs nothing (opaque, stable, de-identified) and means a consumer that
  later needs workspace- or org-level analysis dimensions already has them
  without us touching the resource. This is the no-crosswalk case from
  `vocabulary.md` rule 3.
- **`legacy`** reproduces the exact pre-2026 contract: the user's login (email)
  under the parameter named `id` (**not** `user_id`), plus `org` and `group`
  names, and **no** workspace (the original contract predates workspaces).
  Because it uses the deprecated `id` parameter rather than overloading
  `user_id`, the rule that `user_id`=hex holds everywhere — legacy is fully
  isolated under `id`. It exists solely so the existing consumer keeps working
  while it migrates to `hex`, and retires once no resource uses it.

### PII warning

Any mode that emits a **High**-PII field (`user`, `login_id`, or the legacy
`id`) — i.e. `human`, `both`, and `legacy` — must surface a warning at
resource-creation/edit time:

> This mode includes personally identifiable information (student name and/or
> login) in the resource URL. Use only where justified and approved
> (IRB / FERPA / COPPA).

Only `none` and `hex` are PII-free. Consider additionally gating `human` and
`both` to the `dev` workspace, or requiring an explicit "I understand this URL
will carry PII" acknowledgment, so they aren't selected casually on a production
survey.

---

## Where the setting lives

Add a field to the `Resource` model
(`internal/domain/models/resource.go`):

```go
// URLIdentityMode controls which identity parameters are appended to LaunchURL
// when a member opens this resource. One of: "none", "hex", "human", "both",
// "legacy". Empty is treated as "none". See docs/resource-identification/.
URLIdentityMode string `bson:"url_identity_mode,omitempty" json:"url_identity_mode,omitempty"`
```

- Default (empty / unset) = `none`.
- Validate against the mode set on create/edit (extend `inputval` with a
  `urlidentitymode` validator, mirroring the existing `resourcetype` pattern).
- Resource-level granularity is sufficient. An assignment-level override (the
  same resource emitting different modes for different groups) is **out of scope**
  until a concrete need appears.

---

## Consolidate the two build paths

Replace the two ad-hoc `urlutil.AddOrSetQueryParams(...)` literals
(`memberview.go:124`, `memberlist.go:83`) with a single builder, e.g.:

```go
// internal/app/features/resources/resourceurl/build.go (new)
//
// BuildLaunchURL appends the identity parameters for the given mode to the
// resource's launch URL. ctx carries the resolved values for the current
// member/group/org/workspace (both names and hex IDs).
func BuildLaunchURL(launchURL, mode string, ctx IdentityContext) string
```

Both call sites then pass the resource's `URLIdentityMode` and an
`IdentityContext` populated from the member, group, organization, and workspace
already in scope. Benefits:

- Every call site builds the URL through the same function, so they cannot
  diverge.
- All mode logic lives in one tested place.

`IdentityContext` needs: workspace subdomain + ObjectID, org name + ObjectID,
group name + ObjectID, user full name + ObjectID + login_id. Workspace is already
on the resource (`Resource.WorkspaceID`) and resolvable to a subdomain.

---

## Migration

The blanket-emit behavior must not be yanked out from under the live consumer.
Order matters:

1. **Build** the mode field, the validator, the four modes, and the consolidated
   `BuildLaunchURL`. New resources default to `none`.
2. **Backfill existing resources:**
   - The specific survey/test resources the consumer actually consumes →
     `legacy` (or `hex` if the consumer is already ready — see Open questions).
   - **Everything else → `none`.** This is what stops the universal param leakage,
     including the list-view login_id leak.
3. **Deploy.** Now identity rides only on the resources that opted in, and the
   consumer's resources emit a deliberate, documented scheme.
4. **Migrate the consumer** from `legacy` → `hex` over time, coordinating the
   parameter changes on their side: `id`(login_id) → `user_id`(hex),
   `group`(name) → `group_id`(hex), `org`(name) → `org_id`(hex) or dropped. A
   brief dual-emit window is unnecessary if we flip per-resource and coordinate,
   but is available (distinct keys, no collision) if they want overlap.
5. **Retire `legacy`** once the admin filter (below) shows zero resources in
   `legacy` mode: delete the mode and its branch in `BuildLaunchURL`.

### Tracking legacy for retirement

Add an admin filter / column "Identification mode" on the resources list so all
`legacy` (and `human`/`both`) resources are visible at a glance. Retiring
`legacy` is then a verifiable step ("filter = legacy → 0 results"), not a guess.

---

## Open questions (resolve before/early in implementation)

1. **Legacy parameter confirmed as `id`=login_id** (from pre-2026 email records —
   the original integration read the user's login from a parameter literally named
   `id`, not `user_id` or `login_id`). Remaining question: has the consumer since
   adapted to the hex `user_id` the single-view path has emitted since the
   cutover? If yes, we may skip `legacy` for them and move straight to `hex`; if
   they still parse `id`, `legacy` keeps them working during migration.
2. **Which existing resources does the consumer consume?** Need the explicit list
   to backfill to `legacy`/`hex`; everything else defaults to `none`.
3. **Does the research analysis need workspace/organization as dimensions?** If
   yes, `hex` (which sends `ws_id` + `org_id`) already covers it; if they hold a
   crosswalk and only analyze within-group, the extra IDs are harmless.
4. **Gate `human`/`both`?** Decide whether to restrict to the `dev` workspace or
   require an explicit PII acknowledgment.

## Possible future mode (not building now)

`institutional` — human-readable institutional context with a de-identified user
(`ws`, `org`, `group`, `user_id` hex). Useful for institutional dashboards that
display class/school names but must not identify students. No current consumer
needs it; noted so the mode list can grow coherently if one does.

---

## Affected code (reference)

- `internal/domain/models/resource.go` — add `URLIdentityMode`.
- `internal/app/features/resources/resourceurl/` — new consolidated builder.
- `internal/app/features/resources/memberview.go` (~L124) — use builder.
- `internal/app/features/resources/memberlist.go` (~L83) — use builder.
- `internal/app/features/resources/adminnew.go`, `adminedit.go` — mode selector +
  PII warning in the create/edit form.
- `internal/app/features/resources/templates/` — form control + warning; resource
  list filter/column for the mode.
- `internal/app/system/inputval/` — `urlidentitymode` validator.
