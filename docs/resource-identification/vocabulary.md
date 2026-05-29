# Resource URL Identification — Parameter Vocabulary (Permanent Contract)

_Last updated: 2026-05-28_

This document defines every query parameter StrataHub may append to a resource's
Launch URL when a member opens that resource. **These definitions are a stable
contract.** Once a parameter is published here, its name and meaning do not
change. Which parameters a given resource actually emits is configured
per-resource — see `plan.md` (the resource's "identification mode"). This file
defines the words; the plan defines which words each resource speaks.

A consumer integrating against StrataHub resource URLs should be able to read
this file once and rely on it permanently.

---

## Principles

1. **Data minimization.** The default is to emit *nothing*. Identity travels in
   a URL only when a resource explicitly opts in.
2. **Type-explicit names.** A human-readable value and its hex ObjectID always
   have *different* parameter names (`group` vs `group_id`). They never collide,
   so a URL is self-describing and the same key never appears twice.
3. **Stable keys are hex.** Every `*_id` parameter is an immutable MongoDB
   ObjectID. Human-readable names are mutable and (except `ws`) non-unique. Only
   the hex `*_id` values are safe to use as durable join keys for stored or
   longitudinal data.

---

## The vocabulary

| Param | Meaning | Example | Globally unique? | Mutable? | PII class |
|-------|---------|---------|------------------|----------|-----------|
| `ws` | Workspace subdomain | `mhs` | Yes | Yes (admin can change) | None |
| `ws_id` | Workspace ObjectID (24-char hex) | `695f5a3fa323f290a63b3fce` | Yes | No | None |
| `org` | Organization name | `Intelligence Builders` | **No** | Yes | Low |
| `org_id` | Organization ObjectID (hex) | `69a1...` | Yes | No | None |
| `group` | Group name | `Earth Science` | **No** | Yes | Low |
| `group_id` | Group ObjectID (hex) | `69cdc800d385e3b94310f6e2` | Yes (implies org + ws) | No | None |
| `user` | User's full display name | `Jane Doe` | **No** | Yes | **High** |
| `user_id` | User ObjectID (hex) | `69b7b2328cac2be5f60efb09` | Yes | No | None (pseudonymous) |
| `login_id` | User's login identifier (email) | `jane@school.org` | Yes | Yes (rare) | **High** |

### PII classes

- **None** — `ws`, `ws_id`, `org_id`, `group_id`, `user_id`. Opaque or
  institutional identifiers. `user_id` is pseudonymous: it identifies a person
  only to someone holding the StrataHub crosswalk.
- **Low** — `org`, `group`. Institutional names. Not directly identifying, but
  can contribute to re-identification in small populations.
- **High** — `user` (a person's name) and `login_id` (a person's email). These
  are direct PII. `user` is the sharpest edge in the vocabulary — a name
  re-identifies more reliably than an institutional email handle.

### Deprecated parameters (legacy only)

| Param | Meaning | Example | PII class |
|-------|---------|---------|-----------|
| `id` | User's login (email), under the original pre-2026 parameter name | `jane@school.org` | **High** |

`id` is **not** part of the permanent vocabulary. It exists only because the
original consumer integration (which predates workspaces and the
de-identification work) read the user's login from a parameter literally named
`id`. It is emitted **only** by Legacy mode and retires when Legacy mode is
removed. New integrations must use `user_id` (hex) or `login_id` (email), never
`id`.

---

## Rules

1. **`user_id` always means the hex ObjectID — no exceptions.** The pre-2026
   consumer contract carried the login_id under a separate parameter named `id`
   (see "Deprecated parameters" above), *not* under `user_id`. Legacy mode
   reproduces that old `id` parameter, so `user_id` stays unambiguous everywhere.

2. **Hex `*_id` values are the only durable keys.** They are immutable and
   globally unique. A consumer that stores values for later analysis must key on
   hex, never on names — an admin renaming a group or organization mid-study
   would silently break joins keyed on the human-readable name.

3. **`group_id` resolves the whole hierarchy — but only with the crosswalk.**
   Because a group ObjectID is globally unique, `group_id` alone identifies the
   group, and group → organization → workspace is derivable *by anyone holding
   the StrataHub database*. A de-identified consumer that does **not** hold the
   crosswalk cannot derive workspace or organization from `group_id`; if its
   analysis needs those levels as dimensions, they must be supplied explicitly as
   `ws_id` and `org_id` (still opaque, still de-identified).

4. **Human-readable names are for display and debugging only.** `ws`, `org`,
   `group`, and `user` are mutable, and all but `ws` are non-unique. Never treat
   them as identifiers or durable keys.

---

## Why the workspace uses the subdomain, not the title

A workspace has both a title/label (e.g. "Mission HydroSci") and a subdomain
(e.g. `mhs`). Titles are not constrained to be unique across workspaces; the
subdomain is. So the human-readable workspace parameter `ws` carries the
subdomain. (The subdomain is still mutable by an admin — for a guaranteed-stable
workspace key, use `ws_id`.)
