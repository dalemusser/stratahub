# MHS-010 — Collection-override (switch version) member auth not enforced server-side

**Priority:** P1
**Status:** Implemented (2026-07-05)

## Summary

Switching the collection (game) version via the per-user collection override is
gated **only in the client modal**. The server endpoint
`POST /api/collection-override` accepts the request from any authenticated user —
including a member — without checking the workspace `MHSMemberAuth`
authorization. A member could switch versions on their own by calling the
endpoint directly (e.g. via DevTools), bypassing the teacher/coordinator/admin
authorization the UI implies.

This is inconsistent with jump-to-unit (`set-unit`), which **is** enforced
server-side, and it fits the theme of version-management being added after the
original access model.

## Evidence

- `HandleSetCollectionOverride` carries the comment "For members, requires the
  workspace's MHS member auth method" but contains **no** corresponding check —
  it validates only that the user is authenticated and that the collection
  exists:
  - `api_manifest.go:121-182`
- Contrast with the enforced path, which requires a staff-auth token or keyword
  for members:
  - `progress.go:96-122` (`HandleSetToUnit` — server-side member auth)
- The client does gate the picker behind the auth modal, but that is UI-only:
  - `templates/missionhydrosci_units.gohtml:1261-1269` (`mhsRequestCollectionPicker`)
  - `templates/missionhydrosci_units.gohtml:1312-...` (`mhsSelectCollection` →
    `POST /api/collection-override`)

## Affected code

- `internal/app/features/missionhydrosci/api_manifest.go`
  (`HandleSetCollectionOverride`)
- `internal/app/resources/assets/js/mhs-delivery.js` /
  `templates/missionhydrosci_units.gohtml` (client sends the auth proof)

## Proposed fix

Mirror `HandleSetToUnit`'s server-side enforcement in
`HandleSetCollectionOverride`:
- For `user.Role == "member"`, load workspace settings, read
  `GetMHSMemberAuth()`, and require:
  - `staffauth`: a valid, unconsumed `auth_token`
    (`StaffAuthVerifier.Store.ValidateAndConsumeToken`), or
  - `keyword`: a matching `MHSMemberAuthKeyword`, or
  - `trust`: no extra check.
- Update the client `mhsSelectCollection` call to pass the `auth_token` / keyword
  obtained from the existing modal (the modal already runs before the picker
  opens; carry the proof through to the override POST — or move the auth prompt to
  submit time so a fresh token is sent).

## Implementation notes (2026-07-05)

- Enforcement was applied to **both** endpoints: `POST /api/collection-override`
  (set) and `POST /api/collection-override/clear` (the "Use default" button).
  Clearing an override is also a version switch — an unauthorized revert to
  default could disrupt a staff-pinned QA run — so members need the same
  authorization for both. The "Use default" form is gated through the auth
  modal client-side; a shared `requireMemberAuth` helper in `api_manifest.go`
  enforces server-side.
- The client keeps the modal's auth proof and sends it with the override POST.
  Staff-auth tokens remain single-use (one token per action, consumed by the
  server); a second pick after a 403 prompts re-authorization.

## Risk / notes

- Security/authorization correctness. Also relevant to the research-integrity of
  QA runs: unauthorized version switches could pollute a "clean run."
- `Clear All Downloads` and `Reset all MHS data` (MHS-009) are client-only cache
  operations with no server endpoint, so they cannot be server-enforced; that is
  an accepted limitation. Collection-override **does** have a server endpoint, so
  it can and should enforce.
- Token consumption: `set-unit` consumes the staff-auth token. Decide whether a
  single authorization should cover one action or a short session of actions
  (current design is one-token-per-action).
</content>
