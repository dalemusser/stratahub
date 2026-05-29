# MHS Developer Sentinel User

A well-known `stratahub.users` document is seeded automatically at startup so that Mission HydroSci's editor and localhost play sessions round-trip cleanly through stratalog, stratasave, and mhsgrader.

## TL;DR

- **`_id`:** `ObjectId("000000000000000000000001")` (24-char hex: `000000000000000000000001`)
- **`full_name`:** `MHS Developer Sentinel`
- **`login_id`:** `mhs-developer-sentinel`
- **`role`:** `member`
- **`auth_method`:** `trust`
- **`workspace_id`:** the default workspace (whichever `Startup` resolves)
- Seeded by `bootstrap.ensureMHSDevSentinel` in `internal/app/bootstrap/startup.go`, called once per process start.
- Idempotent: if the document is already present, no change is made.

## Why it exists

MHSBridge — the bridge between the Unity WebGL game and its host page — needs a `user_id` (24-char lowercase hex of a `stratahub.users._id`) to attach to every log and save request. In production this comes from the authenticated stratahub session and is the real user's ObjectID. But two paths have no logged-in session:

1. **Unity Editor builds.** A developer pressing Play in the Unity Editor isn't going through a browser, isn't authenticated, and never touches stratahub. MHSBridge's `_userId` field is initialized to the sentinel hex in the editor branch of `LoadConfig`.
2. **Localhost browser launches.** The host page `MHS-Bridge-index.html` detects `location.hostname === "localhost"` and seeds `window.__mhsBridgeConfig.identity.user_id` with the sentinel hex.

When MHSBridge sends `user_id="000000000000000000000001"` to stratalog and stratasave, those services accept the value at face value — they don't validate against `stratahub.users`. So log and save writes work fine even without this seed.

What *breaks* without the seed is anything that reads MHS data and then joins back to a stratahub user:

- **mhsgrader** computes grades from stratalog event streams and looks up the user to attribute them.
- **Member reports** in stratahub join member rows against captured activity.
- **Support and debugging tools** that look up "who is `000000000000000000000001`?" expect to find a row.

Seeding a real user record makes that join non-empty. Without it, dev play turns into orphaned data that is hard to distinguish from a broken pipeline.

## What it is NOT

- **Not a backdoor login.** `auth_method: trust` is the same auth_method used by other zero-verification-needed paths (see `docs/auth/trust_auth.md`), and `login_id: mhs-developer-sentinel` is not an email address. Real auth still requires either a real account or the configured `STRATAHUB_SUPERADMIN_EMAIL`.
- **Not a service account.** The role is `member`, not `admin` or `coordinator`. The record exists to satisfy joins, not to grant privileges.
- **Not a per-developer record.** It's a single shared placeholder. Every dev who plays from Editor or localhost generates traffic that attaches to this same user_id. That's intentional — it keeps the join graph simple — but it means dev-play data is not separable per dev. If you need that, log in as a real account at adroit.games before launching.

## Where it shows up

- **Members list** — yes. Filterable visually by full name `MHS Developer Sentinel`. There is currently no automatic hide for sentinels.
- **Member reports CSV** — yes. The `user_id` column will contain `000000000000000000000001` for any rows generated from editor/localhost dev play. Use that to filter dev rows out of production analyses.
- **mhsgrader Dashboard** — yes, under the workspace and group(s) you assign the sentinel to. By default it has no group memberships, so it won't appear in any group-scoped view until someone explicitly adds it.

## Removing or relocating it

This isn't recommended (the seeder will recreate it on the next startup), but if you need to detach the sentinel from a particular workspace or group:

- **Change its workspace.** Update the `workspace_id` field directly. The seeder only touches `_id`, so any other modifications you make stick.
- **Remove from a group.** Delete the corresponding row in `group_memberships` where `user_id = ObjectId("000000000000000000000001")`. The seeder doesn't add group memberships.
- **Disable it.** Set `status: "disabled"` — the seeder leaves `status` alone on existing documents.
- **Force a re-seed.** Delete the document and restart stratahub. The seeder will recreate it with default values attached to the default workspace.

## Related files

- `internal/app/bootstrap/startup.go` — the seeder and call site.
- `internal/app/system/validators/validators.go` — `usersSchema()` defines the JSON-Schema validator. The seeded record satisfies it (`full_name`, `role`, `status`, `auth_method` are all present and valid).
- `mhs-updates/mhsbridge-userid-cleanup-052626/Bridge/MHSBridge.cs` — the game-side default that this user record supports.
- `mhs-updates/build-automation-update-051226/MHS-Bridge-index.html` — the host-page default for localhost.
