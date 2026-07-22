# Mission HydroSci — SSO & staff-auth status

**Last updated:** 2026-07-16
**Read this before implementing SSO (Google / Microsoft / ClassLink / Clever).**

## TL;DR

SSO login providers are **not implemented yet** (awaiting legal review of the
privacy policy / terms of service, then provider certificates/keys). Because of
that, the Mission HydroSci **staff-auth member gate deliberately fails closed
for SSO-backed staff accounts.** When you implement SSO, you **must** come back
and give MHS a real inline verification path for SSO staff, or the frictionless
"authorize on the student's device" flow won't work for teachers who sign in
with SSO. This file is the memory of where things stand so that doesn't require
re-deriving the whole situation.

## Background — what the MHS member gate is

Some MHS member actions (jump to a unit, switch collection version, delete
online saved state/settings, and opening the `/missionhydrosci/manage` page in a
non-`trust` workspace) must be **authorized by a staff member** (leader,
coordinator, admin, or superadmin). *Which* authorization is required is the
workspace setting `MHSMemberAuth`:

- `trust` — no check. **This is what the dev workspace uses** so testers/devs get
  straight into the manage controls.
- `staffauth` — **the production method.** A staff member proves who they are
  (login ID + password, or an emailed code), yielding a single-use token /
  sliding unlock session. See `system/staffauth/` and `api_manifest.go`
  (`checkMemberAuth`), `manage.go` (`HandleManageUnlock`).
- `keyword` — a shared classroom keyword. Offered as a possibility; **not used in
  practice.**

The staff proves identity by their StrataHub account's `auth_method`. Today that
is `password` or `email` for the staff accounts used to authorize.

## The problem SSO introduces

`system/staffauth/staffauth.go` `StartAuth` switches on the staff account's
`auth_method`:

| auth_method | Behavior |
|---|---|
| `password` | Issues a challenge; the staff member must enter their password (bcrypt-checked). |
| `email` | Emails a code; the staff member must enter it. |
| `trust` | Mints a verified token immediately (intended — `trust` workspaces). |
| **SSO** (google / microsoft / classlink / clever) | **Fails closed** — returns `ErrUnsupportedAuthMethod`. |

Before 2026-07-16, the SSO/`default` branch **minted a verified token with no
credential check** ("fall back to simple confirmation since we can't initiate an
OAuth flow inline"). That meant: once SSO login shipped and teachers had
`auth_method: "google"`, any student who knew a teacher's login ID (usually their
email) could type it into the gate and receive a full unlock — covering the
irreversible save deletes — attributed to the teacher. The gate the redesign was
built around would have been open for every SSO workspace.

It was latent only because **no active server has any SSO method enabled.** To
keep it latent-and-safe rather than a trap waiting for the SSO deploy, the branch
now fails closed (`ErrUnsupportedAuthMethod`, surfaced to the user as "This staff
account signs in with a method (SSO) that can't authorize here yet").

## What must be done when SSO is implemented

1. **Give SSO staff a real inline verification path.** Options, roughly in order
   of preference:
   - **Re-use the emailed-code path.** The SSO identity *is* an email; treat
     SSO-backed staff like `email` method for authorization purposes (send a code
     to their address, verify it). Lowest new surface, no OAuth redirect inside
     the gate. Likely the right answer.
   - **Interactive OAuth confirmation.** A short-lived challenge that the staff
     member's *own* authenticated browser session approves (they sign in with
     SSO on their own device/tab, which approves the pending challenge on the
     student's device). More work; only needed if the email path is unacceptable.
   - Do **not** simply mint a token from a login ID again.
2. **Remove/replace the `default: return ErrUnsupportedAuthMethod`** in
   `StartAuth` with the chosen path (or route SSO methods explicitly into the
   email path).
3. **Re-verify the full member-gate flow** for an SSO staff account: gate render,
   unlock grant, single-use token consumption, sliding expiry, audit attribution.
4. **Update this file** and the `staffauth.go` comments.

## Pointers

- Fail-close point: `internal/app/system/staffauth/staffauth.go`, `StartAuth`
  `default` branch and `ErrUnsupportedAuthMethod`.
- Gate enforcement: `internal/app/features/missionhydrosci/api_manifest.go`
  (`checkMemberAuth`), `manage.go` (`HandleManageUnlock`, `ServeManage`).
- Unlock session store: `internal/app/system/staffauth/unlock.go`.
- Feature overview and the member-auth table: `README.md` in this directory.
- Original code review that surfaced this: `docs/mission-hydrosci/mhs-code-design-review-071626.md`.
